// Command mdi checks on Windows the MDI windows of step 11: MdiMainWindow
// and MdiChildWindow.
//
// It creates a frame (declarative MdiMainWindow) with the menus File,
// Window (the window menu) and Extra, and three children, each with two
// edit fields and a button. The checks run on a goroutine while the frame
// runs its message loop; every step that touches a window runs on the UI
// thread through Synchronize. Keys are posted only to windows of the test;
// Ctrl and Shift are set with SetKeyboardState for the UI thread only.
//
//   - Structure: the class of each child is walk's MDI child class, its
//     parent is the MDICLIENT window, whose parent is the frame, and
//     Windows gave it WS_EX_MDICHILD. MdiChildren lists the children.
//   - Activation: the last child created is active; Activate makes each
//     child active (WM_MDIGETACTIVE, IsActive, ActiveMdiChild,
//     ActiveMdiChildChanged, Activating and Deactivating).
//   - Layout and bounds: the edit fields of a child have a size, and
//     BoundsPixels returns what SetBoundsPixels set, relative to the MDI
//     client. The MDI client fits between the tool bar and the status bar
//     after a WM_SIZE and after an action is added to the tool bar.
//   - Window menu: it lists the three children after the walk items;
//     WM_COMMAND with the ID of an item activates that child.
//   - Keyboard: Ctrl+F6 activates another child and Ctrl+Shift+F6 the
//     first one again (TranslateMDISysAccel in the message loop);
//     Ctrl+F4 and Close publish Closing, which here cancels. Ctrl+F6 and
//     Ctrl+F4 posted to another (hidden) form of the thread do nothing.
//   - Focus (needs the frame in the foreground, FAIL without it): the
//     focus moves to the first field of a newly activated child and back to
//     the last focused field of a child activated again; Tab moves among
//     the controls of the active child and wraps around inside it; a
//     WinEvent hook sees EVENT_OBJECT_FOCUS for the field; through UI
//     Automation the focused element is inside the child.
//   - Title: SetTitle raises EVENT_OBJECT_NAMECHANGE for the child window,
//     the window menu shows the new title, and UI Automation reports it as
//     the name of the child.
//   - Maximize: the child is maximized, the menu bar gets the system menu
//     and the buttons of the child, the frame title contains the child
//     title; hiding and showing the Extra menu while the child is maximized
//     keeps the menu bar in order (walk's menu positions skip the system
//     menu item); activating another child keeps it maximized; Restore
//     restores the menu bar and the title. Alt+Hyphen (WM_SYSCHAR '-'
//     posted to a field) opens the system menu of the child (only checked
//     in the foreground; the menu is closed with WM_CANCELMODE).
//   - Cascade, TileHorizontally and TileVertically change the bounds; the
//     tiled children do not overlap. Minimize and ArrangeIcons.
//   - Modal dialog owned by a child: its owner is the frame, which is
//     disabled while it runs and enabled after it; the child is active
//     again.
//   - Closing: CloseAllMdiChildren stops at a child whose Closing cancels
//     and closes all children once none cancels; then no child, no active
//     child and no child item in the window menu remain; the MDI client
//     has the accessible name "Documents" and (in the foreground) the
//     focus. The frame's Closing comes first, and canceling it keeps the
//     child without asking it. Closing the frame with a child that cancels
//     keeps both open; the final close of the
//     frame publishes Closing of the child.
//
// It prints PASS, FAIL and INFO lines and exits with code 1 if a check
// failed. With the argument "manual" it shows a frame with three
// documents for a test with a screen reader; events go to standard output
// and the status bar.
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
	"github.com/lpintes/walk/uia"
)

const (
	title      = "walk mdi test"
	childClass = `\o/ Walk_MdiChild_Class \o/`

	wsExMDIChild       = 0x40
	objidWindow        = 0
	eventObjectFocus   = 0x8005
	eventObjectNameChg = 0x800C
	vkF4               = 0x73
	vkF6               = 0x75
	vkTab              = 0x09
	vkEscape           = 0x1B
	vkControl          = 0x11
	vkShift            = 0x10
	wmCancelMode       = 0x1F
	guiInMenuMode      = 0x4
	guiSystemMenuMode  = 0x8
	objidMenu          = 0xFFFFFFFD // OBJID_MENU, -3
	gwOwner            = 4
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procGetClassName        = user32.NewProc("GetClassNameW")
	procGetMenu             = user32.NewProc("GetMenu")
	procGetMenuItemCount    = user32.NewProc("GetMenuItemCount")
	procGetMenuItemID       = user32.NewProc("GetMenuItemID")
	procGetMenuString       = user32.NewProc("GetMenuStringW")
	procGetSubMenu          = user32.NewProc("GetSubMenu")
	procGetKeyboardState    = user32.NewProc("GetKeyboardState")
	procSetKeyboardState    = user32.NewProc("SetKeyboardState")
	procIsWindow            = user32.NewProc("IsWindow")
	procGetWindowText       = user32.NewProc("GetWindowTextW")
	procSetWinEventHook     = user32.NewProc("SetWinEventHook")
	procGetGUIThreadInfo    = user32.NewProc("GetGUIThreadInfo")
	procGetMenuBarInfo      = user32.NewProc("GetMenuBarInfo")
	procGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
)

var (
	failed     bool
	frame      *walk.MdiMainWindow
	windowMenu *walk.Menu
	extraMenu  *walk.Action
	docs       []*doc

	// Written on the UI thread, read through onUI.
	activeChanges []string
)

