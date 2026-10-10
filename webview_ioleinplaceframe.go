// Copyright 2010 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"syscall"
)

import (
	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
)

var webViewIOleInPlaceFrameVtbl *com.VTable

func init() {
	AppendToWalkInit(func() {
		webViewIOleInPlaceFrameVtbl = com.NewVTable(&win.IOleInPlaceFrameVtbl{
			GetWindow:            syscall.NewCallback(webView_IOleInPlaceFrame_GetWindow),
			ContextSensitiveHelp: syscall.NewCallback(webView_IOleInPlaceFrame_ContextSensitiveHelp),
			GetBorder:            syscall.NewCallback(webView_IOleInPlaceFrame_GetBorder),
			RequestBorderSpace:   syscall.NewCallback(webView_IOleInPlaceFrame_RequestBorderSpace),
			SetBorderSpace:       syscall.NewCallback(webView_IOleInPlaceFrame_SetBorderSpace),
			SetActiveObject:      syscall.NewCallback(webView_IOleInPlaceFrame_SetActiveObject),
			InsertMenus:          syscall.NewCallback(webView_IOleInPlaceFrame_InsertMenus),
			SetMenu:              syscall.NewCallback(webView_IOleInPlaceFrame_SetMenu),
			RemoveMenus:          syscall.NewCallback(webView_IOleInPlaceFrame_RemoveMenus),
			SetStatusText:        syscall.NewCallback(webView_IOleInPlaceFrame_SetStatusText),
			EnableModeless:       syscall.NewCallback(webView_IOleInPlaceFrame_EnableModeless),
			TranslateAccelerator: syscall.NewCallback(webView_IOleInPlaceFrame_TranslateAccelerator),
		}, &win.IID_IOleWindow, &win.IID_IOleInPlaceUIWindow, &win.IID_IOleInPlaceFrame)
	})
}

// newWebViewFrame returns the COM object that IOleInPlaceSite.GetWindowContext
// hands to the browser of wv as its in-place frame.
func newWebViewFrame(wv *WebView) *com.Object {
	return com.NewObject(wv, webViewIOleInPlaceFrameVtbl)
}

func webView_IOleInPlaceFrame_GetWindow(this *com.This, lphwnd *win.HWND) uintptr {
	*lphwnd = webViewFromThis(this).hWnd

	return win.S_OK
}

func webView_IOleInPlaceFrame_ContextSensitiveHelp(this *com.This, fEnterMode win.BOOL) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceFrame_GetBorder(this *com.This, lprectBorder *win.RECT) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceFrame_RequestBorderSpace(this *com.This, pborderwidths uintptr) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceFrame_SetBorderSpace(this *com.This, pborderwidths uintptr) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceFrame_SetActiveObject(this *com.This, pActiveObject uintptr, pszObjName *uint16) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceFrame_InsertMenus(this *com.This, hmenuShared win.HMENU, lpMenuWidths uintptr) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceFrame_SetMenu(this *com.This, hmenuShared win.HMENU, holemenu win.HMENU, hwndActiveObject win.HWND) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceFrame_RemoveMenus(this *com.This, hmenuShared win.HMENU) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleInPlaceFrame_SetStatusText(this *com.This, pszStatusText *uint16) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceFrame_EnableModeless(this *com.This, fEnable win.BOOL) uintptr {
	return win.S_OK
}

func webView_IOleInPlaceFrame_TranslateAccelerator(this *com.This, lpmsg *win.MSG, wID uint32) uintptr {
	return win.E_NOTIMPL
}
