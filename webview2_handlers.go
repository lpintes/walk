// Copyright 2026 The Walk Authors. All rights reserved.
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

// The COM objects WebView2 calls back: the environment options, the
// completion handlers and the event handlers of a WebView2. WebView2 calls
// them on the thread that created the environment, the UI thread of the
// widget, from its message loop.

var (
	webView2EnvironmentOptionsVtbl     *com.VTable
	webView2EnvironmentCompletedVtbl   *com.VTable
	webView2ControllerCompletedVtbl    *com.VTable
	webView2ExecuteScriptCompletedVtbl *com.VTable
	webView2NavigationStartingVtbl     *com.VTable
	webView2NavigationCompletedVtbl    *com.VTable
	webView2SourceChangedVtbl          *com.VTable
	webView2HistoryChangedVtbl         *com.VTable
	webView2DocumentTitleChangedVtbl   *com.VTable
	webView2WebMessageReceivedVtbl     *com.VTable
	webView2MoveFocusRequestedVtbl     *com.VTable
)

func init() {
	AppendToWalkInit(func() {
		put := syscall.NewCallback(webView2_EnvironmentOptions_Put)
		webView2EnvironmentOptionsVtbl = com.NewVTable(&win.ICoreWebView2EnvironmentOptionsVtbl{
			Get_AdditionalBrowserArguments:             syscall.NewCallback(webView2_EnvironmentOptions_Get_AdditionalBrowserArguments),
			Put_AdditionalBrowserArguments:             put,
			Get_Language:                               syscall.NewCallback(webView2_EnvironmentOptions_Get_Language),
			Put_Language:                               put,
			Get_TargetCompatibleBrowserVersion:         syscall.NewCallback(webView2_EnvironmentOptions_Get_TargetCompatibleBrowserVersion),
			Put_TargetCompatibleBrowserVersion:         put,
			Get_AllowSingleSignOnUsingOSPrimaryAccount: syscall.NewCallback(webView2_EnvironmentOptions_Get_AllowSingleSignOnUsingOSPrimaryAccount),
			Put_AllowSingleSignOnUsingOSPrimaryAccount: put,
		}, &win.IID_ICoreWebView2EnvironmentOptions)

		webView2EnvironmentCompletedVtbl = com.NewVTable(&win.ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_EnvironmentCompleted_Invoke),
		}, &win.IID_ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler)

		webView2ControllerCompletedVtbl = com.NewVTable(&win.ICoreWebView2CreateCoreWebView2ControllerCompletedHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_ControllerCompleted_Invoke),
		}, &win.IID_ICoreWebView2CreateCoreWebView2ControllerCompletedHandler)

		webView2ExecuteScriptCompletedVtbl = com.NewVTable(&win.ICoreWebView2ExecuteScriptCompletedHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_ExecuteScriptCompleted_Invoke),
		}, &win.IID_ICoreWebView2ExecuteScriptCompletedHandler)

		webView2NavigationStartingVtbl = com.NewVTable(&win.ICoreWebView2NavigationStartingEventHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_NavigationStarting_Invoke),
		}, &win.IID_ICoreWebView2NavigationStartingEventHandler)

		webView2NavigationCompletedVtbl = com.NewVTable(&win.ICoreWebView2NavigationCompletedEventHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_NavigationCompleted_Invoke),
		}, &win.IID_ICoreWebView2NavigationCompletedEventHandler)

		webView2SourceChangedVtbl = com.NewVTable(&win.ICoreWebView2SourceChangedEventHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_SourceChanged_Invoke),
		}, &win.IID_ICoreWebView2SourceChangedEventHandler)

		webView2HistoryChangedVtbl = com.NewVTable(&win.ICoreWebView2HistoryChangedEventHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_HistoryChanged_Invoke),
		}, &win.IID_ICoreWebView2HistoryChangedEventHandler)

		webView2DocumentTitleChangedVtbl = com.NewVTable(&win.ICoreWebView2DocumentTitleChangedEventHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_DocumentTitleChanged_Invoke),
		}, &win.IID_ICoreWebView2DocumentTitleChangedEventHandler)

		webView2WebMessageReceivedVtbl = com.NewVTable(&win.ICoreWebView2WebMessageReceivedEventHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_WebMessageReceived_Invoke),
		}, &win.IID_ICoreWebView2WebMessageReceivedEventHandler)

		webView2MoveFocusRequestedVtbl = com.NewVTable(&win.ICoreWebView2MoveFocusRequestedEventHandlerVtbl{
			Invoke: syscall.NewCallback(webView2_MoveFocusRequested_Invoke),
		}, &win.IID_ICoreWebView2MoveFocusRequestedEventHandler)
	})
}

