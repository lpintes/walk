// Command source drags two real files onto a window with OLE DoDragDrop,
// using the shell IDataObject that Explorer would provide.
// Usage: source <hwnd>. Only moves the mouse pointer (and restores it);
// sends no keyboard or mouse button input. Posts WM_CLOSE to <hwnd> at the end.
// The files are created in walk-olednd in the temporary directory, and their
// paths are printed as "FILES <paths>", in the format target prints "DROP".
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	pOleInit = ole32.NewProc("OleInitialize")
	pDoDrag  = ole32.NewProc("DoDragDrop")
	pParse   = shell32.NewProc("SHParseDisplayName")
	pBindPar = shell32.NewProc("SHBindToParent")
	pILFind  = shell32.NewProc("ILFindLastID")
	pGetCur  = user32.NewProc("GetCursorPos")
	pSetCur  = user32.NewProc("SetCursorPos")
	pWFP     = user32.NewProc("WindowFromPoint")
	pAnc     = user32.NewProc("GetAncestor")
	pClRect  = user32.NewProc("GetClientRect")
	pC2S     = user32.NewProc("ClientToScreen")
	pPost    = user32.NewProc("PostMessageW")

	pPostThread = user32.NewProc("PostThreadMessageW")
)

type point struct{ X, Y int32 }
type rect struct{ L, T, R, B int32 }