type doc struct {
	name                   string
	child                  *walk.MdiChildWindow
	first, second          *walk.LineEdit
	button                 *walk.PushButton
	cancel                 bool
	closing                int
	activating, deactivate int
}

func check(name string, ok bool, format string, args ...any) bool {
	if ok {
		fmt.Println("PASS", name)
		return true
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
	return false
}

func checkErr(name string, err error) bool {
	if err != nil {
		return check(name, false, "%v", err)
	}
	return true
}

func info(format string, args ...any) {
	fmt.Printf("INFO "+format+"\n", args...)
}

// onUI runs f on the UI thread and waits up to ten seconds for it.
func onUI(f func()) bool {
	done := make(chan struct{})
	frame.Synchronize(func() {
		defer close(done)
		f()
	})
	select {
	case <-done:
		return true
	case <-time.After(10 * time.Second):
		check("UI thread", false, "a call on the UI thread did not return within 10 seconds")
		return false
	}
}

func pause() { time.Sleep(300 * time.Millisecond) }

func isWindow(h win.HWND) bool {
	r, _, _ := procIsWindow.Call(uintptr(h))
	return r != 0
}

func className(h win.HWND) string {
	var buf [256]uint16
	procGetClassName.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:])
}

func windowText(h win.HWND) string {
	var buf [512]uint16
	procGetWindowText.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:])
}

type menuItem struct {
	text string
	id   int32 // -1 for a submenu
}

func menuItems(hmenu uintptr) []menuItem {
	n, _, _ := procGetMenuItemCount.Call(hmenu)
	var items []menuItem
	for i := range int32(n) {
		var buf [256]uint16
		procGetMenuString.Call(hmenu, uintptr(i), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), win.MF_BYPOSITION)
		id, _, _ := procGetMenuItemID.Call(hmenu, uintptr(i))
		items = append(items, menuItem{windows.UTF16ToString(buf[:]), int32(id)})
	}
	return items
}

func menuTexts(items []menuItem) []string {
	var s []string
	for _, it := range items {
		s = append(s, it.text)
	}
	return s
}

func menuBar() uintptr {
	h, _, _ := procGetMenu.Call(uintptr(frame.Handle()))
	return h
}

// windowMenuHandle finds the submenu "&Window" of the menu bar.
func windowMenuHandle() uintptr {
	bar := menuBar()
	for i, it := range menuItems(bar) {
		if it.text == "&Window" {
			h, _, _ := procGetSubMenu.Call(bar, uintptr(i))
			return h
		}
	}
	return 0
}

// setModifiers sets the state of Ctrl and Shift for GetKeyState of the
// calling thread, which TranslateMDISysAccel and walk read. It must run on
// the UI thread.
func setModifiers(ctrl, shift bool) {
	var state [256]byte
	procGetKeyboardState.Call(uintptr(unsafe.Pointer(&state[0])))
	set := func(vk int, down bool) {
		if down {
			state[vk] |= 0x80
		} else {
			state[vk] &^= 0x80
		}
	}
	set(vkControl, ctrl)
	set(vkShift, shift)
	procSetKeyboardState.Call(uintptr(unsafe.Pointer(&state[0])))
}

// postKey posts WM_KEYDOWN for vk with Ctrl and Shift as given to the
// focused window of the UI thread (or the active child) and resets the
// modifiers after the message loop has processed it.
func postKey(vk uintptr, ctrl, shift bool) {
	onUI(func() {
		target := win.GetFocus()
		if target == 0 {
			target = win.HWND(win.SendMessage(mdiClient(), win.WM_MDIGETACTIVE, 0, 0))
		}
		setModifiers(ctrl, shift)
		win.PostMessage(target, win.WM_KEYDOWN, vk, 1)
	})
	pause()
	onUI(func() { setModifiers(false, false) })
}

// client is the MDICLIENT window, the parent of the children.
var client win.HWND

func mdiClient() win.HWND {
	return client
}

func activeHWnd() win.HWND {
	var h win.HWND
	onUI(func() { h = win.HWND(win.SendMessage(mdiClient(), win.WM_MDIGETACTIVE, 0, 0)) })
	return h
}

