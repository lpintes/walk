// Copyright 2011 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"syscall"
	"unsafe"

	"github.com/lpintes/walk/internal/win"
)

var notifyIcons = make(map[*NotifyIcon]bool)

func notifyIconWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) (result uintptr) {
	// Retrieve our *NotifyIcon from the message window.
	ptr := win.GetWindowLongPtr(hwnd, win.GWLP_USERDATA)
	ni := ptrFromUintptr[NotifyIcon](ptr)

	switch lParam {
	case win.WM_LBUTTONDOWN:
		ni.publishMouseEvent(&ni.mouseDownPublisher, LeftButton)

	case win.WM_LBUTTONUP:
		ni.publishMouseEvent(&ni.mouseUpPublisher, LeftButton)

	case win.WM_LBUTTONDBLCLK:
		ni.publishMouseEvent(&ni.mouseDoubleClickPublisher, LeftButton)

	case win.WM_RBUTTONDOWN:
		ni.publishMouseEvent(&ni.mouseDownPublisher, RightButton)

	case win.WM_RBUTTONUP:
		ni.publishMouseEvent(&ni.mouseUpPublisher, RightButton)

		ni.showContextMenu(false)

		return 0

	case win.WM_RBUTTONDBLCLK:
		ni.publishMouseEvent(&ni.mouseDoubleClickPublisher, RightButton)

	case win.WM_CONTEXTMENU:
		// With NOTIFYICON_VERSION 3 the shell sends WM_CONTEXTMENU only when
		// the menu is requested with the keyboard (Shift+F10 or the
		// application key).
		ni.showContextMenu(true)

		return 0

	case win.NIN_SELECT, win.NIN_SELECT | win.NINF_KEY:
		// NIN_SELECT follows a left click, NIN_SELECT | NINF_KEY (NIN_KEYSELECT)
		// is sent when the icon is selected with the keyboard (Enter or Space,
		// for example after Windows+B).
		ni.activatedPublisher.Publish()

	case win.NIN_BALLOONUSERCLICK:
		ni.messageClickedPublisher.Publish()
	}

	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

// NotifyIcon represents an icon in the taskbar notification area.
type NotifyIcon struct {
	id                        uint32
	hWnd                      win.HWND
	lastDPI                   int
	contextMenu               *Menu
	icon                      Image
	toolTip                   string
	visible                   bool
	mouseDownPublisher        MouseEventPublisher
	mouseUpPublisher          MouseEventPublisher
	mouseDoubleClickPublisher MouseEventPublisher
	activatedPublisher        EventPublisher
	messageClickedPublisher   EventPublisher
}

// NewNotifyIcon creates and returns a new NotifyIcon.
//
// The NotifyIcon is initially not visible.
func NewNotifyIcon(form Form) (*NotifyIcon, error) {
	fb := form.AsFormBase()
	// Add our notify icon to the status area and make sure it is hidden.
	nid := win.NOTIFYICONDATA{
		HWnd:             fb.hWnd,
		UFlags:           win.NIF_MESSAGE | win.NIF_STATE,
		DwState:          win.NIS_HIDDEN,
		DwStateMask:      win.NIS_HIDDEN,
		UCallbackMessage: notifyIconMessageId,
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid) - unsafe.Sizeof(win.HICON(0)))

	if !win.Shell_NotifyIcon(win.NIM_ADD, &nid) {
		return nil, newError("Shell_NotifyIcon")
	}

	// We want XP-compatible message behavior.
	nid.UVersion = win.NOTIFYICON_VERSION

	if !win.Shell_NotifyIcon(win.NIM_SETVERSION, &nid) {
		return nil, newError("Shell_NotifyIcon")
	}

	// Create and initialize the NotifyIcon already.
	menu, err := NewMenu()
	if err != nil {
		return nil, err
	}
	menu.window = form

	ni := &NotifyIcon{
		id:          nid.UID,
		hWnd:        fb.hWnd,
		contextMenu: menu,
	}

	menu.getDPI = ni.DPI

	// Set our *NotifyIcon as user data for the message window.
	win.SetWindowLongPtr(fb.hWnd, win.GWLP_USERDATA, uintptr(unsafe.Pointer(ni)))

	notifyIcons[ni] = true
	return ni, nil
}

