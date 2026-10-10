// Command dragfinish checks on Windows that walk frees the HDROP of
// WM_DROPFILES with DragFinish. It builds an HDROP itself, posts
// WM_DROPFILES to a walk MainWindow, checks that the DropFiles handler gets
// the paths and then asks GlobalFlags whether the handle was freed.
//
// With the argument "manual" it only opens a window that prints the files
// dropped on it, for a real drag from Explorer.
package main

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/lpintes/walk"
	"github.com/lpintes/walk/internal/win"
	"golang.org/x/sys/windows"
)

const (
	gmemZeroInit      = 0x40
	gmemInvalidHandle = 0x8000
)

// dropFiles is the DROPFILES struct of shlobj_core.h.
type dropFiles struct {
	pFiles uint32
	pt     win.POINT
	fNC    int32
	fWide  int32
}

var procGlobalFlags = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFlags")

func globalFlags(h win.HGLOBAL) uint32 {
	r, _, _ := procGlobalFlags.Call(uintptr(h))
	return uint32(r)
}

var failed bool

func check(name string, ok bool, format string, args ...any) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failed = true
	}
	fmt.Printf("%s %s: %s\n", status, name, fmt.Sprintf(format, args...))
}

func newHDROP(paths []string) win.HGLOBAL {
	var list []uint16
	for _, p := range paths {
		list = append(list, windows.StringToUTF16(p)...)
	}
	list = append(list, 0)

	hdr := uint32(unsafe.Sizeof(dropFiles{}))
	size := uintptr(hdr) + uintptr(len(list))*2
	h := win.GlobalAlloc(win.GMEM_MOVEABLE|gmemZeroInit, size)
	if h == 0 {
		return 0
	}
	p := win.GlobalLock(h)
	*(*dropFiles)(p) = dropFiles{pFiles: hdr, fWide: 1}
	copy(unsafe.Slice((*uint16)(unsafe.Add(p, hdr)), len(list)), list)
	win.GlobalUnlock(h)
	return h
}

func main() {
	// The files need not exist, walk only passes the paths on.
	paths := []string{`C:\tmp\first file.txt`, `C:\tmp\žltý kôň.txt`}

	mw, err := walk.NewMainWindow()
	if err != nil {
		fmt.Println("FAIL NewMainWindow:", err)
		os.Exit(1)
	}
	mw.SetTitle("dragfinish test")
	// Without a layout SetVisible panics in ContainerBase.CreateLayoutItem.
	mw.SetLayout(walk.NewVBoxLayout())

	if len(os.Args) > 1 && os.Args[1] == "manual" {
		// For a real drag from Explorer: print what arrives.
		mw.DropFiles().Attach(func(files []string) {
			fmt.Printf("DROP %q\n", files)
			mw.SetTitle(fmt.Sprintf("dropped %d: %v", len(files), files))
		})
		mw.SetVisible(true)
		mw.Run()
		return
	}

	var hDrop win.HGLOBAL
	got := make(chan []string, 1)
	mw.DropFiles().Attach(func(files []string) {
		got <- files
	})

	hDrop = newHDROP(paths)
	if hDrop == 0 {
		fmt.Println("FAIL GlobalAlloc")
		os.Exit(1)
	}
	fmt.Printf("hDrop flags before: %#x\n", globalFlags(hDrop))

	go func() {
		select {
		case files := <-got:
			ok := len(files) == len(paths)
			for i := 0; ok && i < len(files); i++ {
				ok = files[i] == paths[i]
			}
			check("paths", ok, "got %q", files)
		case <-time.After(5 * time.Second):
			check("paths", false, "handler not called")
		}
		mw.Synchronize(func() {
			flags := globalFlags(hDrop)
			check("freed", flags == gmemInvalidHandle, "GlobalFlags = %#x, want %#x", flags, gmemInvalidHandle)
			mw.Close()
		})
	}()

	win.PostMessage(mw.Handle(), win.WM_DROPFILES, uintptr(hDrop), 0)
	mw.Run()

	if failed {
		os.Exit(1)
	}
}
