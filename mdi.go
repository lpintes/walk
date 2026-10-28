// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"runtime"
	"slices"
	"syscall"
	"unsafe"

	"github.com/lpintes/walk/internal/win"
)

const mdiChildWindowClass = `\o/ Walk_MdiChild_Class \o/`

// mdiFirstChildID is the command ID of the first child in the window menu.
// The MDI client gives the children the IDs from here on (at most ten
// entries, the tenth is "More Windows..."). It is above the IDs of walk
// actions, which count up from 3, and of the system commands (0xF000 to
// 0xF1xx).
const mdiFirstChildID = 0xFF00

func init() {
	AppendToWalkInit(func() {
		MustRegisterWindowClassWithWndProcPtr(mdiChildWindowClass, syscall.NewCallback(mdiChildWndProc))
	})
}

// mdiChildWndProc is the window procedure of the MDI child class. Until
// NewMdiChildWindow has registered the window (during WM_MDICREATE and
// after WM_DESTROY), the messages go to DefMDIChildProc, which an MDI child
// needs instead of DefWindowProc.
func mdiChildWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if windowFromHandle(hwnd) == nil {
		return win.DefMDIChildProc(hwnd, msg, wParam, lParam)
	}

	return defaultWndProc(hwnd, msg, wParam, lParam)
}

// MdiMainWindow is the frame window of a multiple-document interface (MDI)
// application. It is a MainWindow (menu bar, tool bar and status bar)
// whose client area holds the system MDICLIENT window instead of widgets;
// the documents are MdiChildWindows inside it, created with
// NewMdiChildWindow.
//
// Windows handles the MDI behavior: Ctrl+F6 and Ctrl+Tab activate the next
// child, with Shift the previous one, Ctrl+F4 closes the active child,
// Alt+Hyphen opens the system menu of the active child, a maximized child
// puts its system menu and its minimize, restore and close buttons into
// the menu bar and its title into the title of the frame, and the window
// menu set with SetWindowMenu lists the children. Screen readers know
// these windows: an MDI child is a real MDI child window, so its title is
// its accessible name.
//
// The widget list of the frame (Children, Layout) is not shown; put the
// widgets into the children.
type MdiMainWindow struct {
	MainWindow
	windowMenu                     *Menu
	children                       []*MdiChildWindow
	activeChild                    *MdiChildWindow
	activeMdiChildChangedPublisher EventPublisher
}

// NewMdiMainWindow creates a new MDI frame window.
func NewMdiMainWindow() (*MdiMainWindow, error) {
	return NewMdiMainWindowWithCfg(&MainWindowCfg{})
}

// NewMdiMainWindowWithCfg creates a new MDI frame window with the name and
// bounds of cfg.
func NewMdiMainWindowWithCfg(cfg *MainWindowCfg) (*MdiMainWindow, error) {
	mw := new(MdiMainWindow)

	if err := initMainWindow(&mw.MainWindow, mw, cfg); err != nil {
		return nil, err
	}

	succeeded := false
	defer func() {
		if !succeeded {
			mw.Dispose()
		}
	}()

	// CreateWindowEx passes the structure in WM_CREATE of the MDI client,
	// while the frame may get WM_PARENTNOTIFY in Go; the pinner keeps it in
	// place.
	ccs := &win.CLIENTCREATESTRUCT{IdFirstChild: mdiFirstChildID}
	var pinner runtime.Pinner
	pinner.Pin(ccs)
	mw.mdiClient = win.CreateWindowEx(
		win.WS_EX_CLIENTEDGE,
		win.StringToUTF16Ptr("MDICLIENT"),
		nil,
		win.WS_CHILD|win.WS_CLIPCHILDREN|win.WS_VSCROLL|win.WS_HSCROLL|win.WS_VISIBLE|win.MDIS_ALLCHILDSTYLES,
		0, 0, 0, 0,
		mw.hWnd,
		0,
		win.GetModuleHandle(nil),
		unsafe.Pointer(ccs))
	pinner.Unpin()
	if mw.mdiClient == 0 {
		return nil, lastError("CreateWindowEx(MDICLIENT)")
	}

	// The widgets of the frame are not used; the MDI client takes their
	// place.
	mw.clientComposite.SetVisible(false)

	mw.menu.positionOffset = mw.menuBarOffset

	mw.statusBar.VisibleChanged().Attach(mw.layoutMDIClient)

	mw.toolBarSet = mw.watchToolBar
	mw.watchToolBar(mw.toolBar)

	mw.closeChildren = mw.CloseAllMdiChildren

	// Without a name, a screen reader says nothing useful when the focus
	// is in the MDI client, which happens when no child is open.
	mw.SetMdiClientName("Documents")

	mw.layoutMDIClient()

	succeeded = true

	return mw, nil
}

