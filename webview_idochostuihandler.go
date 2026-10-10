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

var webViewIDocHostUIHandlerVtbl *com.VTable

func init() {
	AppendToWalkInit(func() {
		webViewIDocHostUIHandlerVtbl = com.NewVTable(&win.IDocHostUIHandlerVtbl{
			ShowContextMenu:       syscall.NewCallback(webView_IDocHostUIHandler_ShowContextMenu),
			GetHostInfo:           syscall.NewCallback(webView_IDocHostUIHandler_GetHostInfo),
			ShowUI:                syscall.NewCallback(webView_IDocHostUIHandler_ShowUI),
			HideUI:                syscall.NewCallback(webView_IDocHostUIHandler_HideUI),
			UpdateUI:              syscall.NewCallback(webView_IDocHostUIHandler_UpdateUI),
			EnableModeless:        syscall.NewCallback(webView_IDocHostUIHandler_EnableModeless),
			OnDocWindowActivate:   syscall.NewCallback(webView_IDocHostUIHandler_OnDocWindowActivate),
			OnFrameWindowActivate: syscall.NewCallback(webView_IDocHostUIHandler_OnFrameWindowActivate),
			ResizeBorder:          syscall.NewCallback(webView_IDocHostUIHandler_ResizeBorder),
			TranslateAccelerator:  syscall.NewCallback(webView_IDocHostUIHandler_TranslateAccelerator),
			GetOptionKeyPath:      syscall.NewCallback(webView_IDocHostUIHandler_GetOptionKeyPath),
			GetDropTarget:         syscall.NewCallback(webView_IDocHostUIHandler_GetDropTarget),
			GetExternal:           syscall.NewCallback(webView_IDocHostUIHandler_GetExternal),
			TranslateUrl:          syscall.NewCallback(webView_IDocHostUIHandler_TranslateUrl),
			FilterDataObject:      syscall.NewCallback(webView_IDocHostUIHandler_FilterDataObject),
		}, &win.IID_IDocHostUIHandler)
	})
}

func webView_IDocHostUIHandler_ShowContextMenu(this *com.This, dwID uint32, ppt *win.POINT, pcmdtReserved *win.IUnknown, pdispReserved uintptr) uintptr {
	webView := webViewFromThis(this)

	// show context menu
	if webView.NativeContextMenuEnabled() {
		return win.S_FALSE
	}

	return win.S_OK
}

func webView_IDocHostUIHandler_GetHostInfo(this *com.This, pInfo *win.DOCHOSTUIINFO) uintptr {
	pInfo.CbSize = uint32(unsafe.Sizeof(*pInfo))
	pInfo.DwFlags = win.DOCHOSTUIFLAG_NO3DBORDER
	pInfo.DwDoubleClick = win.DOCHOSTUIDBLCLK_DEFAULT

	return win.S_OK
}

func webView_IDocHostUIHandler_ShowUI(this *com.This, dwID uint32, pActiveObject uintptr, pCommandTarget uintptr, pFrame *win.IOleInPlaceFrame, pDoc uintptr) uintptr {
	return win.S_OK
}

func webView_IDocHostUIHandler_HideUI(this *com.This) uintptr {
	return win.S_OK
}

func webView_IDocHostUIHandler_UpdateUI(this *com.This) uintptr {
	return win.S_OK
}

func webView_IDocHostUIHandler_EnableModeless(this *com.This, fEnable win.BOOL) uintptr {
	return win.S_OK
}

func webView_IDocHostUIHandler_OnDocWindowActivate(this *com.This, fActivate win.BOOL) uintptr {
	return win.S_OK
}

func webView_IDocHostUIHandler_OnFrameWindowActivate(this *com.This, fActivate win.BOOL) uintptr {
	return win.S_OK
}

func webView_IDocHostUIHandler_ResizeBorder(this *com.This, prcBorder *win.RECT, pUIWindow uintptr, fRameWindow win.BOOL) uintptr {
	return win.S_OK
}

func webView_IDocHostUIHandler_TranslateAccelerator(this *com.This, lpMsg *win.MSG, pguidCmdGroup *syscall.GUID, nCmdID uint) uintptr {
	return win.S_FALSE
}

func webView_IDocHostUIHandler_GetOptionKeyPath(this *com.This, pchKey *uint16, dw uint) uintptr {
	return win.S_FALSE
}

func webView_IDocHostUIHandler_GetDropTarget(this *com.This, pDropTarget uintptr, ppDropTarget *uintptr) uintptr {
	return win.S_FALSE
}

func webView_IDocHostUIHandler_GetExternal(this *com.This, ppDispatch *uintptr) uintptr {
	*ppDispatch = 0

	return win.S_FALSE
}

func webView_IDocHostUIHandler_TranslateUrl(this *com.This, dwTranslate uint32, pchURLIn *uint16, ppchURLOut **uint16) uintptr {
	*ppchURLOut = nil

	return win.S_FALSE
}

func webView_IDocHostUIHandler_FilterDataObject(this *com.This, pDO uintptr, ppDORet *uintptr) uintptr {
	*ppDORet = 0

	return win.S_FALSE
}
