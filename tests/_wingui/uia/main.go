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
//   - SetFocus, FocusedElement, ElementFromPoint and Equal, which need the
//     window in the foreground: the test waits up to three seconds for
//     it and asks for it once, and fails if it does not get it.
//   - Concurrent calls from several goroutines, elements left to the
//     garbage collector, Release, Close, and an element of a closed
//     window.
//   - The control patterns (step 13 part 2): Invoke and DoDefaultAction
//     on the button, Toggle on the check box, Value on an edit field, Text
//     on a multiline edit field, ExpandCollapse and Value on a combo box,
//     Selection and SelectionItem on a list box, RangeValue on a slider,
//     Window on the window, and ErrPatternNotSupported. Each change is
//     read back through UI Automation and from the walk widget.
//   - The events (step 13 part 3): focus changes (the window must be in the
//     foreground), property changes of a label and a check box, the live
//     region changed event of a label, structure changes when a button is
//     added, notifications from walk.Announce, Close of a subscription,
//     also from its own callback, and Close of the Automation with an open
//     subscription.
//   - Caching (step 13 part 4): FindAllBuildCache, FindFirstBuildCache and
//     BuildUpdatedCache with the Cached getters, a cached value that stays
//     after a change until BuildUpdatedCache, CachedChildren with and
//     without a filter, CachedParent, CachedPropertyValue and
//     PropertyValue.
//   - The Grid, GridItem, Table, TableItem and Scroll patterns (step 13
//     part 4) on a TableView with 30 rows and 2 columns.
//
// The checks run on a goroutine while the window runs its message loop, as
// the package documentation requires. It prints PASS, FAIL and SKIP lines
// and exits with code 1 if a check failed.
//
// With the argument "dump" it shows no window: after three seconds it
// prints the control view tree of the foreground window, one element per
// line, indented by depth, with control type, name, automation ID and
// class name, and the value or state of the elements that have one. The
// properties are read with one cache request for the whole tree. Start
// it, switch to another window with Alt+Tab and wait.
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

// The window and the widgets the pattern checks read back.
var (
	mw             *walk.MainWindow
	pressed        int
	searchEdit     *walk.LineEdit
	optionBox      *walk.CheckBox
	colorBox       *walk.ComboBox
	numberList     *walk.ListBox
	volume         *walk.Slider
	statusLabel    *walk.Label
	disabledButton *walk.PushButton
	table          *walk.TableView
)

// tableRow is a row of the TableView.
type tableRow struct {
	Name  string
	Value string
}

const tableRows = 30

// onUI runs f on the goroutine of the window and waits for it.
func onUI(f func()) {
	done := make(chan struct{})
	mw.Synchronize(func() {
		f()
		close(done)
	})
	<-done
}

