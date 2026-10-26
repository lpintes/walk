// Command dialogs checks on Windows the dialog step (step 4): walk dialogs
// that screen readers recognize, and TaskDialog.
//
//   - A reference message box (MsgBox, a real #32770 dialog) is opened and
//     its UI Automation view is printed as INFO lines: control type,
//     localized control type, MSAA role (LegacyIAccessible), IsDialog and
//     IsModal. It is closed with WM_CLOSE.
//   - A walk Dialog with a static text, an edit field and OK and Cancel
//     buttons. Through UI Automation (package uia) its MSAA role must be
//     ROLE_SYSTEM_DIALOG (18) and IsDialog true, like the message box; its
//     name is the title and the static text is found. An in-process
//     WinEvent hook must see EVENT_SYSTEM_DIALOGSTART when it opens. The
//     test prints "DIALOG_HWND <hwnd>" and "MSAA_READY" and waits up to 15
//     seconds for the named event Local\walk-dialogs-msaa, which run.ps1
//     sets after reading the MSAA objects of the window (client object
//     role dialog, window object role window); without it the wait times
//     out and the test goes on. Then the Cancel button is invoked through
//     UI Automation: Run must return DlgCmdCancel and the hook must see
//     EVENT_SYSTEM_DIALOGEND.
//   - A TaskDialog with custom buttons, common buttons, radio buttons, a
//     verification check box, expandable information and a footer. Through
//     UI Automation it must be a dialog (role 18), show the main
//     instruction, and the test selects the second radio button, checks the
//     check box and invokes a custom button; Show must return them.
//   - A TaskDialog with AllowCancel closed with WM_CLOSE: DlgCmdCancel.
//   - A TaskDialog with a hyperlink: the link is invoked through UI
//     Automation; if OnHyperlinkClicked is called it must get the target
//     (Windows 11 does not call it for Invoke, which is reported as INFO).
//     TDM_CLICK_BUTTON with IDOK closes the dialog: DlgCmdOK.
//
// The checks run on a goroutine while the main window runs its message
// loop; each modal dialog is opened with Synchronize and closed by the
// goroutine. It prints PASS, FAIL and INFO lines and exits with code 1 if
// a check failed.
//
// With the argument "manual" it shows a window with buttons that open a
// walk dialog, task dialogs and a message box, for a test with a screen
// reader. Results go to a polite live region and standard output.
package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
	"github.com/lpintes/walk/uia"
)

const (
	title          = "walk dialogs test"
	referenceTitle = "walk dialogs reference"
	dialogTitle    = "walk dialogs walk dialog"
	dialogText     = "Do you want to save the changes to the report?"
	taskTitle      = "walk dialogs task dialog"
	taskMain       = "Save the changes to the report?"
	cancelTitle    = "walk dialogs task dialog cancel"
	linkTitle      = "walk dialogs task dialog link"
	linkTarget     = "https://example.com/walk-dialogs"

	roleDialog = 18 // ROLE_SYSTEM_DIALOG
)

var (
	failed bool
	mw     *walk.MainWindow
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

func checkErr(name string, err error) bool {
	if err != nil {
		return check(name, false, "%v", err)
	}
	return true
}

func info(format string, args ...any) {
	fmt.Printf("INFO "+format+"\n", args...)
}

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	setWinEventHook = user32.NewProc("SetWinEventHook")
	isWindow        = user32.NewProc("IsWindow")
)

const winEventOutOfContext = 0

// dialogEvents records the EVENT_SYSTEM_DIALOGSTART and DIALOGEND events of
// the process. The hook is out of context, so its callback runs from the
// message loop of the UI thread; the checks read the events from another
// goroutine.
var (
	dialogEventsMu sync.Mutex
	dialogEvents   []string
)

func winEventProc(hook uintptr, event uint32, hwnd win.HWND, idObject, idChild int32, thread, time uint32) uintptr {
	var name string
	switch event {
	case win.EVENT_SYSTEM_DIALOGSTART:
		name = "start"
	case win.EVENT_SYSTEM_DIALOGEND:
		name = "end"
	default:
		return 0
	}
	dialogEventsMu.Lock()
	dialogEvents = append(dialogEvents, fmt.Sprintf("%s %d", name, hwnd))
	dialogEventsMu.Unlock()
	return 0
}

