// Copyright 2010 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

import (
	"golang.org/x/sys/windows"

	"github.com/lpintes/walk/internal/win"
)

type FileDialog struct {
	Title          string
	FilePath       string
	FilePaths      []string
	InitialDirPath string
	Filter         string
	FilterIndex    int
	Flags          uint32
	ShowReadOnlyCB bool
}

func (dlg *FileDialog) show(owner Form, fun func(ofn *win.OPENFILENAME) bool, flags uint32) (accepted bool, err error) {
	ofn := new(win.OPENFILENAME)

	ofn.LStructSize = uint32(unsafe.Sizeof(*ofn))
	if owner != nil {
		ofn.HwndOwner = owner.Handle()
	}

	filter := make([]uint16, len(dlg.Filter)+2)
	copy(filter, win.StringToUTF16(dlg.Filter))
	// Replace '|' with the expected '\0'.
	for i, c := range filter {
		if byte(c) == '|' {
			filter[i] = uint16(0)
		}
	}
	ofn.LpstrFilter = &filter[0]
	ofn.NFilterIndex = uint32(dlg.FilterIndex)

	ofn.LpstrInitialDir = win.StringToUTF16Ptr(dlg.InitialDirPath)
	ofn.LpstrTitle = win.StringToUTF16Ptr(dlg.Title)
	ofn.Flags = win.OFN_FILEMUSTEXIST | flags | dlg.Flags

	if !dlg.ShowReadOnlyCB {
		ofn.Flags |= win.OFN_HIDEREADONLY
	}

	var fileBuf []uint16
	if flags&win.OFN_ALLOWMULTISELECT > 0 {
		fileBuf = make([]uint16, 65536)
	} else {
		fileBuf = make([]uint16, 1024)
		copy(fileBuf, win.StringToUTF16(dlg.FilePath))
	}
	ofn.LpstrFile = &fileBuf[0]
	ofn.NMaxFile = uint32(len(fileBuf))

	if !fun(ofn) {
		errno := win.CommDlgExtendedError()
		if errno != 0 {
			err = newError(fmt.Sprintf("Error %d", errno))
		}
		return
	}

	dlg.FilterIndex = int(ofn.NFilterIndex)

	if flags&win.OFN_ALLOWMULTISELECT > 0 {
		split := func() [][]uint16 {
			var parts [][]uint16

			from := 0
			for i, c := range fileBuf {
				if c == 0 {
					if i == from {
						return parts
					}

					parts = append(parts, fileBuf[from:i])
					from = i + 1
				}
			}

			return parts
		}

		parts := split()

		if len(parts) == 1 {
			dlg.FilePaths = []string{syscall.UTF16ToString(parts[0])}
		} else {
			dirPath := syscall.UTF16ToString(parts[0])
			dlg.FilePaths = make([]string, len(parts)-1)

			for i, fp := range parts[1:] {
				dlg.FilePaths[i] = filepath.Join(dirPath, syscall.UTF16ToString(fp))
			}
		}
	} else {
		dlg.FilePath = syscall.UTF16ToString(fileBuf)
	}

	accepted = true

	return
}

func (dlg *FileDialog) ShowOpen(owner Form) (accepted bool, err error) {
	return dlg.show(owner, win.GetOpenFileName, win.OFN_NOCHANGEDIR)
}

func (dlg *FileDialog) ShowOpenMultiple(owner Form) (accepted bool, err error) {
	return dlg.show(owner, win.GetOpenFileName, win.OFN_ALLOWMULTISELECT|win.OFN_EXPLORER|win.OFN_NOCHANGEDIR)
}

func (dlg *FileDialog) ShowSave(owner Form) (accepted bool, err error) {
	return dlg.show(owner, win.GetSaveFileName, win.OFN_NOCHANGEDIR)
}

// ShowBrowseFolder shows the folder picker of the system file dialog and
// stores the chosen folder in FilePath. The dialog starts in
// InitialDirPath and has the caption Title if they are not empty.
func (dlg *FileDialog) ShowBrowseFolder(owner Form) (accepted bool, err error) {
	if hr := win.OleInitialize(); hr != win.S_OK && hr != win.S_FALSE {
		return false, newError(fmt.Sprint("OleInitialize Error: ", hr))
	}
	defer win.OleUninitialize()

	var fdPtr unsafe.Pointer
	if hr := win.CoCreateInstance(&win.CLSID_FileOpenDialog, nil, win.CLSCTX_INPROC_SERVER, &win.IID_IFileDialog, &fdPtr); win.FAILED(hr) {
		return false, errorFromHRESULT("CoCreateInstance", hr)
	}
	fd := (*win.IFileDialog)(fdPtr)
	defer fd.Release()

	var options uint32
	if hr := fd.GetOptions(&options); win.FAILED(hr) {
		return false, errorFromHRESULT("IFileDialog.GetOptions", hr)
	}
	options |= win.FOS_PICKFOLDERS | win.FOS_FORCEFILESYSTEM | win.FOS_PATHMUSTEXIST | win.FOS_NOCHANGEDIR
	if hr := fd.SetOptions(options); win.FAILED(hr) {
		return false, errorFromHRESULT("IFileDialog.SetOptions", hr)
	}

	if dlg.Title != "" {
		if hr := fd.SetTitle(win.StringToUTF16Ptr(dlg.Title)); win.FAILED(hr) {
			return false, errorFromHRESULT("IFileDialog.SetTitle", hr)
		}
	}

	if dlg.InitialDirPath != "" {
		// A folder that does not exist is ignored, the dialog then starts
		// in its default folder.
		var itemPtr unsafe.Pointer
		if hr := win.SHCreateItemFromParsingName(win.StringToUTF16Ptr(dlg.InitialDirPath), nil, &win.IID_IShellItem, &itemPtr); win.SUCCEEDED(hr) {
			item := (*win.IShellItem)(itemPtr)
			fd.SetFolder(item)
			item.Release()
		}
	}

	var ownerHwnd win.HWND
	if owner != nil {
		ownerHwnd = owner.Handle()
	}

	switch hr := fd.Show(ownerHwnd); {
	case hr == win.HRESULT_FROM_WIN32(win.ERROR_CANCELLED):
		return false, nil
	case win.FAILED(hr):
		return false, errorFromHRESULT("IFileDialog.Show", hr)
	}

	var item *win.IShellItem
	if hr := fd.GetResult(&item); win.FAILED(hr) {
		return false, errorFromHRESULT("IFileDialog.GetResult", hr)
	}
	defer item.Release()

	var path *uint16
	if hr := item.GetDisplayName(win.SIGDN_FILESYSPATH, &path); win.FAILED(hr) {
		return false, errorFromHRESULT("IShellItem.GetDisplayName", hr)
	}
	defer win.CoTaskMemFree(uintptr(unsafe.Pointer(path)))

	dlg.FilePath = windows.UTF16PtrToString(path)
	return dlg.FilePath != "", nil
}
