// Copyright 2012 The win Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package win

import (
	"syscall"
	"unsafe"
)

func (obj *ITaskbarList3) SetProgressState(hwnd HWND, state int) HRESULT {
	ret, _, _ := syscall.SyscallN(obj.LpVtbl.SetProgressState,
		uintptr(unsafe.Pointer(obj)),
		uintptr(hwnd),
		uintptr(state))
	return HRESULT(ret)
}

func (obj *ITaskbarList3) SetOverlayIcon(hwnd HWND, icon HICON, description *uint16) HRESULT {
	ret, _, _ := syscall.SyscallN(obj.LpVtbl.SetOverlayIcon,
		uintptr(unsafe.Pointer(obj)),
		uintptr(hwnd),
		uintptr(icon),
		uintptr(unsafe.Pointer(description)))
	return HRESULT(ret)
}