// waitDialogEvent waits up to two seconds for the event ("start" or "end")
// of hwnd.
func waitDialogEvent(event string, hwnd win.HWND) bool {
	want := fmt.Sprintf("%s %d", event, hwnd)
	for range 20 {
		dialogEventsMu.Lock()
		for _, e := range dialogEvents {
			if e == want {
				dialogEventsMu.Unlock()
				return true
			}
		}
		dialogEventsMu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func dialogEventList() []string {
	dialogEventsMu.Lock()
	defer dialogEventsMu.Unlock()
	return append([]string(nil), dialogEvents...)
}

// findWindow waits up to five seconds for a visible top-level window of the
// class #32770 with the title.
func findWindow(windowTitle string) win.HWND {
	class := win.StringToUTF16Ptr("#32770")
	name := win.StringToUTF16Ptr(windowTitle)
	for range 50 {
		if h := win.FindWindow(class, name); h != 0 && win.IsWindowVisible(h) {
			return h
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

// waitGone waits up to five seconds for the window to be destroyed.
func waitGone(h win.HWND) bool {
	for range 50 {
		if r, _, _ := isWindow.Call(uintptr(h)); r == 0 {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// waitResult waits up to ten seconds for the result of a modal dialog.
func waitResult[T any](c <-chan T) (T, bool) {
	select {
	case r := <-c:
		return r, true
	case <-time.After(10 * time.Second):
		var zero T
		return zero, false
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "manual" {
		manual()
		return
	}

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    title,
		Size:     Size{Width: 400, Height: 200},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "The dialogs of this test open and close by themselves."},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}

	if r, _, _ := setWinEventHook.Call(
		win.EVENT_SYSTEM_DIALOGSTART, win.EVENT_SYSTEM_DIALOGEND,
		0, syscall.NewCallback(winEventProc),
		uintptr(os.Getpid()), 0, winEventOutOfContext); r == 0 {
		check("SetWinEventHook", false, "failed")
	}

	hwnd := mw.Handle()
	checksDone := make(chan struct{})
	go func() {
		defer close(checksDone)
		for i := 0; i < 100 && !win.IsWindowVisible(hwnd); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		run()
		mw.Synchronize(func() { mw.Close() })
	}()
	mw.Run()
	<-checksDone

	if failed {
		os.Exit(1)
	}
}

func run() {
	check("TaskDialogAvailable", walk.TaskDialogAvailable(), "false: the exe has no manifest asking for common controls 6")

	a, err := uia.New()
	if !checkErr("uia New", err) {
		return
	}
	defer a.Close()

	ref := checkReference(a)
	checkWalkDialog(a, ref)
	if walk.TaskDialogAvailable() {
		checkTaskDialog(a)
		checkTaskDialogCancel()
		checkTaskDialogLink(a)
	}
}

// view is what UI Automation reports about a dialog window.
type view struct {
	controlType   uia.ControlType
	localizedType string
	role          int
	isDialog      bool
	isModal       bool
}

func readView(a *uia.Automation, name string, h win.HWND) (view, *uia.Element, bool) {
	var v view
	e, err := a.ElementFromHandle(uintptr(h))
	if !checkErr(name+" ElementFromHandle", err) {
		return v, nil, false
	}
	var errs []error
	var err1, err2, err3, err4, err5 error
	v.controlType, err1 = e.ControlType()
	v.localizedType, err2 = e.LocalizedControlType()
	v.role, err3 = e.Role()
	v.isDialog, err4 = e.IsDialog()
	v.isModal, err5 = e.IsModal()
	for _, err := range []error{err1, err2, err3, err4, err5} {
		if err != nil {
			errs = append(errs, err)
		}
	}
	info("%s: ControlType %v, LocalizedControlType %q, Role %d, IsDialog %v, IsModal %v, errors %v",
		name, v.controlType, v.localizedType, v.role, v.isDialog, v.isModal, errs)
	return v, e, true
}

// checkReference opens a message box and returns its view.
func checkReference(a *uia.Automation) view {
	done := make(chan int, 1)
	mw.Synchronize(func() {
		done <- walk.MsgBox(mw, referenceTitle, "Reference message box, closed by the test.", walk.MsgBoxOK)
	})
	h := findWindow(referenceTitle)
	if !check("reference message box opened", h != 0, "no window %q", referenceTitle) {
		return view{}
	}
	v, e, _ := readView(a, "reference message box", h)
	if e != nil {
		e.Release()
	}
	check("reference message box role", v.role == roleDialog, "Role %d, want %d", v.role, roleDialog)
	win.PostMessage(h, win.WM_CLOSE, 0, 0)
	_, ok := waitResult(done)
	check("reference message box closed", ok, "MsgBox did not return")
	return v
}

func checkWalkDialog(a *uia.Automation, ref view) {
	opened := make(chan win.HWND, 1)
	done := make(chan int, 1)
	mw.Synchronize(func() {
		var dlg *walk.Dialog
		var ok, cancel *walk.PushButton
		if err := (Dialog{
			AssignTo:      &dlg,
			Title:         dialogTitle,
			DefaultButton: &ok,
			CancelButton:  &cancel,
			MinSize:       Size{Width: 300, Height: 150},
			Layout:        VBox{},
			Children: []Widget{
				Label{Text: dialogText},
				LineEdit{Accessibility: Accessibility{Name: "File name"}},
				Composite{
					Layout: HBox{},
					Children: []Widget{
						HSpacer{},
						PushButton{AssignTo: &ok, Text: "OK", OnClicked: func() { dlg.Accept() }},
						PushButton{AssignTo: &cancel, Text: "Cancel", OnClicked: func() { dlg.Cancel() }},
					},
				},
			},
		}).Create(mw); err != nil {
			check("create walk dialog", false, "%v", err)
			opened <- 0
			done <- -1
			return
		}
		opened <- dlg.Handle()
		done <- dlg.Run()
	})

	h := <-opened
	if h == 0 {
		return
	}
	for i := 0; i < 50 && !win.IsWindowVisible(h); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	check("walk dialog EVENT_SYSTEM_DIALOGSTART", waitDialogEvent("start", h), "not raised; events %v", dialogEventList())

	v, e, ok := readView(a, "walk dialog", h)
	if ok {
		check("walk dialog role", v.role == roleDialog, "Role %d, want %d (ROLE_SYSTEM_DIALOG)", v.role, roleDialog)
		check("walk dialog IsDialog", v.isDialog, "false")
		check("walk dialog role like the message box", v.role == ref.role, "Role %d, message box %d", v.role, ref.role)
		check("walk dialog IsDialog like the message box", v.isDialog == ref.isDialog, "IsDialog %v, message box %v", v.isDialog, ref.isDialog)
		if v.isModal != ref.isModal {
			info("walk dialog IsModal %v differs from the message box (%v)", v.isModal, ref.isModal)
		}
		name, err := e.Name()
		check("walk dialog name", err == nil && name == dialogTitle, "%q, %v", name, err)
		text, err := e.FindFirst(uia.ScopeDescendants, uia.And(uia.ByControlType(uia.ControlTypeText), uia.ByName(dialogText)))
		check("walk dialog static text", err == nil && text != nil, "not found, %v", err)
		if text != nil {
			text.Release()
		}
	}

	fmt.Printf("DIALOG_HWND %d\n", h)
	waitForScript()

	if e != nil {
		cancel, err := e.FindFirst(uia.ScopeDescendants, uia.And(uia.ByControlType(uia.ControlTypeButton), uia.ByName("Cancel")))
		if check("walk dialog Cancel button", err == nil && cancel != nil, "not found, %v", err) {
			checkErr("walk dialog Invoke Cancel", cancel.Invoke())
			cancel.Release()
		}
		e.Release()
	}
	cmd, ok := waitResult(done)
	if !ok {
		check("walk dialog closed", false, "Run did not return; closing it")
		win.PostMessage(h, win.WM_CLOSE, 0, 0)
		waitResult(done)
		return
	}
	check("walk dialog result", cmd == walk.DlgCmdCancel, "Run returned %d, want DlgCmdCancel (%d)", cmd, walk.DlgCmdCancel)
	check("walk dialog EVENT_SYSTEM_DIALOGEND", waitDialogEvent("end", h), "not raised; events %v", dialogEventList())
}

// waitForScript creates the named event Local\walk-dialogs-msaa, prints
// MSAA_READY and waits up to 15 seconds for run.ps1 to read the MSAA objects
// of the dialog and set the event. The event exists before MSAA_READY is
// printed, so run.ps1 can always open it.
func waitForScript() {
	ev, err := windows.CreateEvent(nil, 1, 0, windows.StringToUTF16Ptr(`Local\walk-dialogs-msaa`))
	if err != nil {
		info("CreateEvent: %v", err)
		fmt.Println("MSAA_READY")
		return
	}
	defer windows.CloseHandle(ev)
	fmt.Println("MSAA_READY")
	if r, _ := windows.WaitForSingleObject(ev, 15000); r != windows.WAIT_OBJECT_0 {
		info("run.ps1 did not signal Local\\walk-dialogs-msaa; MSAA objects not checked")
	}
}

// findByName finds a descendant of e with the name and the control type.
func findByName(e *uia.Element, what, name string, ct uia.ControlType) *uia.Element {
	found, err := e.FindFirst(uia.ScopeDescendants, uia.And(uia.ByControlType(ct), uia.ByName(name)))
	if err != nil || found == nil {
		check("task dialog "+what, false, "%v %q not found, %v", ct, name, err)
		return nil
	}
	return found
}

func checkTaskDialog(a *uia.Automation) {
	type result struct {
		r   walk.TaskDialogResult
		err error
	}
	done := make(chan result, 1)
	mw.Synchronize(func() {
		td := walk.TaskDialog{
			Title:           taskTitle,
			MainInstruction: taskMain,
			Content:         "The report has unsaved changes.",
			Icon:            walk.TaskDialogIconWarning,
			CommonButtons:   walk.TaskDialogCancel,
			Buttons: []walk.TaskDialogButton{
				{ID: 101, Text: "Save"},
				{ID: 102, Text: "Discard"},
			},
			RadioButtons: []walk.TaskDialogButton{
				{ID: 201, Text: "Close the report"},
				{ID: 202, Text: "Keep the report open"},
			},
			VerificationText:    "Do not ask again",
			ExpandedInformation: "The changes are kept in memory only.",
			Footer:              "Footer text",
		}
		r, err := td.Show(mw)
		done <- result{r, err}
	})

	h := findWindow(taskTitle)
	if !check("task dialog opened", h != 0, "no window %q", taskTitle) {
		select {
		case r := <-done:
			check("task dialog Show", false, "returned early: %+v, %v", r.r, r.err)
		case <-time.After(5 * time.Second):
		}
		return
	}
	v, e, ok := readView(a, "task dialog", h)
	if ok {
		check("task dialog role", v.role == roleDialog, "Role %d, want %d", v.role, roleDialog)
		if t := findByName(e, "main instruction", taskMain, uia.ControlTypeText); t != nil {
			check("task dialog main instruction", true, "")
			t.Release()
		}
		if r := findByName(e, "radio button", "Keep the report open", uia.ControlTypeRadioButton); r != nil {
			err := r.Select()
			if err != nil {
				info("task dialog radio button Select: %v; trying DoDefaultAction", err)
				err = r.DoDefaultAction()
			}
			checkErr("task dialog select radio button", err)
			r.Release()
		}
		if c := findByName(e, "check box", "Do not ask again", uia.ControlTypeCheckBox); c != nil {
			err := c.Toggle()
			if err != nil {
				info("task dialog check box Toggle: %v; trying DoDefaultAction", err)
				err = c.DoDefaultAction()
			}
			checkErr("task dialog check the check box", err)
			c.Release()
		}
		if b := findByName(e, "button", "Discard", uia.ControlTypeButton); b != nil {
			checkErr("task dialog Invoke Discard", b.Invoke())
			b.Release()
		}
		e.Release()
	}

	r, ok := waitResult(done)
	if !ok {
		check("task dialog closed", false, "Show did not return; closing it")
		win.PostMessage(h, win.TDM_CLICK_BUTTON, win.IDCANCEL, 0)
		waitResult(done)
		return
	}
	if checkErr("task dialog Show", r.err) {
		check("task dialog button", r.r.Button == 102, "%d, want 102", r.r.Button)
		check("task dialog radio button", r.r.RadioButton == 202, "%d, want 202", r.r.RadioButton)
		check("task dialog verification", r.r.VerificationChecked, "not checked")
	}
	waitGone(h)
}

func checkTaskDialogCancel() {
	type result struct {
		r   walk.TaskDialogResult
		err error
	}
	done := make(chan result, 1)
	mw.Synchronize(func() {
		td := walk.TaskDialog{
			Title:         cancelTitle,
			Content:       "Closed with WM_CLOSE by the test.",
			CommonButtons: walk.TaskDialogOK,
			AllowCancel:   true,
		}
		r, err := td.Show(mw)
		done <- result{r, err}
	})
	h := findWindow(cancelTitle)
	if !check("cancel task dialog opened", h != 0, "no window %q", cancelTitle) {
		return
	}
	win.PostMessage(h, win.WM_CLOSE, 0, 0)
	r, ok := waitResult(done)
	if !check("cancel task dialog closed", ok, "Show did not return after WM_CLOSE") {
		win.PostMessage(h, win.TDM_CLICK_BUTTON, win.IDOK, 0)
		waitResult(done)
		return
	}
	if checkErr("cancel task dialog Show", r.err) {
		check("cancel task dialog result", r.r.Button == walk.DlgCmdCancel, "%d, want DlgCmdCancel (%d)", r.r.Button, walk.DlgCmdCancel)
	}
	waitGone(h)
}

func checkTaskDialogLink(a *uia.Automation) {
	type result struct {
		r   walk.TaskDialogResult
		err error
	}
	done := make(chan result, 1)
	links := make(chan string, 4)
	mw.Synchronize(func() {
		td := walk.TaskDialog{
			Title:              linkTitle,
			Content:            `Read <a href="` + linkTarget + `">the documentation</a>.`,
			OnHyperlinkClicked: func(href string) { links <- href },
		}
		r, err := td.Show(mw)
		done <- result{r, err}
	})
	h := findWindow(linkTitle)
	if !check("link task dialog opened", h != 0, "no window %q", linkTitle) {
		return
	}
	e, err := a.ElementFromHandle(uintptr(h))
	if checkErr("link task dialog ElementFromHandle", err) {
		link, err := e.FindFirst(uia.ScopeDescendants, uia.ByControlType(uia.ControlTypeHyperlink))
		if check("link task dialog hyperlink", err == nil && link != nil, "not found, %v", err) {
			err := link.Invoke()
			if err != nil {
				info("link Invoke: %v; trying DoDefaultAction", err)
				err = link.DoDefaultAction()
			}
			checkErr("link task dialog Invoke", err)
			link.Release()
			// Invoke returns S_OK, but on Windows 11 the task dialog does
			// not send TDN_HYPERLINK_CLICKED for it. The keyboard and the
			// mouse fire the callback, as the manual test showed, so a
			// missing callback here is only reported.
			href, ok := waitResult(links)
			if ok {
				check("OnHyperlinkClicked", href == linkTarget, "got %q, want %q", href, linkTarget)
			} else {
				info("OnHyperlinkClicked: not called after UI Automation Invoke (known; test it with the keyboard in the manual mode)")
			}
		}
		e.Release()
	}
	win.PostMessage(h, win.TDM_CLICK_BUTTON, win.IDOK, 0)
	r, ok := waitResult(done)
	if !check("link task dialog closed", ok, "Show did not return after TDM_CLICK_BUTTON") {
		win.PostMessage(h, win.WM_CLOSE, 0, 0)
		return
	}
	if checkErr("link task dialog Show", r.err) {
		check("link task dialog result", r.r.Button == walk.DlgCmdOK, "%d, want DlgCmdOK (%d)", r.r.Button, walk.DlgCmdOK)
	}
}

// manual shows a window for a test with a screen reader.
func manual() {
	var result *walk.Label
	// The counter makes every result a new text, so the live region is
	// announced again when the same link is clicked twice.
	var reports int
	report := func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		fmt.Println(s)
		reports++
		result.SetText(fmt.Sprintf("%d: %s", reports, s))
	}

	openDialog := func() {
		var dlg *walk.Dialog
		var ok, cancel *walk.PushButton
		var name *walk.LineEdit
		cmd, err := Dialog{
			AssignTo:      &dlg,
			Title:         "Your name",
			DefaultButton: &ok,
			CancelButton:  &cancel,
			MinSize:       Size{Width: 350, Height: 180},
			Layout:        VBox{},
			Children: []Widget{
				Label{Text: "This dialog asks for your name. A screen reader should say \"dialog\" and read this text when it opens."},
				Composite{
					Layout: HBox{MarginsZero: true},
					Children: []Widget{
						Label{Text: "Name:"},
						LineEdit{AssignTo: &name, Accessibility: Accessibility{Name: "Name"}},
					},
				},
				Composite{
					Layout: HBox{MarginsZero: true},
					Children: []Widget{
						HSpacer{},
						PushButton{AssignTo: &ok, Text: "OK", OnClicked: func() { dlg.Accept() }},
						PushButton{AssignTo: &cancel, Text: "Cancel", OnClicked: func() { dlg.Cancel() }},
					},
				},
			},
		}.Run(mw)
		if err != nil {
			report("Dialog failed: %v", err)
			return
		}
		if cmd == walk.DlgCmdOK {
			report("Dialog: OK, name %q", name.Text())
		} else {
			report("Dialog: closed with result %d", cmd)
		}
	}

	showTask := func(td walk.TaskDialog) {
		r, err := td.Show(mw)
		if err != nil {
			report("Task dialog failed: %v", err)
			return
		}
		report("Task dialog: button %d, radio button %d, check box checked %v", r.Button, r.RadioButton, r.VerificationChecked)
	}

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "walk dialogs manual test",
		Size:     Size{Width: 450, Height: 350},
		Layout:   VBox{},
		Children: []Widget{
			PushButton{Text: "Open a walk dialog", OnClicked: openDialog},
			PushButton{Text: "Open a task dialog", OnClicked: func() {
				showTask(walk.TaskDialog{
					Title:           "Report",
					MainInstruction: "Save the changes to the report?",
					Content:         "The report has unsaved changes. If you discard them, they are lost.",
					Icon:            walk.TaskDialogIconWarning,
					CommonButtons:   walk.TaskDialogCancel,
					Buttons: []walk.TaskDialogButton{
						{ID: 101, Text: "&Save"},
						{ID: 102, Text: "&Discard"},
					},
					RadioButtons: []walk.TaskDialogButton{
						{ID: 201, Text: "Close the report"},
						{ID: 202, Text: "Keep the report open"},
					},
					VerificationText:    "Do not ask again",
					ExpandedInformation: "The changes are kept in memory only until the program ends.",
					Footer:              `More in <a href="https://github.com/lpintes/walk">the walk repository</a>.`,
					OnHyperlinkClicked: func(href string) {
						report("Link: %s", href)
					},
				})
			}},
			PushButton{Text: "Open a task dialog with command links", OnClicked: func() {
				showTask(walk.TaskDialog{
					Title:           "Connection",
					MainInstruction: "How do you want to connect?",
					Icon:            walk.TaskDialogIconInformation,
					CommandLinks:    true,
					Buttons: []walk.TaskDialogButton{
						{ID: 101, Text: "Wired network\nUse the cable plugged into the computer."},
						{ID: 102, Text: "Wireless network\nChoose a network in the next step."},
					},
					AllowCancel: true,
				})
			}},
			PushButton{Text: "Open a message box", OnClicked: func() {
				report("Message box: result %d", walk.MsgBox(mw, "Message box", "A system message box, for comparison.", walk.MsgBoxOKCancel|walk.MsgBoxIconInformation))
			}},
			Label{AssignTo: &result, Text: "Results appear here.", Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite}},
		},
	}).Create(); err != nil {
		fmt.Println("create window:", err)
		os.Exit(1)
	}
	mw.Run()
}
