// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/lpintes/walk/internal/win"
)

// systemWindowClass is the class of the hidden top-level windows that
// receive the power, session and settings messages for SystemService.
const systemWindowClass = `\o/ Walk_System_Class \o/`

var (
	systemMutex      sync.Mutex
	systemWndProcPtr uintptr
	// systemServices maps the ID of a UI thread to its service.
	systemServices = make(map[uint32]*SystemService)
)

// System returns the system events service of the calling thread,
// creating it on the first call.
//
// Windows sends power, session and settings changes to top-level windows,
// so the service owns a hidden top-level window (not a message-only
// window, which gets no broadcasts). A top-level window must have its
// messages processed, or programs that broadcast messages and the
// shutdown of Windows wait for it, so System returns an error on a thread
// that has no walk window: call it on the UI goroutine after the first
// form was created. The events are published on that thread while it
// processes messages, so their handlers may use the windows of the thread
// directly. Attach handlers and call the methods of the service on the
// same thread.
//
// The window lives until Dispose is called. Call Dispose if the thread
// stops processing messages while the process goes on; the window would
// otherwise only go away with the thread, without its notifications
// being unregistered.
//
// The functions that only read settings, such as HighContrast, Battery
// and KeepAwake, need no service and work on any goroutine.
func System() (*SystemService, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid := win.GetCurrentThreadId()

	if wgm.Group(tid) == nil {
		return nil, newError("System: the thread has no walk window")
	}

	systemMutex.Lock()
	// A window destroyed without Dispose, for example because an earlier
	// thread with the same ID ended, is replaced.
	if s := systemServices[tid]; s != nil && win.IsWindow(s.hwnd) && win.GetWindowThreadProcessId(s.hwnd, nil) == tid {
		systemMutex.Unlock()
		return s, nil
	}
	if systemWndProcPtr == 0 {
		systemWndProcPtr = syscall.NewCallback(systemWndProc)
		MustRegisterWindowClassWithWndProcPtr(systemWindowClass, systemWndProcPtr)
	}
	s := &SystemService{threadID: tid}
	// The service must be in the map before CreateWindowEx, which already
	// sends messages to the window procedure.
	systemServices[tid] = s
	systemMutex.Unlock()

	if err := s.create(); err != nil {
		systemMutex.Lock()
		if systemServices[tid] == s {
			delete(systemServices, tid)
		}
		systemMutex.Unlock()
		return nil, err
	}

	return s, nil
}

// SystemService publishes power, session and system settings events and
// reads system settings. Get it with System.
type SystemService struct {
	threadID uint32
	hwnd     win.HWND
	notify   []uintptr // RegisterPowerSettingNotification handles
	wtsOK    bool

	powerSource       PowerSource
	powerSourceKnown  bool
	batterySaver      bool
	batterySaverKnown bool
	energySaver       EnergySaverStatus
	energySaverKnown  bool
	displayState      DisplayState

	highContrast bool
	darkMode     bool
	screenReader bool

	// The answer of the SessionEnding handlers to the current
	// WM_QUERYENDSESSION, shared by the hidden window and the forms with a
	// shutdown block reason (see queryEndSession).
	endQueried       bool
	endQueryCanceled bool
	endQueryTime     time.Time

	suspendingPublisher         EventPublisher
	resumedPublisher            EventPublisher
	resumedByUserPublisher      EventPublisher
	powerStatusChangedPublisher EventPublisher
	powerSourceChangedPublisher EventPublisher
	energySaverChangedPublisher EventPublisher
	displayStateChangedPub      EventPublisher
	sessionEndingPublisher      SessionEndingEventPublisher
	sessionEndedPublisher       SessionEndedEventPublisher
	sessionChangedPublisher     SessionChangedEventPublisher
	sessionLockedPublisher      EventPublisher
	sessionUnlockedPublisher    EventPublisher
	settingsChangedPublisher    SettingsChangedEventPublisher
	highContrastChangedPub      EventPublisher
	darkModeChangedPublisher    EventPublisher
	screenReaderChangedPub      EventPublisher
}

