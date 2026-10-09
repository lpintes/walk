// Copyright 2010 The win Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package win

import (
	"syscall"
	"unsafe"
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