func (mw *MdiMainWindow) defWindowProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == win.WM_SIZE {
		// DefFrameProc would size the MDI client to the whole client area,
		// over the tool bar and the status bar; layoutMDIClient sizes it.
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}

	if msg == win.WM_SETTEXT && mw.mdiClient != 0 {
		// DefFrameProc sets the title (adding the title of a maximized
		// child) but returns 0, which setWindowText takes for a failure.
		win.DefFrameProc(hwnd, mw.mdiClient, msg, wParam, lParam)
		return win.TRUE
	}

	return win.DefFrameProc(hwnd, mw.mdiClient, msg, wParam, lParam)
}

// SetMdiClientName sets the accessible name of the MDI client, the area
// that holds the MDI children. Screen readers say it when the focus is in
// the area itself, which happens when no child is open. The default is
// "Documents"; set a translated name for other languages.
func (mw *MdiMainWindow) SetMdiClientName(name string) error {
	if mw.mdiClient == 0 {
		return newError("the frame is closed")
	}

	return mw.Accessibility().accSetPropertyStr(mw.mdiClient, &win.PROPID_ACC_NAME, win.EVENT_OBJECT_NAMECHANGE, name)
}

// watchToolBar fits the bars and the MDI client again when the tool bar
// changes its size or visibility, for example when actions are added.
func (mw *MdiMainWindow) watchToolBar(tb *ToolBar) {
	if tb == nil {
		mw.layoutBars()
		return
	}

	relayout := func() {
		if mw.toolBar == tb {
			mw.layoutBars()
		}
	}
	tb.BoundsChanged().Attach(relayout)
	tb.VisibleChanged().Attach(relayout)

	mw.layoutBars()
}

// layoutBars places the tool bar, the status bar and the MDI client, as a
// resize of the frame does.
func (mw *MdiMainWindow) layoutBars() {
	if mw.hWnd != 0 && mw.mdiClient != 0 {
		mw.SendMessage(win.WM_SIZE, 0, 0)
	}
}

// MdiChildren returns the MDI children of the frame in the order they were
// created.
func (mw *MdiMainWindow) MdiChildren() []*MdiChildWindow {
	return slices.Clone(mw.children)
}

// ActiveMdiChild returns the active MDI child, or nil if there is none.
func (mw *MdiMainWindow) ActiveMdiChild() *MdiChildWindow {
	return mw.childFromHandle(mw.activeChildHWnd())
}

// ActiveMdiChildChanged returns the event that is published when another MDI
// child becomes active, and when the last one is closed.
func (mw *MdiMainWindow) ActiveMdiChildChanged() *Event {
	return mw.activeMdiChildChangedPublisher.Event()
}

// WindowMenu returns the menu set with SetWindowMenu.
func (mw *MdiMainWindow) WindowMenu() *Menu {
	return mw.windowMenu
}

// SetWindowMenu sets the menu, usually a submenu "Window" of the menu bar,
// to which Windows appends a separator and an item for each visible MDI
// child (up to nine, then "More Windows..."), with a check mark at the
// active one. Choosing an item activates the child. The items of the menu
// itself, for example Cascade or Tile, stay before them.
//
// A nil menu leaves the window menu unchanged, as WM_MDISETMENU does.
func (mw *MdiMainWindow) SetWindowMenu(menu *Menu) error {
	if menu == nil {
		return nil
	}

	win.SendMessage(mw.mdiClient, win.WM_MDISETMENU, uintptr(mw.menu.hMenu), uintptr(menu.hMenu))
	win.DrawMenuBar(mw.hWnd)

	mw.windowMenu = menu

	return nil
}

