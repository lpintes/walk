// Command ods checks on Windows how an owner drawn ListBox chooses the
// selected colors and theme state from DRAWITEMSTRUCT.ItemState.
// Task 4 of TESTING_ON_WINDOWS.md.
//
// It needs trace hooks in walk: apply trace.patch from this directory
// before building and revert it afterwards (see ../README.md). With the
// argument "manual" the window stays open after the checks.
//
// One bug is known and inherited: item 0 is drawn in the hot state at first,
// because ListBox starts with hoverIndex 0 instead of -1. Its checks print
// KNOWN instead of FAIL.
package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lpintes/walk"
	"github.com/lpintes/walk/internal/win"
)

var (
	failed bool
	events []string

	procGetPixel = syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel")
)

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Printf("PASS %s\n", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
}

// checkKnown is check for a known inherited bug: a failure prints KNOWN and
// does not fail the test.
func checkKnown(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Printf("PASS %s\n", name)
		return
	}
	fmt.Printf("KNOWN %s: %s\n", name, fmt.Sprintf(format, args...))
}

func makeLParam(x, y int32) uintptr {
	return uintptr(uint32(uint16(x)) | uint32(uint16(y))<<16)
}

type styler struct {
	items []string
}

func (s *styler) ItemHeightDependsOnWidth() bool { return false }
func (s *styler) DefaultItemHeight() int         { return 24 }
func (s *styler) ItemHeight(index, width int) int {
	return 24
}

func (s *styler) StyleItem(style *walk.ListItemStyle) {
	if style.Canvas() == nil {
		return
	}
	b := style.BoundsPixels()
	// Leave the first 20 pixels free of text, the test reads them.
	b.X += 20
	b.Width -= 20
	style.DrawText(s.items[style.Index()], b, walk.TextSingleLine|walk.TextVCenter)
}

