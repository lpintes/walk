// Command system checks on Windows the power, session and settings events
// and the system settings of step 14 part 2 (walk.System, KeepAwake and
// FormBase.SetShutdownBlockReason). It never changes a setting of the user
// and sends messages only to its own windows.
//
//   - The service: System returns an error on a goroutine whose thread
//     has no walk window. After the main window is created it returns the
//     same object on the same thread; its
//     hidden window has the class \o/ Walk_System_Class \o/, belongs to
//     the thread, is not visible and is a real top-level window (EnumWindows
//     finds it, its parent is the desktop), so it gets broadcasts.
//   - Queries: HighContrast, ScreenReaderRunning, AppsUseDarkTheme (must
//     match the registry), TextScaleFactor, ClientAreaAnimation, Battery,
//     PowerSource, EnergySaver and IsRemoteSession return sane values
//     (printed as INFO). After the messages are pumped for a moment,
//     DisplayState must be known: Windows sends the current value right
//     after RegisterPowerSettingNotification, so this checks the
//     registration with a real system message (only INFO in a Remote
//     Desktop session or Windows Sandbox, where it may not come).
//   - Events, by sending the messages Windows would send to the hidden
//     window: WM_POWERBROADCAST (suspend, both resumes, power status, and
//     PBT_POWERSETTINGCHANGE for the power source, energy saver and display
//     state: first the current value, which publishes nothing, then another
//     value and the current one again, which publish once each; a setting
//     with no data publishes nothing), WM_QUERYENDSESSION (with and without
//     a canceling handler, checking the return value and the reason; with
//     a shutdown block reason on the visible main window, the form must
//     refuse and the hidden window agree, with one SessionEnding for
//     both), WM_ENDSESSION with wParam FALSE, WM_WTSSESSION_CHANGE (lock, unlock,
//     remote connect), WM_SETTINGCHANGE with an area string and with an SPI
//     code, WM_THEMECHANGED and WM_SYSCOLORCHANGE. The derived events
//     HighContrastChanged, DarkModeChanged and ScreenReaderChanged must not
//     be published, because nothing really changed.
//   - KeepAwake with and without the display: no error, release twice.
//   - SetShutdownBlockReason on the main window, read back with
//     ShutdownBlockReason, and removed.
//   - Dispose destroys the hidden window; System then creates a new service
//     with a new window, and the handlers of the old one are not called.
//
// It prints PASS, FAIL and INFO lines and exits with code 1 if a check
// failed.
//
// With the argument "manual" it shows a window with an event log, for a
// test by hand with a screen reader: lock and unlock the computer
// (Windows+L), unplug and plug in the power, turn high contrast on and off
// (Left Alt+Left Shift+Print Screen), switch dark mode, start or stop the
// screen reader. Each event is announced in a polite live region, added to
// the log and printed to standard output.
package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
)

const systemClass = `\o/ Walk_System_Class \o/`

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	procPeekMessage          = user32.NewProc("PeekMessageW")
	procEnumWindows          = user32.NewProc("EnumWindows")
	procEnumThreadWindows    = user32.NewProc("EnumThreadWindows")
	procGetClassName         = user32.NewProc("GetClassNameW")
	procGetDesktopWindow     = user32.NewProc("GetDesktopWindow")
	procGetWindowThreadProcs = user32.NewProc("GetWindowThreadProcessId")
)

var failed bool

