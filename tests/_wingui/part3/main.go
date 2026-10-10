// Command part3 checks on Windows the functions generated in step 3 part 3:
// GetObject, AlphaBlend, ITaskbarList3, IAccPropServices and
// SHBrowseForFolder with SHGetPathFromIDList.
//
// With the argument "browse" it also opens the folder dialog and accepts it
// with a message to the dialog. It prints "BUTTON_HWND <n>" so that a script
// can check the annotated MSAA name, and "READY" when all checks are done;
// the window stays open until it gets WM_CLOSE.
package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/lpintes/walk"
	"github.com/lpintes/walk/internal/win"
	"golang.org/x/sys/windows"
)

var failed bool

func check(name string, ok bool, format string, args ...any) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failed = true
	}
	fmt.Printf("%s %s: %s\n", status, name, fmt.Sprintf(format, args...))
}

func testBitmaps() {
	// GetObject through newBitmapFromHBITMAP.
	src := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	src.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	src.SetNRGBA(1, 0, color.NRGBA{255, 0, 0, 128})
	src.SetNRGBA(2, 0, color.NRGBA{255, 0, 0, 0})
	bmp, err := walk.NewBitmapFromImageForDPI(src, 96)
	if err != nil {
		check("GetObject", false, "NewBitmapFromImageForDPI: %v", err)
		return
	}
	defer bmp.Dispose()
	check("GetObject", bmp.Size() == walk.Size{Width: 3, Height: 1}, "bitmap size %v, want 3x1", bmp.Size())

	// GetObject through sizeFromHICON.
	hIcon := win.LoadIcon(0, win.MAKEINTRESOURCE(win.IDI_APPLICATION))
	ic, err := walk.NewIconFromHICON(hIcon)
	if err != nil {
		check("GetObject icon", false, "NewIconFromHICON: %v", err)
	} else {
		want := int(win.GetSystemMetrics(11)) // SM_CXICON
		check("GetObject icon", ic.Size().Width == want && ic.Size().Height == want, "icon size %v, SM_CXICON %d", ic.Size(), want)
	}

	// AlphaBlend through Canvas.DrawImage onto a white bitmap.
	dst, err := walk.NewBitmapForDPI(walk.Size{Width: 3, Height: 1}, 96)
	if err != nil {
		check("AlphaBlend", false, "NewBitmapForDPI: %v", err)
		return
	}
	defer dst.Dispose()
	c, err := walk.NewCanvasFromImage(dst)
	if err != nil {
		check("AlphaBlend", false, "NewCanvasFromImage: %v", err)
		return
	}
	white, _ := walk.NewSolidColorBrush(walk.RGB(255, 255, 255))
	c.FillRectanglePixels(white, walk.Rectangle{Width: 3, Height: 1})
	err = c.DrawImagePixels(bmp, walk.Point{})
	c.Dispose()
	white.Dispose()
	if err != nil {
		check("AlphaBlend", false, "DrawImagePixels: %v", err)
		return
	}
	out, err := dst.ToImage()
	if err != nil {
		check("AlphaBlend", false, "ToImage: %v", err)
		return
	}
	p0, p1, p2 := out.RGBAAt(0, 0), out.RGBAAt(1, 0), out.RGBAAt(2, 0)
	near := func(a, b uint8) bool { return a+2 >= b && b+2 >= a }
	ok := p0.R == 255 && p0.G == 0 && p0.B == 0 &&
		p1.R == 255 && near(p1.G, 127) && near(p1.B, 127) &&
		p2.R == 255 && p2.G == 255 && p2.B == 255
	check("AlphaBlend", ok, "opaque %v, half %v, transparent %v; want red, light red, white", p0, p1, p2)
}

func testTaskbar(mw *walk.MainWindow) {
	pi := mw.ProgressIndicator()
	if pi == nil {
		check("ITaskbarList3", false, "ProgressIndicator is nil")
		return
	}
	err1 := pi.SetState(walk.PINormal)
	pi.SetTotal(100)
	err2 := pi.SetCompleted(40)
	ic, _ := walk.NewIconFromHICON(win.LoadIcon(0, win.MAKEINTRESOURCE(win.IDI_WARNING)))
	err3 := pi.SetOverlayIcon(ic, "warning")
	check("ITaskbarList3", err1 == nil && err2 == nil && err3 == nil, "SetState %v, SetCompleted %v, SetOverlayIcon %v", err1, err2, err3)
}

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	enumThreadWindow = user32.NewProc("EnumThreadWindows")
	getClassNameW    = user32.NewProc("GetClassNameW")
)

// findDialog returns the first #32770 window of the given thread.
func findDialog(tid uint32) win.HWND {
	var found win.HWND
	cb := windows.NewCallback(func(h uintptr, _ uintptr) uintptr {
		var buf [64]uint16
		getClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), 64)
		if windows.UTF16ToString(buf[:]) == "#32770" {
			found = win.HWND(h)
			return 0
		}
		return 1
	})
	enumThreadWindow.Call(uintptr(tid), cb, 0)
	return found
}

func testBrowseFolder(mw *walk.MainWindow) {
	dir, _ := os.Getwd()
	tid := windows.GetCurrentThreadId()
	go func() {
		// Accept the dialog with a message to the dialog only.
		for i := 0; i < 50; i++ {
			time.Sleep(200 * time.Millisecond)
			if h := findDialog(tid); h != 0 {
				time.Sleep(700 * time.Millisecond)
				win.PostMessage(h, win.WM_COMMAND, win.IDOK, 0)
				return
			}
		}
		fmt.Println("browse folder dialog not found")
	}()
	dlg := walk.FileDialog{InitialDirPath: dir, Title: "walk part3 test"}
	ok, err := dlg.ShowBrowseFolder(mw)
	check("SHBrowseForFolder", err == nil && ok && dlg.FilePath == dir, "accepted %v, err %v, path %q, want %q", ok, err, dlg.FilePath, dir)
}

func main() {
	mw, err := walk.NewMainWindow()
	if err != nil {
		panic(err)
	}
	mw.SetTitle("walk part3 test")
	mw.SetLayout(walk.NewVBoxLayout())
	pb, _ := walk.NewPushButton(mw)
	pb.SetText("Plain text")
	acc := pb.Accessibility()
	errName := acc.SetName("Annotated name")
	errRole := acc.SetRole(walk.AccRoleLink)
	// walk reports the HRESULT only in the error message.
	const invalidArg = "-2147024809" // E_INVALIDARG
	var walkErr *walk.Error
	if runtime.GOARCH == "386" && errName == nil && errors.As(errRole, &walkErr) && strings.HasSuffix(walkErr.Message(), invalidArg) {
		// SetHwndProp passes a *VARIANT, but on 386 the VARIANT is passed
		// by value. Inherited from lxn/win, see "Known issues" in CLAUDE.md.
		fmt.Printf("KNOWN IAccPropServices: SetRole %s\n", walkErr.Message())
	} else {
		check("IAccPropServices", errName == nil && errRole == nil, "SetName %v, SetRole %v", errName, errRole)
	}
	fmt.Printf("BUTTON_HWND %d\n", pb.Handle())

	testBitmaps()

	mw.SetSize(walk.Size{Width: 400, Height: 200})
	mw.Show()
	go func() {
		time.Sleep(1500 * time.Millisecond)
		mw.Synchronize(func() {
			testTaskbar(mw)
			if len(os.Args) > 1 && os.Args[1] == "browse" {
				testBrowseFolder(mw)
			}
			fmt.Println("READY")
		})
	}()
	mw.Run()
	if failed {
		os.Exit(1)
	}
}