func (s *SystemService) create() error {
	hwnd := win.CreateWindowEx(
		win.WS_EX_TOOLWINDOW,
		win.StringToUTF16Ptr(systemWindowClass),
		nil,
		win.WS_POPUP,
		0,
		0,
		0,
		0,
		0,
		0,
		0,
		nil)
	if hwnd == 0 {
		return lastError("CreateWindowEx")
	}
	s.hwnd = hwnd

	s.highContrast, _, _ = HighContrast()
	s.darkMode = AppsUseDarkTheme()
	s.screenReader = ScreenReaderRunning()

	// Windows sends the current value of each setting right after its
	// registration; see powerSettingChanged.
	for _, guid := range []*syscall.GUID{
		&win.GUID_ACDC_POWER_SOURCE,
		&win.GUID_POWER_SAVING_STATUS,
		&win.GUID_ENERGY_SAVER_STATUS,
		&win.GUID_CONSOLE_DISPLAY_STATE,
		&win.GUID_SESSION_DISPLAY_STATUS,
	} {
		// GUID_ENERGY_SAVER_STATUS needs Windows 11; registering it on
		// older versions fails or never sends anything.
		if h := win.RegisterPowerSettingNotification(win.HANDLE(hwnd), guid, win.DEVICE_NOTIFY_WINDOW_HANDLE); h != 0 {
			s.notify = append(s.notify, h)
		}
	}

	// This can fail right after logon, before the Remote Desktop Services
	// service runs; the session events are then not published.
	s.wtsOK = win.WTSRegisterSessionNotification(hwnd, win.NOTIFY_FOR_THIS_SESSION)

	return nil
}

// Dispose destroys the hidden window of the service and unregisters its
// notifications. A later call of System on the thread creates a new
// service; handlers attached to this one are not called any more. Call
// Dispose on the thread of the service.
func (s *SystemService) Dispose() {
	if s.hwnd != 0 {
		win.DestroyWindow(s.hwnd)
	}
}

func (s *SystemService) destroyed() {
	for _, h := range s.notify {
		win.UnregisterPowerSettingNotification(h)
	}
	s.notify = nil
	if s.wtsOK {
		win.WTSUnRegisterSessionNotification(s.hwnd)
		s.wtsOK = false
	}
	s.hwnd = 0

	systemMutex.Lock()
	if systemServices[s.threadID] == s {
		delete(systemServices, s.threadID)
	}
	systemMutex.Unlock()
}

