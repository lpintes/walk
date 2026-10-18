// Command uia checks on Windows the UI Automation client package uia (step
// 13 part 1) against a walk window of its own process:
//
//   - New, ElementFromHandle, Root and FocusedElement.
//   - The properties of the window, a button, a disabled button, a check
//     box and two edit fields, one of them a password field.
//   - FindFirst and FindAll with property, And, Or, Not and
//     PropertyMatch conditions, including a condition with an unsupported
//     value.
//   - The control and raw view tree walkers from a button up to the
//     window and back down, Children and the root.
//   - SetFocus, FocusedElement, ElementFromPoint and Equal, if the window
//     is in the foreground (otherwise these print SKIP).
//   - Concurrent calls from several goroutines, elements left to the
//     garbage collector, Release, Close, and an element of a closed
//     window.
//
// The checks run on a goroutine while the window runs its message loop, as
// the package documentation requires. It prints PASS, FAIL and SKIP lines
// and exits with code 1 if a check failed.
//
// With the argument "dump" it shows no window: after three seconds it
// prints the control view tree of the foreground window, one element per
// line, indented by depth, with control type, name, automation ID and
// class name. Start it, switch to another window with Alt+Tab and wait.
package main

import (
	"errors"
	"fmt"
	"image"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
	"github.com/lpintes/walk/uia"
)

var failed bool

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Println("PASS", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
}

func checkErr(name string, err error) bool {
	if err != nil {
		check(name, false, "%v", err)
		return false
	}
	return true
}

const title = "walk uia test"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "dump" {
		dump()
		return
	}

	var mw *walk.MainWindow
	var pressed int
	if err := (MainWindow{
		AssignTo: &mw,
		Title:    title,
		Size:     Size{Width: 400, Height: 300},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "Search:"},
			LineEdit{Accessibility: Accessibility{Name: "Search"}},
			LineEdit{PasswordMode: true, Accessibility: Accessibility{Name: "Password"}},
			PushButton{Text: "Press me", OnClicked: func() { pressed++ }},
			PushButton{Text: "Disabled", Enabled: false},
			CheckBox{Text: "Option"},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}

	hwnd := uintptr(mw.Handle())
	closed := make(chan struct{})
	checksDone := make(chan struct{})
	go func() {
		defer close(checksDone)
		for i := 0; i < 100 && !win.IsWindowVisible(win.HWND(hwnd)); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		run(hwnd, func() { mw.Synchronize(func() { mw.Close() }) }, closed)
	}()
	mw.Run()
	close(closed)
	<-checksDone

	if failed {
		os.Exit(1)
	}
}

// run runs the checks. closeWindow closes the window; closed is closed
// when its message loop has ended.
func run(hwnd uintptr, closeWindow func(), closed <-chan struct{}) {
	a, err := uia.New()
	if !checkErr("New", err) {
		closeWindow()
		return
	}

	window, err := a.ElementFromHandle(hwnd)
	if !checkErr("ElementFromHandle", err) {
		closeWindow()
		return
	}
	checkWindow(window, hwnd)

	byButton := uia.And(uia.ByControlType(uia.ControlTypeButton), uia.ByClassName("Button"))
	press := findOne(window, "button Press me", uia.And(byButton, uia.ByName("Press me")))
	disabled := findOne(window, "button Disabled", uia.And(byButton, uia.ByName("Disabled")))
	option := findOne(window, "check box Option", uia.ByControlType(uia.ControlTypeCheckBox))
	if press == nil || disabled == nil || option == nil {
		closeWindow()
		return
	}
	checkControls(window, press, disabled, option)
	checkConditions(window, press, disabled, byButton)
	checkTree(a, window, press)
	checkFocus(a, window, press, hwnd)
	checkConcurrency(press)
	checkRelease(a, window, press)

	closeWindow()
	<-closed
	_, err = window.Name()
	check("element of a closed window", errors.Is(err, uia.ErrElementNotAvailable), "Name returned %v, want ErrElementNotAvailable", err)

	check("Close", a.Close() == nil, "Close returned an error")
	_, err = window.Name()
	check("element after Close", errors.Is(err, uia.ErrClosed), "Name returned %v, want ErrClosed", err)
	_, err = a.Root()
	check("Root after Close", errors.Is(err, uia.ErrClosed), "Root returned %v, want ErrClosed", err)
	check("Close again", a.Close() == nil, "second Close returned an error")
}