// foreground waits up to three seconds for the frame to be the foreground
// window; if it has not got it after one second, the test asks for it once.
func foreground() bool {
	hwnd := uintptr(frame.Handle())
	for i := range 30 {
		if h, _, _ := procGetForegroundWindow.Call(); h == hwnd {
			return true
		}
		if i == 10 {
			onUI(func() { procSetForegroundWindow.Call(hwnd) })
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// WinEvents of the process, recorded by an out-of-context hook whose
// callback runs in the message loop of the UI thread.
type winEvent struct {
	event    uint32
	hwnd     win.HWND
	idObject int32
}

var (
	eventsMu sync.Mutex
	events   []winEvent
)

func winEventProc(hook uintptr, event uint32, hwnd win.HWND, idObject, idChild int32, thread, time uint32) uintptr {
	if event == eventObjectFocus || event == eventObjectNameChg {
		eventsMu.Lock()
		events = append(events, winEvent{event, hwnd, idObject})
		eventsMu.Unlock()
	}
	return 0
}

func eventMark() int {
	eventsMu.Lock()
	defer eventsMu.Unlock()
	return len(events)
}

func eventsSince(mark int) []winEvent {
	eventsMu.Lock()
	defer eventsMu.Unlock()
	return slices.Clone(events[mark:])
}

func childDecl(d *doc) MdiChildWindow {
	return MdiChildWindow{
		AssignTo: &d.child,
		Title:    d.name,
		Size:     Size{Width: 320, Height: 200},
		Layout:   VBox{},
		Children: []Widget{
			LineEdit{AssignTo: &d.first, Accessibility: Accessibility{Name: "First field " + d.name}},
			LineEdit{AssignTo: &d.second, Accessibility: Accessibility{Name: "Second field " + d.name}},
			PushButton{AssignTo: &d.button, Text: "Button " + d.name},
		},
		OnActivating:   func() { d.activating++ },
		OnDeactivating: func() { d.deactivate++ },
		OnClosing: func(canceled *bool, reason walk.CloseReason) {
			d.closing++
			*canceled = d.cancel
		},
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "manual" {
		manual()
		return
	}

	for _, name := range []string{"Child one", "Child two", "Child three"} {
		docs = append(docs, &doc{name: name})
	}
	var children []MdiChildWindow
	for _, d := range docs {
		children = append(children, childDecl(d))
	}

	if err := (MdiMainWindow{
		AssignTo:   &frame,
		Title:      title,
		Size:       Size{Width: 900, Height: 650},
		WindowMenu: &windowMenu,
		MenuItems: []MenuItem{
			Menu{Text: "&File", Items: []MenuItem{
				Action{Text: "E&xit", OnTriggered: func() { frame.Close() }},
			}},
			Menu{AssignTo: &windowMenu, Text: "&Window", Items: []MenuItem{
				Action{Text: "&Cascade", OnTriggered: func() { frame.Cascade() }},
				Action{Text: "Tile &Horizontally", OnTriggered: func() { frame.TileHorizontally() }},
				Action{Text: "Tile &Vertically", OnTriggered: func() { frame.TileVertically() }},
				Action{Text: "&Arrange Icons", OnTriggered: func() { frame.ArrangeIcons() }},
			}},
			Menu{AssignActionTo: &extraMenu, Text: "&Extra", Items: []MenuItem{
				Action{Text: "Nothing"},
			}},
		},
		OnActiveMdiChildChanged: func() {
			s := "<none>"
			if c := frame.ActiveMdiChild(); c != nil {
				s = c.Title()
			}
			activeChanges = append(activeChanges, s)
		},
		MdiChildren: children,
	}).Create(); err != nil {
		fmt.Println("FAIL create frame:", err)
		os.Exit(1)
	}

	if r, _, _ := procSetWinEventHook.Call(eventObjectFocus, eventObjectNameChg, 0, syscall.NewCallback(winEventProc), uintptr(os.Getpid()), 0, 0); r == 0 {
		check("SetWinEventHook", false, "failed")
	}

	hwnd := frame.Handle()
	checksDone := make(chan struct{})
	lastDoc := new(doc)
	go func() {
		defer close(checksDone)
		for i := 0; i < 100 && !win.IsWindowVisible(hwnd); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		pause()
		run(lastDoc)
		frame.Synchronize(func() { frame.Close() })
	}()
	frame.Run()
	<-checksDone

	check("final frame close: Closing of the remaining child", lastDoc.closing == 2, "Closing published %d times, want 2 (one canceled, one final)", lastDoc.closing)
	check("final frame close: child destroyed", lastDoc.child == nil || !isWindow(lastDoc.child.Handle()) || lastDoc.child.Handle() == 0, "child window still exists")

	if failed {
		os.Exit(1)
	}
}

func run(lastDoc *doc) {
	fg := foreground()
	check("frame in the foreground", fg, "the frame did not get the foreground; the focus checks are skipped")

	checkStructure()
	checkActivation()
	checkLayoutAndBounds()
	checkWindowMenu()
	checkKeyboard()
	if fg {
		checkFocus()
	}
	checkTitle()
	checkMaximize(fg)
	checkArrange()
	checkModalDialog()
	checkClosing(lastDoc, fg)
}

func checkStructure() {
	var n int
	var order []string
	onUI(func() {
		for _, c := range frame.MdiChildren() {
			order = append(order, c.Title())
		}
		n = len(frame.MdiChildren())
	})
	check("MdiChildren", n == 3 && slices.Equal(order, []string{"Child one", "Child two", "Child three"}), "%d children %q", n, order)

	onUI(func() { client = win.GetParent(docs[0].child.Handle()) })
	check("MDI client class", strings.EqualFold(className(client), "MDIClient"), "parent of the child has class %q", className(client))
	check("MDI client parent", win.GetParent(client) == frame.Handle(), "parent of the MDI client %#x, frame %#x", win.GetParent(client), frame.Handle())
	for _, d := range docs {
		h := d.child.Handle()
		ex := uint32(win.GetWindowLong(h, win.GWL_EXSTYLE))
		check("child class and style "+d.name,
			className(h) == childClass && win.GetParent(h) == client && ex&wsExMDIChild != 0 && ex&win.WS_EX_CONTROLPARENT != 0,
			"class %q, parent %#x (client %#x), ex style %#x", className(h), win.GetParent(h), client, ex)
	}
}

func checkActivation() {
	var titles []string
	var activeTitle string
	onUI(func() {
		titles = slices.Clone(activeChanges)
		if c := frame.ActiveMdiChild(); c != nil {
			activeTitle = c.Title()
		}
	})
	check("last child created is active", activeHWnd() == docs[2].child.Handle() && activeTitle == "Child three", "active %#x %q", activeHWnd(), activeTitle)
	check("ActiveMdiChildChanged while creating", len(titles) > 0 && titles[len(titles)-1] == "Child three", "events %q", titles)

	for i, d := range docs {
		prev := docs[(i+len(docs)-1)%len(docs)]
		var act, deact, events int
		var isActive, sameActive bool
		onUI(func() {
			act, deact = d.activating, prev.deactivate
			events = len(activeChanges)
			d.child.Activate()
		})
		pause()
		onUI(func() {
			isActive = d.child.IsActive()
			sameActive = frame.ActiveMdiChild() == d.child
			act = d.activating - act
			deact = prev.deactivate - deact
			events = len(activeChanges) - events
		})
		check("Activate "+d.name, activeHWnd() == d.child.Handle() && isActive && sameActive, "WM_MDIGETACTIVE %#x, want %#x; IsActive %v, ActiveMdiChild matches %v", activeHWnd(), d.child.Handle(), isActive, sameActive)
		check("Activating and ActiveMdiChildChanged "+d.name, act == 1 && events == 1, "Activating %d times, ActiveMdiChildChanged %d times", act, events)
		check("Deactivating "+prev.name, deact == 1, "Deactivating %d times", deact)
	}
}

func checkLayoutAndBounds() {
	var w, h int
	var got, want walk.Rectangle
	onUI(func() {
		w, h = docs[0].first.WidthPixels(), docs[0].first.HeightPixels()
		want = walk.Rectangle{X: 20, Y: 30, Width: 420, Height: 260}
		docs[0].child.SetBoundsPixels(want)
	})
	pause()
	onUI(func() { got = docs[0].child.BoundsPixels() })
	check("layout of the child", w > 50 && h > 5, "first field %dx%d", w, h)
	check("BoundsPixels relative to the MDI client", got == want, "got %v, want %v", got, want)

	// The MDI client fills the client area between the tool bar and the
	// status bar, also after a WM_SIZE (which DefFrameProc would answer by
	// sizing it over the bars) and after an action is added to the tool
	// bar.
	clientFits := func() (walk.Rectangle, walk.Rectangle) {
		var r win.RECT
		win.GetWindowRect(mdiClient(), &r)
		p := win.POINT{X: r.Left, Y: r.Top}
		win.ScreenToClient(frame.Handle(), &p)
		return walk.Rectangle{X: int(p.X), Y: int(p.Y), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}, frame.ClientBoundsPixels()
	}
	var gotClient, wantClient walk.Rectangle
	onUI(func() {
		frame.SendMessage(win.WM_SIZE, 0, 0)
		gotClient, wantClient = clientFits()
	})
	check("MDI client fits between the bars after WM_SIZE", gotClient == wantClient, "MDI client %v, want %v", gotClient, wantClient)

	var toolBarHeight int
	onUI(func() {
		a := walk.NewAction()
		a.SetText("Tool")
		frame.ToolBar().Actions().Add(a)
	})
	pause()
	onUI(func() {
		gotClient, wantClient = clientFits()
		toolBarHeight = frame.ToolBar().HeightPixels()
	})
	check("MDI client below a tool bar that got an action", gotClient == wantClient && toolBarHeight > 0 && gotClient.Y >= toolBarHeight, "MDI client %v, want %v, tool bar height %d", gotClient, wantClient, toolBarHeight)
}

func checkWindowMenu() {
	var items []menuItem
	onUI(func() { items = menuItems(windowMenuHandle()) })
	info("window menu: %q", menuTexts(items))
	ids := map[string]int32{}
	for _, d := range docs {
		found := false
		for _, it := range items {
			if strings.HasSuffix(it.text, " "+d.name) {
				found = true
				ids[d.name] = it.id
			}
		}
		check("window menu lists "+d.name, found, "items %q", menuTexts(items))
	}
	check("window menu keeps the walk items first", len(items) > 4 && items[0].text == "&Cascade" && items[3].text == "&Arrange Icons", "items %q", menuTexts(items))

	if id, ok := ids["Child two"]; ok {
		onUI(func() { win.PostMessage(frame.Handle(), win.WM_COMMAND, uintptr(uint16(id)), 0) })
		pause()
		check("window menu item activates the child", activeHWnd() == docs[1].child.Handle(), "active %#x, want %#x", activeHWnd(), docs[1].child.Handle())
	}
}

func checkKeyboard() {
	onUI(func() { docs[1].child.Activate() })
	pause()
	postKey(vkF6, true, false)
	next := activeHWnd()
	check("Ctrl+F6 activates another child", next != 0 && next != docs[1].child.Handle(), "active %#x, was %#x", next, docs[1].child.Handle())
	postKey(vkF6, true, true)
	check("Ctrl+Shift+F6 activates the previous child", activeHWnd() == docs[1].child.Handle(), "active %#x, want %#x", activeHWnd(), docs[1].child.Handle())

	// Ctrl+F6 and Ctrl+F4 for another form of the thread (hidden, so that
	// the frame keeps the foreground) must not reach the MDI client.
	var other *walk.MainWindow
	var otherEdit *walk.LineEdit
	onUI(func() {
		var err error
		if other, err = walk.NewMainWindow(); err == nil {
			otherEdit, err = walk.NewLineEdit(other)
		}
		checkErr("create another form", err)
	})
	if otherEdit != nil {
		for _, vk := range []uintptr{vkF6, vkF4} {
			onUI(func() {
				setModifiers(true, false)
				win.PostMessage(otherEdit.Handle(), win.WM_KEYDOWN, vk, 1)
			})
			pause()
			onUI(func() { setModifiers(false, false) })
		}
		check("Ctrl+F6 and Ctrl+F4 in another form leave the children alone", activeHWnd() == docs[1].child.Handle() && isWindow(docs[1].child.Handle()), "active %#x, want %#x", activeHWnd(), docs[1].child.Handle())
	}
	if other != nil {
		onUI(func() { other.Dispose() })
	}

	d := docs[2]
	onUI(func() {
		d.cancel = true
		d.child.Activate()
	})
	pause()
	postKey(vkF4, true, false)
	var closing int
	onUI(func() { closing = d.closing })
	check("Ctrl+F4 publishes Closing", closing == 1, "Closing published %d times", closing)
	check("Closing cancels Ctrl+F4", isWindow(d.child.Handle()) && activeHWnd() == d.child.Handle(), "the child was closed")
	onUI(func() { d.child.Close() })
	onUI(func() { closing = d.closing })
	check("Close publishes Closing and it cancels", closing == 2 && isWindow(d.child.Handle()), "Closing published %d times, window exists %v", closing, isWindow(d.child.Handle()))
	onUI(func() { d.cancel = false })
}

func checkFocus() {
	a, err := uia.New()
	if !checkErr("uia New", err) {
		return
	}
	defer a.Close()

	var focus win.HWND
	onUI(func() {
		docs[0].child.Activate()
		docs[0].second.SetFocus()
		focus = win.GetFocus()
	})
	check("SetFocus in child one", focus == docs[0].second.Handle(), "focus %#x, want %#x", focus, docs[0].second.Handle())

	onUI(func() {
		docs[1].child.Activate()
		focus = win.GetFocus()
	})
	check("focus goes to the first field of the activated child", focus == docs[1].first.Handle(), "focus %#x (class %q), want %#x", focus, className(focus), docs[1].first.Handle())

	mark := eventMark()
	onUI(func() {
		docs[0].child.Activate()
		focus = win.GetFocus()
	})
	check("focus returns to the last focused field", focus == docs[0].second.Handle(), "focus %#x, want %#x", focus, docs[0].second.Handle())
	pause()
	seen := false
	for _, e := range eventsSince(mark) {
		if e.event == eventObjectFocus && e.hwnd == docs[0].second.Handle() {
			seen = true
		}
	}
	check("EVENT_OBJECT_FOCUS for the restored field", seen, "events %v", eventsSince(mark))

	if e, err := a.FocusedElement(); checkErr("uia FocusedElement", err) {
		name, _ := e.Name()
		check("UI Automation focused element", name == "Second field Child one", "name %q", name)
		// Walk up to the child window: a screen reader finds the title
		// of the document there.
		walker := a.ControlView()
		var path []string
		found := false
		for p, err := walker.Parent(e); err == nil && p != nil; p, err = walker.Parent(p) {
			n, _ := p.Name()
			ct, _ := p.ControlType()
			h, _ := p.NativeWindowHandle()
			path = append(path, fmt.Sprintf("%q (%v)", n, ct))
			if win.HWND(h) == docs[0].child.Handle() {
				found = n == "Child one"
			}
			if win.HWND(h) == frame.Handle() {
				break
			}
		}
		info("ancestors of the focused element: %s", strings.Join(path, ", "))
		check("UI Automation: the focused field is inside the child named by its title", found, "ancestors %s", strings.Join(path, ", "))
		e.Release()
	}

	postKey(vkTab, false, false)
	onUI(func() { focus = win.GetFocus() })
	check("Tab moves to the button of the child", focus == docs[0].button.Handle(), "focus %#x (class %q), want %#x", focus, className(focus), docs[0].button.Handle())
	postKey(vkTab, false, false)
	onUI(func() { focus = win.GetFocus() })
	check("Tab wraps around inside the child", focus == docs[0].first.Handle(), "focus %#x (class %q), want %#x", focus, className(focus), docs[0].first.Handle())
}

func checkTitle() {
	d := docs[1]
	const renamed = "Child two renamed"
	mark := eventMark()
	onUI(func() { d.child.SetTitle(renamed) })
	pause()
	seen := false
	for _, e := range eventsSince(mark) {
		if e.event == eventObjectNameChg && e.hwnd == d.child.Handle() && e.idObject == objidWindow {
			seen = true
		}
	}
	check("SetTitle raises EVENT_OBJECT_NAMECHANGE", seen, "events %v", eventsSince(mark))

	var items []menuItem
	onUI(func() { items = menuItems(windowMenuHandle()) })
	listed := false
	for _, it := range items {
		if strings.HasSuffix(it.text, " "+renamed) {
			listed = true
		}
	}
	check("window menu shows the new title", listed, "items %q", menuTexts(items))

	a, err := uia.New()
	if !checkErr("uia New", err) {
		return
	}
	defer a.Close()
	e, err := a.ElementFromHandle(uintptr(d.child.Handle()))
	if !checkErr("uia ElementFromHandle child", err) {
		return
	}
	defer e.Release()
	name, _ := e.Name()
	ct, _ := e.ControlType()
	lct, _ := e.LocalizedControlType()
	role, _ := e.Role()
	info("UI Automation view of the child: Name %q, ControlType %v, LocalizedControlType %q, Role %d", name, ct, lct, role)
	check("UI Automation name of the child", name == renamed, "name %q", name)

	client, err := a.ElementFromHandle(uintptr(mdiClient()))
	if err == nil {
		n, _ := client.Name()
		ct, _ := client.ControlType()
		role, _ := client.Role()
		info("UI Automation view of the MDI client: Name %q, ControlType %v, Role %d", n, ct, role)
		client.Release()
	}
}

func checkMaximize(fg bool) {
	d := docs[0]
	var before, during []menuItem
	var frameTitle string
	var maximized bool
	onUI(func() {
		d.child.Activate()
		before = menuItems(menuBar())
		d.child.Maximize()
	})
	pause()
	onUI(func() {
		maximized = d.child.IsMaximized()
		during = menuItems(menuBar())
		frameTitle = windowText(frame.Handle())
	})
	info("menu bar before %q, maximized %q", menuTexts(before), menuTexts(during))
	check("Maximize", maximized, "IsMaximized false")
	check("maximized child puts its menus into the menu bar", len(during) > len(before) && during[0].id == -1 && during[0].text == "", "before %q, maximized %q", menuTexts(before), menuTexts(during))
	check("frame title shows the maximized child", strings.Contains(frameTitle, title) && strings.Contains(frameTitle, d.name), "frame title %q", frameTitle)

	// walk inserts and removes menu bar items by position; the system menu
	// item of the child comes first while it is maximized.
	var hidden, shown []string
	onUI(func() {
		extraMenu.SetVisible(false)
		hidden = menuTexts(menuItems(menuBar()))
		extraMenu.SetVisible(true)
		shown = menuTexts(menuItems(menuBar()))
	})
	check("hiding a menu while maximized", len(hidden) == len(during)-1 && hidden[0] == "" && slices.Contains(hidden, "&File") && slices.Contains(hidden, "&Window") && !slices.Contains(hidden, "&Extra"), "menu bar %q", hidden)
	check("showing a menu while maximized", slices.Equal(shown, menuTexts(during)), "menu bar %q, want %q", shown, menuTexts(during))

	if fg {
		var gti struct {
			cbSize                                   uint32
			flags                                    uint32
			active, focus, capture, menuOwner, moveS uintptr
			caret                                    uintptr
			rc                                       win.RECT
		}
		gti.cbSize = uint32(unsafe.Sizeof(gti))
		var tid uintptr
		onUI(func() {
			tid, _, _ = procGetWindowThreadPID.Call(uintptr(frame.Handle()), 0)
			d.first.SetFocus()
			win.PostMessage(d.first.Handle(), win.WM_SYSCHAR, '-', 1|1<<29)
		})
		// The system menu runs a modal loop on the UI thread; read its
		// state from here.
		pause()
		procGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti)))
		// The child is maximized, so its system menu is the first item of
		// the frame's menu bar: Windows tracks it as a menu bar popup of
		// the frame (GUI_INMENUMODE, not GUI_SYSTEMMENUMODE), with that
		// item focused.
		var mbi struct {
			cbSize          uint32
			rcBar           win.RECT
			hMenu, hwndMenu uintptr
			flags           uint32 // fBarFocused:1, fFocused:1
		}
		mbi.cbSize = uint32(unsafe.Sizeof(mbi))
		procGetMenuBarInfo.Call(uintptr(frame.Handle()), objidMenu, 1, uintptr(unsafe.Pointer(&mbi)))
		sysMenuItemFocused := mbi.flags&2 != 0
		check("Alt+Hyphen opens the system menu of the child",
			gti.flags&guiSystemMenuMode != 0 || gti.flags&guiInMenuMode != 0 && win.HWND(gti.menuOwner) == frame.Handle() && sysMenuItemFocused,
			"GUI thread flags %#x, menu owner %#x (frame %#x), first menu bar item focused %v", gti.flags, gti.menuOwner, frame.Handle(), sysMenuItemFocused)
		if gti.flags&guiInMenuMode != 0 {
			info("menu owner %#x (child %#x, frame %#x)", gti.menuOwner, d.child.Handle(), frame.Handle())
			closeMenu(tid, win.HWND(gti.menuOwner))
		}
	}

	var other bool
	onUI(func() { docs[1].child.Activate() })
	pause()
	onUI(func() { other = docs[1].child.IsMaximized() })
	check("another activated child is maximized too", other, "IsMaximized false")

	var after []menuItem
	onUI(func() { docs[1].child.Restore() })
	pause()
	onUI(func() {
		maximized = docs[1].child.IsMaximized() || docs[0].child.IsMaximized()
		after = menuItems(menuBar())
		frameTitle = windowText(frame.Handle())
	})
	check("Restore", !maximized, "a child is still maximized")
	check("restored menu bar", slices.Equal(menuTexts(after), menuTexts(before)), "menu bar %q, want %q", menuTexts(after), menuTexts(before))
	check("restored frame title", frameTitle == title, "frame title %q", frameTitle)
}

// closeMenu ends the modal menu loop of the UI thread: WM_CANCELMODE to
// the owner of the menu, then Escape if the menu is still open.
func closeMenu(tid uintptr, owner win.HWND) {
	if owner == 0 {
		owner = frame.Handle()
	}
	win.PostMessage(owner, wmCancelMode, 0, 0)
	for range 3 {
		pause()
		var gti struct {
			cbSize                                   uint32
			flags                                    uint32
			active, focus, capture, menuOwner, moveS uintptr
			caret                                    uintptr
			rc                                       win.RECT
		}
		gti.cbSize = uint32(unsafe.Sizeof(gti))
		procGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti)))
		if gti.flags&guiInMenuMode == 0 {
			return
		}
		win.PostMessage(owner, win.WM_KEYDOWN, vkEscape, 1)
	}
	check("close the system menu", false, "the menu is still open")
}

