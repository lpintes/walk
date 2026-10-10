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
	"time"

	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
)

var webViewDWebBrowserEvents2Vtbl *com.VTable

func init() {
	AppendToWalkInit(func() {
		webViewDWebBrowserEvents2Vtbl = com.NewVTable(&win.DWebBrowserEvents2Vtbl{
			GetTypeInfoCount: syscall.NewCallback(webView_DWebBrowserEvents2_GetTypeInfoCount),
			GetTypeInfo:      syscall.NewCallback(webView_DWebBrowserEvents2_GetTypeInfo),
			GetIDsOfNames:    syscall.NewCallback(webView_DWebBrowserEvents2_GetIDsOfNames),
			Invoke:           syscall.NewCallback(webView_DWebBrowserEvents2_Invoke),
		}, &win.DIID_DWebBrowserEvents2)
	})
}

// The IDispatch methods take as many parameters as in C, so that on 386
// the callbacks pop the right number of bytes from the stack.

func webView_DWebBrowserEvents2_GetTypeInfoCount(this *com.This, pctinfo *uint32) uintptr {
	return win.E_NOTIMPL
}

func webView_DWebBrowserEvents2_GetTypeInfo(this *com.This, iTInfo, lcid, ppTInfo uintptr) uintptr {
	return win.E_NOTIMPL
}

func webView_DWebBrowserEvents2_GetIDsOfNames(this *com.This, riid, rgszNames, cNames, lcid, rgDispId uintptr) uintptr {
	return win.E_NOTIMPL
}

