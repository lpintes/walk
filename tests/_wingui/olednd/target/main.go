// Command target is a walk MainWindow that prints the files dropped on it.
// It prints "HWND <n>" once visible and "DROP <paths>" for every drop.
// Run it together with ../source, see ../../README.md.
package main

import (
	"fmt"
	"os"

	"github.com/lpintes/walk"
	"github.com/lpintes/walk/internal/win"
)

func main() {
	mw, err := walk.NewMainWindow()
	if err != nil {
		fmt.Println("FAIL NewMainWindow:", err)
		os.Exit(1)
	}
	mw.SetTitle("olednd target")
	// Without a layout, SetVisible panics in CreateLayoutItem.
	mw.SetLayout(walk.NewVBoxLayout())
	mw.DropFiles().Attach(func(files []string) {
		fmt.Printf("DROP %q\n", files)
		os.Stdout.Sync()
	})
	mw.SetBounds(walk.Rectangle{X: 100, Y: 100, Width: 500, Height: 400})
	mw.SetVisible(true)
	// Topmost without activation, so that nothing covers the window.
	win.SetWindowPos(mw.Handle(), win.HWND_TOPMOST, 0, 0, 0, 0,
		win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOACTIVATE)
	fmt.Printf("HWND %d\n", mw.Handle())
	os.Stdout.Sync()
	code := mw.Run()
	fmt.Printf("EXIT %d\n", code)
	os.Exit(code)
}