func (ni *NotifyIcon) DPI() int {
	fakeWb := WindowBase{hWnd: win.FindWindow(win.StringToUTF16Ptr("Shell_TrayWnd"), win.StringToUTF16Ptr(""))}
	return fakeWb.DPI()
}

func (ni *NotifyIcon) readdToTaskbar() error {
	nid := win.NOTIFYICONDATA{
		HWnd:             ni.hWnd,
		UFlags:           win.NIF_MESSAGE | win.NIF_STATE,
		DwState:          win.NIS_HIDDEN,
		DwStateMask:      win.NIS_HIDDEN,
		UCallbackMessage: notifyIconMessageId,
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid) - unsafe.Sizeof(win.HICON(0)))

	if !win.Shell_NotifyIcon(win.NIM_ADD, &nid) {
		return newError("Shell_NotifyIcon")
	}

	// We want XP-compatible message behavior.
	nid.UVersion = win.NOTIFYICON_VERSION

	if !win.Shell_NotifyIcon(win.NIM_SETVERSION, &nid) {
		return newError("Shell_NotifyIcon")
	}

	icon := ni.icon
	ni.icon = nil
	err := ni.SetIcon(icon)
	if err != nil {
		return err
	}
	visible := ni.visible
	ni.visible = false
	err = ni.SetVisible(visible)
	if err != nil {
		return err
	}
	tooltip := ni.toolTip
	ni.toolTip = ""
	err = ni.SetToolTip(tooltip)
	if err != nil {
		return err
	}
	return nil
}

func (ni *NotifyIcon) applyDPI() {
	dpi := ni.DPI()
	if dpi == ni.lastDPI {
		return
	}
	ni.lastDPI = dpi
	for _, action := range ni.contextMenu.actions.actions {
		if action.image != nil {
			ni.contextMenu.onActionChanged(action)
		}
	}
	icon := ni.icon
	ni.icon = nil
	if icon != nil {
		ni.SetIcon(icon)
	}
}

func (ni *NotifyIcon) notifyIconData() *win.NOTIFYICONDATA {
	nid := &win.NOTIFYICONDATA{
		UID:  ni.id,
		HWnd: ni.hWnd,
	}
	nid.CbSize = uint32(unsafe.Sizeof(*nid) - unsafe.Sizeof(win.HICON(0)))

	return nid
}

// Dispose releases the operating system resources associated with the
// NotifyIcon.
//
// The associated Icon is not disposed of.
func (ni *NotifyIcon) Dispose() error {
	if ni.hWnd == 0 {
		return nil
	}
	delete(notifyIcons, ni)

	nid := ni.notifyIconData()

	if !win.Shell_NotifyIcon(win.NIM_DELETE, nid) {
		return newError("Shell_NotifyIcon")
	}

	ni.hWnd = 0

	return nil
}

func (ni *NotifyIcon) showMessage(title, info string, iconType uint32, icon Image) error {
	nid := ni.notifyIconData()
	nid.UFlags = win.NIF_INFO
	nid.DwInfoFlags = iconType
	var oldIcon Image
	if iconType == win.NIIF_USER && icon != nil {
		oldIcon = ni.icon
		if err := ni.setNIDIcon(nid, icon); err != nil {
			return err
		}
		nid.UFlags |= win.NIF_ICON
	}
	if title16, err := syscall.UTF16FromString(title); err == nil {
		copy(nid.SzInfoTitle[:], title16)
	}
	if info16, err := syscall.UTF16FromString(info); err == nil {
		copy(nid.SzInfo[:], info16)
	}
	if !win.Shell_NotifyIcon(win.NIM_MODIFY, nid) {
		return newError("Shell_NotifyIcon")
	}
	if oldIcon != nil {
		ni.icon = nil
		ni.SetIcon(oldIcon)
	}

	return nil
}