// Environment options

// webView2EnvironmentOptions implements ICoreWebView2EnvironmentOptions.
// The put methods are ignored: WebView2 only reads the options.
type webView2EnvironmentOptions struct {
	additionalBrowserArguments string
	language                   string
}

// webView2TargetCompatibleBrowserVersion is the oldest runtime version the
// declarations in internal/win need. Every runtime since 86.0.616.0 (the
// first stable WebView2 release) has the interfaces walk uses.
const webView2TargetCompatibleBrowserVersion = "86.0.616.0"

func newWebView2EnvironmentOptions(opts *WebView2Options) *com.Object {
	return com.NewObject(&webView2EnvironmentOptions{
		additionalBrowserArguments: opts.AdditionalBrowserArguments,
		language:                   opts.Language,
	}, webView2EnvironmentOptionsVtbl)
}

// webView2CoTaskMemString stores in value a copy of s allocated with
// CoTaskMemAlloc, which the caller frees.
func webView2CoTaskMemString(s string, value **uint16) uintptr {
	if value == nil {
		return win.E_POINTER
	}
	s16, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return win.E_INVALIDARG
	}
	return uintptr(win.SHStrDup(s16, value))
}

func webView2EnvironmentOptionsFromThis(this *com.This) *webView2EnvironmentOptions {
	return this.Object().Impl().(*webView2EnvironmentOptions)
}

func webView2_EnvironmentOptions_Get_AdditionalBrowserArguments(this *com.This, value **uint16) uintptr {
	return webView2CoTaskMemString(webView2EnvironmentOptionsFromThis(this).additionalBrowserArguments, value)
}

func webView2_EnvironmentOptions_Get_Language(this *com.This, value **uint16) uintptr {
	return webView2CoTaskMemString(webView2EnvironmentOptionsFromThis(this).language, value)
}

func webView2_EnvironmentOptions_Get_TargetCompatibleBrowserVersion(this *com.This, value **uint16) uintptr {
	return webView2CoTaskMemString(webView2TargetCompatibleBrowserVersion, value)
}

func webView2_EnvironmentOptions_Get_AllowSingleSignOnUsingOSPrimaryAccount(this *com.This, allow *win.BOOL) uintptr {
	if allow == nil {
		return win.E_POINTER
	}
	*allow = win.FALSE
	return win.S_OK
}

func webView2_EnvironmentOptions_Put(this *com.This, value uintptr) uintptr {
	return win.E_NOTIMPL
}

// Completion handlers

func webView2FromThis(this *com.This) *WebView2 {
	return this.Object().Impl().(*WebView2)
}

// webView2Event returns the WebView2 of an event handler, or nil after
// the widget has been disposed. The caller defers the function returned
// with it (see WebView2.enterCallback).
func webView2Event(this *com.This) (*WebView2, func()) {
	wv := webView2FromThis(this)
	if wv.disposed {
		return nil, func() {}
	}
	return wv, wv.enterCallback()
}

func webView2_EnvironmentCompleted_Invoke(this *com.This, errorCode uintptr, env *win.ICoreWebView2Environment) uintptr {
	wv := webView2FromThis(this)
	defer wv.enterCallback()()
	wv.onEnvironmentCompleted(win.HRESULT(int32(errorCode)), env)
	return win.S_OK
}

func webView2_ControllerCompleted_Invoke(this *com.This, errorCode uintptr, controller *win.ICoreWebView2Controller) uintptr {
	wv := webView2FromThis(this)
	defer wv.enterCallback()()
	wv.onControllerCompleted(win.HRESULT(int32(errorCode)), controller)
	return win.S_OK
}

// webView2ScriptCallback is the implementation of an
// ICoreWebView2ExecuteScriptCompletedHandler.
type webView2ScriptCallback struct {
	wv       *WebView2
	callback func(resultJSON string, err error)
}

