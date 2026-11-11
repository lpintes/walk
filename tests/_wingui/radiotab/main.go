// Command radiotab checks on Windows that a group of radio buttons is one tab
// stop, as in system dialogs: the checked button, or the first one when none
// is checked. The arrow keys move inside the group.
//
// walk used to give WS_TABSTOP to every radio button, so Tab stopped on each
// of them, and the first focus of a form skipped an unchecked group.
//
// The window has an edit field, group A (three buttons, none checked), group
// B (three buttons, the second checked) and a push button.
//
//   - The tab stops of the groups: only A1 and B2 have WS_TABSTOP, and
//     GetNextDlgTabItem goes edit, A1, B2, button, edit; backwards the
//     other way.
//   - After A3.SetChecked(true): A3 is the tab stop of group A.
//   - Down arrow posted to the focused A3: the focus moves to A1 (the group
//     wraps), A1 is checked, A3 unchecked and A1 the only tab stop of
//     group A.
//   - SetChecked(true) on A2 unchecks A1 (walk did not uncheck the other
//     buttons of the group before) and makes A2 the tab stop.
//   - Tab posted to the focused control, from the edit field: A1, B2, the
//     button, the edit field again; Shift+Tab is not posted, because the
//     dialog manager reads the Shift state of the keyboard.
//   - A dialog whose first control is an unchecked group: the first button
//     of the group has the focus when it opens.
//
// "radiotab manual" leaves the window open for a test with a screen reader.
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
	user32       = windows.NewLazySystemDLL("user32.dll")
	getClassName = user32.NewProc("GetClassNameW")
)

var (
	failed bool
	mw     *walk.MainWindow
	names  = map[win.HWND]string{}
)

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Println("PASS", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: "+format+"\n", append([]any{name}, args...)...)
}

