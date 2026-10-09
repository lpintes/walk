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

// Error codes
const (
	ERROR_INVALID_FUNCTION    = 1
	ERROR_FILE_NOT_FOUND      = 2
	ERROR_INSUFFICIENT_BUFFER = 122
	ERROR_MORE_DATA           = 234
)

// GlobalAlloc flags
const (
	GMEM_ZEROINIT = 0x0040
	GPTR          = GMEM_FIXED | GMEM_ZEROINIT
)

// Predefined locale ids
const (
	LOCALE_CUSTOM_DEFAULT     LCID = 0x0c00
	LOCALE_CUSTOM_UI_DEFAULT  LCID = 0x1400
	LOCALE_CUSTOM_UNSPECIFIED LCID = 0x1000
	LOCALE_INVARIANT          LCID = 0x007f
	LOCALE_SYSTEM_DEFAULT     LCID = 0x0800
)

// LCTYPE constants
const (
	LOCALE_SISO3166CTRYNAME  LCTYPE = 0x5a
	LOCALE_SISO3166CTRYNAME2 LCTYPE = 0x68
	LOCALE_SISO639LANGNAME   LCTYPE = 0x59
	LOCALE_SISO639LANGNAME2  LCTYPE = 0x67
)

var (

	// Functions
	activateActCtx                     *windows.LazyProc
	closeHandle                        *windows.LazyProc
	createActCtx                       *windows.LazyProc
	fileTimeToSystemTime               *windows.LazyProc
	getConsoleTitle                    *windows.LazyProc
	getConsoleWindow                   *windows.LazyProc
	getNumberFormat                    *windows.LazyProc
	getPhysicallyInstalledSystemMemory *windows.LazyProc
	getProfileString                   *windows.LazyProc
	getThreadLocale                    *windows.LazyProc
	getThreadUILanguage                *windows.LazyProc
	globalLock                         *windows.LazyProc
	moveMemory                         *windows.LazyProc
	lockResource                       *windows.LazyProc
	systemTimeToFileTime               *windows.LazyProc
)

type (
	ATOM          uint16
	HANDLE        uintptr
	HGLOBAL       HANDLE
	HINSTANCE     HANDLE
	LCID          uint32
	LCTYPE        uint32
	LANGID        uint16
	HMODULE       uintptr
	HWINEVENTHOOK HANDLE
	HRSRC         uintptr
)

type FILETIME struct {
	DwLowDateTime  uint32
	DwHighDateTime uint32
}

type NUMBERFMT struct {
	NumDigits     uint32
	LeadingZero   uint32
	Grouping      uint32
	LpDecimalSep  *uint16
	LpThousandSep *uint16
	NegativeOrder uint32
}

type SYSTEMTIME struct {
	WYear         uint16
	WMonth        uint16
	WDayOfWeek    uint16
	WDay          uint16
	WHour         uint16
	WMinute       uint16
	WSecond       uint16
	WMilliseconds uint16
}

type ACTCTX struct {
	size                  uint32
	Flags                 uint32
	Source                *uint16 // UTF-16 string
	ProcessorArchitecture uint16
	LangID                uint16
	AssemblyDirectory     *uint16 // UTF-16 string
	ResourceName          *uint16 // UTF-16 string
	ApplicationName       *uint16 // UTF-16 string
	Module                HMODULE
}

func init() {
	// Functions
	activateActCtx = libkernel32.NewProc("ActivateActCtx")
	closeHandle = libkernel32.NewProc("CloseHandle")
	createActCtx = libkernel32.NewProc("CreateActCtxW")
	fileTimeToSystemTime = libkernel32.NewProc("FileTimeToSystemTime")
	getConsoleTitle = libkernel32.NewProc("GetConsoleTitleW")
	getConsoleWindow = libkernel32.NewProc("GetConsoleWindow")
	getNumberFormat = libkernel32.NewProc("GetNumberFormatW")
	getPhysicallyInstalledSystemMemory = libkernel32.NewProc("GetPhysicallyInstalledSystemMemory")
	getProfileString = libkernel32.NewProc("GetProfileStringW")
	getThreadLocale = libkernel32.NewProc("GetThreadLocale")
	getThreadUILanguage = libkernel32.NewProc("GetThreadUILanguage")
	globalLock = libkernel32.NewProc("GlobalLock")
	moveMemory = libkernel32.NewProc("RtlMoveMemory")
	lockResource = libkernel32.NewProc("LockResource")
	systemTimeToFileTime = libkernel32.NewProc("SystemTimeToFileTime")
}