func main() {
	walk.ODSTrace = func(format string, args ...any) {
		events = append(events, fmt.Sprintf(format, args...))
	}

	mw, err := walk.NewMainWindow()
	if err != nil {
		panic(err)
	}
	mw.SetTitle("ods test")
	mw.SetLayout(walk.NewVBoxLayout())
	mw.SetSize(walk.Size{Width: 400, Height: 400})

	var items []string
	for i := 0; i < 8; i++ {
		items = append(items, fmt.Sprint("Item ", i))
	}

	// Directly in the MainWindow the list box would send WM_DRAWITEM to the
	// form window, which does not forward it, so use a Composite like the
	// listbox_ownerdrawing example.
	comp, err := walk.NewComposite(mw)
	if err != nil {
		panic(err)
	}
	comp.SetLayout(walk.NewVBoxLayout())
	lb, err := walk.NewListBoxWithStyle(comp, win.LBS_OWNERDRAWVARIABLE)
	if err != nil {
		panic(err)
	}
	lb.SetItemStyler(&styler{items: items})
	if err := lb.SetModel(items); err != nil {
		panic(err)
	}
	pb, err := walk.NewPushButton(mw)
	if err != nil {
		panic(err)
	}
	pb.SetText("Other")

	hLB := lb.Handle()

	itemRect := func(i int) win.RECT {
		var rc win.RECT
		win.SendMessage(hLB, win.LB_GETITEMRECT, uintptr(i), uintptr(unsafe.Pointer(&rc)))
		return rc
	}
	pixel := func(i int) uint32 {
		rc := itemRect(i)
		hdc := win.GetDC(hLB)
		defer win.ReleaseDC(hLB, hdc)
		c, _, _ := procGetPixel.Call(uintptr(hdc), uintptr(rc.Left+4), uintptr((rc.Top+rc.Bottom)/2))
		return uint32(c)
	}

	window := win.GetSysColor(win.COLOR_WINDOW)
	var focusedSel, unfocusedSel uint32

	// verify checks the last WM_DRAWITEM and DrawBackground of every item
	// and the pixel colors against the expected selection and focus.
	verify := func(step string, sel int, focus bool) {
		var wr win.RECT
		win.GetWindowRect(hLB, &wr)
		ir := itemRect(sel)
		fmt.Printf("  list box %v, visible %v, item rect %v, count %d\n", wr, win.IsWindowVisible(hLB), ir, win.SendMessage(hLB, win.LB_GETCOUNT, 0, 0))
		fmt.Printf("  focus on list: %v, current index %d\n", win.GetFocus() == hLB, lb.CurrentIndex())
		for _, e := range events {
			fmt.Println("  trace:", e)
		}
		lastDraw := map[int]string{}
		lastBG := map[int]string{}
		for _, e := range events {
			var id, action int
			var state uint32
			var focused bool
			if n, _ := fmt.Sscanf(e, "draw id=%d action=%d state=%v focused=%v", &id, &action, &state, &focused); n == 4 {
				lastDraw[id] = e
			}
			var idx, sid int
			if n, _ := fmt.Sscanf(e, "background idx=%d stateID=%d", &idx, &sid); n == 2 {
				lastBG[idx] = e
			}
		}
		for i := range items {
			check := check
			if step == "focus list" && i == 0 {
				// hoverIndex starts at 0, so item 0 is hot at first.
				check = checkKnown
			}
			px := pixel(i)
			wantSel := i == sel
			if wantSel {
				check(fmt.Sprintf("%s item %d pixel selected", step, i), px != window, "pixel %#06x equals COLOR_WINDOW", px)
				if focus {
					focusedSel = px
				} else {
					unfocusedSel = px
				}
			} else {
				check(fmt.Sprintf("%s item %d pixel normal", step, i), px == window, "pixel %#06x, COLOR_WINDOW %#06x", px, window)
			}
			if bg, ok := lastBG[i]; ok {
				want := "stateID=1 "
				if wantSel && focus {
					want = "stateID=3 "
				} else if wantSel {
					want = "stateID=5 "
				}
				check(fmt.Sprintf("%s item %d theme state", step, i), strings.Contains(bg, want), "last %q, want %q", bg, want)
			}
		}
		events = nil
	}

	clickItem := func(i int) {
		rc := itemRect(i)
		x, y := rc.Left+30, (rc.Top+rc.Bottom)/2
		win.SendMessage(hLB, win.WM_MOUSEMOVE, 0, makeLParam(x, y))
		win.SendMessage(hLB, win.WM_LBUTTONDOWN, win.MK_LBUTTON, makeLParam(x, y))
		win.SendMessage(hLB, win.WM_LBUTTONUP, 0, makeLParam(x, y))
		win.SendMessage(hLB, win.WM_MOUSELEAVE, 0, 0)
	}
	key := func(vk uintptr) {
		win.SendMessage(hLB, win.WM_KEYDOWN, vk, 0)
		win.SendMessage(hLB, win.WM_KEYUP, vk, 0)
	}

	steps := []struct {
		name  string
		do    func()
		sel   int
		focus bool
	}{
		{"focus list", func() { win.SetFocus(hLB) }, -1, true},
		{"mouse click item 2", func() { clickItem(2) }, 2, true},
		{"key down", func() { key(win.VK_DOWN) }, 3, true},
		{"key down again", func() { key(win.VK_DOWN) }, 4, true},
		{"focus away", func() { win.SetFocus(pb.Handle()) }, 4, false},
		{"redraw without focus", func() { lb.Invalidate() }, 4, false},
		{"focus back", func() { win.SetFocus(hLB) }, 4, true},
		{"redraw with focus", func() { lb.Invalidate() }, 4, true},
		{"mouse click item 6", func() { clickItem(6) }, 6, true},
	}

	mw.SetVisible(true)
	// Keep the window on top so GetPixel reads its own pixels, without
	// activating it. Place it away from the real mouse cursor, which would
	// otherwise put an item in the hot state.
	var cur win.POINT
	win.GetCursorPos(&cur)
	x := int32(0)
	if cur.X < 600 {
		x = cur.X + 50
	}
	fmt.Printf("cursor %v, window x %d\n", cur, x)
	win.SetWindowPos(mw.Handle(), win.HWND_TOPMOST, x, 0, 0, 0, win.SWP_NOSIZE|win.SWP_NOACTIVATE)

	go func() {
		time.Sleep(time.Second)
		for _, s := range steps {
			mw.Synchronize(func() {
				fmt.Println("STEP", s.name)
				events = nil
				s.do()
			})
			time.Sleep(400 * time.Millisecond)
			done := make(chan struct{})
			mw.Synchronize(func() {
				verify(s.name, s.sel, s.focus)
				close(done)
			})
			<-done
		}
		mw.Synchronize(func() {
			fmt.Printf("selected pixel focused %#06x, not focused %#06x, COLOR_HIGHLIGHT %#06x, COLOR_BTNFACE %#06x\n",
				focusedSel, unfocusedSel, win.GetSysColor(win.COLOR_HIGHLIGHT), win.GetSysColor(win.COLOR_BTNFACE))
		})
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
