// Command hdn checks on Windows which header notification makes a
// TableView with a frozen column call updateLVSizes, and that the frozen and
// the normal list view stay aligned after column width changes.
// Task 3 of TESTING_ON_WINDOWS.md.
//
// It needs trace hooks in walk: apply trace.patch from this directory
// before building and revert it afterwards (see ../README.md). With the
// argument "manual" the window stays open after the checks.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/lpintes/walk"
	"github.com/lpintes/walk/internal/win"
)

type row struct {
	A, B, C string
}

var (
	failed bool
	events []string
)

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Printf("PASS %s\n", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
}

func rect(hwnd win.HWND) win.RECT {
	var rc win.RECT
	win.GetWindowRect(hwnd, &rc)
	return rc
}

func makeLParam(x, y int32) uintptr {
	return uintptr(uint32(uint16(x)) | uint32(uint16(y))<<16)
}

func main() {
	walk.HDNTrace = func(format string, args ...any) {
		s := fmt.Sprintf(format, args...)
		events = append(events, s)
		fmt.Println("  trace:", s)
	}

	mw, err := walk.NewMainWindow()
	if err != nil {
		panic(err)
	}
	mw.SetTitle("hdn test")
	mw.SetLayout(walk.NewVBoxLayout())
	mw.SetSize(walk.Size{Width: 600, Height: 300})

	tv, err := walk.NewTableView(mw)
	if err != nil {
		panic(err)
	}
	for i, name := range []string{"A", "B", "C"} {
		col := walk.NewTableViewColumn()
		col.SetDataMember(name)
		col.SetTitle("Column " + name)
		col.SetWidth(100)
		if i == 0 {
			col.SetFrozen(true)
		}
		if err := tv.Columns().Add(col); err != nil {
			panic(err)
		}
	}
	var rows []*row
	for i := 0; i < 20; i++ {
		rows = append(rows, &row{fmt.Sprint("a", i), fmt.Sprint("b", i), fmt.Sprint("c", i)})
	}
	if err := tv.SetModel(rows); err != nil {
		panic(err)
	}

	f, n, fh, nh := walk.TableViewHandles(tv)
	frozenLV, normalLV, frozenHdr, normalHdr := win.HWND(f), win.HWND(n), win.HWND(fh), win.HWND(nh)

	aligned := func(step string) {
		fr, nr := rect(frozenLV), rect(normalLV)
		cw := int32(win.SendMessage(frozenLV, win.LVM_GETCOLUMNWIDTH, 0, 0))
		fmt.Printf("  frozen LV %d..%d (width %d), normal LV %d..%d, frozen column width %d\n",
			fr.Left, fr.Right, fr.Right-fr.Left, nr.Left, nr.Right, cw)
		// walk converts the width to 96 DPI and back, so allow 1 pixel.
		d := fr.Right - fr.Left - cw
		check(step+" aligned", fr.Right == nr.Left && d >= -1 && d <= 1,
			"frozen right %d, normal left %d, frozen width %d, column width %d",
			fr.Right, nr.Left, fr.Right-fr.Left, cw)
	}

	// expect checks that the action caused exactly one updateLVSizes and
	// that it came right after HDN_ITEMCHANGEDW (0xFFFFFEBF).
	expect := func(step string) {
		var calls int
		var after []string
		for i, e := range events {
			if e == "updateLVSizes" {
				calls++
				if i > 0 {
					after = append(after, events[i-1])
				}
			}
		}
		ok := calls == 1 && len(after) == 1 && strings.HasSuffix(after[0], "code=0xFFFFFEBF")
		check(step+" updateLVSizes once after HDN_ITEMCHANGEDW", ok, "calls %d, preceded by %q", calls, after)
		aligned(step)
		events = nil
	}

	drag := func(hdr win.HWND, dx int32) {
		var rc win.RECT
		win.SendMessage(hdr, win.HDM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&rc)))
		x, y := rc.Right-1, (rc.Top+rc.Bottom)/2
		win.SendMessage(hdr, win.WM_MOUSEMOVE, 0, makeLParam(x, y))
		win.SendMessage(hdr, win.WM_LBUTTONDOWN, win.MK_LBUTTON, makeLParam(x, y))
		win.SendMessage(hdr, win.WM_MOUSEMOVE, win.MK_LBUTTON, makeLParam(x+dx/2, y))
		win.SendMessage(hdr, win.WM_MOUSEMOVE, win.MK_LBUTTON, makeLParam(x+dx, y))
		win.SendMessage(hdr, win.WM_LBUTTONUP, 0, makeLParam(x+dx, y))
	}

	steps := []struct {
		name string
		do   func()
	}{
		{"initial", func() {}},
		{"LVM_SETCOLUMNWIDTH frozen", func() { win.SendMessage(frozenLV, win.LVM_SETCOLUMNWIDTH, 0, 180) }},
		{"LVM_SETCOLUMNWIDTH normal", func() { win.SendMessage(normalLV, win.LVM_SETCOLUMNWIDTH, 0, 150) }},
		{"drag frozen header", func() { drag(frozenHdr, 40) }},
		{"drag normal header", func() { drag(normalHdr, 30) }},
	}

	mw.SetVisible(true)

	go func() {
		time.Sleep(time.Second)
		for i, s := range steps {
			done := make(chan struct{})
			mw.Synchronize(func() {
				fmt.Println("STEP", s.name)
				if i == 0 {
					events = nil
					aligned(s.name)
				} else {
					s.do()
				}
				close(done)
			})
			<-done
			time.Sleep(300 * time.Millisecond)
			if i > 0 {
				done = make(chan struct{})
				mw.Synchronize(func() {
					expect(s.name)
					close(done)
				})
				<-done
			}
		}
		if len(os.Args) > 1 && os.Args[1] == "manual" {
			return
		}
		mw.Synchronize(func() { mw.Close() })
	}()

	mw.Run()

	if failed {
		os.Exit(1)
	}
}