func systemWndProc(hwnd win.HWND, msg uint32, wp, lp uintptr) (result uintptr) {
	// As in defaultWndProc: a panic of a handler goes to App().Panicking.
	defer func() {
		if len(App().panickingPublisher.event.handlers) > 0 {
			var err error
			if x := recover(); x != nil {
				if e, ok := x.(error); ok {
					err = wrapErrorNoPanic(e)
				} else {
					err = newErrorNoPanic(fmt.Sprint(x))
				}
			}
			if err != nil {
				App().panickingPublisher.Publish(err)
			}
		}
	}()

	systemMutex.Lock()
	s := systemServices[win.GetCurrentThreadId()]
	systemMutex.Unlock()

	if s == nil || (s.hwnd != 0 && s.hwnd != hwnd) {
		return win.DefWindowProc(hwnd, msg, wp, lp)
	}

	switch msg {
	case win.WM_POWERBROADCAST:
		switch wp {
		case win.PBT_APMSUSPEND:
			s.suspendingPublisher.Publish()
		case win.PBT_APMRESUMEAUTOMATIC:
			s.resumedPublisher.Publish()
		case win.PBT_APMRESUMESUSPEND:
			s.resumedByUserPublisher.Publish()
		case win.PBT_APMPOWERSTATUSCHANGE:
			s.powerStatusChangedPublisher.Publish()
		case win.PBT_POWERSETTINGCHANGE:
			if lp != 0 {
				s.powerSettingChanged(ptrFromUintptr[win.POWERBROADCAST_SETTING](lp))
			}
		}
		return 1

	case win.WM_QUERYENDSESSION:
		// When a visible form with a shutdown block reason refuses (see
		// formQueryEndSession), Windows shows that form with its reason;
		// the untitled hidden window refuses only without such a form.
		if s.queryEndSession(EndSessionReason(lp)) && !s.hasBlockingForm() {
			return 0
		}
		return 1

	case win.WM_ENDSESSION:
		s.endQueried = false
		s.sessionEndedPublisher.Publish(wp != 0, EndSessionReason(lp))
		return 0

	case win.WM_WTSSESSION_CHANGE:
		change := SessionChange(wp)
		s.sessionChangedPublisher.Publish(change, uint32(lp))
		switch change {
		case SessionLock:
			s.sessionLockedPublisher.Publish()
		case SessionUnlock:
			s.sessionUnlockedPublisher.Publish()
		}
		return 0

	case win.WM_SETTINGCHANGE:
		change := SettingsChange{Kind: SettingsChangeSetting, Action: uint32(wp)}
		if lp != 0 {
			change.Area = win.UTF16PtrToString(ptrFromUintptr[uint16](lp))
		}
		s.settingsChanged(change)
		return 0

	case win.WM_THEMECHANGED:
		s.settingsChanged(SettingsChange{Kind: SettingsChangeTheme})
		return 0

	case win.WM_SYSCOLORCHANGE:
		s.settingsChanged(SettingsChange{Kind: SettingsChangeSysColors})
		return 0

	case win.WM_DESTROY:
		s.destroyed()
		return 0
	}

	return win.DefWindowProc(hwnd, msg, wp, lp)
}

func (s *SystemService) powerSettingChanged(pbs *win.POWERBROADCAST_SETTING) {
	if pbs.DataLength < 4 {
		return
	}
	// Data is a DWORD for the registered settings; it may be unaligned.
	data := unsafe.Slice(&pbs.Data[0], 4)
	value := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24

	// The first notification of each setting comes right after the
	// registration and only gives the current value, so it publishes
	// nothing.
	switch pbs.PowerSetting {
	case win.GUID_ACDC_POWER_SOURCE:
		var src PowerSource
		switch value {
		case 0:
			src = PowerSourceAC
		case 1:
			src = PowerSourceBattery
		case 2:
			src = PowerSourceUPS
		}
		known, old := s.powerSourceKnown, s.powerSource
		s.powerSource, s.powerSourceKnown = src, true
		if known && old != src {
			s.powerSourceChangedPublisher.Publish()
		}

	case win.GUID_POWER_SAVING_STATUS:
		old := s.EnergySaver()
		known := s.batterySaverKnown
		s.batterySaver, s.batterySaverKnown = value != 0, true
		if known && s.EnergySaver() != old {
			s.energySaverChangedPublisher.Publish()
		}

	case win.GUID_ENERGY_SAVER_STATUS:
		old := s.EnergySaver()
		known := s.energySaverKnown
		s.energySaver, s.energySaverKnown = EnergySaverStatus(min(value, 2)), true
		if known && s.EnergySaver() != old {
			s.energySaverChangedPublisher.Publish()
		}

	case win.GUID_CONSOLE_DISPLAY_STATE, win.GUID_SESSION_DISPLAY_STATUS:
		var state DisplayState
		switch value {
		case 0:
			state = DisplayOff
		case 1:
			state = DisplayOn
		case 2:
			state = DisplayDimmed
		}
		old := s.displayState
		s.displayState = state
		if old != DisplayStateUnknown && old != state {
			s.displayStateChangedPub.Publish()
		}
	}
}

// queryEndSession publishes SessionEnding once for a WM_QUERYENDSESSION,
// which every top-level window of the thread gets, and returns whether a
// handler canceled. The answer is reused until WM_ENDSESSION, or for at
// most ten seconds if that does not come.
func (s *SystemService) queryEndSession(reason EndSessionReason) bool {
	if s.endQueried && time.Since(s.endQueryTime) < 10*time.Second {
		return s.endQueryCanceled
	}
	var canceled bool
	s.sessionEndingPublisher.Publish(&canceled, reason)
	s.endQueried, s.endQueryCanceled, s.endQueryTime = true, canceled, time.Now()
	return canceled
}

