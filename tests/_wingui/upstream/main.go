// Command upstream checks on Windows the changes ported from open pull
// requests of lxn/walk (see opened-prs.md):
//
//   - lxn/walk#838: the file dialog filter keeps characters whose UTF-16
//     code ends in 0x7C ('ż' is U+017C). It opens the open dialog and reads
//     the items of its file type combo box.
//   - lxn/walk#771: TreeView does not panic when the model inserts root
//     items, and inserts them at their model position.
//   - lxn/walk#552 and keyboard access: NotifyIcon publishes Activated for
//     NIN_SELECT and NIN_KEYSELECT and MouseDoubleClick for double clicks,
//     and a context menu requested with the keyboard opens next to the icon.
//   - lxn/walk#393: LineEdit.PasswordMode is a bindable property; here it
//     is bound to a "Show password" CheckBox.
//
// It runs the checks, prints PASS and FAIL lines and exits with code 1 if a
// check failed. The notify icon appears in the notification area for a
// moment and its context menu opens and closes by itself.
//
// With the argument "manual" it skips the checks and keeps the window open
// for a test by hand with a screen reader.
package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
	"golang.org/x/sys/windows"
)

const (
	tvmGetNextItem = 0x110A
	tvgnRoot       = 0
	tvgnNext       = 1
	tvifHandle     = 0x10
	cbGetCount     = 0x146
	cbGetLBText    = 0x148
	cbGetLBTextLen = 0x149
	esPassword     = 0x20

	// notifyIconMessage is notifyIconMessageId of walk (window.go), the
	// callback message of every NotifyIcon.
	notifyIconMessage = win.WM_APP

	// The filter of the file dialog check: U+017C has the low byte 0x7C.
	dialogFilter = "Súbory ż (*.txt)|*.txt|Všetky súbory (*.*)|*.*"
)

var (
	user32       = windows.NewLazySystemDLL("user32.dll")
	procSetTimer = user32.NewProc("SetTimer")
	procKillTim  = user32.NewProc("KillTimer")
	procEndMenu  = user32.NewProc("EndMenu")
)

var (
	mw       *walk.MainWindow
	tv       *walk.TreeView
	pwEdit   *walk.LineEdit
	showPw   *walk.CheckBox
	logLabel *walk.Label
	ni       *walk.NotifyIcon
	model    = &rootModel{}
	failed   bool
	logCount int
)

func check(name string, ok bool, format string, args ...any) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failed = true
	}
	fmt.Printf("%s %s: %s\n", status, name, fmt.Sprintf(format, args...))
}

// logEvent prints an event and shows it in the log Label, a polite live
// region, so that a screen reader reads it in the manual test.
func logEvent(format string, args ...any) {
	logCount++
	text := fmt.Sprintf("%d: %s", logCount, fmt.Sprintf(format, args...))
	fmt.Println("EVENT", text)
	if logLabel != nil {
		logLabel.SetText(text)
	}
}

// node is a root item without children.
type node struct{ text string }

func (n *node) Text() string              { return n.text }
func (n *node) Parent() walk.TreeItem     { return nil }
func (n *node) ChildCount() int           { return 0 }
func (n *node) ChildAt(int) walk.TreeItem { return nil }

// rootModel is a TreeModel with root items only.
type rootModel struct {
	walk.TreeModelBase
	roots []*node
}

func (m *rootModel) RootCount() int             { return len(m.roots) }
func (m *rootModel) RootAt(i int) walk.TreeItem { return m.roots[i] }

// insert inserts a root item at index i and publishes ItemInserted. It
// returns the panic of the TreeView, if any.
func (m *rootModel) insert(i int, text string) (p any) {
	n := &node{text}
	m.roots = append(m.roots, nil)
	copy(m.roots[i+1:], m.roots[i:])
	m.roots[i] = n
	defer func() { p = recover() }()
	m.PublishItemInserted(n)
	return nil
}