func findOne(root *uia.Element, name string, cond uia.Condition) *uia.Element {
	e, err := root.FindFirst(uia.ScopeDescendants, cond)
	if !checkErr("FindFirst "+name, err) {
		return nil
	}
	check("FindFirst "+name, e != nil, "not found")
	return e
}

func checkWindow(window *uia.Element, hwnd uintptr) {
	name, err := window.Name()
	check("window Name", err == nil && name == title, "%q, %v", name, err)
	ct, err := window.ControlType()
	check("window ControlType", err == nil && ct == uia.ControlTypeWindow, "%v, %v", ct, err)
	class, err := window.ClassName()
	check("window ClassName", err == nil && class == `\o/ Walk_MainWindow_Class \o/`, "%q, %v", class, err)
	pid, err := window.ProcessID()
	check("window ProcessID", err == nil && pid == os.Getpid(), "%d, %v, want %d", pid, err, os.Getpid())
	h, err := window.NativeWindowHandle()
	check("window NativeWindowHandle", err == nil && h == hwnd, "%d, %v, want %d", h, err, hwnd)
	fw, err := window.FrameworkID()
	check("window FrameworkID", err == nil && fw == "Win32", "%q, %v", fw, err)

	var r win.RECT
	win.GetWindowRect(win.HWND(hwnd), &r)
	want := image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
	got, err := window.BoundingRectangle()
	check("window BoundingRectangle", err == nil && got == want, "%v, %v, want %v", got, err, want)
}

func checkControls(window, press, disabled, option *uia.Element) {
	enabled, err := press.IsEnabled()
	check("button IsEnabled", err == nil && enabled, "%v, %v", enabled, err)
	enabled, err = disabled.IsEnabled()
	check("disabled button IsEnabled", err == nil && !enabled, "%v, %v", enabled, err)
	focusable, err := press.IsKeyboardFocusable()
	check("button IsKeyboardFocusable", err == nil && focusable, "%v, %v", focusable, err)
	offscreen, err := press.IsOffscreen()
	check("button IsOffscreen", err == nil && !offscreen, "%v, %v", offscreen, err)
	control, err := press.IsControlElement()
	check("button IsControlElement", err == nil && control, "%v, %v", control, err)
	lct, err := press.LocalizedControlType()
	check("button LocalizedControlType", err == nil && lct != "", "%q, %v", lct, err)
	h, err := press.NativeWindowHandle()
	check("button NativeWindowHandle", err == nil && h != 0, "%d, %v", h, err)

	wr, _ := window.BoundingRectangle()
	br, err := press.BoundingRectangle()
	check("button BoundingRectangle", err == nil && !br.Empty() && br.In(wr), "%v, %v, window %v", br, err, wr)

	name, err := option.Name()
	check("check box Name", err == nil && name == "Option", "%q, %v", name, err)

	edits, err := window.FindAll(uia.ScopeDescendants, uia.ByControlType(uia.ControlTypeEdit))
	if checkErr("FindAll edits", err) && check2("FindAll edits", len(edits) == 2, "%d edits, want 2", len(edits)) {
		for i, want := range []struct {
			name     string
			password bool
		}{{"Search", false}, {"Password", true}} {
			name, err := edits[i].Name()
			check("edit annotated Name "+want.name, err == nil && name == want.name, "%q, %v", name, err)
			password, err := edits[i].IsPassword()
			check("edit IsPassword "+want.name, err == nil && password == want.password, "%v, %v", password, err)
		}
	}

	ct := uia.ControlTypeButton
	check("ControlType String", ct.String() == "Button" && uia.ControlTypeAppBar.String() == "AppBar" && uia.ControlType(1).String() == "ControlType(1)",
		"%q %q %q", ct.String(), uia.ControlTypeAppBar.String(), uia.ControlType(1).String())
}