// Cascade arranges the MDI children that are not minimized in a cascade.
func (mw *MdiMainWindow) Cascade() {
	win.SendMessage(mw.mdiClient, win.WM_MDICASCADE, 0, 0)
}

// TileHorizontally arranges the MDI children that are not minimized one
// below the other, each as wide as the frame.
func (mw *MdiMainWindow) TileHorizontally() {
	win.SendMessage(mw.mdiClient, win.WM_MDITILE, win.MDITILE_HORIZONTAL, 0)
}

// TileVertically arranges the MDI children that are not minimized side by
// side, each as high as the frame.
func (mw *MdiMainWindow) TileVertically() {
	win.SendMessage(mw.mdiClient, win.WM_MDITILE, win.MDITILE_VERTICAL, 0)
}

// ArrangeIcons arranges the minimized MDI children at the bottom of the
// frame.
func (mw *MdiMainWindow) ArrangeIcons() {
	win.SendMessage(mw.mdiClient, win.WM_MDIICONARRANGE, 0, 0)
}

// ActivateNextMdiChild activates the next MDI child, like Ctrl+F6.
func (mw *MdiMainWindow) ActivateNextMdiChild() {
	win.SendMessage(mw.mdiClient, win.WM_MDINEXT, 0, 0)
}

// ActivatePreviousMdiChild activates the previous MDI child, like
// Ctrl+Shift+F6.
func (mw *MdiMainWindow) ActivatePreviousMdiChild() {
	win.SendMessage(mw.mdiClient, win.WM_MDINEXT, 0, 1)
}

// CloseAllMdiChildren asks every MDI child to close, as Close does, and
// stops at the first one whose Closing event cancels. It reports whether
// all children are closed. The frame calls it when it is closed itself,
// after its own Closing event, if that did not cancel; a child that
// cancels its Closing cancels the close of the frame.
func (mw *MdiMainWindow) CloseAllMdiChildren() bool {
	for _, cw := range slices.Clone(mw.children) {
		if cw.hWnd == 0 {
			continue
		}

		cw.SendMessage(win.WM_CLOSE, 0, 0)

		if cw.hWnd != 0 {
			return false
		}
	}

	return len(mw.children) == 0
}

func (mw *MdiMainWindow) activeChildHWnd() win.HWND {
	if mw.mdiClient == 0 {
		return 0
	}

	return win.HWND(win.SendMessage(mw.mdiClient, win.WM_MDIGETACTIVE, 0, 0))
}

func (mw *MdiMainWindow) childFromHandle(hwnd win.HWND) *MdiChildWindow {
	if hwnd == 0 {
		return nil
	}

	for _, cw := range mw.children {
		if cw.hWnd == hwnd {
			return cw
		}
	}

	return nil
}

// updateActiveChild publishes ActiveMdiChildChanged if the active child is
// not the one seen last time.
func (mw *MdiMainWindow) updateActiveChild() {
	if mw.hWnd == 0 {
		return
	}

	active := mw.ActiveMdiChild()
	if active == mw.activeChild {
		return
	}

	mw.activeChild = active

	mw.activeMdiChildChangedPublisher.Publish()
}

func (mw *MdiMainWindow) removeChild(cw *MdiChildWindow) {
	if i := slices.Index(mw.children, cw); i >= 0 {
		mw.children = slices.Delete(mw.children, i, i+1)
	}
	if mw.activeChild == cw {
		mw.activeChild = nil
		if mw.hWnd != 0 {
			mw.activeMdiChildChangedPublisher.Publish()
		}
	}
}

