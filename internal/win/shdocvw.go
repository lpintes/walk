// Copyright 2010 The win Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package win

import (
	"syscall"
	"unsafe"
)

const (
	DOCHOSTUIDBLCLK_SHOWPROPERTIES = 1
	DOCHOSTUIDBLCLK_SHOWCODE       = 2
)

const (
	DOCHOSTUIFLAG_DIALOG                     = 0x1
	DOCHOSTUIFLAG_DISABLE_HELP_MENU          = 0x2
	DOCHOSTUIFLAG_SCROLL_NO                  = 0x8
	DOCHOSTUIFLAG_DISABLE_SCRIPT_INACTIVE    = 0x10
	DOCHOSTUIFLAG_OPENNEWWIN                 = 0x20
	DOCHOSTUIFLAG_DISABLE_OFFSCREEN          = 0x40
	DOCHOSTUIFLAG_FLAT_SCROLLBAR             = 0x80
	DOCHOSTUIFLAG_DIV_BLOCKDEFAULT           = 0x100
	DOCHOSTUIFLAG_ACTIVATE_CLIENTHIT_ONLY    = 0x200
	DOCHOSTUIFLAG_OVERRIDEBEHAVIORFACTORY    = 0x400
	DOCHOSTUIFLAG_CODEPAGELINKEDFONTS        = 0x800
	DOCHOSTUIFLAG_URL_ENCODING_DISABLE_UTF8  = 0x1000
	DOCHOSTUIFLAG_URL_ENCODING_ENABLE_UTF8   = 0x2000
	DOCHOSTUIFLAG_ENABLE_FORMS_AUTOCOMPLETE  = 0x4000
	DOCHOSTUIFLAG_ENABLE_INPLACE_NAVIGATION  = 0x10000
	DOCHOSTUIFLAG_IME_ENABLE_RECONVERSION    = 0x20000
	DOCHOSTUIFLAG_THEME                      = 0x40000
	DOCHOSTUIFLAG_NOTHEME                    = 0x80000
	DOCHOSTUIFLAG_NOPICS                     = 0x100000
	DOCHOSTUIFLAG_NO3DOUTERBORDER            = 0x200000
	DOCHOSTUIFLAG_DISABLE_EDIT_NS_FIXUP      = 0x400000
	DOCHOSTUIFLAG_LOCAL_MACHINE_ACCESS_CHECK = 0x800000
	DOCHOSTUIFLAG_DISABLE_UNTRUSTEDPROTOCOL  = 0x1000000
)

// BrowserNavConstants
const (
	NavOpenInNewWindow       = 0x1
	NavNoHistory             = 0x2
	NavNoReadFromCache       = 0x4
	NavNoWriteToCache        = 0x8
	NavAllowAutosearch       = 0x10
	NavBrowserBar            = 0x20
	NavHyperlink             = 0x40
	NavEnforceRestricted     = 0x80
	NavNewWindowsManaged     = 0x0100
	NavUntrustedForDownload  = 0x0200
	NavTrustedForActiveX     = 0x0400
	NavOpenInNewTab          = 0x0800
	NavOpenInBackgroundTab   = 0x1000
	NavKeepWordWheelText     = 0x2000
	NavVirtualTab            = 0x4000
	NavBlockRedirectsXDomain = 0x8000
	NavOpenNewForegroundTab  = 0x10000
)

func (wb2 *IWebBrowser2) QueryInterface(riid REFIID, ppvObject *unsafe.Pointer) HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.QueryInterface, 3,
		uintptr(unsafe.Pointer(wb2)),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(ppvObject)))

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Release() HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.Release, 1,
		uintptr(unsafe.Pointer(wb2)),
		0,
		0)

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Refresh() HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.Refresh, 1,
		uintptr(unsafe.Pointer(wb2)),
		0,
		0)

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Put_Left(Left int32) HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.Put_Left, 2,
		uintptr(unsafe.Pointer(wb2)),
		uintptr(Left),
		0)

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Put_Top(Top int32) HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.Put_Top, 2,
		uintptr(unsafe.Pointer(wb2)),
		uintptr(Top),
		0)

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Put_Width(Width int32) HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.Put_Width, 2,
		uintptr(unsafe.Pointer(wb2)),
		uintptr(Width),
		0)

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Put_Height(Height int32) HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.Put_Height, 2,
		uintptr(unsafe.Pointer(wb2)),
		uintptr(Height),
		0)

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Get_LocationURL(pbstrLocationURL **uint16 /*BSTR*/) HRESULT {
	ret, _, _ := syscall.Syscall(wb2.LpVtbl.Get_LocationURL, 2,
		uintptr(unsafe.Pointer(wb2)),
		uintptr(unsafe.Pointer(pbstrLocationURL)),
		0)

	return HRESULT(ret)
}

func (wb2 *IWebBrowser2) Navigate2(URL *VAR_BSTR, Flags *VAR_I4, TargetFrameName *VAR_BSTR, PostData unsafe.Pointer, Headers *VAR_BSTR) HRESULT {
	ret, _, _ := syscall.Syscall6(wb2.LpVtbl.Navigate2, 6,
		uintptr(unsafe.Pointer(wb2)),
		uintptr(unsafe.Pointer(URL)),
		uintptr(unsafe.Pointer(Flags)),
		uintptr(unsafe.Pointer(TargetFrameName)),
		uintptr(PostData),
		uintptr(unsafe.Pointer(Headers)))

	return HRESULT(ret)
}

func (activeObj *IOleInPlaceActiveObject) Release() HRESULT {
	ret, _, _ := syscall.Syscall(activeObj.LpVtbl.Release, 1,
		uintptr(unsafe.Pointer(activeObj)),
		0,
		0)

	return HRESULT(ret)
}

func (activeObj *IOleInPlaceActiveObject) GetWindow(hWndPtr *HWND) HRESULT {
	ret, _, _ := syscall.Syscall(activeObj.LpVtbl.GetWindow, 2,
		uintptr(unsafe.Pointer(activeObj)),
		uintptr(unsafe.Pointer(hWndPtr)),
		0)

	return HRESULT(ret)
}

func (activeObj *IOleInPlaceActiveObject) TranslateAccelerator(msg *MSG) HRESULT {
	ret, _, _ := syscall.Syscall(activeObj.LpVtbl.TranslateAccelerator, 2,
		uintptr(unsafe.Pointer(activeObj)),
		uintptr(unsafe.Pointer(msg)),
		0)

	return HRESULT(ret)
}