func bounds() []walk.Rectangle {
	var r []walk.Rectangle
	onUI(func() {
		for _, d := range docs {
			r = append(r, d.child.BoundsPixels())
		}
	})
	return r
}

func overlap(a, b walk.Rectangle) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}

func noOverlap(r []walk.Rectangle) bool {
	for i := range r {
		for j := i + 1; j < len(r); j++ {
			if overlap(r[i], r[j]) {
				return false
			}
		}
	}
	return true
}

func checkArrange() {
	onUI(func() { frame.TileVertically() })
	pause()
	vertical := bounds()
	info("TileVertically: %v", vertical)
	check("TileVertically", noOverlap(vertical) && vertical[0].Y == vertical[1].Y && vertical[1].Y == vertical[2].Y, "bounds %v", vertical)

	onUI(func() { frame.TileHorizontally() })
	pause()
	horizontal := bounds()
	info("TileHorizontally: %v", horizontal)
	check("TileHorizontally", noOverlap(horizontal) && horizontal[0].X == horizontal[1].X && horizontal[1].X == horizontal[2].X, "bounds %v", horizontal)

	onUI(func() { frame.Cascade() })
	pause()
	cascade := bounds()
	info("Cascade: %v", cascade)
	check("Cascade", !slices.Equal(cascade, horizontal) && !noOverlap(cascade), "bounds %v", cascade)

	var minimized bool
	onUI(func() { docs[2].child.Minimize() })
	pause()
	onUI(func() {
		minimized = docs[2].child.IsMinimized()
		frame.ArrangeIcons()
	})
	pause()
	info("ArrangeIcons: minimized child at %v", bounds()[2])
	check("Minimize", minimized, "IsMinimized false")
	onUI(func() { docs[2].child.Restore() })
	pause()
	onUI(func() { minimized = docs[2].child.IsMinimized() })
	check("Restore a minimized child", !minimized, "still minimized")
}

