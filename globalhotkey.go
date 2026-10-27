// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/lpintes/walk/internal/win"
)

// ErrHotkeyAlreadyRegistered is wrapped by the error NewGlobalHotkey returns
// when the key combination is already registered as a global hotkey, by
// this or another program.
var ErrHotkeyAlreadyRegistered = errors.New("hotkey already registered")

// GlobalHotkeyOptions holds the options of NewGlobalHotkeyWithOptions.
type GlobalHotkeyOptions struct {
	// Repeat makes the hotkey publish Activated again and again while the
	// keys are held down (keyboard autorepeat). By default it is published
	// once per press (MOD_NOREPEAT).
	Repeat bool
}

// GlobalHotkey is a system-wide keyboard shortcut (RegisterHotKey): its
// Activated event is published when the user presses the key combination,
// also while another program has the focus.
//
// The hotkey belongs to its owner window; Windows posts WM_HOTKEY to it,
// and walk publishes Activated on the owner's thread. Disposing of the
// owner unregisters the hotkey.
//
// A handler may bring a form to the foreground with
// FormBase.BringToForeground: Windows allows it, because the key press
// counts as the last input event of the program.
type GlobalHotkey struct {
	wb                 *WindowBase // not changed after NewGlobalHotkey
	threadID           uint32      // of the owner window
	disposed           atomic.Bool
	owner              Window
	id                 int32
	shortcut           Shortcut
	repeat             bool
	activatedPublisher EventPublisher
}

// NewGlobalHotkey registers shortcut as a global hotkey of owner, published
// once per key press. See NewGlobalHotkeyWithOptions.
func NewGlobalHotkey(owner Window, shortcut Shortcut) (*GlobalHotkey, error) {
	return NewGlobalHotkeyWithOptions(owner, shortcut, GlobalHotkeyOptions{})
}

// NewGlobalHotkeyWithOptions registers shortcut as a global hotkey of owner,
// usually a form.
//
// It must be called on the thread of owner (the UI goroutine, or a function
// passed to Synchronize). shortcut must have a key; its modifiers may
// include ModWin. Windows reserves some combinations with the Windows key
// and F12. If the combination is already registered, by this program or
// another one, the error wraps ErrHotkeyAlreadyRegistered.
func NewGlobalHotkeyWithOptions(owner Window, shortcut Shortcut, opts GlobalHotkeyOptions) (*GlobalHotkey, error) {
	if owner == nil || owner.Handle() == 0 {
		return nil, newError("NewGlobalHotkey: owner must be a window that is not disposed")
	}
	if shortcut.Key == 0 {
		return nil, newError("NewGlobalHotkey: the shortcut has no key")
	}
	if shortcut.Modifiers&^(ModShift|ModControl|ModAlt|ModWin) != 0 {
		return nil, newError(fmt.Sprintf("NewGlobalHotkey: unknown modifiers 0x%x", byte(shortcut.Modifiers)))
	}

	wb := owner.AsWindowBase()
	hwnd := wb.hWnd
	if win.GetWindowThreadProcessId(hwnd, nil) != win.GetCurrentThreadId() {
		return nil, newError("NewGlobalHotkey: must be called on the thread of the owner window")
	}

	// RegisterHotKey identifies a hotkey by window and ID; applications
	// use 0x0000 to 0xBFFF. Take the smallest ID the window does not use.
	id := int32(1)
	for slices.ContainsFunc(wb.globalHotkeys, func(h *GlobalHotkey) bool { return h.id == id }) {
		id++
	}
	if id > 0xBFFF {
		return nil, newError("NewGlobalHotkey: too many hotkeys for one window")
	}

	mods := hotkeyModifiers(shortcut.Modifiers)
	if !opts.Repeat {
		mods |= win.MOD_NOREPEAT
	}

	if ok, errno := win.RegisterHotKey(hwnd, id, mods, uint32(shortcut.Key)); !ok {
		if errno == win.ERROR_HOTKEY_ALREADY_REGISTERED {
			return nil, wrapErrorNoPanic(fmt.Errorf("NewGlobalHotkey %s: %w", shortcut, ErrHotkeyAlreadyRegistered))
		}
		return nil, wrapErrorNoPanic(fmt.Errorf("NewGlobalHotkey %s: RegisterHotKey: %w", shortcut, errno))
	}

	h := &GlobalHotkey{
		wb:       wb,
		threadID: win.GetCurrentThreadId(),
		owner:    owner,
		id:       id,
		shortcut: shortcut,
		repeat:   opts.Repeat,
	}
	wb.globalHotkeys = append(wb.globalHotkeys, h)

	return h, nil
}