func webView_DWebBrowserEvents2_Invoke(
	this *com.This,
	dispIdMemberArg uintptr, // DISPID
	riid uintptr, // REFIID
	lcid uintptr, // LCID
	wFlags uintptr, // WORD
	pDispParams *win.DISPPARAMS,
	pVarResult uintptr, // *VARIANT
	pExcepInfo uintptr, // *EXCEPINFO
	puArgErr uintptr, // *UINT
) uintptr {
	dispIdMember := win.DISPID(dispIdMemberArg)

	wv := webViewFromThis(this)

	switch dispIdMember {
	case win.DISPID_BEFORENAVIGATE2:
		rgvargPtr := (*[7]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		eventData := &WebViewNavigatingEventData{
			pDisp:           (*rgvargPtr)[6].MustPDispatch(),
			url:             (*rgvargPtr)[5].MustPVariant(),
			flags:           (*rgvargPtr)[4].MustPVariant(),
			targetFrameName: (*rgvargPtr)[3].MustPVariant(),
			postData:        (*rgvargPtr)[2].MustPVariant(),
			headers:         (*rgvargPtr)[1].MustPVariant(),
			cancel:          (*rgvargPtr)[0].MustPBool(),
		}
		wv.navigatingPublisher.Publish(eventData)

	case win.DISPID_NAVIGATECOMPLETE2:
		rgvargPtr := (*[2]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		url := (*rgvargPtr)[0].MustPVariant()
		urlStr := ""
		if url != nil && url.MustBSTR() != nil {
			urlStr = win.BSTRToString(url.MustBSTR())
		}
		wv.navigatedPublisher.Publish(urlStr)

		wv.urlChangedPublisher.Publish()

	case win.DISPID_DOWNLOADBEGIN:
		wv.downloadingPublisher.Publish()

	case win.DISPID_DOWNLOADCOMPLETE:
		wv.downloadedPublisher.Publish()

	case win.DISPID_DOCUMENTCOMPLETE:
		rgvargPtr := (*[2]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		url := (*rgvargPtr)[0].MustPVariant()
		urlStr := ""
		if url != nil && url.MustBSTR() != nil {
			urlStr = win.BSTRToString(url.MustBSTR())
		}

		// FIXME: Horrible hack to avoid glitch where the document is not displayed.
		time.AfterFunc(time.Millisecond*100, func() {
			wv.Synchronize(func() {
				b := wv.BoundsPixels()
				b.Width++
				wv.SetBoundsPixels(b)
				b.Width--
				wv.SetBoundsPixels(b)
			})
		})

		wv.documentCompletedPublisher.Publish(urlStr)

	case win.DISPID_NAVIGATEERROR:
		rgvargPtr := (*[5]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		eventData := &WebViewNavigatedErrorEventData{
			pDisp:           (*rgvargPtr)[4].MustPDispatch(),
			url:             (*rgvargPtr)[3].MustPVariant(),
			targetFrameName: (*rgvargPtr)[2].MustPVariant(),
			statusCode:      (*rgvargPtr)[1].MustPVariant(),
			cancel:          (*rgvargPtr)[0].MustPBool(),
		}
		wv.navigatedErrorPublisher.Publish(eventData)

	case win.DISPID_NEWWINDOW3:
		rgvargPtr := (*[5]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		eventData := &WebViewNewWindowEventData{
			ppDisp:         (*rgvargPtr)[4].MustPPDispatch(),
			cancel:         (*rgvargPtr)[3].MustPBool(),
			dwFlags:        (*rgvargPtr)[2].MustULong(),
			bstrUrlContext: (*rgvargPtr)[1].MustBSTR(),
			bstrUrl:        (*rgvargPtr)[0].MustBSTR(),
		}
		wv.newWindowPublisher.Publish(eventData)

	case win.DISPID_ONQUIT:
		wv.quittingPublisher.Publish()

	case win.DISPID_WINDOWCLOSING:
		rgvargPtr := (*[2]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		eventData := &WebViewWindowClosingEventData{
			bIsChildWindow: (*rgvargPtr)[1].MustBool(),
			cancel:         (*rgvargPtr)[0].MustPBool(),
		}
		wv.windowClosingPublisher.Publish(eventData)

	case win.DISPID_ONSTATUSBAR:
		rgvargPtr := (*[1]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		statusBar := (*rgvargPtr)[0].MustBool()
		if statusBar != win.VARIANT_FALSE {
			wv.statusBarVisible = true
		} else {
			wv.statusBarVisible = false
		}
		wv.statusBarVisibleChangedPublisher.Publish()

	case win.DISPID_ONTHEATERMODE:
		rgvargPtr := (*[1]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		theaterMode := (*rgvargPtr)[0].MustBool()
		if theaterMode != win.VARIANT_FALSE {
			wv.isTheaterMode = true
		} else {
			wv.isTheaterMode = false
		}
		wv.theaterModeChangedPublisher.Publish()

	case win.DISPID_ONTOOLBAR:
		rgvargPtr := (*[1]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		toolBar := (*rgvargPtr)[0].MustBool()
		if toolBar != win.VARIANT_FALSE {
			wv.toolBarVisible = true
		} else {
			wv.toolBarVisible = false
		}
		wv.toolBarVisibleChangedPublisher.Publish()

	case win.DISPID_ONVISIBLE:
		rgvargPtr := (*[1]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		vVisible := (*rgvargPtr)[0].MustBool()
		if vVisible != win.VARIANT_FALSE {
			wv.browserVisible = true
		} else {
			wv.browserVisible = false
		}
		wv.browserVisibleChangedPublisher.Publish()

	case win.DISPID_COMMANDSTATECHANGE:
		rgvargPtr := (*[2]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		command := (*rgvargPtr)[1].MustLong()
		enable := (*rgvargPtr)[0].MustBool()
		enableBool := (enable != win.VARIANT_FALSE)
		switch command {
		case win.CSC_UPDATECOMMANDS:
			wv.toolBarEnabled = enableBool
			wv.toolBarEnabledChangedPublisher.Publish()

		case win.CSC_NAVIGATEFORWARD:
			wv.canGoForward = enableBool
			wv.canGoForwardChangedPublisher.Publish()

		case win.CSC_NAVIGATEBACK:
			wv.canGoBack = enableBool
			wv.canGoBackChangedPublisher.Publish()
		}

	case win.DISPID_PROGRESSCHANGE:
		rgvargPtr := (*[2]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		wv.progressValue = (*rgvargPtr)[1].MustLong()
		wv.progressMax = (*rgvargPtr)[0].MustLong()
		wv.progressChangedPublisher.Publish()

	case win.DISPID_STATUSTEXTCHANGE:
		rgvargPtr := (*[1]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		sText := (*rgvargPtr)[0].MustBSTR()
		if sText != nil {
			wv.statusText = win.BSTRToString(sText)
		} else {
			wv.statusText = ""
		}
		wv.statusTextChangedPublisher.Publish()

	case win.DISPID_TITLECHANGE:
		rgvargPtr := (*[1]win.VARIANTARG)(unsafe.Pointer(pDispParams.Rgvarg))
		sText := (*rgvargPtr)[0].MustBSTR()
		if sText != nil {
			wv.documentTitle = win.BSTRToString(sText)
		} else {
			wv.documentTitle = ""
		}
		wv.documentTitleChangedPublisher.Publish()
	}

	return win.DISP_E_MEMBERNOTFOUND
}
