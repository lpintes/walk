// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (windows && 386) || (windows && arm)

package win

import (
	"syscall"
	"unsafe"
)

// The methods in this file take a POINT or a VARIANT by value, which the
// generator does not support. On 386 a struct passed by value is copied
// to the stack, so it is passed as one argument per 4 bytes.

func (obj *IUIAutomation) ElementFromPoint(pt POINT, element **IUIAutomationElement) HRESULT {
	r1, _, _ := syscall.SyscallN(obj.LpVtbl.ElementFromPoint,
		uintptr(unsafe.Pointer(obj)),
		uintptr(pt.X),
		uintptr(pt.Y),
		uintptr(unsafe.Pointer(element)))
	return HRESULT(r1)
}

func (obj *IUIAutomation) CreatePropertyCondition(propertyId int32, value *VARIANT, newCondition **IUIAutomationCondition) HRESULT {
	varA, varB, varC, varD := value.split()
	r1, _, _ := syscall.SyscallN(obj.LpVtbl.CreatePropertyCondition,
		uintptr(unsafe.Pointer(obj)),
		uintptr(propertyId),
		varA, varB, varC, varD,
		uintptr(unsafe.Pointer(newCondition)))
	return HRESULT(r1)
}

func (obj *IUIAutomation) CreatePropertyConditionEx(propertyId int32, value *VARIANT, flags int32, newCondition **IUIAutomationCondition) HRESULT {
	varA, varB, varC, varD := value.split()
	r1, _, _ := syscall.SyscallN(obj.LpVtbl.CreatePropertyConditionEx,
		uintptr(unsafe.Pointer(obj)),
		uintptr(propertyId),
		varA, varB, varC, varD,
		uintptr(flags),
		uintptr(unsafe.Pointer(newCondition)))
	return HRESULT(r1)
}
