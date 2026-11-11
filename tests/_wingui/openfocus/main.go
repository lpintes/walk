// Command openfocus checks on Windows that a form gets the foreground and the
// focus when it opens, so that screen readers announce it.
//
// walk shows forms with SW_SHOWNA, which does not activate them; the focus
// that the first layout gives to the first focusable control activates the
// form. A form without a focusable control used to stay in the background,
// and screen readers said nothing about it.
//
//   - A MainWindow with only a label: it must get the foreground within
//     three seconds and have the focus itself. This check comes first,
//     while the process still has the right to take the foreground it got
//     when it was started.
//   - A Dialog owned by it with a label, an edit field and OK and Cancel:
//     the dialog must be the foreground window and the edit field must have
//     the focus.
//   - A Dialog with only a label: the dialog must be the foreground window
//     and have the focus itself.
//   - After each dialog closes, the MainWindow is the foreground window
//     again.
//
// The test never calls SetForegroundWindow, because that is what it checks.
// An out of context WinEvent hook prints the foreground and focus events of
// the process as INFO lines, to compare with what a screen reader says.
// "openfocus manual" opens the same windows but leaves them open, for a test
// with a screen reader.
package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	setWinEventHook = user32.NewProc("SetWinEventHook")
	getClassName    = user32.NewProc("GetClassNameW")
)

const (
	eventSystemForeground = 0x0003
	eventObjectFocus      = 0x8005
)

var (
	failed bool
	start  = time.Now()
	mw     *walk.MainWindow
)

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Println("PASS", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: "+format+"\n", append([]any{name}, args...)...)
}

func className(hwnd win.HWND) string {
	buf := make([]uint16, 256)
	getClassName.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func winEventProc(hook uintptr, event uint32, hwnd win.HWND, idObject, idChild int32, thread, t uint32) uintptr {
	var name string
	switch event {
	case eventSystemForeground:
		name = "foreground"
	case eventObjectFocus:
		name = "focus"
	default:
		return 0
	}
	fmt.Printf("INFO %5d ms %s %#x %q\n", time.Since(start).Milliseconds(), name, hwnd, className(hwnd))
	return 0
}

func hook(event uint32) {
	setWinEventHook.Call(uintptr(event), uintptr(event), 0, syscall.NewCallback(winEventProc), uintptr(os.Getpid()), 0, 0)
}

// onUI runs f on the UI thread and waits for it.
func onUI(f func()) {
	done := make(chan struct{})
	mw.Synchronize(func() {
		defer close(done)
		f()
	})
	<-done
}

// waitFor waits up to three seconds for hwnd to be the foreground window
// and for the focus of the UI thread to be focus, and reports both.
func waitFor(name string, hwnd, focus win.HWND) {
	var fg, foc win.HWND
	for i := 0; i < 30; i++ {
		fg = win.GetForegroundWindow()
		onUI(func() { foc = win.GetFocus() })
		if fg == hwnd && foc == focus {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	check(name+" in the foreground", fg == hwnd, "foreground %#x %q, want %#x", fg, className(fg), hwnd)
	check(name+" focus", foc == focus, "focus %#x %q, want %#x %q", foc, className(foc), focus, className(focus))
}

// runDialog opens a dialog owned by mw, checks it with checkOpen on another
// goroutine, closes it and checks that mw is the foreground window again.
func runDialog(name string, children []Widget, focus func(dlg *walk.Dialog) win.HWND, manual bool) {
	var dlg *walk.Dialog
	var hwnd, want win.HWND
	opened := make(chan struct{})
	closed := make(chan struct{})

	mw.Synchronize(func() {
		defer close(closed)
		if err := (Dialog{
			AssignTo: &dlg,
			Title:    name,
			MinSize:  Size{Width: 300, Height: 150},
			Layout:   VBox{},
			Children: children,
		}).Create(mw); err != nil {
			check(name+" create", false, "%v", err)
			close(opened)
			return
		}
		hwnd = dlg.Handle()
		want = focus(dlg)
		dlg.Starting().Attach(func() { close(opened) })
		dlg.Run()
	})

	<-opened
	if dlg == nil {
		<-closed
		return
	}
	if !manual {
		waitFor(name, hwnd, want)
		onUI(func() { dlg.Cancel() })
	}
	<-closed
	if !manual {
		waitFor("main window after "+name, mw.Handle(), mw.Handle())
	}
}

func main() {
	manual := len(os.Args) > 1 && os.Args[1] == "manual"

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "Open focus test",
		Size:     Size{Width: 400, Height: 200},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "This window has no focusable control."},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}

	hook(eventSystemForeground)
	hook(eventObjectFocus)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if !manual {
			waitFor("main window without a focusable control", mw.Handle(), mw.Handle())
		}

		var edit *walk.LineEdit
		runDialog("Dialog with an edit field", []Widget{
			Label{Text: "City:"},
			LineEdit{AssignTo: &edit, Text: "Bratislava"},
			PushButton{Text: "OK", OnClicked: func() { edit.Form().(*walk.Dialog).Accept() }},
			PushButton{Text: "Cancel", OnClicked: func() { edit.Form().(*walk.Dialog).Cancel() }},
		}, func(*walk.Dialog) win.HWND { return edit.Handle() }, manual)

		runDialog("Dialog without a focusable control", []Widget{
			Label{Text: "This dialog has no focusable control. Close it with Alt+F4."},
		}, func(dlg *walk.Dialog) win.HWND { return dlg.Handle() }, manual)

		if !manual {
			mw.Synchronize(func() { mw.Close() })
		}
	}()
	mw.Run()
	<-done

	if failed {
		os.Exit(1)
	}
}