var (
	iidIUnknown     = windows.GUID{Data1: 0x00000000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDropSource  = windows.GUID{Data1: 0x00000121, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDataObject  = windows.GUID{Data1: 0x0000010e, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIShellFolder = windows.GUID{Data1: 0x000214E6, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

const (
	sOK               = 0
	eNoInterface      = 0x80004002
	dragdropSDrop     = 0x00040100
	dragdropSCancel   = 0x00040101
	dragdropSUseDefCr = 0x00040102
)

// dropSource is a minimal IDropSource.
type dropSource struct {
	vtbl *[5]uintptr
}

var (
	qcdCalls   int
	gfCalls    int
	lastEffect uint32
	lastKeys   uintptr
	vtbl       [5]uintptr
	ds         dropSource
)

func init() {
	vtbl[0] = syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
		g := (*windows.GUID)(unsafe.Pointer(riid))
		if *g == iidIUnknown || *g == iidIDropSource {
			*(*uintptr)(unsafe.Pointer(ppv)) = this
			return sOK
		}
		*(*uintptr)(unsafe.Pointer(ppv)) = 0
		return eNoInterface
	})
	vtbl[1] = syscall.NewCallback(func(this uintptr) uintptr { return 1 })
	vtbl[2] = syscall.NewCallback(func(this uintptr) uintptr { return 1 })
	vtbl[3] = syscall.NewCallback(func(this, escape, keys uintptr) uintptr {
		qcdCalls++
		lastKeys = keys
		if uint32(escape) != 0 {
			return dragdropSCancel
		}
		if qcdCalls < 20 {
			return sOK
		}
		return dragdropSDrop
	})
	vtbl[4] = syscall.NewCallback(func(this, effect uintptr) uintptr {
		gfCalls++
		lastEffect = uint32(effect)
		return dragdropSUseDefCr
	})
	ds.vtbl = &vtbl
}

func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vt := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vt + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func must(name string, hr uintptr) {
	if int32(hr) < 0 {
		fmt.Printf("FAIL %s: hr=%#x\n", name, uint32(hr))
		os.Exit(1)
	}
}

func main() {
	runtime.LockOSThread()
	if len(os.Args) < 2 {
		fmt.Println("usage: source <hwnd>")
		os.Exit(2)
	}
	h, err := strconv.ParseUint(os.Args[1], 10, 64)
	if err != nil {
		fmt.Println("bad hwnd:", err)
		os.Exit(2)
	}
	hwnd := uintptr(h)

	r, _, _ := pOleInit.Call(0)
	must("OleInitialize", r)

	dir := filepath.Join(os.TempDir(), "walk-olednd")
	names := []string{"a.txt", "žltý kôň.txt"}
	os.MkdirAll(dir, 0o755)
	var paths []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("test "+n+"\r\n"), 0o644); err != nil {
			fmt.Println("FAIL write:", err)
			os.Exit(1)
		}
		paths = append(paths, p)
	}
	fmt.Printf("FILES %q\n", paths)

	// Absolute PIDLs, then the parent IShellFolder and child IDs.
	var children []uintptr
	var folder uintptr
	for i, p := range paths {
		var pidl uintptr
		r, _, _ = pParse.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(p))), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0)
		must("SHParseDisplayName "+p, r)
		if i == 0 {
			var last uintptr
			r, _, _ = pBindPar.Call(pidl, uintptr(unsafe.Pointer(&iidIShellFolder)), uintptr(unsafe.Pointer(&folder)), uintptr(unsafe.Pointer(&last)))
			must("SHBindToParent", r)
			children = append(children, last)
		} else {
			last, _, _ := pILFind.Call(pidl)
			children = append(children, last)
		}
	}
	var dataObj uintptr
	// IShellFolder::GetUIObjectOf is vtable index 10.
	r = comCall(folder, 10, 0, uintptr(len(children)), uintptr(unsafe.Pointer(&children[0])),
		uintptr(unsafe.Pointer(&iidIDataObject)), 0, uintptr(unsafe.Pointer(&dataObj)))
	must("GetUIObjectOf", r)
	fmt.Printf("data object %#x\n", dataObj)

	var orig point
	pGetCur.Call(uintptr(unsafe.Pointer(&orig)))
	fmt.Printf("original cursor %d,%d\n", orig.X, orig.Y)
	defer func() {
		pSetCur.Call(uintptr(orig.X), uintptr(orig.Y))
		fmt.Printf("cursor restored to %d,%d\n", orig.X, orig.Y)
	}()

	var rc rect
	pClRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	pt := point{(rc.L + rc.R) / 2, (rc.T + rc.B) / 2}
	pC2S.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
	pSetCur.Call(uintptr(pt.X), uintptr(pt.Y))
	time.Sleep(200 * time.Millisecond)
	under, _, _ := pWFP.Call(uintptr(*(*uint64)(unsafe.Pointer(&pt))))
	root, _, _ := pAnc.Call(under, 2) // GA_ROOT
	fmt.Printf("cursor %d,%d WindowFromPoint=%#x root=%#x target=%#x match=%v\n",
		pt.X, pt.Y, under, root, hwnd, root == hwnd)

	// DoDragDrop's loop waits for mouse messages. Without real mouse input
	// none arrive, so post WM_MOUSEMOVE to this thread only (not global input).
	tid := windows.GetCurrentThreadId()
	stop := make(chan struct{})
	go func() {
		lp := uintptr(uint32(pt.Y)<<16 | uint32(pt.X)&0xffff)
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				pPostThread.Call(uintptr(tid), 0x0200, 0, lp) // WM_MOUSEMOVE
			}
		}
	}()
	// Watchdog: never leave the pointer moved or hang forever.
	go func() {
		time.Sleep(20 * time.Second)
		pSetCur.Call(uintptr(orig.X), uintptr(orig.Y))
		fmt.Printf("FAIL watchdog: DoDragDrop did not return; qcd=%d gf=%d; cursor restored to %d,%d\n",
			qcdCalls, gfCalls, orig.X, orig.Y)
		os.Exit(3)
	}()

	var effect uint32
	start := time.Now()
	r, _, _ = pDoDrag.Call(dataObj, uintptr(unsafe.Pointer(&ds)), 1|2|4, uintptr(unsafe.Pointer(&effect)))
	fmt.Printf("DoDragDrop hr=%#x effect=%d qcd=%d gf=%d lastFeedbackEffect=%d lastKeys=%#x took=%v\n",
		uint32(r), effect, qcdCalls, gfCalls, lastEffect, lastKeys, time.Since(start))
	close(stop)

	time.Sleep(1 * time.Second)
	pPost.Call(hwnd, 0x0010, 0, 0) // WM_CLOSE
	fmt.Println("WM_CLOSE posted")
	runtime.KeepAlive(&ds)
}