// same reports whether a and b are the same element; nil is never the same.
func same(a, b *uia.Element) bool {
	if a == nil || b == nil {
		return false
	}
	ok, err := a.Equal(b)
	return err == nil && ok
}

// check2 is check that also returns ok.
func check2(name string, ok bool, format string, args ...any) bool {
	check(name, ok, format, args...)
	return ok
}

func names(elements []*uia.Element) []string {
	var s []string
	for _, e := range elements {
		name, err := e.Name()
		if err != nil {
			name = err.Error()
		}
		s = append(s, name)
	}
	return s
}

func checkConditions(window, press, disabled *uia.Element, byButton uia.Condition) {
	buttons, err := window.FindAll(uia.ScopeDescendants, byButton)
	got := strings.Join(names(buttons), ",")
	check("FindAll And", err == nil && got == "Press me,Disabled", "%q, %v", got, err)

	buttons, err = window.FindAll(uia.ScopeDescendants, uia.And(byButton, uia.Not(uia.ByName("Press me")), uia.ByControlType(uia.ControlTypeButton)))
	got = strings.Join(names(buttons), ",")
	check("FindAll Not", err == nil && got == "Disabled", "%q, %v", got, err)

	buttons, err = window.FindAll(uia.ScopeDescendants, uia.Or(uia.ByName("Disabled"), uia.ByName("Press me"), uia.ByName("no such name")))
	got = strings.Join(names(buttons), ",")
	check("FindAll Or", err == nil && got == "Press me,Disabled", "%q, %v", got, err)

	found, err := window.FindFirst(uia.ScopeDescendants, uia.And(byButton, uia.Property(uia.PropertyIsEnabled, false)))
	ok := same(found, disabled)
	check("FindFirst bool property", err == nil && ok, "%v, same %v", err, ok)

	found, err = window.FindFirst(uia.ScopeDescendants, uia.PropertyMatch(uia.PropertyName, "PRESS", uia.MatchIgnoreCase|uia.MatchSubstring))
	ok = same(found, press)
	check("FindFirst PropertyMatch", err == nil && ok, "%v, same %v", err, ok)

	found, err = window.FindFirst(uia.ScopeDescendants, uia.ByName("no such name"))
	check("FindFirst no match", err == nil && found == nil, "%v, %v", found, err)

	found, err = window.FindFirst(uia.ScopeElement, nil)
	ok = same(found, window)
	check("FindFirst ScopeElement nil condition", err == nil && ok, "%v, same %v", err, ok)

	_, err = window.FindFirst(uia.ScopeDescendants, uia.Property(uia.PropertyName, 1.5))
	check("unsupported condition value", err != nil, "no error")
	_, err = window.FindFirst(uia.ScopeDescendants, uia.ByName("a\x00b"))
	check("condition string with NUL", err != nil, "no error")
}

func checkTree(a *uia.Automation, window, press *uia.Element) {
	for _, v := range []struct {
		name   string
		walker *uia.TreeWalker
	}{
		{"ControlView", a.ControlView()},
		{"RawView", a.RawView()},
		{"TreeWalker", a.TreeWalker(uia.Property(uia.PropertyIsControlElement, true))},
	} {
		e := press
		var path []string
		var buttonParent *uia.Element
		reached := false
		for range 5 {
			parent, err := v.walker.Parent(e)
			if err != nil || parent == nil {
				break
			}
			if buttonParent == nil {
				buttonParent = parent
			}
			ct, _ := parent.ControlType()
			path = append(path, ct.String())
			if same(parent, window) {
				reached = true
				break
			}
			e = parent
		}
		check(v.name+" Parent up to the window", reached, "path %v", path)
		if !reached {
			continue
		}

		e = buttonParent
		children, err := v.walker.Children(e)
		found := false
		for _, c := range children {
			if same(c, press) {
				found = true
			}
		}
		check(v.name+" Children", err == nil && found, "%v, %v", names(children), err)
		if !found {
			continue
		}

		first, err1 := v.walker.FirstChild(e)
		last, err2 := v.walker.LastChild(e)
		firstSame := same(first, children[0])
		lastSame := same(last, children[len(children)-1])
		check(v.name+" FirstChild and LastChild", err1 == nil && err2 == nil && firstSame && lastSame, "%v %v", err1, err2)
		if len(children) > 1 {
			prev, err := v.walker.PreviousSibling(children[1])
			ok := same(prev, children[0])
			check(v.name+" PreviousSibling", err == nil && ok, "%v", err)
		}
		next, err := v.walker.NextSibling(last)
		check(v.name+" NextSibling of the last child", err == nil && next == nil, "%v, %v", next != nil, err)
	}

	root, err := a.Root()
	if checkErr("Root", err) {
		ct, err := root.ControlType()
		check("Root ControlType", err == nil && ct == uia.ControlTypePane, "%v, %v", ct, err)
		parent, err := a.ControlView().Parent(root)
		check("Root Parent", err == nil && parent == nil, "%v, %v", parent != nil, err)
		parent, err = a.ControlView().Parent(window)
		ok := same(parent, root)
		check("window Parent is Root", err == nil && ok, "%v", err)
	}
}

