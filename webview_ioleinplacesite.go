// Copyright 2010 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"syscall"
	"unsafe"
)

import (
	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
)

var webViewIOleInPlaceSiteVtbl *com.VTable

func init() {
	AppendToWalkInit(func() {
		webViewIOleInPlaceSiteVtbl = com.NewVTable(&win.IOleInPlaceSiteVtbl{
			GetWindow:            syscall.NewCallback(webView_IOleInPlaceSite_GetWindow),
			ContextSensitiveHelp: syscall.NewCallback(webView_IOleInPlaceSite_ContextSensitiveHelp),
			CanInPlaceActivate:   syscall.NewCallback(webView_IOleInPlaceSite_CanInPlaceActivate),
			OnInPlaceActivate:    syscall.NewCallback(webView_IOleInPlaceSite_OnInPlaceActivate),
			OnUIActivate:         syscall.NewCallback(webView_IOleInPlaceSite_OnUIActivate),
			GetWindowContext:     syscall.NewCallback(webView_IOleInPlaceSite_GetWindowContext),
			Scroll:               syscall.NewCallback(webView_IOleInPlaceSite_Scroll),
			OnUIDeactivate:       syscall.NewCallback(webView_IOleInPlaceSite_OnUIDeactivate),
			OnInPlaceDeactivate:  syscall.NewCallback(webView_IOleInPlaceSite_OnInPlaceDeactivate),
			DiscardUndoState:     syscall.NewCallback(webView_IOleInPlaceSite_DiscardUndoState),
			DeactivateAndUndo:    syscall.NewCallback(webView_IOleInPlaceSite_DeactivateAndUndo),
			OnPosRectChange:      syscall.NewCallback(webView_IOleInPlaceSite_OnPosRectChange),
		}, &win.IID_IOleInPlaceSite)
	})
}

func webView_IOleInPlaceSite_GetWindow(this *com.This, lphwnd *win.HWND) uintptr {
	*lphwnd = webViewFromThis(this).hWnd

	return win.S_OK
}

func webView_IOleInPlaceSite_ContextSensitiveHelp(this *com.This, fEnterMode win.BOOL) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceSite_CanInPlaceActivate(this *com.This) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceSite_OnInPlaceActivate(this *com.This) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceSite_OnUIActivate(this *com.This) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceSite_GetWindowContext(this *com.This, lplpFrame *unsafe.Pointer, lplpDoc *uintptr, lprcPosRect, lprcClipRect *win.RECT, lpFrameInfo *win.OLEINPLACEFRAMEINFO) uintptr {
	wv := webViewFromThis(this)

	wv.frame.AddRef()
	*lplpFrame = wv.frame.Interface(0)
	*lplpDoc = 0

	lpFrameInfo.FMDIApp = win.FALSE
	lpFrameInfo.HwndFrame = wv.hWnd
	lpFrameInfo.Haccel = 0
	lpFrameInfo.CAccelEntries = 0

	return win.S_OK
}

func webView_IOleInPlaceSite_Scroll(this *com.This, scrollExtentX, scrollExtentY int32) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceSite_OnUIDeactivate(this *com.This, fUndoable win.BOOL) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceSite_OnInPlaceDeactivate(this *com.This) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceSite_DiscardUndoState(this *com.This) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceSite_DeactivateAndUndo(this *com.This) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceSite_OnPosRectChange(this *com.This, lprcPosRect *win.RECT) uintptr {
	browserObject := webViewFromThis(this).browserObject
	var inPlaceObjectPtr unsafe.Pointer
	if hr := browserObject.QueryInterface(&win.IID_IOleInPlaceObject, &inPlaceObjectPtr); win.FAILED(hr) {
		return uintptr(hr)
	}
	inPlaceObject := (*win.IOleInPlaceObject)(inPlaceObjectPtr)
	defer inPlaceObject.Release()

	return uintptr(inPlaceObject.SetObjectRects(lprcPosRect, lprcPosRect))
}