func ActivateActCtx(ctx HANDLE) (uintptr, bool) {
	var cookie uintptr
	ret, _, _ := syscall.Syscall(activateActCtx.Addr(), 2,
		uintptr(ctx),
		uintptr(unsafe.Pointer(&cookie)),
		0)
	return cookie, ret != 0
}

func CloseHandle(hObject HANDLE) bool {
	ret, _, _ := syscall.Syscall(closeHandle.Addr(), 1,
		uintptr(hObject),
		0,
		0)

	return ret != 0
}

func CreateActCtx(ctx *ACTCTX) HANDLE {
	if ctx != nil {
		ctx.size = uint32(unsafe.Sizeof(*ctx))
	}
	ret, _, _ := syscall.Syscall(
		createActCtx.Addr(),
		1,
		uintptr(unsafe.Pointer(ctx)),
		0,
		0)
	return HANDLE(ret)
}

func FileTimeToSystemTime(lpFileTime *FILETIME, lpSystemTime *SYSTEMTIME) bool {
	ret, _, _ := syscall.Syscall(fileTimeToSystemTime.Addr(), 2,
		uintptr(unsafe.Pointer(lpFileTime)),
		uintptr(unsafe.Pointer(lpSystemTime)),
		0)

	return ret != 0
}

func GetConsoleTitle(lpConsoleTitle *uint16, nSize uint32) uint32 {
	ret, _, _ := syscall.Syscall(getConsoleTitle.Addr(), 2,
		uintptr(unsafe.Pointer(lpConsoleTitle)),
		uintptr(nSize),
		0)

	return uint32(ret)
}

func GetConsoleWindow() HWND {
	ret, _, _ := syscall.Syscall(getConsoleWindow.Addr(), 0,
		0,
		0,
		0)

	return HWND(ret)
}

func GetNumberFormat(Locale LCID, dwFlags uint32, lpValue *uint16, lpFormat *NUMBERFMT, lpNumberStr *uint16, cchNumber int32) int32 {
	ret, _, _ := syscall.Syscall6(getNumberFormat.Addr(), 6,
		uintptr(Locale),
		uintptr(dwFlags),
		uintptr(unsafe.Pointer(lpValue)),
		uintptr(unsafe.Pointer(lpFormat)),
		uintptr(unsafe.Pointer(lpNumberStr)),
		uintptr(cchNumber))

	return int32(ret)
}

func GetPhysicallyInstalledSystemMemory(totalMemoryInKilobytes *uint64) bool {
	if getPhysicallyInstalledSystemMemory.Find() != nil {
		return false
	}
	ret, _, _ := syscall.Syscall(getPhysicallyInstalledSystemMemory.Addr(), 1,
		uintptr(unsafe.Pointer(totalMemoryInKilobytes)),
		0,
		0)

	return ret != 0
}

func GetProfileString(lpAppName, lpKeyName, lpDefault *uint16, lpReturnedString uintptr, nSize uint32) bool {
	ret, _, _ := syscall.Syscall6(getProfileString.Addr(), 5,
		uintptr(unsafe.Pointer(lpAppName)),
		uintptr(unsafe.Pointer(lpKeyName)),
		uintptr(unsafe.Pointer(lpDefault)),
		lpReturnedString,
		uintptr(nSize),
		0)
	return ret != 0
}

func GetThreadLocale() LCID {
	ret, _, _ := syscall.Syscall(getThreadLocale.Addr(), 0,
		0,
		0,
		0)

	return LCID(ret)
}

func GetThreadUILanguage() LANGID {
	if getThreadUILanguage.Find() != nil {
		return 0
	}

	ret, _, _ := syscall.Syscall(getThreadUILanguage.Addr(), 0,
		0,
		0,
		0)

	return LANGID(ret)
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

func SystemTimeToFileTime(lpSystemTime *SYSTEMTIME, lpFileTime *FILETIME) bool {
	ret, _, _ := syscall.Syscall(systemTimeToFileTime.Addr(), 2,
		uintptr(unsafe.Pointer(lpSystemTime)),
		uintptr(unsafe.Pointer(lpFileTime)),
		0)

	return ret != 0
}