// treeTexts returns the texts of the root items of the tree view control
// in their visual order.
func treeTexts() []string {
	var texts []string
	h := tv.SendMessage(tvmGetNextItem, tvgnRoot, 0)
	for h != 0 {
		buf := make([]uint16, 64)
		item := win.TVITEM{
			Mask:       win.TVIF_TEXT | tvifHandle,
			HItem:      win.HTREEITEM(h),
			PszText:    uintptr(unsafe.Pointer(&buf[0])),
			CchTextMax: int32(len(buf)),
		}
		tv.SendMessage(win.TVM_GETITEM, 0, uintptr(unsafe.Pointer(&item)))
		texts = append(texts, windows.UTF16ToString(buf))
		h = tv.SendMessage(tvmGetNextItem, tvgnNext, h)
	}
	return texts
}

func checkTree() {
	p1 := model.insert(0, "A")
	p2 := model.insert(len(model.roots), "C")
	check("TreeView root insert", p1 == nil && p2 == nil, "panics: %v, %v", p1, p2)
	got := strings.Join(treeTexts(), ",")
	check("TreeView root order", got == "A,B,C", "items %q, want \"A,B,C\"", got)
}

func passwordState() (mode bool, style bool) {
	s := uint32(win.GetWindowLong(pwEdit.Handle(), win.GWL_STYLE))
	return pwEdit.PasswordMode(), s&esPassword != 0
}

func checkPassword() {
	changes := 0
	pwEdit.Property("PasswordMode").Changed().Attach(func() { changes++ })

	mode, style := passwordState()
	check("PasswordMode bound, unchecked", mode && style, "PasswordMode %v, ES_PASSWORD %v, want true, true", mode, style)

	showPw.SetChecked(true)
	mode, style = passwordState()
	check("PasswordMode bound, checked", !mode && !style && changes == 1, "PasswordMode %v, ES_PASSWORD %v, %d changes, want false, false, 1", mode, style, changes)

	showPw.SetChecked(false)
	mode, style = passwordState()
	check("PasswordMode bound, unchecked again", mode && style && changes == 2, "PasswordMode %v, ES_PASSWORD %v, %d changes, want true, true, 2", mode, style, changes)
}

func checkNotifyIconEvents() {
	activated, doubleClicks := 0, 0
	var button walk.MouseButton
	h1 := ni.Activated().Attach(func() { activated++ })
	h2 := ni.MouseDoubleClick().Attach(func(x, y int, b walk.MouseButton) {
		doubleClicks++
		button = b
	})
	defer ni.Activated().Detach(h1)
	defer ni.MouseDoubleClick().Detach(h2)

	win.SendMessage(mw.Handle(), notifyIconMessage, 0, win.NIN_SELECT)
	check("NotifyIcon Activated on NIN_SELECT", activated == 1, "%d events, want 1", activated)
	win.SendMessage(mw.Handle(), notifyIconMessage, 0, win.NIN_SELECT|win.NINF_KEY)
	check("NotifyIcon Activated on NIN_KEYSELECT", activated == 2, "%d events, want 2", activated)
	win.SendMessage(mw.Handle(), notifyIconMessage, 0, win.WM_LBUTTONDBLCLK)
	check("NotifyIcon MouseDoubleClick", doubleClicks == 1 && button == walk.LeftButton, "%d events, button %d, want 1, %d", doubleClicks, button, walk.LeftButton)
}

// windowsOfClass returns the visible top-level windows of this process with
// the given class.
func windowsOfClass(class string) []windows.HWND {
	var found []windows.HWND
	pid := uint32(os.Getpid())
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var p uint32
		windows.GetWindowThreadProcessId(h, &p)
		if p == pid && windows.IsWindowVisible(h) && className(h) == class {
			found = append(found, h)
		}
		return 1
	})
	windows.EnumWindows(cb, nil)
	return found
}

