// Command app is the walk side of the live region test (step 10): a
// MainWindow with live regions (a Label, a read-only LineEdit and the
// StatusBar), a Label that is not a live region, and buttons that change
// them or call walk.Announce, for a manual test with a screen reader.
//
// It prints "HANDLES <main> <status> <plain> <edit> <statusbar>" and
// "READY", then waits for ../client to signal the named event
// Local\walk-liveregion-run (at most 20 seconds) and runs the automatic
// checks: it changes the texts, calls Announce, and checks with an
// in-process WinEvent hook that EVENT_OBJECT_LIVEREGIONCHANGED came for
// the live regions and not for the plain Label. It prints "CHECKS DONE"
// afterwards; the window stays open until it gets WM_CLOSE.
//
// With the argument "manual" it skips the automatic checks.
package main

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
	"golang.org/x/sys/windows"
)

var (
	mw     *walk.MainWindow
	status *walk.Label
	plain  *walk.Label
	edit   *walk.LineEdit
	sbItem *walk.StatusBarItem
	failed bool
	count  int

	// liveEvents counts EVENT_OBJECT_LIVEREGIONCHANGED per window. Only the
	// UI thread touches it: the hook is out of context, so its callback
	// runs from the message loop.
	liveEvents = map[win.HWND]int{}
)

func check(name string, ok bool, format string, args ...any) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failed = true
	}
	fmt.Printf("%s %s: %s\n", status, name, fmt.Sprintf(format, args...))
}

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	setWinEventHook = user32.NewProc("SetWinEventHook")
)

const winEventOutOfContext = 0

func winEventProc(hook uintptr, event uint32, hwnd win.HWND, idObject, idChild int32, thread, time uint32) uintptr {
	if event == win.EVENT_OBJECT_LIVEREGIONCHANGED && idObject == win.OBJID_CLIENT {
		liveEvents[hwnd]++
		fmt.Printf("WINEVENT live region changed hwnd %d\n", hwnd)
	}
	return 0
}

// report prints err and shows it in the plain Label, so that a screen reader user
// finds it there too.
func report(what string, err error) {
	if err != nil {
		fmt.Printf("ERROR %s: %v\n", what, err)
		plain.SetText(fmt.Sprintf("%s failed: %v", what, err))
	}
}

func announce(text string, processing walk.AnnouncementProcessing) {
	report("Announce", walk.Announce(mw, text, processing))
}

// announceHost raises the notification event with the host provider of
// the main window instead of walk's own provider, to compare the two.
func announceHost(text string) {
	if !win.UiaRaiseNotificationEventAvailable() {
		report("Host provider", fmt.Errorf("UiaRaiseNotificationEvent not available"))
		return
	}
	var p *win.IRawElementProviderSimple
	if hr := win.UiaHostProviderFromHwnd(mw.Handle(), &p); win.FAILED(hr) {
		report("UiaHostProviderFromHwnd", fmt.Errorf("hr=0x%x", uint32(hr)))
		return
	}
	defer p.Release()
	display := win.SysAllocString(text)
	defer win.SysFreeString(display)
	activity := win.SysAllocString("Walk.Announce.Host")
	defer win.SysFreeString(activity)
	hr := win.UiaRaiseNotificationEvent(p, win.NotificationKind_Other, win.NotificationProcessing_ImportantAll, display, activity)
	fmt.Printf("host provider: UiaClientsAreListening %v, UiaRaiseNotificationEvent hr=0x%x\n", win.UiaClientsAreListening(), uint32(hr))
	if win.FAILED(hr) {
		report("Host provider", fmt.Errorf("UiaRaiseNotificationEvent hr=0x%x", uint32(hr)))
	}
}