// hasBlockingForm returns whether a visible form of the thread has a
// shutdown block reason.
func (s *SystemService) hasBlockingForm() bool {
	for hwnd, wb := range hwnd2WindowBase {
		if _, ok := wb.window.(Form); !ok {
			continue
		}
		if win.GetWindowThreadProcessId(hwnd, nil) == s.threadID && win.IsWindowVisible(hwnd) && hasShutdownBlockReason(hwnd) {
			return true
		}
	}
	return false
}

func hasShutdownBlockReason(hwnd win.HWND) bool {
	var n uint32
	return win.ShutdownBlockReasonQuery(hwnd, nil, &n) && n > 0
}

// formQueryEndSession handles WM_QUERYENDSESSION for a form. A form with
// a shutdown block reason refuses when a SessionEnding handler of the
// service of its thread cancels, so that Windows shows the form with its
// reason. handled is false when the form should give the default answer.
func formQueryEndSession(fb *FormBase, lParam uintptr) (result uintptr, handled bool) {
	systemMutex.Lock()
	s := systemServices[win.GetCurrentThreadId()]
	systemMutex.Unlock()
	if s == nil || s.hwnd == 0 || !hasShutdownBlockReason(fb.hWnd) {
		return 0, false
	}
	if s.queryEndSession(EndSessionReason(lParam)) {
		return 0, true
	}
	return 1, true
}

func (s *SystemService) settingsChanged(change SettingsChange) {
	s.settingsChangedPublisher.Publish(change)

	if hc, _, err := HighContrast(); err == nil && hc != s.highContrast {
		s.highContrast = hc
		s.highContrastChangedPub.Publish()
	}
	if dark := AppsUseDarkTheme(); dark != s.darkMode {
		s.darkMode = dark
		s.darkModeChangedPublisher.Publish()
	}
	if sr := ScreenReaderRunning(); sr != s.screenReader {
		s.screenReader = sr
		s.screenReaderChangedPub.Publish()
	}
}

// Power

// Suspending is published when the computer is about to sleep or
// hibernate (PBT_APMSUSPEND). Handlers must return quickly: Windows
// waits about two seconds for all applications. Computers with modern
// standby may not send it; DisplayStateChanged still tells that the
// display was turned off.
func (s *SystemService) Suspending() *Event {
	return s.suspendingPublisher.Event()
}

// Resumed is published once after every resume from sleep or hibernation
// (PBT_APMRESUMEAUTOMATIC), also when nobody is at the computer, for
// example after a wake timer.
func (s *SystemService) Resumed() *Event {
	return s.resumedPublisher.Event()
}

// ResumedByUser is published after Resumed when the resume was caused by
// the user, for example by a key press (PBT_APMRESUMESUSPEND). Reopen
// network connections and files here.
func (s *SystemService) ResumedByUser() *Event {
	return s.resumedByUserPublisher.Event()
}

// PowerStatusChanged is published when the power status changes, for
// example the battery level or the power source (PBT_APMPOWERSTATUSCHANGE).
// Read the new values with Battery and SystemService.PowerSource.
func (s *SystemService) PowerStatusChanged() *Event {
	return s.powerStatusChangedPublisher.Event()
}

// PowerSourceChanged is published when the computer switches between AC
// power, its battery and an uninterruptible power supply
// (GUID_ACDC_POWER_SOURCE). Read the new source with PowerSource.
func (s *SystemService) PowerSourceChanged() *Event {
	return s.powerSourceChangedPublisher.Event()
}

// EnergySaverChanged is published when energy saver (battery saver
// before Windows 11) is turned on or off or changes its level. Read the
// new status with EnergySaver.
func (s *SystemService) EnergySaverChanged() *Event {
	return s.energySaverChangedPublisher.Event()
}