func className(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, _ := windows.GetClassName(h, &buf[0], int32(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// whileModal calls tick every 200 ms from a thread timer until tick returns
// true or 25 ticks passed (then giveUp runs). Modal loops of menus and
// dialogs dispatch the timer, so tick runs on the UI thread while the
// modal loop started after whileModal is active.
func whileModal(tick func() bool, giveUp func()) {
	var id uintptr
	n := 0
	cb := syscall.NewCallback(func(hwnd, msg, idEvent, time uintptr) uintptr {
		n++
		if tick() || n >= 25 {
			procKillTim.Call(0, id)
			if n >= 25 {
				giveUp()
			}
		}
		return 0
	})
	id, _, _ = procSetTimer.Call(0, 0, 200, cb)
}

func intersect(a, b win.RECT) bool {
	return a.Left < b.Right && b.Left < a.Right && a.Top < b.Bottom && b.Top < a.Bottom
}

func checkKeyboardMenu() {
	nii := win.NOTIFYICONIDENTIFIER{HWnd: mw.Handle()}
	nii.CbSize = uint32(unsafe.Sizeof(nii))
	var icon win.RECT
	hr := win.Shell_NotifyIconGetRect(&nii, &icon)
	if win.FAILED(hr) {
		fmt.Printf("INFO Shell_NotifyIconGetRect failed with 0x%x, the menu opens at the cursor\n", uint32(hr))
	}

	var menu win.RECT
	found := false
	whileModal(func() bool {
		ws := windowsOfClass("#32768")
		if len(ws) == 0 {
			return false
		}
		win.GetWindowRect(win.HWND(ws[0]), &menu)
		found = true
		procEndMenu.Call()
		return true
	}, func() { procEndMenu.Call() })

	// What the shell sends for Shift+F10 on a selected icon.
	win.SendMessage(mw.Handle(), notifyIconMessage, 0, win.WM_CONTEXTMENU)

	check("NotifyIcon keyboard menu opened", found, "menu window found: %v", found)
	if !found || win.FAILED(hr) {
		return
	}
	fmt.Printf("INFO icon %+v, menu %+v\n", icon, menu)
	beside := menu.Bottom <= icon.Top+1 || menu.Top >= icon.Bottom-1 ||
		menu.Right <= icon.Left+1 || menu.Left >= icon.Right-1
	near := menu.Left <= icon.Right+10 && menu.Right >= icon.Left-10 &&
		menu.Top <= icon.Bottom+10 && menu.Bottom >= icon.Top-10
	check("NotifyIcon keyboard menu at the icon", !intersect(menu, icon) && beside && near,
		"icon %d,%d-%d,%d, menu %d,%d-%d,%d", icon.Left, icon.Top, icon.Right, icon.Bottom, menu.Left, menu.Top, menu.Right, menu.Bottom)
}

// comboItems returns the items of every ComboBox child of the window.
func comboItems(dlg windows.HWND) [][]string {
	var all [][]string
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		if className(h) != "ComboBox" {
			return 1
		}
		var items []string
		n := int(win.SendMessage(win.HWND(h), cbGetCount, 0, 0))
		for i := 0; i < n && i < 50; i++ {
			l := int(win.SendMessage(win.HWND(h), cbGetLBTextLen, uintptr(i), 0))
			if l < 0 || l > 1000 {
				continue
			}
			buf := make([]uint16, l+1)
			win.SendMessage(win.HWND(h), cbGetLBText, uintptr(i), uintptr(unsafe.Pointer(&buf[0])))
			items = append(items, windows.UTF16ToString(buf))
		}
		all = append(all, items)
		return 1
	})
	windows.EnumChildWindows(dlg, cb, nil)
	return all
}

func checkFileDialog() {
	var combos [][]string
	found := false
	whileModal(func() bool {
		ws := windowsOfClass("#32770")
		if len(ws) == 0 {
			return false
		}
		combos = comboItems(ws[0])
		found = true
		win.PostMessage(win.HWND(ws[0]), win.WM_COMMAND, win.IDCANCEL, 0)
		return true
	}, func() {
		for _, w := range windowsOfClass("#32770") {
			win.PostMessage(win.HWND(w), win.WM_COMMAND, win.IDCANCEL, 0)
		}
	})

	dlg := walk.FileDialog{Title: "walk filter test", Filter: dialogFilter}
	accepted, err := dlg.ShowOpen(mw)
	check("FileDialog opened and canceled", found && !accepted && err == nil, "dialog found %v, accepted %v, error %v", found, accepted, err)

	ok := false
	for _, items := range combos {
		if strings.Join(items, "|") == "Súbory ż (*.txt)|Všetky súbory (*.*)" {
			ok = true
		}
	}
	check("FileDialog filter with U+017C", ok, "combo box items %q, want one with %q", combos, []string{"Súbory ż (*.txt)", "Všetky súbory (*.*)"})
}

func runChecks() {
	checkTree()
	checkPassword()
	checkNotifyIconEvents()
	checkKeyboardMenu()
	checkFileDialog()
	fmt.Println("CHECKS DONE")
	mw.Close()
}

func main() {
	manual := len(os.Args) > 1 && os.Args[1] == "manual"
	model.roots = []*node{{"B"}}

	err := MainWindow{
		AssignTo: &mw,
		Title:    "walk upstream test",
		Size:     Size{Width: 500, Height: 450},
		Layout:   VBox{},
		Children: []Widget{
			Label{
				AssignTo:      &logLabel,
				Text:          "Events appear here.",
				Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite},
			},
			CheckBox{
				AssignTo: &showPw,
				Name:     "showPw",
				Text:     "&Show password",
			},
			LineEdit{
				AssignTo:      &pwEdit,
				PasswordMode:  Bind("!showPw.Checked"),
				Text:          "secret",
				Accessibility: Accessibility{Name: "Password"},
			},
			TreeView{
				AssignTo:      &tv,
				Accessibility: Accessibility{Name: "Root items"},
			},
			PushButton{
				Text: "Add root item at the &top",
				OnClicked: func() {
					if p := model.insert(0, fmt.Sprintf("Top %d", len(model.roots))); p != nil {
						logEvent("TreeView panicked: %v", p)
					}
				},
			},
			PushButton{
				Text: "Add root item at the &end",
				OnClicked: func() {
					if p := model.insert(len(model.roots), fmt.Sprintf("End %d", len(model.roots))); p != nil {
						logEvent("TreeView panicked: %v", p)
					}
				},
			},
			PushButton{
				Text: "Open &file dialog",
				OnClicked: func() {
					dlg := walk.FileDialog{Title: "walk filter test", Filter: dialogFilter}
					if _, err := dlg.ShowOpen(mw); err != nil {
						logEvent("ShowOpen failed: %v", err)
					}
				},
			},
		},
	}.Create()
	if err != nil {
		fmt.Println("FAIL create:", err)
		os.Exit(1)
	}

	if err := tv.SetModel(model); err != nil {
		fmt.Println("FAIL SetModel:", err)
		os.Exit(1)
	}

	ni, err = walk.NewNotifyIcon(mw)
	if err != nil {
		fmt.Println("FAIL NewNotifyIcon:", err)
		os.Exit(1)
	}
	defer ni.Dispose()
	ni.SetIcon(walk.IconInformation())
	ni.SetToolTip("walk upstream test")
	ni.Activated().Attach(func() { logEvent("Notify icon activated") })
	ni.MouseDoubleClick().Attach(func(x, y int, b walk.MouseButton) { logEvent("Notify icon double click, button %d", b) })
	hello := walk.NewAction()
	hello.SetText("Say &hello")
	hello.Triggered().Attach(func() { logEvent("Hello from the context menu") })
	ni.ContextMenu().Actions().Add(hello)
	if err := ni.SetVisible(true); err != nil {
		fmt.Println("FAIL NotifyIcon SetVisible:", err)
		os.Exit(1)
	}

	fmt.Println("READY")
	if !manual {
		go mw.Synchronize(runChecks)
	}

	mw.Run()
	if failed {
		os.Exit(1)
	}
}
