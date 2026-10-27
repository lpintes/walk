// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/lpintes/walk/internal/win"
)

// SingleInstanceEventHandler is called with the command line arguments
// (without the program name) and the working directory of another instance
// of the program.
type SingleInstanceEventHandler func(args []string, workingDir string)

type singleInstanceEventHandlerInfo struct {
	handler SingleInstanceEventHandler
	once    bool
}

// SingleInstanceEvent is published when another instance of the program
// hands its command line over; see SingleInstance.
type SingleInstanceEvent struct {
	handlers []singleInstanceEventHandlerInfo
}

// Attach adds a handler and returns a handle for Detach.
func (e *SingleInstanceEvent) Attach(handler SingleInstanceEventHandler) int {
	handlerInfo := singleInstanceEventHandlerInfo{handler, false}

	for i, h := range e.handlers {
		if h.handler == nil {
			e.handlers[i] = handlerInfo
			return i
		}
	}

	e.handlers = append(e.handlers, handlerInfo)

	return len(e.handlers) - 1
}

// Detach removes the handler with the handle returned by Attach.
func (e *SingleInstanceEvent) Detach(handle int) {
	e.handlers[handle].handler = nil
}

// Once adds a handler that is detached after its first call.
func (e *SingleInstanceEvent) Once(handler SingleInstanceEventHandler) {
	i := e.Attach(handler)
	e.handlers[i].once = true
}

// SingleInstanceEventPublisher publishes a SingleInstanceEvent.
type SingleInstanceEventPublisher struct {
	event SingleInstanceEvent
}

// Event returns the event of the publisher.
func (p *SingleInstanceEventPublisher) Event() *SingleInstanceEvent {
	return &p.event
}

// Publish calls the handlers of the event.
func (p *SingleInstanceEventPublisher) Publish(args []string, workingDir string) {
	for i, h := range p.event.handlers {
		if h.handler != nil {
			h.handler(args, workingDir)

			if h.once {
				p.event.Detach(i)
			}
		}
	}
}

// SingleInstanceTimeout is how long NewSingleInstance waits for the first
// instance of the program: until it listens (while it is still starting)
// and until it has accepted the arguments.
const SingleInstanceTimeout = 30 * time.Second

// singleInstanceCopyDataID identifies the WM_COPYDATA messages of
// SingleInstance ("WKS" and the format version 1).
const singleInstanceCopyDataID = 0x574B5301

// singleInstanceMaxData is the largest WM_COPYDATA payload accepted.
const singleInstanceMaxData = 1 << 20

// singleInstancePublishMsg is posted by the window of the first instance to
// itself, so that the event is published after WM_COPYDATA has returned.
const singleInstancePublishMsg = win.WM_APP + 1

const singleInstanceWindowClass = `\o/ Walk_SingleInstance_Class \o/`

// singleInstanceMessage is the data the second instance sends, as JSON.
type singleInstanceMessage struct {
	Args       []string `json:"args"`
	WorkingDir string   `json:"dir"`
}

var (
	singleInstanceClassOnce sync.Once
	singleInstanceClassErr  error
	singleInstanceMutex     sync.Mutex
	singleInstanceByHWnd    = make(map[win.HWND]*SingleInstance)
)

// SingleInstance makes sure that a program runs only once in the user's
// session (Windows logon session). A second start hands its command line
// arguments and working directory to the first instance, which publishes
// OtherInstanceStarted on its UI thread and brings its main window to the
// foreground, and then exits.
//
// Typical use, on the UI goroutine (main) before any window:
//
//	si, err := walk.NewSingleInstance("Example.Editor")
//	if err != nil {
//		log.Fatal(err)
//	}
//	if !si.IsFirst() {
//		return // the first instance got os.Args[1:]
//	}
//	defer si.Close()
//	mw := ... // create the main window
//	si.OtherInstanceStarted().Attach(func(args []string, dir string) {
//		... // open the files in args, relative to dir
//	})
//	if err := si.Listen(mw); err != nil {
//		log.Print(err)
//	}
//	mw.Run()
//
// The first instance is the one that owns a named mutex in the Local\
// namespace (one per session), so of two programs started at the same time
// exactly one becomes the first. The others wait for it to call Listen,
// also when it is still starting, and send it a WM_COPYDATA message to a
// hidden message-only window.
type SingleInstance struct {
	appID     string
	name      string
	first     bool
	mutex     win.HANDLE
	hWnd      win.HWND
	threadID  uint32 // of NewSingleInstance, which owns the mutex
	form      Form
	pending   []singleInstanceMessage
	publisher SingleInstanceEventPublisher
}