// focusEmptyClient gives the focus to the MDI client when the last visible
// child is gone while the frame is active, so that it is not lost: the
// focus was in the destroyed child. The MDI client has an accessible name
// (SetMdiClientName), which screen readers say. The frame itself cannot
// keep the focus: DefFrameProc passes it on to the MDI client.
func (mw *MdiMainWindow) focusEmptyClient() {
	if mw.mdiClient == 0 || mw.activeChildHWnd() != 0 || win.GetActiveWindow() != mw.hWnd {
		return
	}

	if focus := win.GetFocus(); focus == 0 || focus != mw.mdiClient && !win.IsChild(mw.hWnd, focus) {
		win.SetFocus(mw.mdiClient)
	}
}

// menuBarOffset returns 1 while the active child is maximized: then the MDI
// client has inserted the system menu of the child as the first item of
// the menu bar.
func (mw *MdiMainWindow) menuBarOffset() int {
	if mw.mdiClient == 0 {
		return 0
	}

	maximized := new(win.BOOL)
	var pinner runtime.Pinner
	pinner.Pin(maximized)
	defer pinner.Unpin()

	if win.SendMessage(mw.mdiClient, win.WM_MDIGETACTIVE, 0, uintptr(unsafe.Pointer(maximized))) != 0 && *maximized != 0 {
		return 1
	}

	return 0
}

// layoutMDIClient fits the MDI client into the client area between the tool
// bar and the status bar.
func (mw *MdiMainWindow) layoutMDIClient() {
	if mw.mdiClient == 0 || mw.statusBar == nil {
		return
	}

	b := mw.ClientBoundsPixels()
	if b != windowBoundsInParent(mw.mdiClient) {
		win.MoveWindow(mw.mdiClient, int32(b.X), int32(b.Y), int32(b.Width), int32(b.Height), true)
	}
}

// windowBoundsInParent returns the bounds of a child window in the client
// coordinates of its parent.
func windowBoundsInParent(hwnd win.HWND) Rectangle {
	var r win.RECT
	if !win.GetWindowRect(hwnd, &r) {
		return Rectangle{}
	}

	b := rectangleFromRECT(r)
	p := b.Location().toPOINT()
	win.ScreenToClient(win.GetParent(hwnd), &p)
	b.X, b.Y = int(p.X), int(p.Y)

	return b
}

// prepareFocusRestore makes the activation of the frame give the focus to
// the active child, which passes it on to its last focused control, unless
// the focus was outside the MDI client (for example in the tool bar) or in
// the active child.
func (mw *MdiMainWindow) prepareFocusRestore() {
	active := mw.activeChildHWnd()
	if active == 0 {
		return
	}

	p := mw.prevFocusHWnd
	if p == 0 || !win.IsWindow(p) || p == mw.mdiClient || win.IsChild(mw.mdiClient, p) && p != active && !win.IsChild(active, p) {
		mw.prevFocusHWnd = active
	}
}

// applyDPIToChildren does for the MDI children what WM_DPICHANGED does for
// the frame; only top-level windows get the message.
func (mw *MdiMainWindow) applyDPIToChildren(dpi int) {
	for _, cw := range mw.children {
		wasSuspended := cw.Suspended()
		cw.SetSuspended(true)

		cw.clientComposite.ApplyDPI(dpi)
		cw.ApplyDPI(dpi)
		applyDPIToDescendants(cw.window, dpi)

		cw.SetSuspended(wasSuspended)

		cw.SetIcon(cw.icon)
		cw.startLayout()
	}
}

// SetVisible shows or hides the frame.
func (mw *MdiMainWindow) SetVisible(visible bool) {
	wasVisible := win.IsWindowVisible(mw.hWnd)

	mw.MainWindow.SetVisible(visible)

	if visible {
		mw.layoutMDIClient()

		if !wasVisible {
			// Other forms are activated by the focus their first layout
			// gives to their first control (applyLayoutResults). The
			// widgets of the frame are hidden and those of the children
			// are not its own, so the frame takes the focus itself;
			// DefFrameProc passes it on to the MDI client, which gives it
			// to the active child.
			win.SetFocus(mw.hWnd)
		}
	}
}

