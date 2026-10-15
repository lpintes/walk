// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"fmt"
	"path/filepath"
	"runtime"
	"unsafe"

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