// checkModalDialog runs a modal dialog owned by a child: the frame, the
// real owner, is disabled while it runs and enabled again after it, and
// the child is the active child again.
func checkModalDialog() {
	var owner win.HWND
	var frameEnabled, frameEnabledAfter bool
	var activeAfter win.HWND
	onUI(func() {
		d := docs[0]
		dlg, err := walk.NewDialog(d.child)
		if !checkErr("create a dialog owned by a child", err) {
			return
		}
		dlg.SetTitle("Modal dialog")
		dlg.Starting().Attach(func() {
			owner = win.GetWindow(dlg.Handle(), gwOwner)
			frameEnabled = win.IsWindowEnabled(frame.Handle())
			win.PostMessage(dlg.Handle(), win.WM_CLOSE, 0, 0)
		})
		docs[1].child.Activate()
		d.child.Activate()
		dlg.Run()
		dlg.Dispose()
		frameEnabledAfter = win.IsWindowEnabled(frame.Handle())
		activeAfter = win.HWND(win.SendMessage(mdiClient(), win.WM_MDIGETACTIVE, 0, 0))
	})
	pause()
	check("dialog owned by a child is owned by the frame", owner == frame.Handle(), "owner %#x, frame %#x", owner, frame.Handle())
	check("modal dialog of a child disables the frame", !frameEnabled, "the frame was enabled while the dialog ran")
	check("frame enabled after the dialog", frameEnabledAfter, "the frame is still disabled")
	check("owner child active after the dialog", activeAfter == docs[0].child.Handle(), "active %#x, want %#x", activeAfter, docs[0].child.Handle())
}

