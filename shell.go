// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk/internal/win"
)

// withCOM runs f on a locked OS thread with COM initialized, which the
// shell objects need.
func withCOM(f func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if hr := win.OleInitialize(); hr != win.S_OK && hr != win.S_FALSE {
		return newError(fmt.Sprint("OleInitialize Error: ", hr))
	}
	defer win.OleUninitialize()

	return f()
}

// shellItem returns the shell item of a file system path. The caller must
// release it.
func shellItem(path string) (*win.IShellItem, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, wrapError(err)
	}
	var itemPtr unsafe.Pointer
	if hr := win.SHCreateItemFromParsingName(win.StringToUTF16Ptr(abs), nil, &win.IID_IShellItem, &itemPtr); win.FAILED(hr) {
		return nil, errorFromHRESULT("SHCreateItemFromParsingName", hr)
	}
	return (*win.IShellItem)(itemPtr), nil
}

// MoveToRecycleBin moves files and folders to the Recycle Bin, where the
// user can restore them.
//
// It shows no confirmation, progress or error dialog, with one exception:
// if an item cannot be recycled, for example on a network drive, Windows
// asks the user whether to delete it permanently, with owner (which may be
// nil) as the owner of that dialog. If the user declines, or any item
// cannot be moved, MoveToRecycleBin returns an error.
func MoveToRecycleBin(owner Form, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}

	return withCOM(func() error {
		var opPtr unsafe.Pointer
		if hr := win.CoCreateInstance(&win.CLSID_FileOperation, nil, win.CLSCTX_INPROC_SERVER, &win.IID_IFileOperation, &opPtr); win.FAILED(hr) {
			return errorFromHRESULT("CoCreateInstance", hr)
		}
		op := (*win.IFileOperation)(opPtr)
		defer op.Release()

		flags := uint32(win.FOF_ALLOWUNDO | win.FOFX_RECYCLEONDELETE | win.FOF_NOCONFIRMATION |
			win.FOF_WANTNUKEWARNING | win.FOF_NOERRORUI | win.FOF_SILENT)
		if hr := op.SetOperationFlags(flags); win.FAILED(hr) {
			return errorFromHRESULT("IFileOperation.SetOperationFlags", hr)
		}
		if owner != nil {
			if hr := op.SetOwnerWindow(owner.Handle()); win.FAILED(hr) {
				return errorFromHRESULT("IFileOperation.SetOwnerWindow", hr)
			}
		}

		for _, path := range paths {
			item, err := shellItem(path)
			if err != nil {
				return err
			}
			hr := op.DeleteItem(item, nil)
			item.Release()
			if win.FAILED(hr) {
				return errorFromHRESULT("IFileOperation.DeleteItem", hr)
			}
		}

		if hr := op.PerformOperations(); win.FAILED(hr) {
			return errorFromHRESULT("IFileOperation.PerformOperations", hr)
		}

		var aborted win.BOOL
		if hr := op.GetAnyOperationsAborted(&aborted); win.FAILED(hr) {
			return errorFromHRESULT("IFileOperation.GetAnyOperationsAborted", hr)
		}
		if aborted != 0 {
			return newError("moving to the Recycle Bin was aborted")
		}

		return nil
	})
}

// ShellLink is the content of a shortcut file (.lnk), as shown in the
// properties of a shortcut in Explorer.
type ShellLink struct {
	TargetPath       string   // file or folder the shortcut opens
	Arguments        string   // command line arguments for the target
	WorkingDirectory string   // folder the target starts in
	Description      string   // comment shown as the tooltip of the shortcut
	IconPath         string   // file with the icon; empty for the icon of the target
	IconIndex        int      // index of the icon in IconPath
	Hotkey           Shortcut // keys that open the shortcut from the desktop or Start menu; zero for none
}

const shellLinkBufLen = 32768

// LoadShellLink reads a shortcut file.
func LoadShellLink(path string) (*ShellLink, error) {
	var link *ShellLink
	err := withShellLink(func(sl *win.IShellLink, pf *win.IPersistFile) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return wrapError(err)
		}
		pathPtr, err := windows.UTF16PtrFromString(abs)
		if err != nil {
			return wrapError(err)
		}
		if hr := pf.Load(pathPtr, win.STGM_READ); win.FAILED(hr) {
			return errorFromHRESULT("IPersistFile.Load", hr)
		}

		buf := make([]uint16, shellLinkBufLen)
		get := func(name string, f func(buf *uint16, cch int32) win.HRESULT) (string, error) {
			clear(buf)
			if hr := f(&buf[0], int32(len(buf))); win.FAILED(hr) {
				return "", errorFromHRESULT("IShellLink."+name, hr)
			}
			return windows.UTF16ToString(buf), nil
		}

		l := new(ShellLink)
		if l.TargetPath, err = get("GetPath", func(buf *uint16, cch int32) win.HRESULT {
			return sl.GetPath(buf, cch, nil, 0)
		}); err != nil {
			return err
		}
		if l.Arguments, err = get("GetArguments", sl.GetArguments); err != nil {
			return err
		}
		if l.WorkingDirectory, err = get("GetWorkingDirectory", sl.GetWorkingDirectory); err != nil {
			return err
		}
		if l.Description, err = get("GetDescription", sl.GetDescription); err != nil {
			return err
		}
		var iconIndex int32
		if l.IconPath, err = get("GetIconLocation", func(buf *uint16, cch int32) win.HRESULT {
			return sl.GetIconLocation(buf, cch, &iconIndex)
		}); err != nil {
			return err
		}
		l.IconIndex = int(iconIndex)

		var hotkey uint16
		if hr := sl.GetHotkey(&hotkey); win.FAILED(hr) {
			return errorFromHRESULT("IShellLink.GetHotkey", hr)
		}
		// The low byte is the virtual key, the high byte holds the
		// HOTKEYF_SHIFT, HOTKEYF_CONTROL and HOTKEYF_ALT flags, which
		// have the values of ModShift, ModControl and ModAlt.
		l.Hotkey = Shortcut{
			Modifiers: Modifiers(hotkey>>8) & (ModShift | ModControl | ModAlt),
			Key:       Key(hotkey & 0xff),
		}

		link = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return link, nil
}

