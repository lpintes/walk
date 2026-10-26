// Command hotkeys checks on Windows the global hotkeys and the single
// instance support of step 14 part 1.
//
// Single instance (walk.NewSingleInstance), with copies of this program
// started as other instances (the environment variable WALK_HOTKEYS_CHILD
// selects the child mode):
//
//   - The test itself claims an application ID and is the first instance.
//   - A copy started before the test calls Listen (the first instance is
//     still starting) must wait, then hand over its arguments (with spaces,
//     non-ASCII characters and an empty one) and its working directory, and
//     exit with code 0 (exitForwarded). OtherInstanceStarted must get them
//     on the UI thread.
//   - A copy started after Listen does the same.
//   - Two copies started at the same time with a new application ID:
//     exactly one becomes the first instance (exit code 2, exitFirst) and
//     gets the arguments of the other, which exits with code 0.
//   - Whether the window got the foreground is printed as INFO (Windows
//     decides; the test does not own the foreground right).
//
// Global hotkeys (walk.NewGlobalHotkey): one of Ctrl+Alt+Shift+F9 to F11
// is registered (all taken by other programs gives SKIP), the same
// combination again must fail with ErrHotkeyAlreadyRegistered, also in a
// copy of this program (exit code 3, exitTaken). WM_HOTKEY posted to the
// window with the hotkey's ID must publish Activated once on the UI thread,
// with another ID not at all. Also checked: the options (Repeat), Win as a
// modifier (INFO if Windows reserves the combination), invalid arguments,
// registration from another thread, Dispose (the combination is free
// again, a later WM_HOTKEY is ignored), Dispose from another goroutine and
// the disposal of the owner window. No keyboard input is sent.
//
// It prints PASS, FAIL, INFO and SKIP lines and exits with code 1 if a
// check failed.
//
// With the argument "manual" it is a test by hand with a screen reader: it
// claims the application ID walk.tests.hotkeys.manual, registers
// Ctrl+Alt+Shift+W and announces each press in a polite live region after
// bringing its window to the foreground. Starting "hotkeys.exe manual" again
// with more arguments hands them to the running window, which announces
// them; the second start exits with code 0.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
)

// Exit codes of the child modes.
const (
	exitForwarded = 0 // another instance got the arguments
	exitError     = 1
	exitFirst     = 2 // became the first instance and got the arguments of another one
	exitTaken     = 3 // the hotkey was registered by another program
	exitFree      = 4 // the hotkey could be registered
)

const childEnv = "WALK_HOTKEYS_CHILD"

var (
	failed   bool
	mw       *walk.MainWindow
	uiThread uint32
)

