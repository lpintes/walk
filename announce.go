// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
)

// AnnouncementProcessing enum defines how screen readers process an announcement made with
// Announce while they are speaking. The values are those of the UI Automation
// NotificationProcessing enum.
type AnnouncementProcessing int32

// Announcement processing
const (
	// AnnounceImportantAll: speak the announcement, interrupting speech, and speak every one
	// of several announcements in a row.
	AnnounceImportantAll AnnouncementProcessing = win.NotificationProcessing_ImportantAll

	// AnnounceImportantMostRecent: speak the announcement, interrupting speech; of several
	// announcements in a row only the most recent one.
	AnnounceImportantMostRecent AnnouncementProcessing = win.NotificationProcessing_ImportantMostRecent

	// AnnounceAll: speak every announcement after the current speech.
	AnnounceAll AnnouncementProcessing = win.NotificationProcessing_All

	// AnnounceMostRecent: speak the announcement after the current speech; of several
	// announcements in a row only the most recent one.
	AnnounceMostRecent AnnouncementProcessing = win.NotificationProcessing_MostRecent

	// AnnounceCurrentThenMostRecent: finish the announcement being spoken, then speak the
	// most recent one of the announcements that followed it.
	AnnounceCurrentThenMostRecent AnnouncementProcessing = win.NotificationProcessing_CurrentThenMostRecent
)

// announceActivityID is the prefix of the activity IDs of announcements made with Announce;
// each announcement gets "Walk.Announce.<n>" with a sequence number n.
const announceActivityID = "Walk.Announce"

// announceSeq numbers announcements. NVDA coalesces notification events with the same sender,
// kind, processing and activity ID that arrive in quick succession and then speaks only the
// first two, so each announcement gets its own activity ID.
var announceSeq atomic.Uint64

// Announce asks screen readers to speak text without moving focus and without a visible
// control, for example to report that a background task finished or that input is invalid.
// window is the window the announcement belongs to, usually the form or the widget it is
// about; screen readers may ignore announcements from a window that is not in the foreground.
//
// Announce raises a UI Automation notification event, which needs Windows 10 version 1709 or
// later; on older versions it returns an error. It does nothing if no UI Automation client,
// such as a screen reader, is listening. Screen readers differ in how they honor processing.
func Announce(window Window, text string, processing AnnouncementProcessing) error {
	return announce(window.Handle(), text, processing)
}

func announce(hwnd win.HWND, text string, processing AnnouncementProcessing) error {
	if !win.UiaRaiseNotificationEventAvailable() {
		return newError("UiaRaiseNotificationEvent not available (Windows 10 version 1709 or later needed)")
	}
	if strings.IndexByte(text, 0) >= 0 {
		return newError("text contains NUL")
	}
	if !win.UiaClientsAreListening() {
		return nil
	}

	provider := newAnnounceProvider(hwnd)
	defer provider.Release()

	displayString := win.SysAllocString(text)
	defer win.SysFreeString(displayString)
	activityID := win.SysAllocString(announceActivityID + "." + strconv.FormatUint(announceSeq.Add(1), 10))
	defer win.SysFreeString(activityID)

	hr := win.UiaRaiseNotificationEvent(
		(*win.IRawElementProviderSimple)(provider.Interface(0)),
		win.NotificationKind_Other,
		int32(processing),
		displayString,
		activityID)
	if win.FAILED(hr) {
		return errorFromHRESULT("UiaRaiseNotificationEvent", hr)
	}
	return nil
}

// announceProvider is a minimal UI Automation provider for a window, the source of the
// notification events raised by Announce. It has no properties or patterns of its own: UI
// Automation takes them from the host provider of the window.
type announceProvider struct {
	hwnd win.HWND
}

var (
	announceProviderOnce sync.Once
	announceProviderVtbl *com.VTable
)

func newAnnounceProvider(hwnd win.HWND) *com.Object {
	announceProviderOnce.Do(func() {
		announceProviderVtbl = com.NewVTable(&win.IRawElementProviderSimpleVtbl{
			Get_ProviderOptions:        syscall.NewCallback(announceProvider_Get_ProviderOptions),
			GetPatternProvider:         syscall.NewCallback(announceProvider_GetPatternProvider),
			GetPropertyValue:           syscall.NewCallback(announceProvider_GetPropertyValue),
			Get_HostRawElementProvider: syscall.NewCallback(announceProvider_Get_HostRawElementProvider),
		}, &win.IID_IRawElementProviderSimple)
	})
	return com.NewObject(&announceProvider{hwnd: hwnd}, announceProviderVtbl)
}

func announceProvider_Get_ProviderOptions(this *com.This, pRetVal *int32) uintptr {
	if pRetVal == nil {
		return win.E_POINTER
	}
	// UseComThreading: UI Automation calls the provider on the thread that created it, the
	// thread of the window, through COM.
	*pRetVal = win.ProviderOptions_ServerSideProvider | win.ProviderOptions_UseComThreading
	return win.S_OK
}

func announceProvider_GetPatternProvider(this *com.This, patternId int32, pRetVal **win.IUnknown) uintptr {
	if pRetVal == nil {
		return win.E_POINTER
	}
	*pRetVal = nil
	return win.S_OK
}

func announceProvider_GetPropertyValue(this *com.This, propertyId int32, pRetVal *win.VARIANT) uintptr {
	if pRetVal == nil {
		return win.E_POINTER
	}
	*pRetVal = win.VARIANT{} // VT_EMPTY: not supported, ask the host provider
	return win.S_OK
}

func announceProvider_Get_HostRawElementProvider(this *com.This, pRetVal **win.IRawElementProviderSimple) uintptr {
	if pRetVal == nil {
		return win.E_POINTER
	}
	hwnd := this.Object().Impl().(*announceProvider).hwnd
	return uintptr(win.UiaHostProviderFromHwnd(hwnd, pRetVal))
}
