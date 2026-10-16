// Copyright 2010 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk/internal/win"
)

// KnownFolder identifies a folder the shell knows, such as the documents
// or downloads folder of the user, for KnownFolderPath.
type KnownFolder int

const (
	KnownFolderDesktop        KnownFolder = iota // desktop of the user
	KnownFolderDocuments                         // documents of the user
	KnownFolderDownloads                         // downloads of the user
	KnownFolderMusic                             // music of the user
	KnownFolderPictures                          // pictures of the user
	KnownFolderVideos                            // videos of the user
	KnownFolderProfile                           // profile folder of the user, such as C:\Users\Name
	KnownFolderRoamingAppData                    // application data that roams with the user (AppData\Roaming)
	KnownFolderLocalAppData                      // application data of the user on this computer (AppData\Local)
	KnownFolderProgramData                       // application data of all users (C:\ProgramData)
	KnownFolderProgramFiles                      // program files of the architecture of the running program
	KnownFolderPrograms                          // programs folder of the Start menu of the user
	KnownFolderStartup                           // programs started when the user signs in
	KnownFolderWindows                           // Windows folder
	KnownFolderSystem                            // System32 folder
)

var knownFolderIDs = [...]*syscall.GUID{
	KnownFolderDesktop:        &win.FOLDERID_Desktop,
	KnownFolderDocuments:      &win.FOLDERID_Documents,
	KnownFolderDownloads:      &win.FOLDERID_Downloads,
	KnownFolderMusic:          &win.FOLDERID_Music,
	KnownFolderPictures:       &win.FOLDERID_Pictures,
	KnownFolderVideos:         &win.FOLDERID_Videos,
	KnownFolderProfile:        &win.FOLDERID_Profile,
	KnownFolderRoamingAppData: &win.FOLDERID_RoamingAppData,
	KnownFolderLocalAppData:   &win.FOLDERID_LocalAppData,
	KnownFolderProgramData:    &win.FOLDERID_ProgramData,
	KnownFolderProgramFiles:   &win.FOLDERID_ProgramFiles,
	KnownFolderPrograms:       &win.FOLDERID_Programs,
	KnownFolderStartup:        &win.FOLDERID_Startup,
	KnownFolderWindows:        &win.FOLDERID_Windows,
	KnownFolderSystem:         &win.FOLDERID_System,
}

// KnownFolderPath returns the path of a known folder. It follows where the
// user moved the folder, for example the documents folder to another drive.
func KnownFolderPath(folder KnownFolder) (string, error) {
	if folder < 0 || int(folder) >= len(knownFolderIDs) {
		return "", newError("invalid known folder")
	}

	var path *uint16
	hr := win.SHGetKnownFolderPath(knownFolderIDs[folder], 0, 0, &path)
	// The path must be freed even if the function fails.
	defer win.CoTaskMemFree(uintptr(unsafe.Pointer(path)))
	if win.FAILED(hr) {
		return "", errorFromHRESULT("SHGetKnownFolderPath", hr)
	}

	return windows.UTF16PtrToString(path), nil
}

func knownFolderPath(id win.CSIDL) (string, error) {
	var buf [win.MAX_PATH]uint16

	if !win.SHGetSpecialFolderPath(0, &buf[0], id, false) {
		return "", newError("SHGetSpecialFolderPath failed")
	}

	return syscall.UTF16ToString(buf[0:]), nil
}

func AppDataPath() (string, error) {
	return knownFolderPath(win.CSIDL_APPDATA)
}

func CommonAppDataPath() (string, error) {
	return knownFolderPath(win.CSIDL_COMMON_APPDATA)
}

func LocalAppDataPath() (string, error) {
	return knownFolderPath(win.CSIDL_LOCAL_APPDATA)
}

func PersonalPath() (string, error) {
	return knownFolderPath(win.CSIDL_PERSONAL)
}

func SystemPath() (string, error) {
	return knownFolderPath(win.CSIDL_SYSTEM)
}

func DriveNames() ([]string, error) {
	bufLen := win.GetLogicalDriveStrings(0, nil)
	if bufLen == 0 {
		return nil, lastError("GetLogicalDriveStrings")
	}
	buf := make([]uint16, bufLen+1)

	bufLen = win.GetLogicalDriveStrings(bufLen+1, &buf[0])
	if bufLen == 0 {
		return nil, lastError("GetLogicalDriveStrings")
	}

	var names []string

	for i := 0; i < len(buf)-2; {
		name := syscall.UTF16ToString(buf[i:])
		names = append(names, name)
		i += len(name) + 1
	}

	return names, nil
}
