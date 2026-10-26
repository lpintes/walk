// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

import (
	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
)

// This file replaces WebView2Loader.dll of the WebView2 SDK, so that
// programs need not ship it. It finds the client DLL of the WebView2
// runtime, EBWebView\<arch>\EmbeddedBrowserWebView.dll in the runtime
// folder, and calls its entry point
// CreateWebViewEnvironmentWithOptionsInternal, as WebView2Loader.dll does.
//
// The approach follows OpenWebView2Loader by John Chadwick
// (github.com/jchv/OpenWebView2Loader) and its Go port in the
// webviewloader package of github.com/wailsapp/go-webview2 (ISC
// license, Copyright (c) 2020 John Chadwick, Copyright (c) 2022 Wails
// Project Developers): the registry values, the order of the channels and
// the arguments of the entry point come from there.

// ErrWebView2RuntimeNotFound is returned (wrapped) when no WebView2 runtime
// is installed and no WebView2Loader.dll is next to the executable.
var ErrWebView2RuntimeNotFound = errors.New("WebView2 runtime not found")

// webView2ChannelGUIDs are the EdgeUpdate client IDs of the WebView2
// runtime channels, in the order WebView2Loader.dll tries them: stable,
// beta, dev and canary.
var webView2ChannelGUIDs = []string{
	"{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}",
	"{2CD8A007-E189-409D-A2C8-9AF4EF3C72AA}",
	"{0D50BFEC-CD6A-4F9A-964C-C7416E3ACB10}",
	"{65C35B14-6C1D-4122-AC46-7148CC9D6497}",
}

// webView2ClientDLLInFolder returns the path of the client DLL of the
// runtime in folder, or "" if it does not exist.
func webView2ClientDLLInFolder(folder string) string {
	var arch string
	switch runtime.GOARCH {
	case "386":
		arch = "x86"
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return ""
	}
	path := filepath.Join(folder, "EBWebView", arch, "EmbeddedBrowserWebView.dll")
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return ""
	}
	return path
}

