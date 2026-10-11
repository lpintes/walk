// Command client is the UI Automation side of the live region test (step
// 10). It plays the part of a screen reader: it reads the LiveSetting
// property of the windows of ../app, subscribes to the live region changed
// and notification events of its main window, signals the named event
// Local\walk-liveregion-run so that the app changes its texts and calls
// walk.Announce, and checks the events it gets within five seconds.
//
// Usage: client MAIN STATUS PLAIN EDIT STATUSBAR, with the window handles
// the app prints on its HANDLES line.
package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
	"golang.org/x/sys/windows"
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

// UI Automation declarations; internal/win declares only what walk uses.
// The vtable indexes include the IUnknown methods.
var (
	clsidCUIAutomation8                      = win.CLSID{Data1: 0xe22ad333, Data2: 0xb25f, Data3: 0x460c, Data4: [8]byte{0x83, 0xd0, 0x05, 0x81, 0x10, 0x73, 0x95, 0xc9}}
	iidIUIAutomation5                        = win.IID{Data1: 0x25f700c8, Data2: 0xd816, Data3: 0x4057, Data4: [8]byte{0xa9, 0xdc, 0x3c, 0xbd, 0xee, 0x77, 0xe2, 0x56}}
	iidIUIAutomationEventHandler             = win.IID{Data1: 0x146c3c17, Data2: 0xf12e, Data3: 0x4e22, Data4: [8]byte{0x8c, 0x27, 0xf8, 0x94, 0xb9, 0xb7, 0x9c, 0x69}}
	iidIUIAutomationNotificationEventHandler = win.IID{Data1: 0xc7cb2637, Data2: 0xe6c2, Data3: 0x4d0c, Data4: [8]byte{0x85, 0xde, 0x49, 0x48, 0xc0, 0x21, 0x75, 0xc7}}
)

const (
	// IUIAutomation (IUIAutomation5 for AddNotificationEventHandler)
	idxElementFromHandle           = 6
	idxAddAutomationEventHandler   = 32
	idxRemoveAllEventHandlers      = 41
	idxAddNotificationEventHandler = 68

	// IUIAutomationElement
	idxGetCurrentPropertyValue      = 10
	idxGetCurrentName               = 23
	idxGetCurrentNativeWindowHandle = 36

	uiaLiveRegionChangedEventId = 20024
	uiaLiveSettingPropertyId    = 30135
	treeScopeSubtree            = 7
)

type eventHandlerVtbl struct {
	QueryInterface        uintptr
	AddRef                uintptr
	Release               uintptr
	HandleAutomationEvent uintptr
}

type notificationEventHandlerVtbl struct {
	QueryInterface          uintptr
	AddRef                  uintptr
	Release                 uintptr
	HandleNotificationEvent uintptr
}

// call calls method index of the COM interface obj.
func call(obj unsafe.Pointer, index int, args ...uintptr) win.HRESULT {
	vtbl := *(*unsafe.Pointer)(obj)
	method := *(*uintptr)(unsafe.Add(vtbl, index*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(obj)}, args...)...)
	return win.HRESULT(r)
}

func release(obj unsafe.Pointer) {
	call(obj, 2)
}

type event struct {
	kind     string // "live" or "notification"
	hwnd     win.HWND
	name     string
	display  string
	activity string
}

var events = make(chan event, 100)

func elementInfo(element unsafe.Pointer) (win.HWND, string) {
	var hwnd win.HWND
	call(element, idxGetCurrentNativeWindowHandle, uintptr(unsafe.Pointer(&hwnd)))
	var name *uint16
	call(element, idxGetCurrentName, uintptr(unsafe.Pointer(&name)))
	defer win.SysFreeString(name)
	return hwnd, windows.UTF16PtrToString(name)
}

func handleAutomationEvent(this *com.This, sender unsafe.Pointer, eventId int32) uintptr {
	if eventId == uiaLiveRegionChangedEventId {
		hwnd, name := elementInfo(sender)
		events <- event{kind: "live", hwnd: hwnd, name: name}
	}
	return win.S_OK
}

func handleNotificationEvent(this *com.This, sender unsafe.Pointer, kind, processing int32, display, activity *uint16) uintptr {
	hwnd, name := elementInfo(sender)
	events <- event{
		kind:     "notification",
		hwnd:     hwnd,
		name:     name,
		display:  windows.UTF16PtrToString(display),
		activity: windows.UTF16PtrToString(activity),
	}
	return win.S_OK
}

