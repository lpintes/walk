// Copyright 2010 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"syscall"
	"unsafe"
)

import (
	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
)

var webViewIOleClientSiteVtbl *com.VTable

func init() {
	AppendToWalkInit(func() {
		webViewIOleClientSiteVtbl = com.NewVTable(&win.IOleClientSiteVtbl{
			SaveObject:             syscall.NewCallback(webView_IOleClientSite_SaveObject),
			GetMoniker:             syscall.NewCallback(webView_IOleClientSite_GetMoniker),
			GetContainer:           syscall.NewCallback(webView_IOleClientSite_GetContainer),
			ShowObject:             syscall.NewCallback(webView_IOleClientSite_ShowObject),
			OnShowWindow:           syscall.NewCallback(webView_IOleClientSite_OnShowWindow),
			RequestNewObjectLayout: syscall.NewCallback(webView_IOleClientSite_RequestNewObjectLayout),
		}, &win.IID_IOleClientSite)
	})
}

// newWebViewSite returns the COM object that hosts the browser of wv. It
// implements IOleClientSite, IOleInPlaceSite, IDocHostUIHandler and the
// DWebBrowserEvents2 sink.
func newWebViewSite(wv *WebView) *com.Object {
	return com.NewObject(wv,
		webViewIOleClientSiteVtbl,
		webViewIOleInPlaceSiteVtbl,
		webViewIDocHostUIHandlerVtbl,
		webViewDWebBrowserEvents2Vtbl)
}

// Interfaces of the object newWebViewSite returns, in the order of its
// vtables.
const (
	webViewSiteIOleClientSite = iota
	webViewSiteIOleInPlaceSite
	webViewSiteIDocHostUIHandler
	webViewSiteDWebBrowserEvents2
)

// webViewFromThis returns the WebView that implements the COM object
// this belongs to.
func webViewFromThis(this *com.This) *WebView {
	return this.Object().Impl().(*WebView)
}

func webView_IOleClientSite_SaveObject(this *com.This) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleClientSite_GetMoniker(this *com.This, dwAssign, dwWhichMoniker uint32, ppmk *unsafe.Pointer) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleClientSite_GetContainer(this *com.This, ppContainer *unsafe.Pointer) uintptr {
	*ppContainer = nil

	return win.E_NOINTERFACE
}

func webView_IOleClientSite_ShowObject(this *com.This) uintptr {
	return win.S_OK
}

func webView_IOleClientSite_OnShowWindow(this *com.This, fShow win.BOOL) uintptr {
	return win.E_NOTIMPL
}

func webView_IOleClientSite_RequestNewObjectLayout(this *com.This) uintptr {
	return win.E_NOTIMPL
}