// findInstalledWebView2ClientDLL returns the client DLL of the installed
// (Evergreen) WebView2 runtime, or "" if there is none.
//
// EdgeUpdate keeps its keys in the 32-bit registry view (under
// WOW6432Node on 64-bit Windows); KEY_WOW64_32KEY selects that view on
// every architecture and is ignored on 32-bit Windows. The machine-wide
// installation (HKEY_LOCAL_MACHINE) is tried before the per-user one
// (HKEY_CURRENT_USER).
func findInstalledWebView2ClientDLL() string {
	for _, guid := range webView2ChannelGUIDs {
		for _, root := range []win.HKEY{win.HKEY_LOCAL_MACHINE, win.HKEY_CURRENT_USER} {
			// The value WebView2Loader.dll reads: the folder of the
			// current version.
			if folder, ok := webView2RegString(root, `SOFTWARE\Microsoft\EdgeUpdate\ClientState\`+guid, "EBWebView"); ok && folder != "" {
				if dll := webView2ClientDLLInFolder(folder); dll != "" {
					return dll
				}
			}

			// The documented detection: the version in "pv" of the Clients
			// key, in the default installation folder.
			pv, ok := webView2RegString(root, `SOFTWARE\Microsoft\EdgeUpdate\Clients\`+guid, "pv")
			if !ok || pv == "" || pv == "0.0.0.0" {
				continue
			}
			var base string
			if root == win.HKEY_LOCAL_MACHINE {
				base = os.Getenv("ProgramFiles(x86)")
				if base == "" {
					base = os.Getenv("ProgramFiles")
				}
			} else {
				base = os.Getenv("LOCALAPPDATA")
			}
			if base == "" {
				continue
			}
			if dll := webView2ClientDLLInFolder(filepath.Join(base, "Microsoft", "EdgeWebView", "Application", pv)); dll != "" {
				return dll
			}
		}
	}
	return ""
}

// webView2RegString reads a string value from the 32-bit registry view.
func webView2RegString(root win.HKEY, subKey, valueName string) (string, bool) {
	subKey16, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return "", false
	}
	name16, err := syscall.UTF16PtrFromString(valueName)
	if err != nil {
		return "", false
	}

	var hKey win.HKEY
	if win.RegOpenKeyEx(root, subKey16, 0, win.KEY_READ|win.KEY_WOW64_32KEY, &hKey) != win.ERROR_SUCCESS {
		return "", false
	}
	defer win.RegCloseKey(hKey)

	var typ, size uint32
	if win.RegQueryValueEx(hKey, name16, nil, &typ, nil, &size) != win.ERROR_SUCCESS || typ != win.REG_SZ || size == 0 {
		return "", false
	}
	buf := make([]uint16, size/2+1)
	if win.RegQueryValueEx(hKey, name16, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != win.ERROR_SUCCESS {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// createWebView2Environment starts the creation of a WebView2 environment.
// handler (an ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler)
// is called on this thread when it is done; envOptions is an
// ICoreWebView2EnvironmentOptions.
//
// With browserExecutableFolder set, the fixed version runtime in that
// folder is used, otherwise the installed one. If no runtime is found
// this way, WebView2Loader.dll next to the executable is used if it is
// there.
func createWebView2Environment(browserExecutableFolder, userDataFolder string, envOptions, handler *com.Object) error {
	userDataFolder16, err := syscall.UTF16PtrFromString(userDataFolder)
	if err != nil {
		return err
	}

	var dllPath string
	runtimeType := win.WEBVIEW2_RUNTIME_TYPE_INSTALLED
	if browserExecutableFolder != "" {
		folder := browserExecutableFolder
		if !filepath.IsAbs(folder) {
			if exe, err := os.Executable(); err == nil {
				folder = filepath.Join(filepath.Dir(exe), folder)
			}
		}
		dllPath = webView2ClientDLLInFolder(folder)
		runtimeType = win.WEBVIEW2_RUNTIME_TYPE_REDISTRIBUTABLE
	} else {
		dllPath = findInstalledWebView2ClientDLL()
	}

	if dllPath != "" {
		dll, err := syscall.LoadDLL(dllPath)
		if err != nil {
			return fmt.Errorf("loading %s: %w", dllPath, err)
		}
		// The DLL stays loaded for the lifetime of the program.
		proc, err := dll.FindProc("CreateWebViewEnvironmentWithOptionsInternal")
		if err != nil {
			return fmt.Errorf("%s: %w", dllPath, err)
		}
		// HRESULT CreateWebViewEnvironmentWithOptionsInternal(
		//     bool checkRunningInstance, int runtimeType,
		//     PCWSTR userDataFolder, IUnknown* environmentOptions,
		//     ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler* handler)
		hr, _, _ := syscall.SyscallN(proc.Addr(),
			1,
			uintptr(runtimeType),
			uintptr(unsafe.Pointer(userDataFolder16)),
			uintptr(envOptions.Interface(0)),
			uintptr(handler.Interface(0)))
		if win.FAILED(win.HRESULT(int32(hr))) {
			return fmt.Errorf("CreateWebViewEnvironmentWithOptionsInternal: HRESULT %#08x", uint32(hr))
		}
		return nil
	}

	// Fallback: WebView2Loader.dll of the SDK next to the executable.
	exe, err := os.Executable()
	if err != nil {
		return ErrWebView2RuntimeNotFound
	}
	loaderPath := filepath.Join(filepath.Dir(exe), "WebView2Loader.dll")
	if _, err := os.Stat(loaderPath); err != nil {
		return ErrWebView2RuntimeNotFound
	}
	dll, err := syscall.LoadDLL(loaderPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", loaderPath, err)
	}
	proc, err := dll.FindProc("CreateCoreWebView2EnvironmentWithOptions")
	if err != nil {
		return fmt.Errorf("%s: %w", loaderPath, err)
	}
	var browserFolder16 *uint16
	if browserExecutableFolder != "" {
		if browserFolder16, err = syscall.UTF16PtrFromString(browserExecutableFolder); err != nil {
			return err
		}
	}
	hr, _, _ := syscall.SyscallN(proc.Addr(),
		uintptr(unsafe.Pointer(browserFolder16)),
		uintptr(unsafe.Pointer(userDataFolder16)),
		uintptr(envOptions.Interface(0)),
		uintptr(handler.Interface(0)))
	if win.FAILED(win.HRESULT(int32(hr))) {
		if uint32(hr)&0xFFFF == uint32(win.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("CreateCoreWebView2EnvironmentWithOptions: %w", ErrWebView2RuntimeNotFound)
		}
		return fmt.Errorf("CreateCoreWebView2EnvironmentWithOptions: HRESULT %#08x", uint32(hr))
	}
	return nil
}

// defaultWebView2UserDataFolder returns %LOCALAPPDATA%\<exe name>\WebView2.
func defaultWebView2UserDataFolder() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	return filepath.Join(base, name, "WebView2"), nil
}