// WndProc is the window procedure of the frame. Messages it does not handle
// go to DefFrameProc.
func (mw *MdiMainWindow) WndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_ACTIVATE:
		if win.LOWORD(uint32(wParam)) != win.WA_INACTIVE {
			mw.prepareFocusRestore()
		}

	case win.WM_COMMAND:
		// Commands that are not walk actions come from the window menu
		// (activate a child, More Windows...) or from the system menu of
		// a maximized child in the menu bar; DefFrameProc handles them.
		if lParam == 0 {
			if _, ok := actionsById[win.LOWORD(uint32(wParam))]; !ok {
				return win.DefFrameProc(hwnd, mw.mdiClient, msg, wParam, lParam)
			}
		}

	case win.WM_SIZE:
		// MainWindow places the tool bar and the status bar, then the
		// MDI client fills the rest; DefFrameProc does not get it.
		result := mw.MainWindow.WndProc(hwnd, msg, wParam, lParam)

		mw.layoutMDIClient()

		return result

	case win.WM_WINDOWPOSCHANGED:
		result := mw.MainWindow.WndProc(hwnd, msg, wParam, lParam)

		if wp := ptrFromUintptr[win.WINDOWPOS](lParam); wp.Flags&win.SWP_NOSIZE == 0 {
			mw.layoutMDIClient()
		}

		return result

	case win.WM_DPICHANGED:
		result := mw.MainWindow.WndProc(hwnd, msg, wParam, lParam)

		mw.applyDPIToChildren(int(win.HIWORD(uint32(wParam))))

		return result

	case win.WM_DESTROY:
		result := mw.MainWindow.WndProc(hwnd, msg, wParam, lParam)

		// The MDI client and the children are destroyed after the frame;
		// from now on the frame calls DefWindowProc.
		if mw.group != nil {
			mw.group.accClearHwndProps(mw.mdiClient)
		}
		mw.mdiClient = 0

		return result
	}

	return mw.MainWindow.WndProc(hwnd, msg, wParam, lParam)
}

// MdiChildWindow is a document window inside an MdiMainWindow. It is a Form
// with a layout and widgets, like a Dialog, but a real MDI child window
// (created with WM_MDICREATE) whose window procedure calls
// DefMDIChildProc.
//
// Closing is published when the user closes the child (Ctrl+F4, the close
// button, the system menu) or Close is called, and can cancel it.
// Activating and Deactivating are published when the child becomes the
// active child or stops being it. When a child is activated, the keyboard
// focus goes to the control of the child that had it last, or else to the
// first control; Tab moves among the controls of the active child.
//
// The Form of the widgets in a child, as returned by their Form method, is
// the frame, so that the shortcuts of the menu bar work in all children.
// Bounds are relative to the client area of the MDI client.
type MdiChildWindow struct {
	FormBase
	frame      *MdiMainWindow
	destroying bool
}

var _ Form = (*MdiChildWindow)(nil)