// DisplayStateChanged is published when the display is turned off, dimmed
// or turned on (GUID_CONSOLE_DISPLAY_STATE, and GUID_SESSION_DISPLAY_STATUS
// for the display of the session, which also works in Remote Desktop).
// Read the new state with DisplayState.
func (s *SystemService) DisplayStateChanged() *Event {
	return s.displayStateChangedPub.Event()
}

// PowerSource is where the computer gets its power from.
type PowerSource int

// Power sources
const (
	PowerSourceUnknown PowerSource = iota
	PowerSourceAC
	PowerSourceBattery
	// PowerSourceUPS is a short-term source such as an uninterruptible
	// power supply.
	PowerSourceUPS
)

// EnergySaverStatus says whether energy saver is on.
type EnergySaverStatus int

// Energy saver states, the values of ENERGY_SAVER_STATUS
const (
	EnergySaverOff EnergySaverStatus = iota
	EnergySaverStandard
	EnergySaverHighSavings
)

// DisplayState is the state of the display.
type DisplayState int

// Display states
const (
	DisplayStateUnknown DisplayState = iota
	DisplayOff
	DisplayOn
	DisplayDimmed
)

// PowerSource returns where the computer gets its power from, as last
// reported to the service, or from GetSystemPowerStatus before the first
// report.
func (s *SystemService) PowerSource() PowerSource {
	if s.powerSourceKnown {
		return s.powerSource
	}
	var sps win.SYSTEM_POWER_STATUS
	if !win.GetSystemPowerStatus(&sps) {
		return PowerSourceUnknown
	}
	switch sps.ACLineStatus {
	case 0:
		return PowerSourceBattery
	case 1:
		return PowerSourceAC
	}
	return PowerSourceUnknown
}

// EnergySaver returns the energy saver status as last reported to the
// service (GUID_ENERGY_SAVER_STATUS on Windows 11, GUID_POWER_SAVING_STATUS
// before), or from GetSystemPowerStatus before the first report.
func (s *SystemService) EnergySaver() EnergySaverStatus {
	if s.energySaverKnown {
		return s.energySaver
	}
	if s.batterySaverKnown {
		if s.batterySaver {
			return EnergySaverStandard
		}
		return EnergySaverOff
	}
	var sps win.SYSTEM_POWER_STATUS
	if win.GetSystemPowerStatus(&sps) && sps.SystemStatusFlag&1 != 0 {
		return EnergySaverStandard
	}
	return EnergySaverOff
}

// DisplayState returns the state of the display as last reported to the
// service. It is DisplayStateUnknown until Windows sends the first report,
// shortly after System created the service; in a Remote Desktop session
// or in Windows Sandbox there may be none.
func (s *SystemService) DisplayState() DisplayState {
	return s.displayState
}

// BatteryStatus describes the battery of the computer.
type BatteryStatus struct {
	// Present is false for a computer without a battery or when Windows
	// does not know.
	Present bool
	// Charging is true while the battery is charging.
	Charging bool
	// Percent is the remaining charge from 0 to 100, or -1 if unknown.
	Percent int
	// Remaining is the estimated remaining time on battery, or -1 if
	// unknown, for example on AC power.
	Remaining time.Duration
}

// Battery returns the status of the battery (GetSystemPowerStatus). It
// may be called from any goroutine.
func Battery() (BatteryStatus, error) {
	var sps win.SYSTEM_POWER_STATUS
	if !win.GetSystemPowerStatus(&sps) {
		return BatteryStatus{Percent: -1, Remaining: -1}, lastError("GetSystemPowerStatus")
	}
	bs := BatteryStatus{
		Present:   sps.BatteryFlag != 128 && sps.BatteryFlag != 255,
		Charging:  sps.BatteryFlag != 255 && sps.BatteryFlag&8 != 0,
		Percent:   -1,
		Remaining: -1,
	}
	if sps.BatteryLifePercent <= 100 {
		bs.Percent = int(sps.BatteryLifePercent)
	}
	if sps.BatteryLifeTime != 0xFFFFFFFF {
		bs.Remaining = time.Duration(sps.BatteryLifeTime) * time.Second
	}
	return bs, nil
}