func checkClosing(lastDoc *doc, fg bool) {
	var ok bool
	var left int
	onUI(func() {
		docs[1].cancel = true
		ok = frame.CloseAllMdiChildren()
		left = len(frame.MdiChildren())
	})
	check("CloseAllMdiChildren stops at a canceled child", !ok && left == 2 && !isWindow(docs[0].child.Handle()) && isWindow(docs[1].child.Handle()) && isWindow(docs[2].child.Handle()),
		"result %v, %d children left", ok, left)

	var active *walk.MdiChildWindow
	var changes []string
	var items []menuItem
	onUI(func() {
		docs[1].cancel = false
		ok = frame.CloseAllMdiChildren()
		left = len(frame.MdiChildren())
		active = frame.ActiveMdiChild()
		changes = slices.Clone(activeChanges)
		items = menuItems(windowMenuHandle())
	})
	pause()
	check("CloseAllMdiChildren", ok && left == 0 && active == nil, "result %v, %d children left, active %v", ok, left, active)
	check("ActiveMdiChildChanged after the last child", len(changes) > 0 && changes[len(changes)-1] == "<none>", "events %q", changes)
	check("window menu without children", len(items) == 4, "items %q", menuTexts(items))
	check("no MDI child windows left", win.GetWindow(mdiClient(), win.GW_CHILD) == 0, "the MDI client still has a child window")

	if a, err := uia.New(); checkErr("uia New", err) {
		if e, err := a.ElementFromHandle(uintptr(mdiClient())); checkErr("uia ElementFromHandle(MDI client)", err) {
			name, _ := e.Name()
			check("accessible name of the MDI client", name == "Documents", "name %q", name)
			e.Release()
		}
		a.Close()
	}
	if fg {
		var focus win.HWND
		onUI(func() { focus = win.GetFocus() })
		check("focus in the MDI client after the last child", focus == mdiClient(), "focus %#x (class %q), MDI client %#x", focus, className(focus), mdiClient())
	}

	lastDoc.name = "Last child"
	lastDoc.cancel = true
	var err error
	onUI(func() { err = childDecl(lastDoc).Create(frame) })
	if !checkErr("create a child after closing all", err) {
		return
	}
	pause()

	// The Closing event of the frame comes first; canceling it keeps the
	// children without asking them.
	frameCancel := true
	frameClosing := 0
	var handle int
	onUI(func() {
		handle = frame.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
			frameClosing++
			*canceled = frameCancel
		})
		frame.Close()
	})
	pause()
	var childClosing int
	onUI(func() { childClosing = lastDoc.closing })
	check("frame Closing comes first and its cancel keeps the children", frameClosing == 1 && childClosing == 0 && isWindow(lastDoc.child.Handle()), "frame Closing %d times, child Closing %d times, child exists %v", frameClosing, childClosing, isWindow(lastDoc.child.Handle()))
	onUI(func() {
		frameCancel = false
		frame.Closing().Detach(handle)
	})

	onUI(func() { frame.Close() })
	pause()
	check("frame close canceled by a child", isWindow(frame.Handle()) && isWindow(lastDoc.child.Handle()) && lastDoc.closing == 1, "frame exists %v, child exists %v, Closing %d times", isWindow(frame.Handle()), isWindow(lastDoc.child.Handle()), lastDoc.closing)
	onUI(func() { lastDoc.cancel = false })
}

