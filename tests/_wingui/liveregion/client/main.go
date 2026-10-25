// Command client is the UI Automation side of the live region test (step
// 10). It plays the part of a screen reader: it reads the LiveSetting
// property of the windows of ../app, subscribes to the live region changed
// and notification events of its main window, signals the named event
// Local\walk-liveregion-run so that the app changes its texts and calls
// walk.Announce, and checks the events it gets within five seconds.
//
// It uses the UI Automation client package uia (since step 13 part 4;
// before, it had COM code of its own).
//
// Usage: client MAIN STATUS PLAIN EDIT STATUSBAR, with the window handles
// the app prints on its HANDLES line.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk/uia"
)

var failed bool

func check(name string, ok bool, format string, args ...any) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failed = true
	}
	fmt.Printf("%s %s: %s\n", status, name, fmt.Sprintf(format, args...))
}

type event struct {
	kind     string // "live" or "notification"
	hwnd     uintptr
	name     string
	display  string
	activity string
}

var events = make(chan event, 100)

// elementInfo returns the window handle and the name of an event sender.
// The subscriptions call it on their goroutines, after the event.
func elementInfo(e *uia.Element) (uintptr, string) {
	hwnd, _ := e.NativeWindowHandle()
	name, _ := e.Name()
	e.Release()
	return hwnd, name
}

func main() {
	if len(os.Args) != 6 {
		fmt.Println("usage: client MAIN STATUS PLAIN EDIT STATUSBAR")
		os.Exit(2)
	}
	var hwnds [5]uintptr
	for i := range hwnds {
		n, err := strconv.ParseUint(os.Args[i+1], 10, 64)
		if err != nil {
			fmt.Println("bad handle:", os.Args[i+1])
			os.Exit(2)
		}
		hwnds[i] = uintptr(n)
	}
	mainHwnd, statusHwnd, plainHwnd, editHwnd, barHwnd := hwnds[0], hwnds[1], hwnds[2], hwnds[3], hwnds[4]

	a, err := uia.New()
	if err != nil {
		fmt.Println("FAIL uia.New:", err)
		os.Exit(1)
	}
	defer a.Close()

	elementFromHandle := func(hwnd uintptr) *uia.Element {
		element, err := a.ElementFromHandle(hwnd)
		if err != nil {
			fmt.Printf("FAIL ElementFromHandle %d: %v\n", hwnd, err)
			os.Exit(1)
		}
		return element
	}

	for _, w := range []struct {
		name string
		hwnd uintptr
		want int
	}{
		{"Label", statusHwnd, 1},
		{"plain Label", plainHwnd, 0},
		{"LineEdit", editHwnd, 2},
		{"StatusBar", barHwnd, 1},
	} {
		element := elementFromHandle(w.hwnd)
		got, err := element.PropertyValue(uia.PropertyLiveSetting)
		check("UIA LiveSetting "+w.name, err == nil && got == w.want,
			"value %v (%T), %v, want %d", got, got, err, w.want)
		element.Release()
	}

	root := elementFromHandle(mainHwnd)
	defer root.Release()

	liveSub, err := root.OnEvent(uia.EventLiveRegionChanged, uia.ScopeSubtree, func(sender *uia.Element, id uia.EventID) {
		hwnd, name := elementInfo(sender)
		events <- event{kind: "live", hwnd: hwnd, name: name}
	})
	check("OnEvent LiveRegionChanged", err == nil, "%v", err)
	notificationSub, err := root.OnNotification(uia.ScopeSubtree, func(sender *uia.Element, n uia.Notification) {
		hwnd, name := elementInfo(sender)
		events <- event{
			kind:     "notification",
			hwnd:     hwnd,
			name:     name,
			display:  n.Text,
			activity: n.ActivityID,
		}
	})
	check("OnNotification", err == nil, "%v", err)

	name, _ := windows.UTF16PtrFromString(`Local\walk-liveregion-run`)
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		fmt.Println("FAIL OpenEvent:", err)
		os.Exit(1)
	}
	windows.SetEvent(ev)
	windows.CloseHandle(ev)

	live := map[uintptr]int{}
	var announced, editAnnounced bool
	timeout := time.After(5 * time.Second)
collect:
	for {
		select {
		case e := <-events:
			fmt.Printf("EVENT %s hwnd %d name %q display %q activity %q\n", e.kind, e.hwnd, e.name, e.display, e.activity)
			switch {
			case e.kind == "live" && e.hwnd == 0 && e.name == "Bar 1":
				// The StatusBar raises the event for the part, which has no window
				// of its own; its name is the new text.
				live[barHwnd]++
			case e.kind == "live":
				live[e.hwnd]++
			case e.display == "Announcement 1" && strings.HasPrefix(e.activity, "Walk.Announce."):
				announced = true
			case e.kind == "notification" && e.hwnd == editHwnd && e.display == "Error 1":
				// A LineEdit live region announces its new text instead of
				// raising the live region changed event.
				editAnnounced = true
			}
		case <-timeout:
			break collect
		}
	}

	if liveSub != nil {
		liveSub.Close()
	}
	if notificationSub != nil {
		notificationSub.Close()
	}

	for _, w := range []struct {
		name string
		hwnd uintptr
		want bool
	}{
		{"Label", statusHwnd, true},
		{"plain Label", plainHwnd, false},
		{"LineEdit", editHwnd, false},
		{"StatusBar", barHwnd, true},
	} {
		check("UIA live region event "+w.name, (live[w.hwnd] > 0) == w.want, "%d events, want them: %v", live[w.hwnd], w.want)
	}
	check("UIA notification event LineEdit", editAnnounced, "Error 1 from the LineEdit received: %v", editAnnounced)
	check("UIA notification event", announced, "Announcement 1 with activity Walk.Announce.<n> received: %v", announced)

	if failed {
		a.Close()
		os.Exit(1)
	}
}