// KeepAwake asks Windows not to sleep because of user inactivity while
// the program does long work, and with display also not to turn the
// display off. reason is shown to administrators by powercfg /requests.
// Call the returned release function when the work is done; calling it
// more than once does nothing. It does not prevent sleep the user asks
// for (the power button, closing the lid or the Start menu).
//
// KeepAwake uses a power request (PowerCreateRequest and PowerSetRequest)
// rather than SetThreadExecutionState: each call has its own request, so
// several pieces of work do not cancel each other, the state does not
// belong to an OS thread (goroutines move between threads), and the
// reason is visible. It may be called from any goroutine.
//
// A release function that is never called keeps the request until the
// process exits.
func KeepAwake(reason string, display bool) (release func(), err error) {
	r, err := syscall.UTF16PtrFromString(reason)
	if err != nil {
		return nil, wrapError(err)
	}
	ctx := win.REASON_CONTEXT{
		Version:            win.POWER_REQUEST_CONTEXT_VERSION,
		Flags:              win.POWER_REQUEST_CONTEXT_SIMPLE_STRING,
		SimpleReasonString: r,
	}
	h := win.PowerCreateRequest(&ctx)
	runtime.KeepAlive(r)
	if h == ^win.HANDLE(0) || h == 0 {
		return nil, lastError("PowerCreateRequest")
	}

	types := []int32{win.PowerRequestSystemRequired}
	if display {
		types = append(types, win.PowerRequestDisplayRequired)
	}
	var set []int32
	for _, t := range types {
		if !win.PowerSetRequest(h, t) {
			err := lastError("PowerSetRequest")
			for _, t := range set {
				win.PowerClearRequest(h, t)
			}
			win.CloseHandle(h)
			return nil, err
		}
		set = append(set, t)
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			for _, t := range set {
				win.PowerClearRequest(h, t)
			}
			win.CloseHandle(h)
		})
	}, nil
}

// Shutdown and sessions

// EndSessionReason tells why the session ends, from the lParam of
// WM_QUERYENDSESSION and WM_ENDSESSION. Zero means shutdown or restart.
type EndSessionReason uint32

// End session reasons; more than one may be set.
const (
	// EndSessionCloseApp: the application must close, for example
	// because an installer or Windows Update needs its files (Restart
	// Manager).
	EndSessionCloseApp EndSessionReason = win.ENDSESSION_CLOSEAPP
	// EndSessionCritical: Windows forces the application to close.
	EndSessionCritical EndSessionReason = win.ENDSESSION_CRITICAL
	// EndSessionLogoff: the user logs off.
	EndSessionLogoff EndSessionReason = win.ENDSESSION_LOGOFF
)

// SessionEnding is published when the user logs off or Windows shuts
// down or restarts (WM_QUERYENDSESSION), once although every top-level
// window of the thread gets the message. A handler that sets *canceled to
// true asks Windows to wait; Windows then shows the applications that
// block it with their reasons and the user may end the session anyway.
// Cancel only together with FormBase.SetShutdownBlockReason on a visible
// form: that form then refuses the end of the session and Windows shows
// it with the reason. Without such a form the hidden window of the
// service refuses, and Windows shows the program without a reason or may
// close it. Do not show dialogs here.
func (s *SystemService) SessionEnding() *SessionEndingEvent {
	return s.sessionEndingPublisher.Event()
}

// SessionEnded is published after SessionEnding (WM_ENDSESSION). ending
// is true when the session really ends: the process may be terminated
// any time after the handlers return, so save data here. ending is false
// when some application canceled the end of the session.
func (s *SystemService) SessionEnded() *SessionEndedEvent {
	return s.sessionEndedPublisher.Event()
}