func main() {
	manual := len(os.Args) > 1 && os.Args[1] == "manual"

	err := MainWindow{
		AssignTo: &mw,
		Title:    "walk live region test",
		Size:     Size{Width: 500, Height: 400},
		Layout:   VBox{},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &sbItem, Text: "Ready", Width: 300},
		},
		Children: []Widget{
			Label{
				AssignTo:      &status,
				Text:          "Status: idle",
				Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite},
			},
			Label{
				AssignTo: &plain,
				Text:     "Plain label, not a live region",
			},
			LineEdit{
				AssignTo:      &edit,
				ReadOnly:      true,
				Text:          "No error",
				Accessibility: Accessibility{Name: "Error", LiveSetting: AccLiveSettingAssertive},
			},
			PushButton{
				Text: "Change status (polite label)",
				OnClicked: func() {
					count++
					report("Label.SetText", status.SetText(fmt.Sprintf("Status %d: %d files copied", count, count*7)))
				},
			},
			PushButton{
				Text: "Show error (assertive line edit)",
				OnClicked: func() {
					count++
					report("LineEdit.SetText", edit.SetText(fmt.Sprintf("Error %d: the name is too long", count)))
				},
			},
			PushButton{
				Text: "Change status bar (polite)",
				OnClicked: func() {
					count++
					report("StatusBarItem.SetText", sbItem.SetText(fmt.Sprintf("Status bar %d", count)))
				},
			},
			PushButton{
				Text: "Change plain label (not announced)",
				OnClicked: func() {
					count++
					report("Label.SetText", plain.SetText(fmt.Sprintf("Plain label %d", count)))
				},
			},
			PushButton{
				Text: "Announce",
				OnClicked: func() {
					count++
					announce(fmt.Sprintf("Announcement %d: download finished", count), walk.AnnounceImportantAll)
				},
			},
			PushButton{
				Text: "Announce three times, most recent only",
				OnClicked: func() {
					for i := 1; i <= 3; i++ {
						announce(fmt.Sprintf("Most recent %d of 3", i), walk.AnnounceImportantMostRecent)
					}
				},
			},
			PushButton{
				Text: "Announce three times, all",
				OnClicked: func() {
					for i := 1; i <= 3; i++ {
						announce(fmt.Sprintf("All %d of 3", i), walk.AnnounceAll)
					}
				},
			},
			PushButton{
				Text: "Announce through the host provider",
				OnClicked: func() {
					count++
					announceHost(fmt.Sprintf("Host announcement %d", count))
				},
			},
		},
	}.Create()
	if err != nil {
		fmt.Println("FAIL create:", err)
		os.Exit(1)
	}

	// The declarative builder ignores errors of SetLiveSetting; the setting
	// is only stored when SetHwndProp succeeded.
	check("SetLiveSetting Label", status.Accessibility().LiveSetting() == walk.AccLiveSettingPolite, "LiveSetting %d, want 1", status.Accessibility().LiveSetting())
	check("SetLiveSetting LineEdit", edit.Accessibility().LiveSetting() == walk.AccLiveSettingAssertive, "LiveSetting %d, want 2", edit.Accessibility().LiveSetting())
	err = mw.StatusBar().Accessibility().SetLiveSetting(walk.AccLiveSettingPolite)
	check("SetLiveSetting StatusBar", err == nil, "%v", err)

	if r, _, _ := setWinEventHook.Call(
		win.EVENT_OBJECT_LIVEREGIONCHANGED, win.EVENT_OBJECT_LIVEREGIONCHANGED,
		0, syscall.NewCallback(winEventProc),
		uintptr(os.Getpid()), 0, winEventOutOfContext); r == 0 {
		check("SetWinEventHook", false, "failed")
	}

	fmt.Printf("HANDLES %d %d %d %d %d\n", mw.Handle(), status.Handle(), plain.Handle(), edit.Handle(), mw.StatusBar().Handle())
	fmt.Println("READY")

	if !manual {
		go waitAndRun()
	}

	mw.Run()
	if failed {
		os.Exit(1)
	}
}

func waitAndRun() {
	name, _ := windows.UTF16PtrFromString(`Local\walk-liveregion-run`)
	ev, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		mw.Synchronize(func() { check("CreateEvent", false, "%v", err) })
	} else {
		r, _ := windows.WaitForSingleObject(ev, 20000)
		if r != windows.WAIT_OBJECT_0 {
			fmt.Println("no client signaled, running the checks anyway")
		}
	}
	mw.Synchronize(runChecks)
}

func runChecks() {
	err1 := status.SetText("Status 1")
	err2 := plain.SetText("Plain 1")
	err3 := edit.SetText("Error 1")
	err4 := sbItem.SetText("Bar 1")
	check("SetText", err1 == nil && err2 == nil && err3 == nil && err4 == nil, "Label %v, plain Label %v, LineEdit %v, StatusBarItem %v", err1, err2, err3, err4)
	fmt.Printf("UiaClientsAreListening %v\n", win.UiaClientsAreListening())
	err := walk.Announce(mw, "Announcement 1", walk.AnnounceImportantAll)
	check("Announce", err == nil, "%v", err)
	// Live region notifying does not need a listener, so the hook sees the
	// events in any case.
	go func() {
		time.Sleep(time.Second)
		mw.Synchronize(func() {
			for _, w := range []struct {
				name string
				hwnd win.HWND
				want bool
			}{
				{"Label", status.Handle(), true},
				{"plain Label", plain.Handle(), false},
				{"LineEdit", edit.Handle(), true},
				{"StatusBar", mw.StatusBar().Handle(), true},
			} {
				n := liveEvents[w.hwnd]
				check("live region event "+w.name, (n > 0) == w.want, "%d events, want them: %v", n, w.want)
			}
			fmt.Println("CHECKS DONE")
		})
	}()
}