func describe(hwnd win.HWND) string {
	if n, ok := names[hwnd]; ok {
		return n
	}
	buf := make([]uint16, 256)
	getClassName.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return fmt.Sprintf("%#x %q", hwnd, syscall.UTF16ToString(buf))
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

func hasTabStop(w walk.Window) bool {
	return win.GetWindowLong(w.Handle(), win.GWL_STYLE)&win.WS_TABSTOP != 0
}

// checkStops checks which buttons of the groups have WS_TABSTOP.
func checkStops(name string, buttons []*walk.RadioButton, want ...*walk.RadioButton) {
	var got []string
	onUI(func() {
		for _, b := range buttons {
			if hasTabStop(b) {
				got = append(got, describe(b.Handle()))
			}
		}
	})
	var exp []string
	for _, b := range want {
		exp = append(exp, describe(b.Handle()))
	}
	check(name, fmt.Sprint(got) == fmt.Sprint(exp), "tab stops %v, want %v", got, exp)
}

// checkOrder checks the tab order GetNextDlgTabItem gives from the first
// window of want.
func checkOrder(name string, previous bool, want ...walk.Window) {
	var got, exp []string
	onUI(func() {
		hwnd := want[0].Handle()
		got = append(got, describe(hwnd))
		for range len(want) - 1 {
			hwnd = win.GetNextDlgTabItem(mw.Handle(), hwnd, previous)
			got = append(got, describe(hwnd))
		}
	})
	for _, w := range want {
		exp = append(exp, describe(w.Handle()))
	}
	check(name, fmt.Sprint(got) == fmt.Sprint(exp), "order %v, want %v", got, exp)
}

// key posts a key press to the focused window, lets the message loop handle
// it and returns the focused window afterwards.
func key(vk uintptr) win.HWND {
	onUI(func() {
		hwnd := win.GetFocus()
		win.PostMessage(hwnd, win.WM_KEYDOWN, vk, 1)
		win.PostMessage(hwnd, win.WM_KEYUP, vk, 1|3<<30)
	})
	time.Sleep(200 * time.Millisecond)
	var focus win.HWND
	onUI(func() { focus = win.GetFocus() })
	return focus
}

func checkFocus(name string, focus win.HWND, want walk.Window) {
	check(name, focus == want.Handle(), "focus %s, want %s", describe(focus), describe(want.Handle()))
}

func main() {
	manual := len(os.Args) > 1 && os.Args[1] == "manual"

	var edit *walk.LineEdit
	var a1, a2, a3, b1, b2, b3 *walk.RadioButton
	var button *walk.PushButton

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "Radio button tab stop test",
		Size:     Size{Width: 400, Height: 300},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "Name:"},
			LineEdit{AssignTo: &edit, Text: "Edit"},
			Composite{Layout: HBox{}, Children: []Widget{
				RadioButton{AssignTo: &a1, Text: "A1"},
				RadioButton{AssignTo: &a2, Text: "A2"},
				RadioButton{AssignTo: &a3, Text: "A3"},
			}},
			Composite{Layout: HBox{}, Children: []Widget{
				RadioButton{AssignTo: &b1, Text: "B1"},
				RadioButton{AssignTo: &b2, Text: "B2"},
				RadioButton{AssignTo: &b3, Text: "B3"},
			}},
			PushButton{AssignTo: &button, Text: "Button"},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}
	b2.SetChecked(true)

	names[edit.Handle()] = "edit"
	names[button.Handle()] = "button"
	for _, b := range []*walk.RadioButton{a1, a2, a3, b1, b2, b3} {
		names[b.Handle()] = b.Text()
	}
	all := []*walk.RadioButton{a1, a2, a3, b1, b2, b3}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if manual {
			return
		}
		// Let the first layout give the focus.
		time.Sleep(300 * time.Millisecond)

		checkStops("tab stops at start", all, a1, b2)
		checkOrder("tab order at start", false, edit, a1, b2, button, edit)
		checkOrder("reverse tab order at start", true, edit, button, b2, a1, edit)

		onUI(func() { a3.SetChecked(true) })
		checkStops("tab stops after SetChecked(A3)", all, a3, b2)
		checkOrder("tab order after SetChecked(A3)", false, edit, a3, b2, button)

		onUI(func() { a3.SetFocus() })
		focus := key(win.VK_DOWN)
		checkFocus("Down arrow on A3 focuses A1", focus, a1)
		var a1Checked, a3Checked bool
		onUI(func() { a1Checked, a3Checked = a1.Checked(), a3.Checked() })
		check("Down arrow on A3 checks A1", a1Checked && !a3Checked, "A1 checked %t, A3 checked %t", a1Checked, a3Checked)
		checkStops("tab stops after the arrow key", all, a1, b2)

		var a2Checked bool
		onUI(func() {
			a2.SetChecked(true)
			a1Checked, a2Checked = a1.Checked(), a2.Checked()
		})
		check("SetChecked(A2) unchecks A1", a2Checked && !a1Checked, "A1 checked %t, A2 checked %t", a1Checked, a2Checked)
		checkStops("tab stops after SetChecked(A2)", all, a2, b2)
		onUI(func() { a1.SetChecked(true) })

		onUI(func() { edit.SetFocus() })
		checkFocus("Tab from the edit field", key(win.VK_TAB), a1)
		checkFocus("Tab from group A", key(win.VK_TAB), b2)
		checkFocus("Tab from group B", key(win.VK_TAB), button)
		checkFocus("Tab from the button", key(win.VK_TAB), edit)

		var c1, c2 *walk.RadioButton
		var dlg *walk.Dialog
		opened := make(chan struct{})
		closed := make(chan struct{})
		mw.Synchronize(func() {
			defer close(closed)
			if err := (Dialog{
				AssignTo: &dlg,
				Title:    "Unchecked group first",
				Layout:   VBox{},
				Children: []Widget{
					RadioButton{AssignTo: &c1, Text: "C1"},
					RadioButton{AssignTo: &c2, Text: "C2"},
					LineEdit{Text: "Edit"},
				},
			}).Create(mw); err != nil {
				check("dialog create", false, "%v", err)
				close(opened)
				return
			}
			dlg.Starting().Attach(func() { close(opened) })
			dlg.Run()
		})
		<-opened
		if dlg != nil {
			time.Sleep(300 * time.Millisecond)
			var focus win.HWND
			onUI(func() {
				names[c1.Handle()] = "C1"
				focus = win.GetFocus()
			})
			checkFocus("first focus of a dialog starting with an unchecked group", focus, c1)
			onUI(func() { dlg.Cancel() })
		}
		<-closed

		mw.Synchronize(func() { mw.Close() })
	}()
	mw.Run()
	<-done

	if failed {
		os.Exit(1)
	}
}