// manual shows a frame with three documents for a test with a screen
// reader.
func manual() {
	var status *walk.StatusBarItem
	var count int
	report := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		fmt.Println(s)
		status.SetText(s)
	}

	var newDocument func()
	newDocument = func() {
		count++
		name := fmt.Sprintf("Document %d", count)
		var child *walk.MdiChildWindow
		var nameEdit *walk.LineEdit
		var notes *walk.TextEdit
		if err := (MdiChildWindow{
			AssignTo: &child,
			Title:    name,
			Size:     Size{Width: 420, Height: 260},
			Layout:   VBox{},
			Children: []Widget{
				Label{Text: "&Name:"},
				LineEdit{AssignTo: &nameEdit, Accessibility: Accessibility{Name: "Name"}},
				Label{Text: "N&otes:"},
				TextEdit{AssignTo: &notes, Accessibility: Accessibility{Name: "Notes"}},
				PushButton{Text: "&Rename to the name", OnClicked: func() {
					if t := strings.TrimSpace(nameEdit.Text()); t != "" {
						child.SetTitle(t)
					}
				}},
			},
			OnActivating:   func() { fmt.Println("Activating", child.Title()) },
			OnDeactivating: func() { fmt.Println("Deactivating", child.Title()) },
			OnClosing: func(canceled *bool, reason walk.CloseReason) {
				if notes.Text() != "" && walk.MsgBox(frame, "Close", child.Title()+" has notes. Close it anyway?", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
					*canceled = true
					report("Closing of %s canceled", child.Title())
				}
			},
		}).Create(frame); err != nil {
			report("create: %v", err)
		}
	}

	if err := (MdiMainWindow{
		AssignTo:   &frame,
		Title:      "walk mdi manual test",
		Size:       Size{Width: 900, Height: 650},
		WindowMenu: &windowMenu,
		MenuItems: []MenuItem{
			Menu{Text: "&File", Items: []MenuItem{
				Action{Text: "&New", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyN}, OnTriggered: newDocument},
				Separator{},
				Action{Text: "E&xit", OnTriggered: func() { frame.Close() }},
			}},
			Menu{AssignTo: &windowMenu, Text: "&Window", Items: []MenuItem{
				Action{Text: "&Cascade", OnTriggered: func() { frame.Cascade() }},
				Action{Text: "Tile &Horizontally", OnTriggered: func() { frame.TileHorizontally() }},
				Action{Text: "Tile &Vertically", OnTriggered: func() { frame.TileVertically() }},
				Action{Text: "&Arrange Icons", OnTriggered: func() { frame.ArrangeIcons() }},
				Action{Text: "Close A&ll", OnTriggered: func() { frame.CloseAllMdiChildren() }},
			}},
		},
		StatusBarItems: []StatusBarItem{{AssignTo: &status, Text: "Ready", Width: 400}},
		OnActiveMdiChildChanged: func() {
			if c := frame.ActiveMdiChild(); c != nil {
				report("Active document: %s", c.Title())
			} else {
				report("No document")
			}
		},
	}).Create(); err != nil {
		fmt.Println("create frame:", err)
		os.Exit(1)
	}
	for range 3 {
		newDocument()
	}
	frame.Run()
}
