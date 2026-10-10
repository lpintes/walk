// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package com

import (
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"github.com/lpintes/walk/internal/win"
)

var (
	iidTest1 = testIID(1)
	iidTest2 = testIID(2)
	iidOther = testIID(3)
)

func testIID(n byte) win.IID {
	return win.IID{
		Data1: 0x6d1c3f6e,
		Data2: 0x1f2a,
		Data3: 0x4b8e,
		Data4: [8]byte{0x9b, 0x51, 0x2e, 0x3f, 0x4a, 0x5b, 0x6c, n},
	}
}

type testImpl struct {
	count uint32
}

func test_GetTypeInfoCount(this *This, pctinfo *uint32) uintptr {
	*pctinfo = this.Object().Impl().(*testImpl).count
	return win.S_OK
}

func test_E_NOTIMPL(this *This) uintptr {
	return win.E_NOTIMPL
}

var (
	testVTablesOnce sync.Once
	testVTable1     *VTable
	testVTable2     *VTable
)

// testVTables returns two vtables, created once because callbacks are
// never freed.
func testVTables() (*VTable, *VTable) {
	testVTablesOnce.Do(func() {
		notImpl := syscall.NewCallback(test_E_NOTIMPL)
		testVTable1 = NewVTable(&win.IDispatchVtbl{
			GetTypeInfoCount: syscall.NewCallback(test_GetTypeInfoCount),
			GetTypeInfo:      notImpl,
			GetIDsOfNames:    notImpl,
			Invoke:           notImpl,
		}, &iidTest1)
		testVTable2 = NewVTable(&win.IUnknownVtbl{}, &iidTest2)
	})
	return testVTable1, testVTable2
}

// call calls the method at index method of the vtable of the interface
// pointer this.
func call(this unsafe.Pointer, method int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(this)
	fn := *(*uintptr)(unsafe.Add(vtbl, method*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(this)}, args...)...)
	return r
}

const (
	methodQueryInterface   = 0
	methodAddRef           = 1
	methodRelease          = 2
	methodGetTypeInfoCount = 3
)

func callQueryInterface(t *testing.T, this unsafe.Pointer, iid *win.IID) (unsafe.Pointer, uintptr) {
	t.Helper()
	var p unsafe.Pointer
	hr := call(this, methodQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&p)))
	return p, hr
}

func TestObject(t *testing.T) {
	vt1, vt2 := testVTables()
	impl := &testImpl{count: 42}
	o := NewObject(impl, vt1, vt2)

	if o.Impl() != impl {
		t.Fatalf("Impl() = %v, want %v", o.Impl(), impl)
	}

	first := o.Interface(0)
	second := o.Interface(1)

	if p, hr := callQueryInterface(t, second, &win.IID_IUnknown); hr != win.S_OK || p != first {
		t.Errorf("QueryInterface(IID_IUnknown) = %p, %#x; want %p, S_OK", p, hr, first)
	}
	if p, hr := callQueryInterface(t, first, &iidTest2); hr != win.S_OK || p != second {
		t.Errorf("QueryInterface(iidTest2) = %p, %#x; want %p, S_OK", p, hr, second)
	}
	if p, hr := callQueryInterface(t, second, &iidTest1); hr != win.S_OK || p != first {
		t.Errorf("QueryInterface(iidTest1) = %p, %#x; want %p, S_OK", p, hr, first)
	}
	if p, hr := callQueryInterface(t, first, &iidOther); hr != win.E_NOINTERFACE || p != nil {
		t.Errorf("QueryInterface(iidOther) = %p, %#x; want nil, E_NOINTERFACE", p, hr)
	}
	if hr := call(first, methodQueryInterface, uintptr(unsafe.Pointer(&iidTest1)), 0); hr != win.E_POINTER {
		t.Errorf("QueryInterface with nil ppv = %#x, want E_POINTER", hr)
	}

	// 1 from NewObject plus 3 from the successful QueryInterface calls.
	if n := call(first, methodAddRef); n != 5 {
		t.Errorf("AddRef = %d, want 5", n)
	}
	for want := uintptr(4); want >= 1; want-- {
		if n := call(second, methodRelease); n != want {
			t.Errorf("Release = %d, want %d", n, want)
		}
	}

	var count uint32
	if hr := call(first, methodGetTypeInfoCount, uintptr(unsafe.Pointer(&count))); hr != win.S_OK || count != 42 {
		t.Errorf("GetTypeInfoCount = %d, %#x; want 42, S_OK", count, hr)
	}

	liveMutex.Lock()
	_, live := liveObjects[o]
	liveMutex.Unlock()
	if !live {
		t.Error("object with references is not in liveObjects")
	}

	if n := o.Release(); n != 0 {
		t.Errorf("last Release = %d, want 0", n)
	}

	liveMutex.Lock()
	_, live = liveObjects[o]
	liveMutex.Unlock()
	if live {
		t.Error("released object is still in liveObjects")
	}
}

func TestNewVTableMissingMethod(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewVTable with a missing method did not panic")
		}
	}()
	NewVTable(&win.IDispatchVtbl{GetTypeInfoCount: 1})
}

func TestNewVTableNotAVTable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewVTable with a struct that is not a vtable did not panic")
		}
	}()
	NewVTable(&win.RECT{})
}