// ShowMessage displays a neutral message balloon above the NotifyIcon.
//
// The NotifyIcon must be visible before calling this method.
func (ni *NotifyIcon) ShowMessage(title, info string) error {
	return ni.showMessage(title, info, win.NIIF_NONE, nil)
}

// ShowInfo displays an info message balloon above the NotifyIcon.
//
// The NotifyIcon must be visible before calling this method.
func (ni *NotifyIcon) ShowInfo(title, info string) error {
	return ni.showMessage(title, info, win.NIIF_INFO, nil)
}

// ShowWarning displays a warning message balloon above the NotifyIcon.
//
// The NotifyIcon must be visible before calling this method.
func (ni *NotifyIcon) ShowWarning(title, info string) error {
	return ni.showMessage(title, info, win.NIIF_WARNING, nil)
}

// ShowError displays an error message balloon above the NotifyIcon.
//
// The NotifyIcon must be visible before calling this method.
func (ni *NotifyIcon) ShowError(title, info string) error {
	return ni.showMessage(title, info, win.NIIF_ERROR, nil)
}

// ShowCustom displays a custom icon message balloon above the NotifyIcon.
// If icon is nil, the main notification icon is used instead of a custom one.
//
// The NotifyIcon must be visible before calling this method.
func (ni *NotifyIcon) ShowCustom(title, info string, icon Image) error {
	return ni.showMessage(title, info, win.NIIF_USER, icon)
}

// ContextMenu returns the context menu of the NotifyIcon.
func (ni *NotifyIcon) ContextMenu() *Menu {
	return ni.contextMenu
}

// Icon returns the Icon of the NotifyIcon.
func (ni *NotifyIcon) Icon() Image {
	return ni.icon
}

// SetIcon sets the Icon of the NotifyIcon.
func (ni *NotifyIcon) SetIcon(icon Image) error {
	if icon == ni.icon {
		return nil
	}

	nid := ni.notifyIconData()
	nid.UFlags = win.NIF_ICON
	if icon == nil {
		nid.HIcon = 0
	} else {
		if err := ni.setNIDIcon(nid, icon); err != nil {
			return err
		}
	}

	if !win.Shell_NotifyIcon(win.NIM_MODIFY, nid) {
		return newError("Shell_NotifyIcon")
	}

	ni.icon = icon

	return nil
}

func (ni *NotifyIcon) setNIDIcon(nid *win.NOTIFYICONDATA, icon Image) error {
	dpi := ni.DPI()
	ic, err := iconCache.Icon(icon, dpi)
	if err != nil {
		return err
	}
	nid.HIcon = ic.handleForDPI(dpi)

	return nil
}

// ToolTip returns the tool tip text of the NotifyIcon.
func (ni *NotifyIcon) ToolTip() string {
	return ni.toolTip
}

// SetToolTip sets the tool tip text of the NotifyIcon.
func (ni *NotifyIcon) SetToolTip(toolTip string) error {
	if toolTip == ni.toolTip {
		return nil
	}

	nid := ni.notifyIconData()
	nid.UFlags = win.NIF_TIP
	copy(nid.SzTip[:], win.StringToUTF16(toolTip))

	if !win.Shell_NotifyIcon(win.NIM_MODIFY, nid) {
		return newError("Shell_NotifyIcon")
	}

	ni.toolTip = toolTip

	return nil
}

// Visible returns if the NotifyIcon is visible.
func (ni *NotifyIcon) Visible() bool {
	return ni.visible
}

// SetVisible sets if the NotifyIcon is visible.
func (ni *NotifyIcon) SetVisible(visible bool) error {
	if visible == ni.visible {
		return nil
	}

	nid := ni.notifyIconData()
	nid.UFlags = win.NIF_STATE
	nid.DwStateMask = win.NIS_HIDDEN
	if !visible {
		nid.DwState = win.NIS_HIDDEN
	}

	if !win.Shell_NotifyIcon(win.NIM_MODIFY, nid) {
		return newError("Shell_NotifyIcon")
	}

	ni.visible = visible

	return nil
}