// NewSingleInstance claims appID for this program in the user's session.
//
// appID identifies the program (and not a version or a window), for
// example "Company.Product"; it must not be empty. Backslashes in it are
// replaced with underscores.
//
// If no other instance of the program with this appID runs, it returns a
// SingleInstance whose IsFirst is true; the program then creates its main
// window and calls Listen. Otherwise it sends the command line arguments
// (os.Args[1:]) and the working directory of this process to the first
// instance, lets it take the foreground (AllowSetForegroundWindow), and
// returns a SingleInstance whose IsFirst is false; the program should then
// exit. It returns an error if the first instance does not call Listen or
// accept the arguments within SingleInstanceTimeout, for example because
// it hangs, and at once if the first instance runs elevated while this one
// does not: Windows does not pass WM_COPYDATA to a process with a higher
// integrity level.
//
// It must be called on the UI goroutine of the program, which it locks to
// its OS thread: the mutex belongs to the thread, and Windows releases it
// when the thread exits.
func NewSingleInstance(appID string) (*SingleInstance, error) {
	if appID == "" {
		return nil, newError("NewSingleInstance: empty appID")
	}

	si := &SingleInstance{appID: appID, name: singleInstanceName(appID)}
	mutexName, err := syscall.UTF16PtrFromString(`Local\` + si.name)
	if err != nil {
		return nil, wrapErrorNoPanic(fmt.Errorf("NewSingleInstance: %w", err))
	}

	runtime.LockOSThread()
	si.threadID = win.GetCurrentThreadId()
	fail := func(err error) (*SingleInstance, error) {
		runtime.UnlockOSThread()
		return nil, err
	}

	deadline := time.Now().Add(SingleInstanceTimeout)
	var sendErr error
	for {
		// An elevated first instance creates the mutex with a security
		// descriptor that does not let a normal process open it.
		elevated := false
		if h, errno := win.CreateMutex(0, false, mutexName); h == 0 {
			if errno != win.ERROR_ACCESS_DENIED {
				return fail(errnoError("NewSingleInstance: CreateMutex", errno))
			}
			elevated = true
		} else {
			// Owning the mutex makes the first instance;
			// WAIT_ABANDONED means that the previous first instance
			// ended without releasing it.
			switch r, errno := win.WaitForSingleObject(h, 0); r {
			case win.WAIT_OBJECT_0, win.WAIT_ABANDONED:
				si.first = true
				si.mutex = h
				return si, nil

			case win.WAIT_TIMEOUT:
				win.CloseHandle(h)

			default:
				err := errnoError("NewSingleInstance: WaitForSingleObject", errno)
				win.CloseHandle(h)
				return fail(err)
			}
		}

		hwnd, err := si.findFirst()
		if err != nil {
			return fail(err)
		}
		if hwnd != 0 {
			errno, err := si.sendToFirst(hwnd, deadline)
			if err == nil {
				runtime.UnlockOSThread()
				return si, nil
			}
			if elevated || errno == win.ERROR_ACCESS_DENIED {
				return fail(newError(fmt.Sprintf("NewSingleInstance: the first instance of %q runs with higher privileges (elevated), and Windows does not let this instance pass its arguments to it", appID)))
			}
			// The first instance may be ending (its window is gone or
			// its message loop has stopped) or still busy; try again,
			// maybe as the first.
			sendErr = err
		}

		// The first instance is still starting (it has not called
		// Listen yet) or just ending; try again, maybe as the first.
		if time.Now().After(deadline) {
			if sendErr != nil {
				return fail(sendErr)
			}
			return fail(newError(fmt.Sprintf("NewSingleInstance: the first instance of %q does not listen", appID)))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// singleInstanceName returns the name of the mutex (after Local\) and of
// the message window for appID.
func singleInstanceName(appID string) string {
	const prefix = "walk.SingleInstance."
	name := prefix + strings.ReplaceAll(appID, `\`, "_")
	if len(name) > 200 {
		sum := sha256.Sum256([]byte(appID))
		name = prefix + hex.EncodeToString(sum[:])
	}
	return name
}

// findFirst returns the message window of the first instance, or 0 if it
// does not exist yet.
func (si *SingleInstance) findFirst() (win.HWND, error) {
	class, err := syscall.UTF16PtrFromString(singleInstanceWindowClass)
	if err != nil {
		return 0, err
	}
	title, err := syscall.UTF16PtrFromString(si.name)
	if err != nil {
		return 0, err
	}
	return win.FindWindowEx(win.HWND_MESSAGE, 0, class, title), nil
}

// sendToFirst sends the arguments and the working directory of this
// process to the message window of the first instance. On failure it also
// returns the error code of SendMessageTimeout, if any.
func (si *SingleInstance) sendToFirst(hwnd win.HWND, deadline time.Time) (syscall.Errno, error) {
	dir, _ := os.Getwd()
	data, err := json.Marshal(singleInstanceMessage{Args: os.Args[1:], WorkingDir: dir})
	if err != nil {
		return 0, wrapErrorNoPanic(fmt.Errorf("NewSingleInstance: %w", err))
	}
	if len(data) > singleInstanceMaxData {
		return 0, newError("NewSingleInstance: the command line is too long")
	}

	// The second instance usually has the foreground right (the user just
	// started it); pass it on, so that the first instance may activate
	// its window.
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	if pid != 0 {
		win.AllowSetForegroundWindow(pid)
	}

	// cds may stay on the stack; that is safe, because the stack of a
	// goroutine does not move during a system call, and Windows copies
	// the data into the receiving process before the sender waits.
	cds := &win.COPYDATASTRUCT{
		DwData: singleInstanceCopyDataID,
		CbData: uint32(len(data)),
		LpData: unsafe.Pointer(unsafe.SliceData(data)),
	}
	timeout := time.Until(deadline)
	if timeout < time.Second {
		timeout = time.Second
	}
	var result uintptr
	r, errno := win.SendMessageTimeout(hwnd, win.WM_COPYDATA, 0, uintptr(unsafe.Pointer(cds)), win.SMTO_ABORTIFHUNG, uint32(timeout/time.Millisecond), &result)
	runtime.KeepAlive(cds)
	runtime.KeepAlive(data)
	if r == 0 {
		return errno, newErrorNoPanic(fmt.Sprintf("NewSingleInstance: SendMessageTimeout: %v", errno))
	}
	if result != 1 {
		return 0, newErrorNoPanic("NewSingleInstance: the first instance did not accept the arguments")
	}
	return 0, nil
}

// IsFirst returns whether this is the first instance of the program. If it
// is false, NewSingleInstance has passed the arguments to the first
// instance and the program should exit.
func (si *SingleInstance) IsFirst() bool {
	return si.first
}

// AppID returns the application ID passed to NewSingleInstance.
func (si *SingleInstance) AppID() string {
	return si.appID
}

// OtherInstanceStarted returns the event that is published on the thread
// that called Listen when another instance of the program has handed over
// its command line arguments and working directory. form passed to Listen
// is already restored and in the foreground then.
func (si *SingleInstance) OtherInstanceStarted() *SingleInstanceEvent {
	return si.publisher.Event()
}

// Listen creates the hidden message-only window through which later
// instances reach this one. Call it on the UI thread, after attaching to
// OtherInstanceStarted and shortly before the message loop runs (for
// example right before Run of the main window): later instances wait until
// then, and the event is published from the message loop.
//
// form, which may be nil, is restored if it is minimized and brought to the
// foreground each time another instance starts; a hidden form stays
// hidden (a handler may show it). When form is disposed, the message
// window is destroyed too, so later instances no longer reach this one;
// they become the first instance once Close releases the claim or the
// program exits.
//
// Listen returns an error if this is not the first instance or Listen has
// already been called.
func (si *SingleInstance) Listen(form Form) error {
	if !si.first {
		return newError("SingleInstance.Listen: not the first instance")
	}
	if si.hWnd != 0 || si.mutex == 0 {
		return newError("SingleInstance.Listen: already listening or closed")
	}

	singleInstanceClassOnce.Do(func() {
		hInst := win.GetModuleHandle(nil)
		var wc win.WNDCLASSEX
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		wc.LpfnWndProc = syscall.NewCallback(singleInstanceWndProc)
		wc.HInstance = hInst
		wc.LpszClassName = win.StringToUTF16Ptr(singleInstanceWindowClass)
		if win.RegisterClassEx(&wc) == 0 {
			singleInstanceClassErr = lastError("SingleInstance.Listen: RegisterClassEx")
		}
	})
	if singleInstanceClassErr != nil {
		return singleInstanceClassErr
	}

	// The window gets its title, by which other instances find it, only
	// after it is in the map.
	hwnd := win.CreateWindowEx(0, win.StringToUTF16Ptr(singleInstanceWindowClass), nil, 0, 0, 0, 0, 0, win.HWND_MESSAGE, 0, 0, nil)
	if hwnd == 0 {
		return lastError("SingleInstance.Listen: CreateWindowEx")
	}
	si.hWnd = hwnd
	si.form = form
	if form != nil {
		form.Disposing().Once(func() {
			if si.hWnd == hwnd {
				si.destroyWindow()
			}
		})
	}
	singleInstanceMutex.Lock()
	singleInstanceByHWnd[hwnd] = si
	singleInstanceMutex.Unlock()

	title, err := syscall.UTF16PtrFromString(si.name)
	if err == nil && !win.SetWindowText(hwnd, title) {
		err = lastError("SingleInstance.Listen: SetWindowText")
	}
	if err != nil {
		si.destroyWindow()
		return err
	}
	return nil
}

// Close stops listening and releases the claim, so that the next start of
// the program becomes the first instance. It must be called on the UI
// goroutine that called NewSingleInstance (and Listen), otherwise it returns
// an error; Windows releases everything when the program exits anyway.
// Close does nothing for an instance that is not the first.
func (si *SingleInstance) Close() error {
	if !si.first || si.mutex == 0 {
		return nil
	}
	if win.GetCurrentThreadId() != si.threadID {
		return newError("SingleInstance.Close: must be called on the thread that called NewSingleInstance")
	}
	if si.hWnd != 0 {
		si.destroyWindow()
	}
	win.ReleaseMutex(si.mutex)
	win.CloseHandle(si.mutex)
	si.mutex = 0
	runtime.UnlockOSThread()
	return nil
}

func (si *SingleInstance) destroyWindow() {
	if si.hWnd == 0 {
		return
	}
	singleInstanceMutex.Lock()
	delete(singleInstanceByHWnd, si.hWnd)
	singleInstanceMutex.Unlock()
	win.DestroyWindow(si.hWnd)
	si.hWnd = 0
	si.form = nil
	si.pending = nil
}

func singleInstanceWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	singleInstanceMutex.Lock()
	si := singleInstanceByHWnd[hwnd]
	singleInstanceMutex.Unlock()
	if si == nil {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case win.WM_COPYDATA:
		cds := ptrFromUintptr[win.COPYDATASTRUCT](lParam)
		if cds == nil || cds.DwData != singleInstanceCopyDataID || cds.LpData == nil ||
			cds.CbData == 0 || cds.CbData > singleInstanceMaxData {
			return 0
		}
		if si.form != nil && si.form.Handle() == 0 {
			// The form is gone; do not accept arguments nobody handles.
			return 0
		}
		// The data is valid only during this message; Unmarshal copies it.
		var m singleInstanceMessage
		if err := json.Unmarshal(unsafe.Slice((*byte)(cds.LpData), cds.CbData), &m); err != nil {
			return 0
		}
		if m.Args == nil {
			m.Args = []string{}
		}
		si.pending = append(si.pending, m)

		// Activate now, while the foreground right that the other
		// instance passed on is still valid; publish after returning,
		// so that the other instance does not wait for the handlers.
		if si.form != nil {
			si.form.AsFormBase().BringToForeground()
		}
		win.PostMessage(hwnd, singleInstancePublishMsg, 0, 0)
		return 1

	case win.WM_DESTROY:
		// Destroyed by someone else than destroyWindow.
		singleInstanceMutex.Lock()
		delete(singleInstanceByHWnd, hwnd)
		singleInstanceMutex.Unlock()
		si.hWnd = 0
		si.form = nil

	case singleInstancePublishMsg:
		pending := si.pending
		si.pending = nil
		for _, m := range pending {
			si.publisher.Publish(m.Args, m.WorkingDir)
		}
		return 0
	}

	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}
