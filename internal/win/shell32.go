// Copyright 2010 The win Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package win

import (
	"golang.org/x/sys/windows"
	"syscall"
	"unsafe"
)

type CSIDL uint32
type HDROP HANDLE

type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            HICON
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         syscall.GUID
	HBalloonIcon     HICON
}

type SHFILEINFO struct {
	HIcon         HICON
	IIcon         int32
	DwAttributes  uint32
	SzDisplayName [MAX_PATH]uint16
	SzTypeName    [80]uint16
}

var (

	// Functions
	dragAcceptFiles    *windows.LazyProc
	shParseDisplayName *windows.LazyProc
)

func init() {
	// Functions
	dragAcceptFiles = libshell32.NewProc("DragAcceptFiles")
	shParseDisplayName = libshell32.NewProc("SHParseDisplayName")
}

func DragAcceptFiles(hWnd HWND, fAccept bool) bool {
	ret, _, _ := syscall.Syscall(dragAcceptFiles.Addr(), 2,
		uintptr(hWnd),
		uintptr(BoolToBOOL(fAccept)),
		0)

	return ret != 0
}

func SHParseDisplayName(pszName *uint16, pbc uintptr, ppidl *uintptr, sfgaoIn uint32, psfgaoOut *uint32) HRESULT {
	ret, _, _ := syscall.Syscall6(shParseDisplayName.Addr(), 5,
		uintptr(unsafe.Pointer(pszName)),
		pbc,
		uintptr(unsafe.Pointer(ppidl)),
		0,
		uintptr(unsafe.Pointer(psfgaoOut)),
		0)

	return HRESULT(ret)
}