func main() {
	if len(os.Args) != 6 {
		fmt.Println("usage: client MAIN STATUS PLAIN EDIT STATUSBAR")
		os.Exit(2)
	}
	var hwnds [5]win.HWND
	for i := range hwnds {
		n, err := strconv.ParseUint(os.Args[i+1], 10, 64)
		if err != nil {
			fmt.Println("bad handle:", os.Args[i+1])
			os.Exit(2)
		}
		hwnds[i] = win.HWND(n)
	}
	mainHwnd, statusHwnd, plainHwnd, editHwnd, barHwnd := hwnds[0], hwnds[1], hwnds[2], hwnds[3], hwnds[4]

	runtime.LockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		fmt.Println("FAIL CoInitializeEx:", err)
		os.Exit(1)
	}
	defer windows.CoUninitialize()

	var uia unsafe.Pointer
	if hr := win.CoCreateInstance(&clsidCUIAutomation8, nil, win.CLSCTX_INPROC_SERVER, &iidIUIAutomation5, &uia); win.FAILED(hr) {
		fmt.Printf("FAIL CoCreateInstance CUIAutomation8: hr=0x%x\n", uint32(hr))
		os.Exit(1)
	}
	defer release(uia)

	elementFromHandle := func(hwnd win.HWND) unsafe.Pointer {
		var element unsafe.Pointer
		if hr := call(uia, idxElementFromHandle, uintptr(hwnd), uintptr(unsafe.Pointer(&element))); win.FAILED(hr) {
			fmt.Printf("FAIL ElementFromHandle %d: hr=0x%x\n", hwnd, uint32(hr))
			os.Exit(1)
		}
		return element
	}

	for _, w := range []struct {
		name string
		hwnd win.HWND
		want int32
	}{
		{"Label", statusHwnd, 1},
		{"plain Label", plainHwnd, 0},
		{"LineEdit", editHwnd, 2},
		{"StatusBar", barHwnd, 1},
	} {
		element := elementFromHandle(w.hwnd)
		var v win.VARIANT
		hr := call(element, idxGetCurrentPropertyValue, uiaLiveSettingPropertyId, uintptr(unsafe.Pointer(&v)))
		got, err := v.Long()
		check("UIA LiveSetting "+w.name, !win.FAILED(hr) && err == nil && got == w.want,
			"hr=0x%x vt=%d value %d (%v), want %d", uint32(hr), v.Vt, got, err, w.want)
		release(element)
	}

	root := elementFromHandle(mainHwnd)
	defer release(root)

	liveHandler := com.NewObject(nil, com.NewVTable(&eventHandlerVtbl{
		HandleAutomationEvent: syscall.NewCallback(handleAutomationEvent),
	}, &iidIUIAutomationEventHandler))
	defer liveHandler.Release()
	notificationHandler := com.NewObject(nil, com.NewVTable(&notificationEventHandlerVtbl{
		HandleNotificationEvent: syscall.NewCallback(handleNotificationEvent),
	}, &iidIUIAutomationNotificationEventHandler))
	defer notificationHandler.Release()

	hr := call(uia, idxAddAutomationEventHandler, uiaLiveRegionChangedEventId, uintptr(root), treeScopeSubtree, 0, uintptr(liveHandler.Interface(0)))
	check("AddAutomationEventHandler", !win.FAILED(hr), "hr=0x%x", uint32(hr))
	hr = call(uia, idxAddNotificationEventHandler, uintptr(root), treeScopeSubtree, 0, uintptr(notificationHandler.Interface(0)))
	check("AddNotificationEventHandler", !win.FAILED(hr), "hr=0x%x", uint32(hr))

	name, _ := windows.UTF16PtrFromString(`Local\walk-liveregion-run`)
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		fmt.Println("FAIL OpenEvent:", err)
		os.Exit(1)
	}
	windows.SetEvent(ev)
	windows.CloseHandle(ev)

	live := map[win.HWND]int{}
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

	call(uia, idxRemoveAllEventHandlers)

	for _, w := range []struct {
		name string
		hwnd win.HWND
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
		os.Exit(1)
	}
}