// hotkeyModifiers converts walk modifiers to the MOD_* flags of
// RegisterHotKey.
func hotkeyModifiers(m Modifiers) uint32 {
	var mods uint32
	if m&ModShift != 0 {
		mods |= win.MOD_SHIFT
	}
	if m&ModControl != 0 {
		mods |= win.MOD_CONTROL
	}
	if m&ModAlt != 0 {
		mods |= win.MOD_ALT
	}
	if m&ModWin != 0 {
		mods |= win.MOD_WIN
	}
	return mods
}

// Shortcut returns the key combination of the hotkey.
func (h *GlobalHotkey) Shortcut() Shortcut {
	return h.shortcut
}

// Repeat returns whether Activated is published repeatedly while the keys
// are held down.
func (h *GlobalHotkey) Repeat() bool {
	return h.repeat
}

// Owner returns the window the hotkey was registered for.
func (h *GlobalHotkey) Owner() Window {
	return h.owner
}

// ID returns the identifier of the hotkey that Windows passes in the
// wParam of WM_HOTKEY to the owner window. It is unique per owner window.
func (h *GlobalHotkey) ID() int {
	return int(h.id)
}

// Activated returns the event that is published on the owner's thread when
// the user presses the key combination.
func (h *GlobalHotkey) Activated() *Event {
	return h.activatedPublisher.Event()
}

// IsDisposed returns whether the hotkey is no longer registered.
//
// After Dispose on another thread than the owner's it becomes true only
// when the owner's thread has unregistered the hotkey.
func (h *GlobalHotkey) IsDisposed() bool {
	return h.disposed.Load()
}

// Dispose unregisters the hotkey. Called on another thread than the
// owner's, it unregisters the hotkey later on the owner's thread through
// Synchronize. Disposing of the owner window disposes of its hotkeys.
func (h *GlobalHotkey) Dispose() {
	if win.GetCurrentThreadId() != h.threadID {
		// The state of the hotkey and its window is only touched on
		// the owner's thread.
		h.wb.Synchronize(h.unregister)
		return
	}

	h.unregister()
}

// unregister unregisters the hotkey on the owner's thread.
func (h *GlobalHotkey) unregister() {
	if h.disposed.Load() {
		return
	}
	h.disposed.Store(true)
	wb := h.wb
	wb.globalHotkeys = slices.DeleteFunc(wb.globalHotkeys, func(x *GlobalHotkey) bool { return x == h })
	if wb.hWnd != 0 {
		win.UnregisterHotKey(wb.hWnd, h.id)
	}
}

// handleHotkey publishes Activated of the hotkey of wb with the ID of a
// WM_HOTKEY message. It returns false if wb has no such hotkey.
func (wb *WindowBase) handleHotkey(wParam uintptr) bool {
	for _, h := range wb.globalHotkeys {
		if uintptr(h.id) == wParam {
			h.activatedPublisher.Publish()
			return true
		}
	}
	return false
}

// disposeGlobalHotkeys unregisters the hotkeys of wb before its window is
// destroyed.
func (wb *WindowBase) disposeGlobalHotkeys() {
	for _, h := range slices.Clone(wb.globalHotkeys) {
		h.unregister()
	}
}