// foreground waits up to three seconds for the window to be in the
// foreground. The window usually gets it some time after it opens, for
// example while the window of a previous test is still closing; if it
// has not got it after one second, the test asks for it once.
func foreground(hwnd uintptr) bool {
	for i := range 30 {
		if uintptr(win.GetForegroundWindow()) == hwnd {
			return true
		}
		if i == 10 {
			onUI(func() { win.SetForegroundWindow(win.HWND(hwnd)) })
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "dump" {
		dump()
		return
	}

	var rows []*tableRow
	for i := range tableRows {
		rows = append(rows, &tableRow{Name: fmt.Sprintf("Item %d", i+1), Value: fmt.Sprintf("Value %d", i+1)})
	}

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    title,
		Size:     Size{Width: 400, Height: 600},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "Search:"},
			LineEdit{AssignTo: &searchEdit, Accessibility: Accessibility{Name: "Search"}},
			LineEdit{PasswordMode: true, Accessibility: Accessibility{Name: "Password"}},
			PushButton{Text: "Press me", OnClicked: func() { pressed++ }},
			PushButton{AssignTo: &disabledButton, Text: "Disabled", Enabled: false},
			CheckBox{AssignTo: &optionBox, Text: "Option"},
			TextEdit{Text: "line one\r\nline two", Accessibility: Accessibility{Name: "Notes"}},
			ComboBox{AssignTo: &colorBox, Model: []string{"Red", "Green", "Blue"}, CurrentIndex: 0, Accessibility: Accessibility{Name: "Color"}},
			ListBox{AssignTo: &numberList, Model: []string{"One", "Two", "Three"}, Accessibility: Accessibility{Name: "Numbers"}},
			Slider{AssignTo: &volume, MinValue: 0, MaxValue: 100, Value: 30, Accessibility: Accessibility{Name: "Volume"}},
			TableView{
				AssignTo: &table,
				Columns: []TableViewColumn{
					{Title: "Name", DataMember: "Name", Width: 120},
					{Title: "Value", DataMember: "Value", Width: 120},
				},
				Model:   rows,
				MinSize: Size{Height: 100},
				MaxSize: Size{Height: 100},
			},
			Label{AssignTo: &statusLabel, Text: "Status", Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite}},
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
	checkPatterns(window, press, option)
	checkCache(a, window, press, disabled)
	checkGrid(window, press)
	open := checkEvents(a, window, hwnd)
	checkRelease(a, window, press)

	closeWindow()
	<-closed
	_, err = window.Name()
	check("element of a closed window", errors.Is(err, uia.ErrElementNotAvailable), "Name returned %v, want ErrElementNotAvailable", err)

	check("Close", a.Close() == nil, "Close returned an error")
	check("Subscription Close after Automation Close", open == nil || open.Close() == nil, "error")
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

	// The multiline TextEdit is a Document, not an Edit. The class name
	// keeps out the cells of the TableView, should its proxy report them
	// as edits.
	edits, err := window.FindAll(uia.ScopeDescendants, uia.And(uia.ByControlType(uia.ControlTypeEdit), uia.ByClassName("Edit")))
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
	if !check2("window in the foreground for SetFocus", foreground(hwnd), "foreground window %#x, want %#x", win.GetForegroundWindow(), hwnd) {
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
	// One cache request reads the properties of the whole control view
	// tree. If it fails, walk the tree and read the properties of each
	// element instead.
	cached, err := e.BuildUpdatedCache(uia.CacheRequest{
		Properties: []uia.PropertyID{uia.PropertyControlType, uia.PropertyName, uia.PropertyAutomationID, uia.PropertyClassName},
		Scope:      uia.ScopeSubtree,
	})
	if err != nil || cached == nil {
		fmt.Printf("BuildUpdatedCache failed (%v), walking the tree instead\n", err)
		dumpWalk(a.ControlView(), e, 0)
		return
	}
	dumpElement(cached, 0)
}

// dumpWalk prints e and the elements below it, walking the tree with w
// and reading the current properties.
func dumpWalk(w *uia.TreeWalker, e *uia.Element, depth int) {
	ct, _ := e.ControlType()
	name, _ := e.Name()
	id, _ := e.AutomationID()
	class, _ := e.ClassName()
	fmt.Printf("%s%v, name %q, automation ID %q, class %q%s\n", strings.Repeat("  ", depth), ct, name, id, class, state(e))
	children, err := w.Children(e)
	if err != nil {
		fmt.Printf("%s  error: %v\n", strings.Repeat("  ", depth), err)
	}
	for _, c := range children {
		dumpWalk(w, c, depth+1)
		c.Release()
	}
}

// dumpElement prints e and the elements below it from the cached tree.
func dumpElement(e *uia.Element, depth int) {
	ct, _ := e.CachedControlType()
	name, _ := e.CachedName()
	id, _ := e.CachedAutomationID()
	class, _ := e.CachedClassName()
	fmt.Printf("%s%v, name %q, automation ID %q, class %q%s\n", strings.Repeat("  ", depth), ct, name, id, class, state(e))
	children, err := e.CachedChildren()
	if err != nil {
		fmt.Printf("%s  error: %v\n", strings.Repeat("  ", depth), err)
	}
	for _, c := range children {
		dumpElement(c, depth+1)
		c.Release()
	}
}

// state describes the value or state of e for dump, or returns "".
func state(e *uia.Element) string {
	var parts []string
	if v, err := e.Value(); err == nil && v != "" {
		parts = append(parts, fmt.Sprintf("value %q", v))
	}
	if t, err := e.ToggleState(); err == nil {
		parts = append(parts, t.String())
	}
	if s, err := e.ExpandCollapseState(); err == nil && s != uia.LeafNode {
		parts = append(parts, s.String())
	}
	if selected, err := e.IsSelected(); err == nil && selected {
		parts = append(parts, "selected")
	}
	if rv, err := e.RangeValue(); err == nil {
		parts = append(parts, fmt.Sprintf("range value %g from %g to %g", rv.Value, rv.Minimum, rv.Maximum))
	}
	if rows, columns, err := e.GridSize(); err == nil {
		parts = append(parts, fmt.Sprintf("%d rows, %d columns", rows, columns))
	}
	if pos, err := e.GridPosition(); err == nil {
		parts = append(parts, fmt.Sprintf("row %d, column %d", pos.Row+1, pos.Column+1))
	}
	if enabled, err := e.IsEnabled(); err == nil && !enabled {
		parts = append(parts, "disabled")
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ", ")
}

// patternErr checks that err wraps ErrPatternNotSupported.
func patternErr(name string, err error) {
	check(name, errors.Is(err, uia.ErrPatternNotSupported), "%v, want ErrPatternNotSupported", err)
}

func checkPatterns(window, press, option *uia.Element) {
	// Invoke and DoDefaultAction click the button.
	before := 0
	onUI(func() { before = pressed })
	err := press.Invoke()
	time.Sleep(200 * time.Millisecond)
	after := 0
	onUI(func() { after = pressed })
	check("Invoke", err == nil && after == before+1, "%v, clicked %d times", err, after-before)
	action, err := press.DefaultAction()
	check("DefaultAction", err == nil && action != "", "%q, %v", action, err)
	err = press.DoDefaultAction()
	time.Sleep(200 * time.Millisecond)
	onUI(func() { after = pressed })
	check("DoDefaultAction", err == nil && after == before+2, "%v, clicked %d times", err, after-before)
	_, err = press.ToggleState()
	patternErr("ToggleState of a button", err)

	// Toggle the check box on and off again.
	state, err := option.ToggleState()
	check("ToggleState", err == nil && state == uia.ToggleOff, "%v, %v", state, err)
	err = option.Toggle()
	state, err2 := option.ToggleState()
	checked := false
	onUI(func() { checked = optionBox.Checked() })
	check("Toggle", err == nil && err2 == nil && state == uia.ToggleOn && checked, "%v %v, state %v, walk Checked %v", err, err2, state, checked)
	option.Toggle()

	// Value of the search field.
	search := findOne(window, "edit Search", uia.And(uia.ByControlType(uia.ControlTypeEdit), uia.ByName("Search")))
	if search != nil {
		err := search.SetValue("hello")
		value, err2 := search.Value()
		text := ""
		onUI(func() { text = searchEdit.Text() })
		check("SetValue and Value", err == nil && err2 == nil && value == "hello" && text == "hello", "%v %v, %q, walk Text %q", err, err2, value, text)
		readOnly, err := search.IsValueReadOnly()
		check("IsValueReadOnly", err == nil && !readOnly, "%v, %v", readOnly, err)
		err = search.SetValue("a\x00b")
		check("SetValue with NUL", err != nil, "no error")
		_, err = search.RangeValue()
		patternErr("RangeValue of an edit field", err)
	}

	// Text of the multiline edit field, which UI Automation exposes as a
	// Document.
	notes := findOne(window, "document Notes", uia.And(uia.ByControlType(uia.ControlTypeDocument), uia.ByName("Notes")))
	if notes != nil {
		text, err := notes.Text()
		check("Text", err == nil && text == "line one\r\nline two", "%q, %v", text, err)
		sel, err := notes.SelectedText()
		check("SelectedText", err == nil, "%q, %v", sel, err)
	}

	// The combo box expands and collapses and shows the selected item.
	combo := findOne(window, "combo box Color", uia.ByControlType(uia.ControlTypeComboBox))
	if combo != nil {
		value, err := combo.Value()
		check("combo box Value", err == nil && value == "Red", "%q, %v", value, err)
		err = combo.Expand()
		time.Sleep(200 * time.Millisecond)
		state, err2 := combo.ExpandCollapseState()
		check("Expand", err == nil && err2 == nil && state == uia.Expanded, "%v %v, %v", err, err2, state)
		err = combo.Collapse()
		time.Sleep(200 * time.Millisecond)
		state, err2 = combo.ExpandCollapseState()
		check("Collapse", err == nil && err2 == nil && state == uia.Collapsed, "%v %v, %v", err, err2, state)
		index := -1
		onUI(func() { index = colorBox.CurrentIndex() })
		check("combo box unchanged", index == 0, "CurrentIndex %d", index)
	}

	// Select an item of the list box.
	list := findOne(window, "list Numbers", uia.And(uia.ByControlType(uia.ControlTypeList), uia.ByName("Numbers")))
	if list != nil {
		two := findOne(list, "list item Two", uia.And(uia.ByControlType(uia.ControlTypeListItem), uia.ByName("Two")))
		if two != nil {
			err := two.Select()
			selected, err2 := two.IsSelected()
			index := -1
			onUI(func() { index = numberList.CurrentIndex() })
			check("Select and IsSelected", err == nil && err2 == nil && selected && index == 1, "%v %v, selected %v, walk CurrentIndex %d", err, err2, selected, index)
			selection, err := list.Selection()
			check("Selection", err == nil && len(selection) == 1 && same(selection[0], two), "%v, %v", names(selection), err)
			container, err := two.SelectionContainer()
			check("SelectionContainer", err == nil && same(container, list), "%v", err)
			multiple, err := list.CanSelectMultiple()
			check("CanSelectMultiple", err == nil && !multiple, "%v, %v", multiple, err)
			err = two.ScrollIntoView()
			check("ScrollIntoView", err == nil, "%v", err)
		}
	}

	// Move the slider.
	slider := findOne(window, "slider Volume", uia.ByControlType(uia.ControlTypeSlider))
	if slider != nil {
		rv, err := slider.RangeValue()
		check("RangeValue", err == nil && rv.Value == 30 && rv.Minimum == 0 && rv.Maximum == 100 && !rv.IsReadOnly, "%+v, %v", rv, err)
		err = slider.SetRangeValue(60)
		rv, err2 := slider.RangeValue()
		value := -1
		onUI(func() { value = volume.Value() })
		check("SetRangeValue", err == nil && err2 == nil && rv.Value == 60 && value == 60, "%v %v, %+v, walk Value %d", err, err2, rv, value)
	}

	// The window pattern of the main window.
	vs, err := window.WindowVisualState()
	check("WindowVisualState", err == nil && vs == uia.WindowNormal, "%v, %v", vs, err)
	modal, err := window.IsModal()
	check("IsModal", err == nil && !modal, "%v, %v", modal, err)
	patternErr("CloseWindow of a button", press.CloseWindow())

	check("pattern state names", uia.ToggleOn.String() == "on" && uia.Expanded.String() == "expanded" && uia.WindowMinimized.String() == "minimized",
		"%q %q %q", uia.ToggleOn.String(), uia.Expanded.String(), uia.WindowMinimized.String())
}

// waitFor waits up to two seconds for a value from c that ok accepts and
// returns the values it got.
func waitFor[T any](c <-chan T, ok func(T) bool) (bool, []T) {
	var got []T
	timeout := time.After(2 * time.Second)
	for {
		select {
		case v := <-c:
			got = append(got, v)
			if ok(v) {
				return true, got
			}
		case <-timeout:
			return false, got
		}
	}
}

type propertyEvent struct {
	sender *uia.Element
	id     uia.PropertyID
	value  any
}

// checkEvents checks the events. It returns a subscription it leaves open
// for Close of the Automation.
func checkEvents(a *uia.Automation, window *uia.Element, hwnd uintptr) *uia.Subscription {
	status := findOne(window, "text Status", uia.And(uia.ByControlType(uia.ControlTypeText), uia.ByName("Status")))
	disabled := findOne(window, "button Disabled", uia.ByName("Disabled"))
	search := findOne(window, "edit Search", uia.And(uia.ByControlType(uia.ControlTypeEdit), uia.ByName("Search")))
	option := findOne(window, "check box Option", uia.ByName("Option"))
	if status == nil || disabled == nil || search == nil || option == nil {
		return nil
	}

	// Property changes: the name of the label and the toggle state of the
	// check box. The proxies of the Win32 controls raise no IsEnabled
	// change for EnableWindow, and they send the new toggle state as
	// VT_EMPTY, so only the event is checked, not its value.
	const toggleStateProperty = uia.PropertyID(30086) // UIA_ToggleToggleStatePropertyId
	properties := make(chan propertyEvent, 100)
	sub, err := window.OnPropertyChanged(uia.ScopeSubtree, func(sender *uia.Element, id uia.PropertyID, value any) {
		properties <- propertyEvent{sender, id, value}
	}, uia.PropertyName, toggleStateProperty)
	if checkErr("OnPropertyChanged", err) {
		onUI(func() { statusLabel.SetText("Changed 1") })
		ok, got := waitFor(properties, func(e propertyEvent) bool {
			return e.id == uia.PropertyName && e.value == "Changed 1" && same(e.sender, status)
		})
		check("property changed Name", ok, "got %d events: %v", len(got), got)
		onUI(func() { optionBox.SetChecked(!optionBox.Checked()) })
		ok, got = waitFor(properties, func(e propertyEvent) bool {
			return e.id == toggleStateProperty && same(e.sender, option)
		})
		check("property changed ToggleState", ok, "got %d events: %v", len(got), got)

		check("Subscription Close", sub.Close() == nil, "error")
		for len(properties) > 0 {
			<-properties
		}
		onUI(func() { statusLabel.SetText("Changed 2") })
		time.Sleep(500 * time.Millisecond)
		check("no events after Close", len(properties) == 0, "%d events", len(properties))
		check("Subscription Close again", sub.Close() == nil, "error")
	}

	// The live region changed event of the label, from a subscription
	// that closes itself in its callback.
	live := make(chan *uia.Element, 100)
	var liveSub *uia.Subscription
	var liveMu sync.Mutex
	newSub, err := window.OnEvent(uia.EventLiveRegionChanged, uia.ScopeSubtree, func(sender *uia.Element, id uia.EventID) {
		live <- sender
		liveMu.Lock()
		s := liveSub
		liveMu.Unlock()
		if s != nil {
			s.Close()
		}
	})
	liveMu.Lock()
	liveSub = newSub
	liveMu.Unlock()
	if checkErr("OnEvent LiveRegionChanged", err) {
		// The event comes from the window of the Label, which has the live
		// setting, not from the Static control inside it.
		var labelHwnd uintptr
		onUI(func() { labelHwnd = uintptr(statusLabel.Handle()) })
		labelElement, err := a.ElementFromHandle(labelHwnd)
		checkErr("ElementFromHandle Label", err)
		onUI(func() { statusLabel.SetText("Changed 3") })
		ok, got := waitFor(live, func(e *uia.Element) bool { return same(e, labelElement) })
		check("live region changed event", ok, "got %d events", len(got))
		// The callback closed the subscription without a deadlock.
		onUI(func() { statusLabel.SetText("Changed 4") })
		time.Sleep(500 * time.Millisecond)
		check("Close in the callback", len(live) == 0, "%d events after Close", len(live))
	}

	// Structure changes when a button is added.
	structure := make(chan uia.StructureChange, 100)
	sub, err = window.OnStructureChanged(uia.ScopeSubtree, func(sender *uia.Element, change uia.StructureChange) {
		structure <- change
	})
	if checkErr("OnStructureChanged", err) {
		onUI(func() {
			if b, err := walk.NewPushButton(searchEdit.Parent()); err == nil {
				b.SetText("Added")
			}
		})
		ok, got := waitFor(structure, func(c uia.StructureChange) bool { return true })
		check("structure changed", ok, "got %v", got)
		sub.Close()
	}

	// Notifications from walk.Announce.
	notifications := make(chan uia.Notification, 100)
	sub, err = window.OnNotification(uia.ScopeSubtree, func(sender *uia.Element, n uia.Notification) {
		notifications <- n
	})
	if checkErr("OnNotification", err) {
		var announceErr error
		onUI(func() { announceErr = walk.Announce(mw, "Hello from walk", walk.AnnounceAll) })
		ok, got := waitFor(notifications, func(n uia.Notification) bool {
			return n.Text == "Hello from walk" && strings.HasPrefix(n.ActivityID, "Walk.Announce.") &&
				n.Processing == uia.ProcessingAll && n.Kind == uia.NotificationOther
		})
		check("notification", announceErr == nil && ok, "%v, got %+v", announceErr, got)
		sub.Close()
	}

	// Focus changes, which need the window in the foreground.
	if check2("window in the foreground for OnFocusChanged", foreground(hwnd), "foreground window %#x, want %#x", win.GetForegroundWindow(), hwnd) {
		focus := make(chan *uia.Element, 100)
		sub, err = a.OnFocusChanged(func(focused *uia.Element) { focus <- focused })
		if checkErr("OnFocusChanged", err) {
			onUI(func() { searchEdit.SetFocus() })
			ok, got := waitFor(focus, func(e *uia.Element) bool { return same(e, search) })
			check("focus changed", ok, "got %d events", len(got))
			sub.Close()
		}
	}

	check("event names", uia.EventLiveRegionChanged.String() == "LiveRegionChanged" && uia.ChildAdded.String() == "ChildAdded" &&
		uia.NotificationOther.String() == "Other" && uia.ProcessingAll.String() == "All",
		"%q %q %q %q", uia.EventLiveRegionChanged.String(), uia.ChildAdded.String(), uia.NotificationOther.String(), uia.ProcessingAll.String())

	_, err = window.OnPropertyChanged(uia.ScopeSubtree, func(*uia.Element, uia.PropertyID, any) {})
	check("OnPropertyChanged without properties", err != nil, "no error")

	// Left open for Close of the Automation.
	open, err := window.OnPropertyChanged(uia.ScopeElement, func(*uia.Element, uia.PropertyID, any) {}, uia.PropertyName)
	checkErr("OnPropertyChanged left open", err)
	return open
}