func check(name string, ok bool, format string, args ...any) bool {
	if ok {
		fmt.Println("PASS", name)
		return true
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
	return false
}

func info(format string, args ...any) {
	fmt.Printf("INFO "+format+"\n", args...)
}

// msg returns the message of err without the stack of walk errors.
func msg(err error) string {
	var we *walk.Error
	if errors.As(err, &we) {
		return we.Message()
	}
	if err == nil {
		return "<nil>"
	}
	return err.Error()
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

// received is what OtherInstanceStarted got.
type received struct {
	args     []string
	dir      string
	onThread bool
}

// child is a copy of this program started as another instance.
type child struct {
	cmd  *exec.Cmd
	out  strings.Builder
	done chan struct{}
	code int
	err  error
}

func startChild(mode, dir string, args ...string) *child {
	exe, _ := os.Executable()
	c := &child{cmd: exec.Command(exe, args...), done: make(chan struct{})}
	c.cmd.Env = append(os.Environ(), childEnv+"="+mode)
	c.cmd.Dir = dir
	c.cmd.Stdout = &c.out
	c.cmd.Stderr = &c.out
	if err := c.cmd.Start(); err != nil {
		c.err = err
		c.code = -1
		close(c.done)
		return c
	}
	go func() {
		defer close(c.done)
		c.err = c.cmd.Wait()
		c.code = c.cmd.ProcessState.ExitCode()
	}()
	return c
}

// wait waits for the child to exit, at most d.
func (c *child) wait(d time.Duration) bool {
	select {
	case <-c.done:
		return true
	case <-time.After(d):
		c.cmd.Process.Kill()
		<-c.done
		return false
	}
}

func (c *child) running() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *child) output() string {
	return strings.TrimSpace(c.out.String())
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func main() {
	if mode := os.Getenv(childEnv); mode != "" {
		os.Exit(runChild(mode))
	}
	if len(os.Args) > 1 && os.Args[1] == "manual" {
		manual()
		return
	}

	uiThread = win.GetCurrentThreadId()
	appID := fmt.Sprintf("walk.tests.hotkeys.%d", os.Getpid())
	si, err := walk.NewSingleInstance(appID)
	if !check("NewSingleInstance first instance", err == nil && si != nil && si.IsFirst(), "err %s", msg(err)) {
		os.Exit(1)
	}
	check("SingleInstance AppID", si.AppID() == appID, "%q", si.AppID())

	dir, err := os.MkdirTemp("", "walk-hotkeys-test-")
	if err != nil {
		check("MkdirTemp", false, "%v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	// Started while this instance is still starting: it must wait for
	// Listen.
	earlyArgs := []string{"alpha", "with space", "ľščťžýáíé", "", `C:\some "quoted" path`}
	early := startChild("instance:"+appID, dir, earlyArgs...)

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "walk hotkeys test",
		Size:     Size{Width: 400, Height: 200},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "The checks of this test run by themselves."},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}

	got := make(chan received, 8)
	si.OtherInstanceStarted().Attach(func(args []string, workingDir string) {
		got <- received{args, workingDir, win.GetCurrentThreadId() == uiThread}
	})

	time.Sleep(1500 * time.Millisecond)
	if early.running() {
		check("second instance waits while the first is starting", true, "")
	} else {
		check("second instance waits while the first is starting", false,
			"exited with %d before Listen: %s", early.code, early.output())
	}

	err = si.Listen(mw)
	check("Listen", err == nil, "%s", msg(err))
	err = si.Listen(mw)
	check("Listen twice is an error", err != nil, "no error")

	checksDone := make(chan struct{})
	go func() {
		defer close(checksDone)
		checkSingleInstance(appID, early, earlyArgs, dir, got)
		checkRace(dir)
		checkHotkeys()
		mw.Synchronize(func() { mw.Close() })
	}()
	mw.Run()
	<-checksDone

	check("SingleInstance Close", si.Close() == nil, "error")

	if failed {
		os.Exit(1)
	}
}

func checkForwarded(name string, c *child, args []string, dir string, got <-chan received) {
	if !c.wait(2 * walk.SingleInstanceTimeout) {
		check(name+": second instance exits", false, "killed after %v: %s", 2*walk.SingleInstanceTimeout, c.output())
		return
	}
	check(name+": second instance exits with exitForwarded", c.code == exitForwarded && strings.Contains(c.output(), "FORWARDED"),
		"exit code %d, output %q", c.code, c.output())

	select {
	case r := <-got:
		check(name+": arguments", slices.Equal(r.args, args), "got %q, want %q", r.args, args)
		check(name+": working directory", samePath(r.dir, dir), "got %q, want %q", r.dir, dir)
		check(name+": published on the UI thread", r.onThread, "on another thread")
	case <-time.After(5 * time.Second):
		check(name+": OtherInstanceStarted", false, "not published")
	}

	var fg bool
	onUI(func() { fg = win.GetForegroundWindow() == mw.Handle() })
	info("%s: the window is the foreground window afterwards: %v", name, fg)
}

func checkSingleInstance(appID string, early *child, earlyArgs []string, dir string, got <-chan received) {
	checkForwarded("started before Listen", early, earlyArgs, dir, got)

	later := startChild("instance:"+appID, dir, "beta")
	checkForwarded("started after Listen", later, []string{"beta"}, dir, got)

	noArgs := startChild("instance:"+appID, os.TempDir())
	checkForwarded("started without arguments", noArgs, []string{}, os.TempDir(), got)
}

// checkRace starts two instances of a new application ID at the same time.
func checkRace(dir string) {
	appID := fmt.Sprintf("walk.tests.hotkeys.race.%d", os.Getpid())
	a := startChild("instance:"+appID, dir, "race", "1")
	b := startChild("instance:"+appID, dir, "race", "2")
	okA := a.wait(2 * walk.SingleInstanceTimeout)
	okB := b.wait(2 * walk.SingleInstanceTimeout)
	if !check("race: both instances exit", okA && okB, "a %v %q, b %v %q", okA, a.output(), okB, b.output()) {
		return
	}
	first, second, secondArg := a, b, "2"
	if b.code == exitFirst {
		first, second, secondArg = b, a, "1"
	}
	check("race: one first and one forwarded instance",
		first.code == exitFirst && second.code == exitForwarded,
		"exit codes %d and %d, outputs %q and %q", a.code, b.code, a.output(), b.output())
	want, _ := json.Marshal([]string{"race", secondArg})
	check("race: the first got the arguments of the other", strings.Contains(first.output(), "RECEIVED "+string(want)),
		"output %q", first.output())
}

// postHotkey posts WM_HOTKEY as Windows does: wParam is the ID, the low
// word of lParam the MOD_* flags and the high word the virtual key.
func postHotkey(id int, mods uint32, key walk.Key) {
	win.PostMessage(mw.Handle(), win.WM_HOTKEY, uintptr(id), uintptr(mods)|uintptr(key)<<16)
}

// counter counts the Activated events of a hotkey.
type counter struct {
	mu       sync.Mutex
	n        int
	onThread bool
}

func (c *counter) attach(h *walk.GlobalHotkey) {
	c.onThread = true
	h.Activated().Attach(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.n++
		c.onThread = c.onThread && win.GetCurrentThreadId() == uiThread
	})
}

func (c *counter) get() (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n, c.onThread
}

// settle lets the message loop handle what was posted.
func settle() {
	time.Sleep(300 * time.Millisecond)
	onUI(func() {})
}

func register(owner walk.Window, s walk.Shortcut, opts walk.GlobalHotkeyOptions) (h *walk.GlobalHotkey, err error) {
	onUI(func() { h, err = walk.NewGlobalHotkeyWithOptions(owner, s, opts) })
	return
}

func checkHotkeys() {
	check("Modifiers String with Win", (walk.ModWin|walk.ModControl).String() == "Win+Ctrl" && walk.ModWin.String() == "Win",
		"%q %q", (walk.ModWin | walk.ModControl).String(), walk.ModWin.String())

	all := walk.ModControl | walk.ModAlt | walk.ModShift
	var h *walk.GlobalHotkey
	var s walk.Shortcut
	var err error
	for _, key := range []walk.Key{walk.KeyF9, walk.KeyF10, walk.KeyF11} {
		s = walk.Shortcut{Modifiers: all, Key: key}
		h, err = register(mw, s, walk.GlobalHotkeyOptions{})
		if err == nil || !errors.Is(err, walk.ErrHotkeyAlreadyRegistered) {
			break
		}
		info("%s is registered by another program", s)
	}
	if errors.Is(err, walk.ErrHotkeyAlreadyRegistered) {
		fmt.Println("SKIP global hotkeys: Ctrl+Alt+Shift+F9 to F11 are all registered by other programs")
		return
	}
	if !check("NewGlobalHotkey "+s.String(), err == nil, "%s", msg(err)) {
		return
	}
	check("GlobalHotkey Shortcut, Owner, ID, Repeat", h.Shortcut() == s && h.Owner() == walk.Window(mw) && h.ID() >= 1 && !h.Repeat(),
		"%v %v %d %v", h.Shortcut(), h.Owner(), h.ID(), h.Repeat())
	info("registered %s with ID %d", s, h.ID())
	var c counter
	c.attach(h)

	// The same combination again, in this program and in another one.
	_, err = register(mw, s, walk.GlobalHotkeyOptions{})
	check("same combination again: ErrHotkeyAlreadyRegistered", errors.Is(err, walk.ErrHotkeyAlreadyRegistered), "err %s", msg(err))
	info("error text: %s", msg(err))
	other := startChild(fmt.Sprintf("hotkey:%d:%d", s.Modifiers, s.Key), os.TempDir())
	other.wait(30 * time.Second)
	check("same combination in another program: ErrHotkeyAlreadyRegistered", other.code == exitTaken,
		"exit code %d, output %q", other.code, other.output())

	// Delivery.
	mods := uint32(win.MOD_CONTROL | win.MOD_ALT | win.MOD_SHIFT)
	postHotkey(h.ID(), mods, s.Key)
	settle()
	n, onThread := c.get()
	check("WM_HOTKEY publishes Activated once", n == 1, "%d times", n)
	check("Activated on the UI thread", onThread, "on another thread")
	postHotkey(h.ID()+100, mods, s.Key)
	settle()
	n, _ = c.get()
	check("WM_HOTKEY with another ID is ignored", n == 1, "%d times", n)

	// Invalid arguments and the wrong thread.
	_, err = register(mw, walk.Shortcut{Modifiers: walk.ModControl}, walk.GlobalHotkeyOptions{})
	check("NewGlobalHotkey without a key is an error", err != nil, "no error")
	_, err = register(nil, s, walk.GlobalHotkeyOptions{})
	check("NewGlobalHotkey without an owner is an error", err != nil, "no error")
	_, err = walk.NewGlobalHotkey(mw, walk.Shortcut{Modifiers: all, Key: walk.KeyF12})
	check("NewGlobalHotkey on another thread is an error", err != nil, "no error")

	// Options: autorepeat; a second hotkey gets another ID.
	s2 := walk.Shortcut{Modifiers: walk.ModControl | walk.ModAlt | walk.ModShift, Key: walk.KeyF8}
	h2, err := register(mw, s2, walk.GlobalHotkeyOptions{Repeat: true})
	if err == nil {
		check("Repeat option", h2.Repeat() && h2.ID() != h.ID(), "repeat %v, IDs %d and %d", h2.Repeat(), h2.ID(), h.ID())
		var c2 counter
		c2.attach(h2)
		postHotkey(h2.ID(), mods, s2.Key)
		settle()
		n2, _ := c2.get()
		n, _ = c.get()
		check("WM_HOTKEY reaches only its hotkey", n2 == 1 && n == 1, "second %d, first %d", n2, n)
		onUI(h2.Dispose)
	} else {
		info("%s with Repeat: %s", s2, msg(err))
	}

	// The Windows key.
	sw := walk.Shortcut{Modifiers: walk.ModWin | all, Key: walk.KeyF9}
	hw, err := register(mw, sw, walk.GlobalHotkeyOptions{})
	if err == nil {
		check("Win modifier", hw.Shortcut().String() == "Win+Alt+Ctrl+Shift+F9", "%q", hw.Shortcut().String())
		onUI(hw.Dispose)
	} else {
		info("%s: %s", sw, msg(err))
	}

	// Dispose frees the combination; a late WM_HOTKEY is ignored.
	oldID := h.ID()
	onUI(h.Dispose)
	check("Dispose", h.IsDisposed(), "not disposed")
	postHotkey(oldID, mods, s.Key)
	settle()
	n, _ = c.get()
	check("WM_HOTKEY after Dispose is ignored", n == 1, "%d times", n)
	h3, err := register(mw, s, walk.GlobalHotkeyOptions{})
	if check("register again after Dispose", err == nil, "%s", msg(err)) {
		// Dispose from another goroutine goes through Synchronize.
		h3.Dispose()
		settle()
		check("Dispose from another goroutine", h3.IsDisposed(), "not disposed")
		h4, err := register(mw, s, walk.GlobalHotkeyOptions{})
		if check("register again after Dispose from another goroutine", err == nil, "%s", msg(err)) {
			onUI(h4.Dispose)
		}
	}

	// Disposing of the owner unregisters its hotkeys.
	var w2 *walk.MainWindow
	var h5 *walk.GlobalHotkey
	onUI(func() {
		w2, err = walk.NewMainWindow()
		if err == nil {
			h5, err = walk.NewGlobalHotkey(w2, s)
		}
	})
	if check("hotkey of a second window", err == nil, "%s", msg(err)) {
		onUI(w2.Dispose)
		check("owner Dispose disposes the hotkey", h5.IsDisposed(), "not disposed")
		h6, err := register(mw, s, walk.GlobalHotkeyOptions{})
		if check("register again after owner Dispose", err == nil, "%s", msg(err)) {
			onUI(h6.Dispose)
		}
	}
}

// runChild runs a child mode and returns the exit code.
func runChild(mode string) int {
	kind, arg, _ := strings.Cut(mode, ":")
	switch kind {
	case "instance":
		return childInstance(arg)
	case "hotkey":
		return childHotkey(arg)
	}
	fmt.Println("unknown child mode", mode)
	return exitError
}

// childInstance is another instance of the program with the application
// ID appID. If it becomes the first instance, it waits for one other
// instance and prints its arguments.
func childInstance(appID string) int {
	si, err := walk.NewSingleInstance(appID)
	if err != nil {
		fmt.Println("ERROR", msg(err))
		return exitError
	}
	if !si.IsFirst() {
		fmt.Println("FORWARDED")
		return exitForwarded
	}
	defer si.Close()

	var w *walk.MainWindow
	if err := (MainWindow{
		AssignTo: &w,
		Title:    "walk hotkeys test, first instance " + appID,
		Size:     Size{Width: 300, Height: 100},
		Layout:   VBox{},
	}).Create(); err != nil {
		fmt.Println("ERROR", err)
		return exitError
	}
	code := exitError
	si.OtherInstanceStarted().Attach(func(args []string, dir string) {
		b, _ := json.Marshal(args)
		fmt.Printf("RECEIVED %s %s\n", b, dir)
		code = exitFirst
		w.Close()
	})
	if err := si.Listen(w); err != nil {
		fmt.Println("ERROR", msg(err))
		return exitError
	}
	go func() {
		time.Sleep(walk.SingleInstanceTimeout)
		w.Synchronize(func() {
			fmt.Println("ERROR no other instance")
			w.Close()
		})
	}()
	w.Run()
	return code
}

// childHotkey tries to register the hotkey "mods:key" for a window.
func childHotkey(arg string) int {
	m, k, _ := strings.Cut(arg, ":")
	mods, _ := strconv.Atoi(m)
	key, _ := strconv.Atoi(k)
	w, err := walk.NewMainWindow()
	if err != nil {
		fmt.Println("ERROR", err)
		return exitError
	}
	defer w.Dispose()
	h, err := walk.NewGlobalHotkey(w, walk.Shortcut{Modifiers: walk.Modifiers(mods), Key: walk.Key(key)})
	switch {
	case errors.Is(err, walk.ErrHotkeyAlreadyRegistered):
		fmt.Println("TAKEN", msg(err))
		return exitTaken
	case err != nil:
		fmt.Println("ERROR", msg(err))
		return exitError
	}
	h.Dispose()
	fmt.Println("FREE")
	return exitFree
}

func manual() {
	si, err := walk.NewSingleInstance("walk.tests.hotkeys.manual")
	if err != nil {
		fmt.Println("FAIL NewSingleInstance:", msg(err))
		os.Exit(1)
	}
	if !si.IsFirst() {
		fmt.Println("Handed the arguments to the running instance.")
		return
	}
	defer si.Close()

	shortcut := walk.Shortcut{Modifiers: walk.ModControl | walk.ModAlt | walk.ModShift, Key: walk.KeyW}
	var status *walk.Label
	var toggle *walk.PushButton
	var hotkey *walk.GlobalHotkey
	presses := 0
	report := func(text string) {
		fmt.Println(text)
		status.SetText(text)
	}
	registerHotkey := func() {
		h, err := walk.NewGlobalHotkey(mw, shortcut)
		if err != nil {
			report(fmt.Sprintf("Could not register %s: %s", shortcut, msg(err)))
			return
		}
		hotkey = h
		h.Activated().Attach(func() {
			presses++
			if err := mw.BringToForeground(); err != nil {
				fmt.Println("BringToForeground:", msg(err))
			}
			report(fmt.Sprintf("%s pressed, %d times so far.", shortcut, presses))
		})
		toggle.SetText("&Unregister the hotkey")
		report(fmt.Sprintf("%s is registered. Switch to another program and press it.", shortcut))
	}

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "walk hotkeys manual test",
		Size:     Size{Width: 500, Height: 250},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "Press Ctrl+Alt+Shift+W anywhere. Start hotkeys.exe manual again with more arguments to test the single instance."},
			PushButton{AssignTo: &toggle, Text: "&Register the hotkey", OnClicked: func() {
				if hotkey != nil && !hotkey.IsDisposed() {
					hotkey.Dispose()
					hotkey = nil
					toggle.SetText("&Register the hotkey")
					report(fmt.Sprintf("%s is unregistered.", shortcut))
					return
				}
				registerHotkey()
			}},
			Label{AssignTo: &status, Text: "Starting", Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite}},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL", err)
		os.Exit(1)
	}

	si.OtherInstanceStarted().Attach(func(args []string, dir string) {
		report(fmt.Sprintf("Another instance started in %s with %d arguments: %s", dir, len(args), strings.Join(args, ", ")))
	})
	if err := si.Listen(mw); err != nil {
		fmt.Println("Listen:", msg(err))
	}
	registerHotkey()
	mw.Run()
}