// NewMdiChildWindow creates a new hidden MDI child in frame. Show shows and
// activates it. Without a position or size set, Windows places it below
// and to the right of the previous one.
func NewMdiChildWindow(frame *MdiMainWindow) (*MdiChildWindow, error) {
	if frame == nil || frame.mdiClient == 0 {
		return nil, newError("frame is not an open MDI frame")
	}

	cw := &MdiChildWindow{frame: frame}
	cw.mdiChild = true

	// Without WS_VISIBLE (allowed by MDIS_ALLCHILDSTYLES) the MDI client
	// does not activate the child, so that it is registered with walk
	// before it gets WM_MDIACTIVATE and the focus.
	const style = win.WS_CHILD | win.WS_CLIPSIBLINGS | win.WS_CLIPCHILDREN | win.WS_OVERLAPPEDWINDOW

	mcs := &win.MDICREATESTRUCT{
		SzClass: win.StringToUTF16Ptr(mdiChildWindowClass),
		SzTitle: win.StringToUTF16Ptr(""),
		HOwner:  win.HANDLE(win.GetModuleHandle(nil)),
		X:       win.CW_USEDEFAULT,
		Y:       win.CW_USEDEFAULT,
		Cx:      win.CW_USEDEFAULT,
		Cy:      win.CW_USEDEFAULT,
		Style:   style,
	}
	// The child gets messages in Go while WM_MDICREATE runs; the pinner
	// keeps the structure in place.
	var pinner runtime.Pinner
	pinner.Pin(mcs)
	hwnd := win.HWND(win.SendMessage(frame.mdiClient, win.WM_MDICREATE, 0, uintptr(unsafe.Pointer(mcs))))
	pinner.Unpin()
	if hwnd == 0 {
		return nil, lastError("WM_MDICREATE")
	}

	cw.hWnd = hwnd

	if err := initWindowWithCfg(&windowCfg{
		Window:    cw,
		ClassName: mdiChildWindowClass,
		Style:     style,
	}); err != nil {
		if win.IsWindow(hwnd) {
			// Destroy the window through the MDI client, which keeps its
			// list of children and the window menu right.
			win.SendMessage(frame.mdiClient, win.WM_MDIDESTROY, uintptr(hwnd), 0)
		}
		return nil, err
	}

	succeeded := false
	defer func() {
		if !succeeded {
			cw.Dispose()
		}
	}()

	// IsDialogMessage of the message loop moves the focus among the
	// controls inside the child composite.
	if err := cw.ensureExtendedStyleBits(win.WS_EX_CONTROLPARENT, true); err != nil {
		return nil, err
	}

	frame.children = append(frame.children, cw)

	succeeded = true

	return cw, nil
}

func (cw *MdiChildWindow) defWindowProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == win.WM_SETTEXT {
		// Like DefFrameProc, DefMDIChildProc sets the title (and updates
		// the frame title and the window menu) but returns 0, which
		// setWindowText takes for a failure.
		win.DefMDIChildProc(hwnd, msg, wParam, lParam)
		return win.TRUE
	}

	return win.DefMDIChildProc(hwnd, msg, wParam, lParam)
}

// NeedsWmSize makes the child get WM_SIZE and WM_MOVE, which
// DefMDIChildProc needs to handle maximizing.
func (cw *MdiChildWindow) NeedsWmSize() bool {
	return true
}

// Frame returns the MDI frame of the child.
func (cw *MdiChildWindow) Frame() *MdiMainWindow {
	return cw.frame
}

// Run shows the child and returns 0. An MDI child has no message loop of
// its own; Run of the frame runs it.
func (cw *MdiChildWindow) Run() int {
	cw.Show()

	return 0
}

// Activate makes the child the active MDI child.
func (cw *MdiChildWindow) Activate() error {
	if cw.frame.mdiClient == 0 {
		return newError("the frame is closed")
	}

	win.SendMessage(cw.frame.mdiClient, win.WM_MDIACTIVATE, uintptr(cw.hWnd), 0)

	return nil
}

// IsActive reports whether the child is the active MDI child.
func (cw *MdiChildWindow) IsActive() bool {
	return cw.hWnd != 0 && cw.frame.activeChildHWnd() == cw.hWnd
}

// Maximize maximizes the child. While a child is maximized, the other
// children are maximized too when they are activated.
func (cw *MdiChildWindow) Maximize() {
	if cw.frame.mdiClient != 0 {
		win.SendMessage(cw.frame.mdiClient, win.WM_MDIMAXIMIZE, uintptr(cw.hWnd), 0)
	}
}

// Minimize minimizes the child, as its system menu does.
func (cw *MdiChildWindow) Minimize() {
	cw.SendMessage(win.WM_SYSCOMMAND, win.SC_MINIMIZE, 0)
}

// Restore restores the child from the maximized or minimized state.
func (cw *MdiChildWindow) Restore() {
	if cw.frame.mdiClient != 0 {
		win.SendMessage(cw.frame.mdiClient, win.WM_MDIRESTORE, uintptr(cw.hWnd), 0)
	}
}

// IsMaximized reports whether the child is maximized.
func (cw *MdiChildWindow) IsMaximized() bool {
	return win.IsZoomed(cw.hWnd)
}

