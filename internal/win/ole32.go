// Copyright 2010 The win Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package win

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type IID syscall.GUID
type CLSID syscall.GUID
type REFIID *IID
type REFCLSID *CLSID

func EqualREFIID(a, b REFIID) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if a.Data1 != b.Data1 || a.Data2 != b.Data2 || a.Data3 != b.Data3 {
		return false
	}

	for i := 0; i < 8; i++ {
		if a.Data4[i] != b.Data4[i] {
			return false
		}
	}

	return true
}

func (cf *IClassFactory) Release() uint32 {
	ret, _, _ := syscall.Syscall(cf.LpVtbl.Release, 1,
		uintptr(unsafe.Pointer(cf)),
		0,
		0)

	return uint32(ret)
}

func (cf *IClassFactory) CreateInstance(pUnkOuter *IUnknown, riid REFIID, ppvObject *unsafe.Pointer) HRESULT {
	ret, _, _ := syscall.Syscall6(cf.LpVtbl.CreateInstance, 4,
		uintptr(unsafe.Pointer(cf)),
		uintptr(unsafe.Pointer(pUnkOuter)),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(ppvObject)),
		0,
		0)

	return HRESULT(ret)
}

func (cp *IConnectionPoint) Release() uint32 {
	ret, _, _ := syscall.Syscall(cp.LpVtbl.Release, 1,
		uintptr(unsafe.Pointer(cp)),
		0,
		0)

	return uint32(ret)
}

func (cp *IConnectionPoint) Advise(pUnkSink unsafe.Pointer, pdwCookie *uint32) HRESULT {
	ret, _, _ := syscall.Syscall(cp.LpVtbl.Advise, 3,
		uintptr(unsafe.Pointer(cp)),
		uintptr(pUnkSink),
		uintptr(unsafe.Pointer(pdwCookie)))

	return HRESULT(ret)
}

func (cp *IConnectionPoint) Unadvise(dwCookie uint32) HRESULT {
	ret, _, _ := syscall.Syscall(cp.LpVtbl.Unadvise, 2,
		uintptr(unsafe.Pointer(cp)),
		uintptr(dwCookie),
		0)

	return HRESULT(ret)
}

func (cpc *IConnectionPointContainer) Release() uint32 {
	ret, _, _ := syscall.Syscall(cpc.LpVtbl.Release, 1,
		uintptr(unsafe.Pointer(cpc)),
		0,
		0)

	return uint32(ret)
}

func (cpc *IConnectionPointContainer) FindConnectionPoint(riid REFIID, ppCP **IConnectionPoint) HRESULT {
	ret, _, _ := syscall.Syscall(cpc.LpVtbl.FindConnectionPoint, 3,
		uintptr(unsafe.Pointer(cpc)),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(ppCP)))

	return HRESULT(ret)
}

func (obj *IOleInPlaceObject) Release() uint32 {
	ret, _, _ := syscall.Syscall(obj.LpVtbl.Release, 1,
		uintptr(unsafe.Pointer(obj)),
		0,
		0)

	return uint32(ret)
}

func (obj *IOleInPlaceObject) SetObjectRects(lprcPosRect, lprcClipRect *RECT) HRESULT {
	ret, _, _ := syscall.Syscall(obj.LpVtbl.SetObjectRects, 3,
		uintptr(unsafe.Pointer(obj)),
		uintptr(unsafe.Pointer(lprcPosRect)),
		uintptr(unsafe.Pointer(lprcClipRect)))

	return HRESULT(ret)
}

func (obj *IOleObject) QueryInterface(riid REFIID, ppvObject *unsafe.Pointer) HRESULT {
	ret, _, _ := syscall.Syscall(obj.LpVtbl.QueryInterface, 3,
		uintptr(unsafe.Pointer(obj)),
		uintptr(unsafe.Pointer(riid)),
		uintptr(unsafe.Pointer(ppvObject)))

	return HRESULT(ret)
}

func (obj *IOleObject) Release() uint32 {
	ret, _, _ := syscall.Syscall(obj.LpVtbl.Release, 1,
		uintptr(unsafe.Pointer(obj)),
		0,
		0)

	return uint32(ret)
}

func (obj *IOleObject) SetClientSite(pClientSite *IOleClientSite) HRESULT {
	ret, _, _ := syscall.Syscall(obj.LpVtbl.SetClientSite, 2,
		uintptr(unsafe.Pointer(obj)),
		uintptr(unsafe.Pointer(pClientSite)),
		0)

	return HRESULT(ret)
}

func (obj *IOleObject) SetHostNames(szContainerApp, szContainerObj *uint16) HRESULT {
	ret, _, _ := syscall.Syscall(obj.LpVtbl.SetHostNames, 3,
		uintptr(unsafe.Pointer(obj)),
		uintptr(unsafe.Pointer(szContainerApp)),
		uintptr(unsafe.Pointer(szContainerObj)))

	return HRESULT(ret)
}

func (obj *IOleObject) Close(dwSaveOption uint32) HRESULT {
	ret, _, _ := syscall.Syscall(obj.LpVtbl.Close, 2,
		uintptr(unsafe.Pointer(obj)),
		uintptr(dwSaveOption),
		0)

	return HRESULT(ret)
}

func (obj *IOleObject) DoVerb(iVerb int32, lpmsg *MSG, pActiveSite *IOleClientSite, lindex int32, hwndParent HWND, lprcPosRect *RECT) HRESULT {
	ret, _, _ := syscall.Syscall9(obj.LpVtbl.DoVerb, 7,
		uintptr(unsafe.Pointer(obj)),
		uintptr(iVerb),
		uintptr(unsafe.Pointer(lpmsg)),
		uintptr(unsafe.Pointer(pActiveSite)),
		uintptr(lindex),
		uintptr(hwndParent),
		uintptr(unsafe.Pointer(lprcPosRect)),
		0,
		0)

	return HRESULT(ret)
}

type COAUTHIDENTITY struct {
	User           *uint16
	UserLength     uint32
	Domain         *uint16
	DomainLength   uint32
	Password       *uint16
	PasswordLength uint32
	Flags          uint32
}

type COAUTHINFO struct {
	dwAuthnSvc           uint32
	dwAuthzSvc           uint32
	pwszServerPrincName  *uint16
	dwAuthnLevel         uint32
	dwImpersonationLevel uint32
	pAuthIdentityData    *COAUTHIDENTITY
	dwCapabilities       uint32
}

type COSERVERINFO struct {
	dwReserved1 uint32
	pwszName    *uint16
	pAuthInfo   *COAUTHINFO
	dwReserved2 uint32
}

var (
	oleInitialize *windows.LazyProc
)

func init() {
	// Functions
	oleInitialize = libole32.NewProc("OleInitialize")
}

func OleInitialize() HRESULT {
	ret, _, _ := syscall.Syscall(oleInitialize.Addr(), 1, // WTF, why does 0 not work here?
		0,
		0,
		0)

	return HRESULT(ret)
}