// SetShutdownBlockReason sets the reason Windows shows for this form
// when the application cancels the end of the session (see
// SystemService.SessionEnding), for example "Saving the document"
// (ShutdownBlockReasonCreate). An empty reason removes it
// (ShutdownBlockReasonDestroy). Set it only while the work runs. It must
// be called on the thread of the form.
func (fb *FormBase) SetShutdownBlockReason(reason string) error {
	if reason == "" {
		if hasShutdownBlockReason(fb.hWnd) {
			win.ShutdownBlockReasonDestroy(fb.hWnd)
		}
		return nil
	}
	r, err := syscall.UTF16PtrFromString(reason)
	if err != nil {
		return wrapError(err)
	}
	if !win.ShutdownBlockReasonCreate(fb.hWnd, r) {
		return lastError("ShutdownBlockReasonCreate")
	}
	return nil
}

// ShutdownBlockReason returns the reason set with SetShutdownBlockReason,
// or an empty string (ShutdownBlockReasonQuery).
func (fb *FormBase) ShutdownBlockReason() string {
	var n uint32
	if !win.ShutdownBlockReasonQuery(fb.hWnd, nil, &n) || n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	if !win.ShutdownBlockReasonQuery(fb.hWnd, &buf[0], &n) {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// SessionChange is a change of the Windows session the program runs in,
// the wParam of WM_WTSSESSION_CHANGE.
type SessionChange uint32

// Session changes
const (
	SessionConsoleConnect    SessionChange = win.WTS_CONSOLE_CONNECT
	SessionConsoleDisconnect SessionChange = win.WTS_CONSOLE_DISCONNECT
	SessionRemoteConnect     SessionChange = win.WTS_REMOTE_CONNECT
	SessionRemoteDisconnect  SessionChange = win.WTS_REMOTE_DISCONNECT
	SessionLogon             SessionChange = win.WTS_SESSION_LOGON
	SessionLogoff            SessionChange = win.WTS_SESSION_LOGOFF
	SessionLock              SessionChange = win.WTS_SESSION_LOCK
	SessionUnlock            SessionChange = win.WTS_SESSION_UNLOCK
	SessionRemoteControl     SessionChange = win.WTS_SESSION_REMOTE_CONTROL
	SessionCreate            SessionChange = win.WTS_SESSION_CREATE
	SessionTerminate         SessionChange = win.WTS_SESSION_TERMINATE
)

// SessionChanged is published when the session of the program is locked
// or unlocked, connected or disconnected locally or by Remote Desktop,
// and so on (WM_WTSSESSION_CHANGE, registered with
// WTSRegisterSessionNotification for this session only).
func (s *SystemService) SessionChanged() *SessionChangedEvent {
	return s.sessionChangedPublisher.Event()
}

// SessionLocked is published when the session is locked, for example with
// Windows+L (after SessionChanged with SessionLock).
func (s *SystemService) SessionLocked() *Event {
	return s.sessionLockedPublisher.Event()
}

// SessionUnlocked is published when the session is unlocked (after
// SessionChanged with SessionUnlock).
func (s *SystemService) SessionUnlocked() *Event {
	return s.sessionUnlockedPublisher.Event()
}

// IsRemoteSession returns whether the program runs in a Remote Desktop
// session (SM_REMOTESESSION). It may be called from any goroutine.
func IsRemoteSession() bool {
	return win.GetSystemMetrics(win.SM_REMOTESESSION) != 0
}

// System settings

// SettingsChangeKind is the message behind a SettingsChange.
type SettingsChangeKind int

// Kinds of settings changes
const (
	// SettingsChangeSetting: WM_SETTINGCHANGE, a setting changed.
	SettingsChangeSetting SettingsChangeKind = iota
	// SettingsChangeTheme: WM_THEMECHANGED, the visual style changed.
	SettingsChangeTheme
	// SettingsChangeSysColors: WM_SYSCOLORCHANGE, a system color changed.
	SettingsChangeSysColors
)

// SettingsChange describes a change of system settings.
type SettingsChange struct {
	Kind SettingsChangeKind
	// Action is the SystemParametersInfo action that changed the setting
	// (an SPI_SET* value) for SettingsChangeSetting, or 0.
	Action uint32
	// Area names the changed area for SettingsChangeSetting, for example
	// "ImmersiveColorSet" (dark mode or accent color), "Environment",
	// "intl" or "Policy", or is empty.
	Area string
}

// SettingsChanged is published for every WM_SETTINGCHANGE,
// WM_THEMECHANGED and WM_SYSCOLORCHANGE. Windows sends several of them
// for one change; HighContrastChanged, DarkModeChanged and
// ScreenReaderChanged are published after them only when their value
// really changed.
func (s *SystemService) SettingsChanged() *SettingsChangedEvent {
	return s.settingsChangedPublisher.Event()
}

// HighContrastChanged is published when high contrast is turned on or off
// (for example with Left Alt+Left Shift+Print Screen).
func (s *SystemService) HighContrastChanged() *Event {
	return s.highContrastChangedPub.Event()
}

// DarkModeChanged is published when the user switches between the light
// and the dark mode for applications (see AppsUseDarkTheme).
func (s *SystemService) DarkModeChanged() *Event {
	return s.darkModeChangedPublisher.Event()
}

// ScreenReaderChanged is published when a screen reader starts or stops
// (see ScreenReaderRunning).
func (s *SystemService) ScreenReaderChanged() *Event {
	return s.screenReaderChangedPub.Event()
}

// HighContrast returns whether high contrast is on and the name of the
// high contrast scheme (SPI_GETHIGHCONTRAST). The scheme may be empty
// although high contrast is on: Windows 11 does not report the name of
// its contrast themes. It may be called from any goroutine.
func HighContrast() (on bool, scheme string, err error) {
	var hc win.HIGHCONTRAST
	hc.CbSize = uint32(unsafe.Sizeof(hc))
	if !win.SystemParametersInfo(win.SPI_GETHIGHCONTRAST, hc.CbSize, unsafe.Pointer(&hc), 0) {
		return false, "", lastError("SystemParametersInfo(SPI_GETHIGHCONTRAST)")
	}
	// lpszDefaultScheme points into memory of user32.
	if hc.LpszDefaultScheme != nil {
		scheme = win.UTF16PtrToString(hc.LpszDefaultScheme)
	}
	return hc.DwFlags&win.HCF_HIGHCONTRASTON != 0, scheme, nil
}

// ScreenReaderRunning returns whether a screen reader has told Windows
// that it runs (SPI_GETSCREENREADER). NVDA, JAWS and Narrator set this
// flag. It may be called from any goroutine.
func ScreenReaderRunning() bool {
	var on win.BOOL
	if !win.SystemParametersInfo(win.SPI_GETSCREENREADER, 0, unsafe.Pointer(&on), 0) {
		return false
	}
	return on != 0
}

// ClientAreaAnimation returns whether the user wants animations in
// windows ("Animation effects" in Settings, SPI_GETCLIENTAREAANIMATION).
// It is true if Windows cannot tell. It may be called from any goroutine.
func ClientAreaAnimation() bool {
	var on win.BOOL
	if !win.SystemParametersInfo(win.SPI_GETCLIENTAREAANIMATION, 0, unsafe.Pointer(&on), 0) {
		return true
	}
	return on != 0
}

// AppsUseDarkTheme returns whether the user chose the dark mode for
// applications, from the registry value AppsUseLightTheme under
// HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize,
// which Windows 10 1809 and later use; Microsoft documents no function for
// it. Without the value it returns false. While high contrast is on, its
// colors apply instead. It may be called from any goroutine.
func AppsUseDarkTheme() bool {
	v, err := RegistryKeyUint32(CurrentUserKey(), `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme")
	return err == nil && v == 0
}

// TextScaleFactor returns the text size the user chose in the
// accessibility settings ("Text size") in percent, from 100 to 225, from
// the registry value TextScaleFactor under
// HKEY_CURRENT_USER\Software\Microsoft\Accessibility. Without the value
// it returns 100. It is separate from the display scaling (DPI). It may
// be called from any goroutine.
func TextScaleFactor() int {
	v, err := RegistryKeyUint32(CurrentUserKey(), `Software\Microsoft\Accessibility`, "TextScaleFactor")
	if err != nil || v < 100 || v > 225 {
		return 100
	}
	return int(v)
}
