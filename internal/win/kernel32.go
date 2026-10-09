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

var (
	globalLock   *windows.LazyProc
	moveMemory   *windows.LazyProc
	lockResource *windows.LazyProc
)

type (
	ATOM      uint16
	HANDLE    uintptr
	HGLOBAL   HANDLE
	HINSTANCE HANDLE
	LCID      uint32
	LCTYPE    uint32
	HMODULE   uintptr
	HRSRC     uintptr
)

func init() {
	// Functions
	globalLock = libkernel32.NewProc("GlobalLock")
	moveMemory = libkernel32.NewProc("RtlMoveMemory")
	lockResource = libkernel32.NewProc("LockResource")
}

func GlobalLock(hMem HGLOBAL) unsafe.Pointer {
	ret, _, _ := syscall.Syscall(globalLock.Addr(), 1,
		uintptr(hMem),
		0,
		0)

	return unsafe.Pointer(ret)
}

func MoveMemory(destination, source unsafe.Pointer, length uintptr) {
	syscall.Syscall(moveMemory.Addr(), 3,
		uintptr(unsafe.Pointer(destination)),
		uintptr(source),
		uintptr(length))
}

func LockResource(hResData HGLOBAL) uintptr {
	ret, _, _ := syscall.Syscall(lockResource.Addr(), 1,
		uintptr(hResData),
		0,
		0)

	return ret
}