func webView2_ExecuteScriptCompleted_Invoke(this *com.This, errorCode uintptr, resultJSON *uint16) uintptr {
	sc := this.Object().Impl().(*webView2ScriptCallback)
	if sc.callback == nil {
		return win.S_OK
	}
	defer sc.wv.enterCallback()()
	if hr := win.HRESULT(int32(errorCode)); win.FAILED(hr) {
		sc.callback("", webView2Error("ExecuteScript", hr))
		return win.S_OK
	}
	// resultJSON belongs to WebView2; it is not freed here.
	sc.callback(win.UTF16PtrToString(resultJSON), nil)
	return win.S_OK
}

// Event handlers. One object, created by each WebView2, implements all of
// them; its vtables are in the order of the constants below.

const (
	webView2EventsNavigationStarting = iota
	webView2EventsNavigationCompleted
	webView2EventsSourceChanged
	webView2EventsHistoryChanged
	webView2EventsDocumentTitleChanged
	webView2EventsWebMessageReceived
	webView2EventsMoveFocusRequested
)

func newWebView2EventHandlers(wv *WebView2) *com.Object {
	return com.NewObject(wv,
		webView2NavigationStartingVtbl,
		webView2NavigationCompletedVtbl,
		webView2SourceChangedVtbl,
		webView2HistoryChangedVtbl,
		webView2DocumentTitleChangedVtbl,
		webView2WebMessageReceivedVtbl,
		webView2MoveFocusRequestedVtbl)
}

func webView2_NavigationStarting_Invoke(this *com.This, sender *win.ICoreWebView2, args *win.ICoreWebView2NavigationStartingEventArgs) uintptr {
	wv, leave := webView2Event(this)
	defer leave()
	var uri *uint16
	if wv != nil && args != nil && win.SUCCEEDED(args.Get_Uri(&uri)) {
		wv.navigationStartingPublisher.Publish(win.CoTaskMemString(uri))
	}
	return win.S_OK
}

func webView2_NavigationCompleted_Invoke(this *com.This, sender *win.ICoreWebView2, args *win.ICoreWebView2NavigationCompletedEventArgs) uintptr {
	wv, leave := webView2Event(this)
	defer leave()
	if wv == nil {
		return win.S_OK
	}
	var success win.BOOL
	var status int32
	if args != nil {
		args.Get_IsSuccess(&success)
		args.Get_WebErrorStatus(&status)
	}
	wv.navigationCompletedPublisher.Publish(success != 0, WebView2WebErrorStatus(status))
	return win.S_OK
}

func webView2_SourceChanged_Invoke(this *com.This, sender *win.ICoreWebView2, args unsafe.Pointer) uintptr {
	wv, leave := webView2Event(this)
	defer leave()
	if wv != nil {
		wv.onSourceChanged()
	}
	return win.S_OK
}

func webView2_HistoryChanged_Invoke(this *com.This, sender *win.ICoreWebView2, args unsafe.Pointer) uintptr {
	wv, leave := webView2Event(this)
	defer leave()
	if wv != nil {
		wv.historyChangedPublisher.Publish()
	}
	return win.S_OK
}

func webView2_DocumentTitleChanged_Invoke(this *com.This, sender *win.ICoreWebView2, args unsafe.Pointer) uintptr {
	wv, leave := webView2Event(this)
	defer leave()
	if wv != nil {
		wv.onDocumentTitleChanged()
	}
	return win.S_OK
}

func webView2_WebMessageReceived_Invoke(this *com.This, sender *win.ICoreWebView2, args *win.ICoreWebView2WebMessageReceivedEventArgs) uintptr {
	wv, leave := webView2Event(this)
	defer leave()
	var json *uint16
	if wv != nil && args != nil && win.SUCCEEDED(args.Get_WebMessageAsJson(&json)) {
		wv.webMessageReceivedPublisher.Publish(win.CoTaskMemString(json))
	}
	return win.S_OK
}

func webView2_MoveFocusRequested_Invoke(this *com.This, sender *win.ICoreWebView2Controller, args *win.ICoreWebView2MoveFocusRequestedEventArgs) uintptr {
	wv, leave := webView2Event(this)
	defer leave()
	if wv == nil || args == nil {
		return win.S_OK
	}
	var reason int32
	if win.FAILED(args.Get_Reason(&reason)) {
		return win.S_OK
	}
	if wv.moveFocusOut(reason) {
		args.Put_Handled(win.TRUE)
	}
	return win.S_OK
}