// showContextMenu shows the context menu at the mouse cursor, or next to the
// icon when byKeyboard is true and the icon position is known.
func (ni *NotifyIcon) showContextMenu(byKeyboard bool) {
	if ni.contextMenu.Actions().Len() == 0 {
		return
	}

	win.SetForegroundWindow(ni.hWnd)

	ni.applyDPI()

	flags := uint32(win.TPM_NOANIMATION | win.TPM_RETURNCMD)
	var p win.POINT
	var tpm *win.TPMPARAMS
	if byKeyboard {
		if rect, ok := ni.iconRect(); ok {
			// Open the menu at the icon without covering it.
			p = win.POINT{X: rect.Left, Y: rect.Top}
			tpm = &win.TPMPARAMS{RcExclude: rect}
			tpm.CbSize = uint32(unsafe.Sizeof(*tpm))
			flags |= win.TPM_VERTICAL
		}
	}
	if tpm == nil && !win.GetCursorPos(&p) {
		lastError("GetCursorPos")
	}

	actionId := uint16(win.TrackPopupMenuEx(
		ni.contextMenu.hMenu,
		flags,
		p.X,
		p.Y,
		ni.hWnd,
		tpm))

	// The TrackPopupMenu documentation asks for this, so that the next menu
	// works when this one was dismissed by switching to another task.
	win.PostMessage(ni.hWnd, win.WM_NULL, 0, 0)

	if actionId != 0 {
		if action, ok := actionsById[actionId]; ok {
			action.raiseTriggered()
		}
	}
}

// iconRect returns the screen rectangle of the icon.
func (ni *NotifyIcon) iconRect() (win.RECT, bool) {
	nii := win.NOTIFYICONIDENTIFIER{
		HWnd: ni.hWnd,
		UID:  ni.id,
	}
	nii.CbSize = uint32(unsafe.Sizeof(nii))

	var rect win.RECT
	if win.FAILED(win.Shell_NotifyIconGetRect(&nii, &rect)) {
		return win.RECT{}, false
	}

	return rect, true
}

func (ni *NotifyIcon) publishMouseEvent(publisher *MouseEventPublisher, button MouseButton) {
	var p win.POINT
	if !win.GetCursorPos(&p) {
		lastError("GetCursorPos")
	}

	publisher.Publish(int(p.X), int(p.Y), button)
}

// MouseDown returns the event that is published when a mouse button is pressed
// while the cursor is over the NotifyIcon.
func (ni *NotifyIcon) MouseDown() *MouseEvent {
	return ni.mouseDownPublisher.Event()
}

// MouseUp returns the event that is published when a mouse button is released
// while the cursor is over the NotifyIcon.
func (ni *NotifyIcon) MouseUp() *MouseEvent {
	return ni.mouseUpPublisher.Event()
}

// MouseDoubleClick returns the event that is published when a mouse button is
// double clicked while the cursor is over the NotifyIcon.
func (ni *NotifyIcon) MouseDoubleClick() *MouseEvent {
	return ni.mouseDoubleClickPublisher.Event()
}

// Activated returns the event that is published when the user selects the
// NotifyIcon, either with a left click or with the keyboard (Enter or Space,
// for example after moving to the notification area with Windows+B).
//
// Prefer it to MouseDown or MouseUp for the main action of the icon, so that
// keyboard and screen reader users can trigger it too.
func (ni *NotifyIcon) Activated() *Event {
	return ni.activatedPublisher.Event()
}

// MessageClicked occurs when the user clicks a message shown with ShowMessage or
// one of its iconed variants.
func (ni *NotifyIcon) MessageClicked() *Event {
	return ni.messageClickedPublisher.Event()
}