func check(name string, ok bool, format string, args ...any) bool {
	if ok {
		fmt.Println("PASS", name)
		return true
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
	return false
}

// pump dispatches the messages of this thread for d.
func pump(d time.Duration) {
	var msg win.MSG
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for {
			r, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1) // PM_REMOVE
			if r == 0 {
				break
			}
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func className(hwnd win.HWND) string {
	var buf [256]uint16
	n, _, _ := procGetClassName.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

// threadWindows returns the top-level windows of this thread with the
// class of the system service.
func threadWindows() []win.HWND {
	var found []win.HWND
	cb := syscall.NewCallback(func(hwnd win.HWND, _ uintptr) uintptr {
		if className(hwnd) == systemClass {
			found = append(found, hwnd)
		}
		return 1
	})
	procEnumThreadWindows.Call(uintptr(win.GetCurrentThreadId()), cb, 0)
	return found
}

// enumWindowsHas returns whether EnumWindows, which enumerates the
// top-level windows that get broadcasts (not message-only windows), finds
// hwnd.
func enumWindowsHas(hwnd win.HWND) bool {
	var found bool
	cb := syscall.NewCallback(func(h win.HWND, _ uintptr) uintptr {
		if h == hwnd {
			found = true
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// counter records how often each event was published and with what.
type counter struct {
	n    map[string]int
	last map[string]string
}

func newCounter() *counter {
	return &counter{n: map[string]int{}, last: map[string]string{}}
}

func (c *counter) hit(name, args string) {
	c.n[name]++
	c.last[name] = args
}

func (c *counter) reset() {
	clear(c.n)
	clear(c.last)
}

// only reports whether exactly the given events were published, once each.
func (c *counter) only(names ...string) bool {
	if len(c.n) != len(names) {
		return false
	}
	for _, n := range names {
		if c.n[n] != 1 {
			return false
		}
	}
	return true
}

func (c *counter) String() string {
	var parts []string
	for k, v := range c.n {
		parts = append(parts, fmt.Sprintf("%s=%d(%s)", k, v, c.last[k]))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func attach(s *walk.SystemService, c *counter, cancel *bool) {
	plain := map[string]*walk.Event{
		"Suspending":          s.Suspending(),
		"Resumed":             s.Resumed(),
		"ResumedByUser":       s.ResumedByUser(),
		"PowerStatusChanged":  s.PowerStatusChanged(),
		"PowerSourceChanged":  s.PowerSourceChanged(),
		"EnergySaverChanged":  s.EnergySaverChanged(),
		"DisplayStateChanged": s.DisplayStateChanged(),
		"SessionLocked":       s.SessionLocked(),
		"SessionUnlocked":     s.SessionUnlocked(),
		"HighContrastChanged": s.HighContrastChanged(),
		"DarkModeChanged":     s.DarkModeChanged(),
		"ScreenReaderChanged": s.ScreenReaderChanged(),
	}
	for name, e := range plain {
		e.Attach(func() { c.hit(name, "") })
	}
	s.SessionEnding().Attach(func(canceled *bool, reason walk.EndSessionReason) {
		c.hit("SessionEnding", fmt.Sprintf("%#x", uint32(reason)))
		if *cancel {
			*canceled = true
		}
	})
	s.SessionEnded().Attach(func(ending bool, reason walk.EndSessionReason) {
		c.hit("SessionEnded", fmt.Sprintf("%v %#x", ending, uint32(reason)))
	})
	s.SessionChanged().Attach(func(change walk.SessionChange, id uint32) {
		c.hit("SessionChanged", fmt.Sprintf("%d %d", change, id))
	})
	s.SettingsChanged().Attach(func(change walk.SettingsChange) {
		c.hit("SettingsChanged", fmt.Sprintf("%d %#x %s", change.Kind, change.Action, change.Area))
	})
}

// powerSetting builds a POWERBROADCAST_SETTING with a DWORD value.
func powerSetting(guid syscall.GUID, value uint32, length uint32) []byte {
	buf := make([]byte, 24)
	*(*syscall.GUID)(unsafe.Pointer(&buf[0])) = guid
	*(*uint32)(unsafe.Pointer(&buf[16])) = length
	*(*uint32)(unsafe.Pointer(&buf[20])) = value
	return buf
}

func sendPowerSetting(hwnd win.HWND, guid syscall.GUID, value, length uint32) uintptr {
	buf := powerSetting(guid, value, length)
	r := win.SendMessage(hwnd, win.WM_POWERBROADCAST, win.PBT_POWERSETTINGCHANGE, uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	return r
}

func testQueries(s *walk.SystemService) {
	hc, scheme, err := walk.HighContrast()
	fmt.Printf("INFO HighContrast on=%v scheme=%q err=%v\n", hc, scheme, err)
	check("HighContrast", err == nil, "%v", err)

	fmt.Printf("INFO ScreenReaderRunning=%v\n", walk.ScreenReaderRunning())
	fmt.Printf("INFO ClientAreaAnimation=%v\n", walk.ClientAreaAnimation())
	fmt.Printf("INFO IsRemoteSession=%v\n", walk.IsRemoteSession())

	dark := walk.AppsUseDarkTheme()
	want := false
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetIntegerValue("AppsUseLightTheme"); err == nil {
			want = v == 0
		}
		k.Close()
	}
	fmt.Printf("INFO AppsUseDarkTheme=%v\n", dark)
	check("AppsUseDarkTheme matches the registry", dark == want, "got %v, registry says %v", dark, want)

	tsf := walk.TextScaleFactor()
	fmt.Printf("INFO TextScaleFactor=%d\n", tsf)
	check("TextScaleFactor range", tsf >= 100 && tsf <= 225, "%d", tsf)

	bs, err := walk.Battery()
	fmt.Printf("INFO Battery %+v err=%v\n", bs, err)
	check("Battery", err == nil && bs.Percent >= -1 && bs.Percent <= 100, "%+v %v", bs, err)

	fmt.Printf("INFO PowerSource=%d EnergySaver=%d DisplayState=%d\n", s.PowerSource(), s.EnergySaver(), s.DisplayState())
}

func sourceValue(p walk.PowerSource) uint32 {
	switch p {
	case walk.PowerSourceBattery:
		return 1
	case walk.PowerSourceUPS:
		return 2
	}
	return 0
}

func testEvents(s *walk.SystemService, hwnd win.HWND, mw *walk.MainWindow) {
	c := newCounter()
	var cancel bool
	attach(s, c, &cancel)

	send := func(name string, msg uint32, wp, lp uintptr, want uintptr, events ...string) {
		c.reset()
		r := win.SendMessage(hwnd, msg, wp, lp)
		check(name, r == want && c.only(events...), "result %d (want %d), events %v (want %v)", r, want, c, events)
	}

	send("Suspending", win.WM_POWERBROADCAST, win.PBT_APMSUSPEND, 0, 1, "Suspending")
	send("Resumed", win.WM_POWERBROADCAST, win.PBT_APMRESUMEAUTOMATIC, 0, 1, "Resumed")
	send("ResumedByUser", win.WM_POWERBROADCAST, win.PBT_APMRESUMESUSPEND, 0, 1, "ResumedByUser")
	send("PowerStatusChanged", win.WM_POWERBROADCAST, win.PBT_APMPOWERSTATUSCHANGE, 0, 1, "PowerStatusChanged")

	// Power source.
	p0 := s.PowerSource()
	v0 := sourceValue(p0)
	v1 := uint32(1)
	if v0 == 1 {
		v1 = 0
	}
	c.reset()
	sendPowerSetting(hwnd, win.GUID_ACDC_POWER_SOURCE, v0, 4)
	check("power source: the current value publishes nothing", len(c.n) == 0, "events %v", c)
	p0 = s.PowerSource()
	c.reset()
	sendPowerSetting(hwnd, win.GUID_ACDC_POWER_SOURCE, v1, 4)
	p1 := s.PowerSource()
	check("PowerSourceChanged", c.only("PowerSourceChanged") && p1 != p0, "events %v, source %d -> %d", c, p0, p1)
	c.reset()
	sendPowerSetting(hwnd, win.GUID_ACDC_POWER_SOURCE, v0, 4)
	check("PowerSourceChanged back", c.only("PowerSourceChanged") && s.PowerSource() == p0, "events %v, source %d", c, s.PowerSource())

	// Energy saver, through the Windows 11 setting.
	e0 := s.EnergySaver()
	e1 := walk.EnergySaverStandard
	if e0 != walk.EnergySaverOff {
		e1 = walk.EnergySaverOff
	}
	c.reset()
	sendPowerSetting(hwnd, win.GUID_ENERGY_SAVER_STATUS, uint32(e0), 4)
	check("energy saver: the current value publishes nothing", len(c.n) == 0, "events %v", c)
	c.reset()
	sendPowerSetting(hwnd, win.GUID_ENERGY_SAVER_STATUS, uint32(e1), 4)
	check("EnergySaverChanged", c.only("EnergySaverChanged") && s.EnergySaver() == e1, "events %v, status %d", c, s.EnergySaver())
	c.reset()
	sendPowerSetting(hwnd, win.GUID_ENERGY_SAVER_STATUS, uint32(e0), 4)
	check("EnergySaverChanged back", c.only("EnergySaverChanged") && s.EnergySaver() == e0, "events %v, status %d", c, s.EnergySaver())

	// Display state: 0 off, 1 on, 2 dimmed.
	d0 := s.DisplayState()
	dv0 := uint32(1)
	switch d0 {
	case walk.DisplayOff:
		dv0 = 0
	case walk.DisplayDimmed:
		dv0 = 2
	}
	c.reset()
	sendPowerSetting(hwnd, win.GUID_CONSOLE_DISPLAY_STATE, dv0, 4)
	check("display state: the current value publishes nothing", len(c.n) == 0, "events %v", c)
	d0 = s.DisplayState()
	c.reset()
	sendPowerSetting(hwnd, win.GUID_CONSOLE_DISPLAY_STATE, 2, 4)
	if dv0 == 2 {
		sendPowerSetting(hwnd, win.GUID_CONSOLE_DISPLAY_STATE, 1, 4)
	}
	check("DisplayStateChanged", c.only("DisplayStateChanged") && s.DisplayState() != d0, "events %v, state %d", c, s.DisplayState())
	c.reset()
	sendPowerSetting(hwnd, win.GUID_CONSOLE_DISPLAY_STATE, dv0, 4)
	check("DisplayStateChanged back", c.only("DisplayStateChanged") && s.DisplayState() == d0, "events %v, state %d", c, s.DisplayState())

	c.reset()
	sendPowerSetting(hwnd, win.GUID_ACDC_POWER_SOURCE, v1, 0)
	check("power setting without data publishes nothing", len(c.n) == 0 && s.PowerSource() == p0, "events %v", c)

	// Shutdown. Each round of queries ends with WM_ENDSESSION, as from
	// Windows; wParam FALSE means that the session does not end.
	send("SessionEnding not canceled", win.WM_QUERYENDSESSION, 0, win.ENDSESSION_LOGOFF, 1, "SessionEnding")
	check("SessionEnding reason", c.last["SessionEnding"] == "0x80000000", "%q", c.last["SessionEnding"])
	send("SessionEnded", win.WM_ENDSESSION, 0, win.ENDSESSION_CLOSEAPP, 0, "SessionEnded")
	check("SessionEnded arguments", c.last["SessionEnded"] == "false 0x1", "%q", c.last["SessionEnded"])
	cancel = true
	send("SessionEnding canceled without a blocking form", win.WM_QUERYENDSESSION, 0, win.ENDSESSION_CLOSEAPP, 0, "SessionEnding")
	send("SessionEnded after the cancel", win.WM_ENDSESSION, 0, 0, 0, "SessionEnded")

	// With a shutdown block reason on the visible main window, the form
	// refuses and the hidden window agrees, so that Windows shows the form
	// with its reason; the handlers run once for both windows.
	if err := mw.SetShutdownBlockReason("walk system test: saving"); err != nil {
		check("SetShutdownBlockReason for the query", false, "%v", err)
	}
	c.reset()
	rHidden := win.SendMessage(hwnd, win.WM_QUERYENDSESSION, 0, 0)
	rForm := win.SendMessage(mw.Handle(), win.WM_QUERYENDSESSION, 0, 0)
	check("SessionEnding canceled with a blocking form", rHidden == 1 && rForm == 0 && c.only("SessionEnding"),
		"hidden window %d (want 1), form %d (want 0), events %v", rHidden, rForm, c)
	send("SessionEnded after the form refused", win.WM_ENDSESSION, 0, 0, 0, "SessionEnded")
	cancel = false
	c.reset()
	rForm = win.SendMessage(mw.Handle(), win.WM_QUERYENDSESSION, 0, 0)
	rHidden = win.SendMessage(hwnd, win.WM_QUERYENDSESSION, 0, 0)
	check("SessionEnding not canceled with a blocking form", rHidden == 1 && rForm == 1 && c.only("SessionEnding"),
		"hidden window %d, form %d (want 1), events %v", rHidden, rForm, c)
	send("SessionEnded after the form agreed", win.WM_ENDSESSION, 0, 0, 0, "SessionEnded")
	mw.SetShutdownBlockReason("")

	// Session changes.
	send("SessionLocked", win.WM_WTSSESSION_CHANGE, win.WTS_SESSION_LOCK, 7, 0, "SessionChanged", "SessionLocked")
	check("SessionChanged arguments", c.last["SessionChanged"] == fmt.Sprintf("%d 7", walk.SessionLock), "%q", c.last["SessionChanged"])
	send("SessionUnlocked", win.WM_WTSSESSION_CHANGE, win.WTS_SESSION_UNLOCK, 7, 0, "SessionChanged", "SessionUnlocked")
	send("SessionChanged remote connect", win.WM_WTSSESSION_CHANGE, win.WTS_REMOTE_CONNECT, 7, 0, "SessionChanged")

	// Settings. Nothing really changes, so the derived events stay quiet.
	area, _ := syscall.UTF16PtrFromString("ImmersiveColorSet")
	send("SettingsChanged area", win.WM_SETTINGCHANGE, 0, uintptr(unsafe.Pointer(area)), 0, "SettingsChanged")
	runtime.KeepAlive(area)
	check("SettingsChanged area value", c.last["SettingsChanged"] == "0 0x0 ImmersiveColorSet", "%q", c.last["SettingsChanged"])
	send("SettingsChanged SPI", win.WM_SETTINGCHANGE, win.SPI_SETHIGHCONTRAST, 0, 0, "SettingsChanged")
	check("SettingsChanged SPI value", c.last["SettingsChanged"] == "0 0x43 ", "%q", c.last["SettingsChanged"])
	send("SettingsChanged theme", win.WM_THEMECHANGED, 0, 0, 0, "SettingsChanged")
	check("SettingsChanged theme kind", c.last["SettingsChanged"] == "1 0x0 ", "%q", c.last["SettingsChanged"])
	send("SettingsChanged system colors", win.WM_SYSCOLORCHANGE, 0, 0, 0, "SettingsChanged")
	check("SettingsChanged system colors kind", c.last["SettingsChanged"] == "2 0x0 ", "%q", c.last["SettingsChanged"])
}

func testKeepAwake() {
	for _, display := range []bool{false, true} {
		release, err := walk.KeepAwake("walk system test", display)
		if check(fmt.Sprintf("KeepAwake display=%v", display), err == nil && release != nil, "%v", err) {
			release()
			release()
		}
	}
}

func testShutdownBlockReason(mw *walk.MainWindow) {
	const reason = "walk system test: saving"
	err := mw.SetShutdownBlockReason(reason)
	got := mw.ShutdownBlockReason()
	check("SetShutdownBlockReason", err == nil && got == reason, "err %v, reason %q", err, got)
	err = mw.SetShutdownBlockReason("")
	got = mw.ShutdownBlockReason()
	check("SetShutdownBlockReason empty", err == nil && got == "", "err %v, reason %q", err, got)
}

func testDispose(s *walk.SystemService, hwnd win.HWND) {
	old := newCounter()
	s.Suspending().Attach(func() { old.hit("old", "") })
	s.Dispose()
	check("Dispose destroys the window", !win.IsWindow(hwnd) && len(threadWindows()) == 0, "windows %v", threadWindows())

	s2, err := walk.System()
	if !check("System after Dispose returns no error", err == nil, "%v", err) {
		return
	}
	ws := threadWindows()
	check("System after Dispose", s2 != s && len(ws) == 1 && ws[0] != hwnd, "same %v, windows %v", s2 == s, ws)
	if len(ws) == 1 {
		c := newCounter()
		s2.Suspending().Attach(func() { c.hit("new", "") })
		win.SendMessage(ws[0], win.WM_POWERBROADCAST, win.PBT_APMSUSPEND, 0)
		check("new service publishes, old does not", c.n["new"] == 1 && old.n["old"] == 0, "new %d, old %d", c.n["new"], old.n["old"])
	}
	s2.Dispose()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "manual" {
		manual()
		return
	}
	runtime.LockOSThread()

	// A thread without a walk window gets no service: its hidden window
	// would never process messages.
	done := make(chan error)
	go func() {
		runtime.LockOSThread() // the thread ends with the goroutine
		_, err := walk.System()
		done <- err
	}()
	err := <-done
	check("System on a thread without a walk window", err != nil, "no error")

	var mw *walk.MainWindow
	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "walk system test",
		Layout:   VBox{},
		Children: []Widget{Label{Text: "System events test"}},
	}).Create(); err != nil {
		check("MainWindow", false, "%v", err)
		os.Exit(1)
	}
	mw.Show()

	s, err := walk.System()
	if !check("System", err == nil && s != nil, "%v", err) {
		os.Exit(1)
	}
	s2, _ := walk.System()
	check("System returns the same service", s2 == s, "different services")
	ws := threadWindows()
	if !check("one hidden window", len(ws) == 1, "windows %v", ws) {
		os.Exit(1)
	}
	hwnd := ws[0]
	desktop, _, _ := procGetDesktopWindow.Call()
	var pid uint32
	tid, _, _ := procGetWindowThreadProcs.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
	check("hidden window is top-level", win.GetAncestor(hwnd, win.GA_PARENT) == win.HWND(desktop) && enumWindowsHas(hwnd),
		"parent %#x, desktop %#x, EnumWindows %v", win.GetAncestor(hwnd, win.GA_PARENT), desktop, enumWindowsHas(hwnd))
	check("hidden window is not visible", !win.IsWindowVisible(hwnd), "visible")
	check("hidden window belongs to this thread", uint32(tid) == win.GetCurrentThreadId() && int(pid) == os.Getpid(), "thread %d, process %d", tid, pid)

	// Windows sends the current power settings right after the
	// registration. In a Remote Desktop session and in Windows Sandbox
	// (which uses Remote Desktop) the display notifications may not come.
	deadline := time.Now().Add(5 * time.Second)
	for s.DisplayState() == walk.DisplayStateUnknown && time.Now().Before(deadline) {
		pump(50 * time.Millisecond)
	}
	pump(300 * time.Millisecond)
	if s.DisplayState() == walk.DisplayStateUnknown && (walk.IsRemoteSession() || strings.EqualFold(os.Getenv("USERNAME"), "WDAGUtilityAccount")) {
		fmt.Println("INFO DisplayState unknown after 5 seconds in a remote session or Windows Sandbox")
	} else {
		check("DisplayState known after registration", s.DisplayState() != walk.DisplayStateUnknown, "unknown after 5 seconds")
	}

	testQueries(s)
	testEvents(s, hwnd, mw)
	testKeepAwake()
	testShutdownBlockReason(mw)
	testDispose(s, hwnd)

	mw.Dispose()
	if failed {
		os.Exit(1)
	}
}

func manual() {
	var mw *walk.MainWindow
	var status *walk.Label
	var logEdit *walk.TextEdit
	var block *walk.CheckBox
	var s *walk.SystemService

	report := func(text string) {
		line := time.Now().Format("15:04:05") + " " + text
		fmt.Println(line)
		logEdit.AppendText(line + "\r\n")
		status.SetText(text)
	}
	describe := func() string {
		hc, scheme, _ := walk.HighContrast()
		bs, _ := walk.Battery()
		return fmt.Sprintf("power source %d, energy saver %d, display %d, battery %d%%, high contrast %v %q, dark mode %v, screen reader %v, text scale %d%%, animations %v, remote session %v",
			s.PowerSource(), s.EnergySaver(), s.DisplayState(), bs.Percent, hc, scheme, walk.AppsUseDarkTheme(), walk.ScreenReaderRunning(), walk.TextScaleFactor(), walk.ClientAreaAnimation(), walk.IsRemoteSession())
	}
	var release func()

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "walk system manual test",
		Layout:   VBox{},
		Children: []Widget{
			Label{AssignTo: &status, Text: "Ready", Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite}},
			PushButton{Text: "&Read status", OnClicked: func() { report(describe()) }},
			PushButton{Text: "&Keep awake on or off", OnClicked: func() {
				if release != nil {
					release()
					release = nil
					report("Keep awake off")
					return
				}
				r, err := walk.KeepAwake("walk system manual test", true)
				if err != nil {
					report("KeepAwake: " + err.Error())
					return
				}
				release = r
				report("Keep awake on")
			}},
			CheckBox{AssignTo: &block, Text: "&Block sign-out and shutdown", OnCheckedChanged: func() {
				reason := ""
				if block.Checked() {
					reason = "walk system manual test blocks the shutdown"
				}
				if err := mw.SetShutdownBlockReason(reason); err != nil {
					report("SetShutdownBlockReason: " + err.Error())
					return
				}
				report(fmt.Sprintf("Block shutdown %v", block.Checked()))
			}},
			Label{Text: "Event &log:"},
			TextEdit{AssignTo: &logEdit, ReadOnly: true, VScroll: true, MinSize: Size{Width: 500, Height: 250}},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL", err)
		os.Exit(1)
	}
	var err error
	if s, err = walk.System(); err != nil {
		fmt.Println("FAIL", err)
		os.Exit(1)
	}

	events := map[string]*walk.Event{
		"Suspending":            s.Suspending(),
		"Resumed":               s.Resumed(),
		"Resumed by user":       s.ResumedByUser(),
		"Power status changed":  s.PowerStatusChanged(),
		"Locked":                s.SessionLocked(),
		"Unlocked":              s.SessionUnlocked(),
		"Display state changed": s.DisplayStateChanged(),
		"Power source changed":  s.PowerSourceChanged(),
		"Energy saver changed":  s.EnergySaverChanged(),
		"High contrast changed": s.HighContrastChanged(),
		"Dark mode changed":     s.DarkModeChanged(),
		"Screen reader changed": s.ScreenReaderChanged(),
	}
	for name, e := range events {
		e.Attach(func() { report(name + ": " + describe()) })
	}
	s.SessionChanged().Attach(func(change walk.SessionChange, id uint32) {
		fmt.Printf("%s session change %d, session %d\n", time.Now().Format("15:04:05"), change, id)
	})
	s.SettingsChanged().Attach(func(change walk.SettingsChange) {
		fmt.Printf("%s settings change kind %d, action %#x, area %q\n", time.Now().Format("15:04:05"), change.Kind, change.Action, change.Area)
	})
	s.SessionEnding().Attach(func(canceled *bool, reason walk.EndSessionReason) {
		*canceled = block.Checked()
		report(fmt.Sprintf("Session ending, reason %#x, canceled %v", uint32(reason), *canceled))
	})
	s.SessionEnded().Attach(func(ending bool, reason walk.EndSessionReason) {
		report(fmt.Sprintf("Session ended %v, reason %#x", ending, uint32(reason)))
	})

	mw.Run()
	if release != nil {
		release()
	}
}