// IsMinimized reports whether the child is minimized.
func (cw *MdiChildWindow) IsMinimized() bool {
	return win.IsIconic(cw.hWnd)
}

// SetOwner returns an error: an MDI child belongs to its frame.
func (cw *MdiChildWindow) SetOwner(owner Form) error {
	return newError("an MDI child window has no owner")
}

// Bounds returns the outer bounds of the child in 1/96" units, relative to
// the client area of the MDI client.
func (cw *MdiChildWindow) Bounds() Rectangle {
	return cw.RectangleTo96DPI(cw.BoundsPixels())
}

// BoundsPixels returns the outer bounds of the child in native pixels,
// relative to the client area of the MDI client.
func (cw *MdiChildWindow) BoundsPixels() Rectangle {
	return windowBoundsInParent(cw.hWnd)
}

// SetVisible shows or hides the child. Showing it activates it; hiding the
// active child activates the next one.
func (cw *MdiChildWindow) SetVisible(visible bool) {
	client := cw.frame.mdiClient

	if !visible && client != 0 && cw.IsActive() {
		win.SendMessage(client, win.WM_MDINEXT, uintptr(cw.hWnd), 0)
	}

	cw.FormBase.SetVisible(visible)

	if client != 0 {
		if visible {
			win.SendMessage(client, win.WM_MDIACTIVATE, uintptr(cw.hWnd), 0)
		}
		win.SendMessage(client, win.WM_MDIREFRESHMENU, 0, 0)
		win.DrawMenuBar(cw.frame.hWnd)
	}
}

// Dispose destroys the child through the MDI client (WM_MDIDESTROY), so that
// the client activates the next child and updates the window menu.
func (cw *MdiChildWindow) Dispose() {
	if hwnd := cw.hWnd; hwnd != 0 && !cw.destroying && hwnd2WindowBase[hwnd] != nil && cw.frame.mdiClient != 0 {
		cw.destroying = true

		// WM_DESTROY calls Dispose again, which does the rest.
		win.SendMessage(cw.frame.mdiClient, win.WM_MDIDESTROY, uintptr(hwnd), 0)

		if cw.hWnd == 0 {
			cw.frame.updateActiveChild()
			cw.frame.focusEmptyClient()
			return
		}
	}

	cw.frame.removeChild(cw)

	cw.FormBase.Dispose()
}

func (cw *MdiChildWindow) restoreFocus() {
	if win.IsIconic(cw.hWnd) {
		// A minimized child keeps the focus itself, as in other MDI
		// applications.
		return
	}

	if p := cw.prevFocusHWnd; p != 0 && win.IsChild(cw.hWnd, p) && win.IsWindowVisible(p) && win.IsWindowEnabled(p) {
		win.SetFocus(p)
		return
	}

	cw.clientComposite.focusFirstCandidateDescendant()
}

// WndProc is the window procedure of the child. Messages it does not handle
// go to DefMDIChildProc.
func (cw *MdiChildWindow) WndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_GETMINMAXINFO:
		// DefMDIChildProc sets the maximized size, FormBase the minimum
		// size of the layout.
		win.DefMDIChildProc(hwnd, msg, wParam, lParam)
		if cw.Suspended() || cw.proposedSize == (Size{}) {
			return 0
		}

	case win.WM_MDIACTIVATE:
		if win.HWND(wParam) == hwnd {
			if focus := win.GetFocus(); focus != 0 && win.IsChild(hwnd, focus) {
				cw.prevFocusHWnd = webView2FocusOwner(focus)
			}

			cw.deactivatingPublisher.Publish()
		}

		if win.HWND(lParam) == hwnd {
			cw.frame.updateActiveChild()

			cw.activatingPublisher.Publish()
		}

	case win.WM_SETFOCUS:
		// The MDI client gives the focus to the child when it activates
		// it; pass it on to a control.
		result := cw.FormBase.WndProc(hwnd, msg, wParam, lParam)

		cw.restoreFocus()

		return result
	}

	return cw.FormBase.WndProc(hwnd, msg, wParam, lParam)
}