// Save writes the shortcut to a file, which should have the extension
// .lnk. An existing file is replaced.
func (l *ShellLink) Save(path string) error {
	return withShellLink(func(sl *win.IShellLink, pf *win.IPersistFile) error {
		set := func(name, value string, f func(*uint16) win.HRESULT) error {
			p, err := windows.UTF16PtrFromString(value)
			if err != nil {
				return wrapError(err)
			}
			if hr := f(p); win.FAILED(hr) {
				return errorFromHRESULT("IShellLink."+name, hr)
			}
			return nil
		}

		if err := set("SetPath", l.TargetPath, sl.SetPath); err != nil {
			return err
		}
		if err := set("SetArguments", l.Arguments, sl.SetArguments); err != nil {
			return err
		}
		if err := set("SetWorkingDirectory", l.WorkingDirectory, sl.SetWorkingDirectory); err != nil {
			return err
		}
		if err := set("SetDescription", l.Description, sl.SetDescription); err != nil {
			return err
		}
		if l.IconPath != "" {
			if err := set("SetIconLocation", l.IconPath, func(p *uint16) win.HRESULT {
				return sl.SetIconLocation(p, int32(l.IconIndex))
			}); err != nil {
				return err
			}
		}
		hotkey := uint16(l.Hotkey.Modifiers&(ModShift|ModControl|ModAlt))<<8 | uint16(l.Hotkey.Key&0xff)
		if hr := sl.SetHotkey(hotkey); win.FAILED(hr) {
			return errorFromHRESULT("IShellLink.SetHotkey", hr)
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return wrapError(err)
		}
		absPtr, err := windows.UTF16PtrFromString(abs)
		if err != nil {
			return wrapError(err)
		}
		if hr := pf.Save(absPtr, true); win.FAILED(hr) {
			return errorFromHRESULT("IPersistFile.Save", hr)
		}
		return nil
	})
}

// withShellLink runs f with a new shell link object and its IPersistFile
// interface.
func withShellLink(f func(sl *win.IShellLink, pf *win.IPersistFile) error) error {
	return withCOM(func() error {
		var slPtr unsafe.Pointer
		if hr := win.CoCreateInstance(&win.CLSID_ShellLink, nil, win.CLSCTX_INPROC_SERVER, &win.IID_IShellLinkW, &slPtr); win.FAILED(hr) {
			return errorFromHRESULT("CoCreateInstance", hr)
		}
		sl := (*win.IShellLink)(slPtr)
		defer sl.Release()

		var pfPtr unsafe.Pointer
		if hr := sl.QueryInterface((*syscall.GUID)(&win.IID_IPersistFile), &pfPtr); win.FAILED(hr) {
			return errorFromHRESULT("IShellLink.QueryInterface", hr)
		}
		pf := (*win.IPersistFile)(pfPtr)
		defer pf.Release()

		return f(sl, pf)
	})
}

// ShellOpen opens a file, folder or web address as a double click in
// Explorer does: a document in its default program, a folder in Explorer, a
// URL in the default browser. Dialogs Windows shows, for example to choose
// a program for a file type without one, are owned by owner, which may be
// nil.
func ShellOpen(owner Form, target string) error {
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return wrapError(err)
	}
	var hwnd win.HWND
	if owner != nil {
		hwnd = owner.Handle()
	}

	return withCOM(func() error {
		// SEE_MASK_NOASYNC makes ShellExecuteEx finish before it returns.
		// Without it the shell may complete the launch later on this
		// thread, which would be cut short by OleUninitialize or would
		// never happen on a goroutine without a message loop.
		// SEE_MASK_FLAG_NO_UI returns errors, for example a missing file,
		// to the caller instead of showing a message box.
		sei := win.SHELLEXECUTEINFO{
			FMask:  win.SEE_MASK_NOASYNC | win.SEE_MASK_FLAG_NO_UI,
			Hwnd:   hwnd,
			LpFile: targetPtr,
			NShow:  win.SW_SHOWNORMAL,
		}
		sei.CbSize = uint32(unsafe.Sizeof(sei))
		if win.ShellExecuteEx(&sei) {
			return nil
		}
		errno := syscall.Errno(win.GetLastError())
		if sei.HInstApp == win.SE_ERR_NOASSOC {
			return newError(fmt.Sprintf("ShellExecuteEx: no program is associated with %q", target))
		}
		return newError(fmt.Sprintf("ShellExecuteEx failed for %q: %v (error %d)", target, errno, uint32(errno)))
	})
}

// ShowInFolder opens the folder that holds a file or folder in Explorer and
// selects it there.
func ShowInFolder(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return wrapError(err)
	}
	absPtr, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return wrapError(err)
	}

	return withCOM(func() error {
		var pidl uintptr
		if hr := win.SHParseDisplayName(absPtr, nil, &pidl, 0, nil); win.FAILED(hr) {
			return errorFromHRESULT("SHParseDisplayName", hr)
		}
		defer win.CoTaskMemFree(pidl)

		// With no child items, the folder of pidl is opened and the item
		// itself is selected.
		if hr := win.SHOpenFolderAndSelectItems(pidl, 0, nil, 0); win.FAILED(hr) {
			return errorFromHRESULT("SHOpenFolderAndSelectItems", hr)
		}
		return nil
	})
}