func checkFocus(a *uia.Automation, window, press *uia.Element, hwnd uintptr) {
	if uintptr(win.GetForegroundWindow()) != hwnd {
		fmt.Println("SKIP SetFocus, FocusedElement and ElementFromPoint: the window is not in the foreground")
		return
	}
	edit, err := window.FindFirst(uia.ScopeDescendants, uia.ByControlType(uia.ControlTypeEdit))
	if !checkErr("FindFirst edit", err) {
		return
	}
	for _, e := range []*uia.Element{edit, press} {
		name, _ := e.Name()
		err := e.SetFocus()
		time.Sleep(200 * time.Millisecond)
		focused, err2 := a.FocusedElement()
		ok := same(focused, e)
		has, _ := e.HasKeyboardFocus()
		check("SetFocus and FocusedElement "+name, err == nil && err2 == nil && ok && has, "%v %v same %v has %v", err, err2, ok, has)
	}

	r, _ := press.BoundingRectangle()
	center := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	at, err := a.ElementFromPoint(center)
	ok := same(at, press)
	check("ElementFromPoint", err == nil && ok, "%v at %v, same %v", err, center, ok)
}

func checkConcurrency(press *uia.Element) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	bad := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				if name, err := press.Name(); err != nil || name != "Press me" {
					mu.Lock()
					bad++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	check("concurrent calls", bad == 0, "%d of 200 calls failed", bad)
}

func checkRelease(a *uia.Automation, window, press *uia.Element) {
	// Elements left to the garbage collector are released on the thread
	// of the Automation before its next call.
	for range 500 {
		window.FindFirst(uia.ScopeDescendants, uia.ByName("Press me"))
	}
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	name, err := press.Name()
	check("after finalizers", err == nil && name == "Press me", "%q, %v", name, err)

	e, err := window.FindFirst(uia.ScopeDescendants, uia.ByName("Press me"))
	if checkErr("FindFirst for Release", err) {
		check("Release", e.Release() == nil, "error")
		_, err = e.Name()
		check("Name after Release", errors.Is(err, uia.ErrReleased), "%v, want ErrReleased", err)
		check("Release again", e.Release() == nil, "error")
	}
}

func dump() {
	fmt.Println("Switch to the window to dump; it is read in 3 seconds.")
	time.Sleep(3 * time.Second)
	a, err := uia.New()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer a.Close()
	e, err := a.ElementFromHandle(uintptr(win.GetForegroundWindow()))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	dumpElement(a.ControlView(), e, 0)
}

func dumpElement(w *uia.TreeWalker, e *uia.Element, depth int) {
	ct, _ := e.ControlType()
	name, _ := e.Name()
	id, _ := e.AutomationID()
	class, _ := e.ClassName()
	fmt.Printf("%s%v, name %q, automation ID %q, class %q\n", strings.Repeat("  ", depth), ct, name, id, class)
	children, err := w.Children(e)
	if err != nil {
		fmt.Printf("%s  error: %v\n", strings.Repeat("  ", depth), err)
	}
	for _, c := range children {
		dumpElement(w, c, depth+1)
		c.Release()
	}
}
