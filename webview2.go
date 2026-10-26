// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

import (
	"github.com/lpintes/walk/internal/com"
	"github.com/lpintes/walk/internal/win"
)

const webView2WindowClass = `\o/ Walk_WebView2_Class \o/`

func init() {
	AppendToWalkInit(func() {
		MustRegisterWindowClass(webView2WindowClass)
	})
}

// ErrWebView2NotReady is returned by the methods of a WebView2 that need
// the browser before it has been created (see [WebView2.Ready]) or after
// its creation failed.
var ErrWebView2NotReady = errors.New("WebView2 is not ready")

// WebView2Options are the options of [NewWebView2WithOptions]. The zero
// value selects the defaults.
type WebView2Options struct {
	// UserDataFolder is the folder where WebView2 keeps cookies, the
	// cache and other browser data. The default is
	// %LOCALAPPDATA%\<executable name>\WebView2. WebView2 widgets sharing
	// a folder share one browser process.
	UserDataFolder string

	// BrowserExecutableFolder is the folder of a fixed version WebView2
	// runtime shipped with the program, absolute or relative to the
	// executable. Empty selects the installed (Evergreen) runtime.
	BrowserExecutableFolder string

	// AdditionalBrowserArguments are command line switches for the
	// browser process.
	AdditionalBrowserArguments string

	// Language is the display language of the browser UI (context menus,
	// dialogs) and the Accept-Language header, for example "sk-SK".
	// Empty selects the language of Windows.
	Language string
}

// WebView2WebErrorStatus is the COREWEBVIEW2_WEB_ERROR_STATUS of a failed
// navigation; 0 means unknown.
type WebView2WebErrorStatus int

// WebView2 is a browser widget based on Microsoft Edge WebView2
// (Chromium). It needs the WebView2 runtime, which is part of Windows 11
// and installed on most Windows 10 machines; WebView2Loader.dll is not
// needed.
//
// The browser is created asynchronously: NewWebView2 returns at once and
// [WebView2.Ready] is published when the browser can be used, or
// [WebView2.CreationFailed] if it cannot be created (for example without
// a runtime). [WebView2.SetURL] and [WebView2.NavigateToString] called
// before that are remembered and the last one is carried out when the
// browser is ready; the other methods that need the browser return
// [ErrWebView2NotReady] until then.
//
// When the widget gets the keyboard focus, it moves the focus into the web
// content; Tab and Shift+Tab past the first or last element of the page
// move the focus to the next or previous control of the dialog.
type WebView2 struct {
	WidgetBase
	options     WebView2Options
	environment *win.ICoreWebView2Environment
	controller  *win.ICoreWebView2Controller
	webView     *win.ICoreWebView2
	events      *com.Object // the event handlers, see newWebView2EventHandlers
	creationErr error
	disposed    bool
	oleInit     bool

	// removeHandlers remove the event handlers added to the browser.
	removeHandlers []func()

	// boundsForm is the form whose BoundsChanged event the widget is
	// attached to, with the handle boundsHandle.
	boundsForm   Form
	boundsHandle int

	// callbackDepth counts the WebView2 callbacks running on the stack;
	// the browser is not released from inside one of them.
	callbackDepth int

	pendingNavigation func() error
	url               string
	title             string

	readyPublisher               EventPublisher
	creationFailedPublisher      ErrorEventPublisher
	navigationStartingPublisher  StringEventPublisher
	navigationCompletedPublisher WebView2NavigationCompletedEventPublisher
	urlChangedPublisher          EventPublisher
	titleChangedPublisher        EventPublisher
	historyChangedPublisher      EventPublisher
	webMessageReceivedPublisher  StringEventPublisher
}

// NewWebView2 creates a WebView2 with the default options.
func NewWebView2(parent Container) (*WebView2, error) {
	return NewWebView2WithOptions(parent, WebView2Options{})
}

// NewWebView2WithOptions creates a WebView2 with the given options.
//
// It returns an error if the window cannot be created or the creation of
// the browser cannot be started, for example because no WebView2 runtime
// is installed (the error then wraps [ErrWebView2RuntimeNotFound]).
// Errors found later are published with [WebView2.CreationFailed].
func NewWebView2WithOptions(parent Container, options WebView2Options) (*WebView2, error) {
	wv := &WebView2{options: options}

	if err := InitWidget(
		wv,
		parent,
		webView2WindowClass,
		win.WS_CLIPCHILDREN|win.WS_TABSTOP|win.WS_VISIBLE,
		0); err != nil {
		return nil, err
	}

	succeeded := false
	defer func() {
		if !succeeded {
			wv.Dispose()
		}
	}()

	// WebView2 needs a single-threaded apartment.
	if hr := win.OleInitialize(); hr != win.S_OK && hr != win.S_FALSE {
		return nil, newError(fmt.Sprint("OleInitialize Error: ", hr))
	}
	wv.oleInit = true

	userDataFolder := options.UserDataFolder
	if userDataFolder == "" {
		var err error
		if userDataFolder, err = defaultWebView2UserDataFolder(); err != nil {
			return nil, wrapError(err)
		}
	}

	envOptions := newWebView2EnvironmentOptions(&options)
	defer envOptions.Release()
	handler := com.NewObject(wv, webView2EnvironmentCompletedVtbl)
	defer handler.Release()

	if err := createWebView2Environment(options.BrowserExecutableFolder, userDataFolder, envOptions, handler); err != nil {
		return nil, wrapError(err)
	}

	wv.MustRegisterProperty("URL", NewProperty(
		func() any {
			return wv.URL()
		},
		func(v any) error {
			return wv.SetURL(assertStringOr(v, ""))
		},
		wv.urlChangedPublisher.Event()))

	wv.MustRegisterProperty("Title", NewReadOnlyProperty(
		func() any {
			return wv.Title()
		},
		wv.titleChangedPublisher.Event()))

	succeeded = true

	return wv, nil
}

// Dispose closes the browser and destroys the widget.
func (wv *WebView2) Dispose() {
	if !wv.disposed {
		wv.disposed = true
		oleUninitialize := wv.oleInit
		wv.oleInit = false
		wv.releaseBrowser(oleUninitialize)
	}
	wv.WidgetBase.Dispose()
}

// releaseBrowser removes the event handlers, closes the browser, releases
// the COM objects and, if oleUninitialize is true, calls OleUninitialize.
// Inside a WebView2 callback (for example a NavigationCompleted handler
// that disposes the widget) the browser is only hidden at once; the rest
// is done by the message loop after the callback has returned, because
// WebView2 still uses its objects then.
func (wv *WebView2) releaseBrowser(oleUninitialize bool) {
	if wv.boundsForm != nil {
		wv.boundsForm.BoundsChanged().Detach(wv.boundsHandle)
		wv.boundsForm = nil
	}

	removeHandlers, events := wv.removeHandlers, wv.events
	webView, controller, environment := wv.webView, wv.controller, wv.environment
	wv.removeHandlers, wv.events = nil, nil
	wv.webView, wv.controller, wv.environment = nil, nil, nil

	release := func() {
		for _, remove := range removeHandlers {
			remove()
		}
		if webView != nil {
			webView.Release()
		}
		if controller != nil {
			controller.Close()
			controller.Release()
		}
		if environment != nil {
			environment.Release()
		}
		if events != nil {
			events.Release()
		}
		if oleUninitialize {
			win.OleUninitialize()
		}
	}

	if wv.callbackDepth > 0 && wv.hWnd != 0 {
		if root := win.GetAncestor(wv.hWnd, win.GA_ROOT); root != 0 {
			if controller != nil {
				controller.Put_IsVisible(win.FALSE)
			}
			// The message loop runs the queued functions after each
			// message, so this runs right after the callback returns.
			wv.group.Synchronize(release)
			win.PostMessage(root, syncMsgId, 0, 0)
			return
		}
	}
	release()
}

// enterCallback is called at the start of each WebView2 callback; the
// function it returns at its end.
func (wv *WebView2) enterCallback() func() {
	wv.callbackDepth++
	return func() { wv.callbackDepth-- }
}

// IsReady reports whether the browser has been created.
func (wv *WebView2) IsReady() bool {
	return wv.webView != nil
}

// Ready is published once, when the browser has been created.
func (wv *WebView2) Ready() *Event {
	return wv.readyPublisher.Event()
}

// CreationFailed is published once if the browser cannot be created.
func (wv *WebView2) CreationFailed() *ErrorEvent {
	return wv.creationFailedPublisher.Event()
}

// CreationError returns the error that prevented the creation of the
// browser, or nil.
func (wv *WebView2) CreationError() error {
	return wv.creationErr
}

func (wv *WebView2) failCreation(err error) {
	if wv.creationErr != nil {
		return
	}
	wv.creationErr = err
	wv.pendingNavigation = nil
	wv.releaseBrowser(false)
	wv.creationFailedPublisher.Publish(err)
}

func webView2Error(funcName string, hr win.HRESULT) error {
	return newErrorNoPanic(fmt.Sprintf("%s: HRESULT %#08x", funcName, uint32(hr)))
}

// webView2CreationError is webView2Error for the asynchronous creation of
// the browser; a missing runtime gives an error wrapping
// ErrWebView2RuntimeNotFound, as from NewWebView2.
func webView2CreationError(funcName string, hr win.HRESULT) error {
	if hr == win.HRESULT_FROM_WIN32(win.ERROR_FILE_NOT_FOUND) {
		return wrapErrorNoPanic(fmt.Errorf("%s: %w", funcName, ErrWebView2RuntimeNotFound))
	}
	return webView2Error(funcName, hr)
}

func (wv *WebView2) onEnvironmentCompleted(hr win.HRESULT, env *win.ICoreWebView2Environment) {
	if wv.disposed || wv.creationErr != nil {
		return
	}
	if win.FAILED(hr) || env == nil {
		wv.failCreation(webView2CreationError("CreateCoreWebView2Environment", hr))
		return
	}

	// env is only lent to the handler; QueryInterface adds the reference
	// the widget keeps, as WebView2Loader.dll does.
	var p unsafe.Pointer
	if hr := env.QueryInterface(&win.IID_ICoreWebView2Environment, &p); win.FAILED(hr) {
		wv.failCreation(webView2Error("ICoreWebView2Environment.QueryInterface", hr))
		return
	}
	wv.environment = (*win.ICoreWebView2Environment)(p)

	handler := com.NewObject(wv, webView2ControllerCompletedVtbl)
	defer handler.Release()
	if hr := wv.environment.CreateCoreWebView2Controller(wv.hWnd,
		(*win.ICoreWebView2CreateCoreWebView2ControllerCompletedHandler)(handler.Interface(0))); win.FAILED(hr) {
		wv.failCreation(webView2Error("ICoreWebView2Environment.CreateCoreWebView2Controller", hr))
	}
}

func (wv *WebView2) onControllerCompleted(hr win.HRESULT, controller *win.ICoreWebView2Controller) {
	if wv.creationErr != nil {
		return
	}
	if wv.disposed {
		if controller != nil && win.SUCCEEDED(hr) {
			controller.Close()
		}
		return
	}
	if win.FAILED(hr) || controller == nil {
		wv.failCreation(webView2CreationError("CreateCoreWebView2Controller", hr))
		return
	}

	controller.AddRef()
	wv.controller = controller

	var webView *win.ICoreWebView2
	if hr := controller.Get_CoreWebView2(&webView); win.FAILED(hr) || webView == nil {
		wv.failCreation(webView2Error("ICoreWebView2Controller.Get_CoreWebView2", hr))
		return
	}

	// From here on releaseBrowser releases webView.
	wv.webView = webView
	wv.events = newWebView2EventHandlers(wv)
	ev := func(i int) unsafe.Pointer { return wv.events.Interface(i) }
	for _, h := range []struct {
		name   string
		add    func(token *win.EventRegistrationToken) win.HRESULT
		remove func(token win.EventRegistrationToken) win.HRESULT
	}{
		{"NavigationStarting", func(t *win.EventRegistrationToken) win.HRESULT {
			return webView.Add_NavigationStarting((*win.ICoreWebView2NavigationStartingEventHandler)(ev(webView2EventsNavigationStarting)), t)
		}, webView.Remove_NavigationStarting},
		{"NavigationCompleted", func(t *win.EventRegistrationToken) win.HRESULT {
			return webView.Add_NavigationCompleted((*win.ICoreWebView2NavigationCompletedEventHandler)(ev(webView2EventsNavigationCompleted)), t)
		}, webView.Remove_NavigationCompleted},
		{"SourceChanged", func(t *win.EventRegistrationToken) win.HRESULT {
			return webView.Add_SourceChanged((*win.ICoreWebView2SourceChangedEventHandler)(ev(webView2EventsSourceChanged)), t)
		}, webView.Remove_SourceChanged},
		{"HistoryChanged", func(t *win.EventRegistrationToken) win.HRESULT {
			return webView.Add_HistoryChanged((*win.ICoreWebView2HistoryChangedEventHandler)(ev(webView2EventsHistoryChanged)), t)
		}, webView.Remove_HistoryChanged},
		{"DocumentTitleChanged", func(t *win.EventRegistrationToken) win.HRESULT {
			return webView.Add_DocumentTitleChanged((*win.ICoreWebView2DocumentTitleChangedEventHandler)(ev(webView2EventsDocumentTitleChanged)), t)
		}, webView.Remove_DocumentTitleChanged},
		{"WebMessageReceived", func(t *win.EventRegistrationToken) win.HRESULT {
			return webView.Add_WebMessageReceived((*win.ICoreWebView2WebMessageReceivedEventHandler)(ev(webView2EventsWebMessageReceived)), t)
		}, webView.Remove_WebMessageReceived},
		{"MoveFocusRequested", func(t *win.EventRegistrationToken) win.HRESULT {
			return controller.Add_MoveFocusRequested((*win.ICoreWebView2MoveFocusRequestedEventHandler)(ev(webView2EventsMoveFocusRequested)), t)
		}, controller.Remove_MoveFocusRequested},
	} {
		var token win.EventRegistrationToken
		if hr := h.add(&token); win.FAILED(hr) {
			wv.failCreation(webView2Error("Add_"+h.name, hr))
			return
		}
		remove := h.remove
		wv.removeHandlers = append(wv.removeHandlers, func() { remove(token) })
	}

	// Popups of the page, such as the list of a select element, are
	// positioned from the parent window, so the browser must know when
	// the form moves too.
	if form := wv.Form(); form != nil {
		wv.boundsForm = form
		wv.boundsHandle = form.BoundsChanged().Attach(func() {
			if wv.controller != nil {
				wv.controller.NotifyParentWindowPositionChanged()
			}
		})
	}

	wv.updateBounds()
	wv.controller.Put_IsVisible(win.BoolToBOOL(wv.visible()))
	if win.GetFocus() == wv.hWnd {
		wv.controller.MoveFocus(win.COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
	}

	if nav := wv.pendingNavigation; nav != nil {
		wv.pendingNavigation = nil
		if err := nav(); err != nil {
			// There is no caller to return the error to.
			wv.navigationCompletedPublisher.Publish(false, 0)
		}
	}

	wv.readyPublisher.Publish()
}

// visible reports whether the widget window itself has the WS_VISIBLE
// style. The ancestors do not matter: the browser windows are children of
// the widget and are hidden with it, and the widget gets no WM_SHOWWINDOW
// when an ancestor is shown, for example a TabPage that becomes current.
func (wv *WebView2) visible() bool {
	return wv.hWnd != 0 && win.GetWindowLong(wv.hWnd, win.GWL_STYLE)&win.WS_VISIBLE != 0
}

func (wv *WebView2) updateBounds() {
	if wv.controller == nil {
		return
	}
	var rc win.RECT
	win.GetClientRect(wv.hWnd, &rc)
	wv.controller.Put_Bounds(rc)
}

// moveFocusOut moves the focus from the web content to the next or
// previous control of the dialog and reports whether it did.
func (wv *WebView2) moveFocusOut(reason int32) bool {
	if reason != win.COREWEBVIEW2_MOVE_FOCUS_REASON_NEXT && reason != win.COREWEBVIEW2_MOVE_FOCUS_REASON_PREVIOUS {
		return false
	}
	root := win.GetAncestor(wv.hWnd, win.GA_ROOT)
	if root == 0 {
		return false
	}
	next := win.GetNextDlgTabItem(root, wv.hWnd, reason == win.COREWEBVIEW2_MOVE_FOCUS_REASON_PREVIOUS)
	if next == 0 || next == wv.hWnd {
		// The widget is the only tab stop: keep the focus in the page
		// and let it wrap around.
		return false
	}
	win.SetFocus(next)
	return true
}

// webView2FocusOwner returns the WebView2 widget window if hwnd is one of
// the browser windows inside it, otherwise hwnd. A form remembers the
// focused window when it is deactivated; for focus in the web content it
// must remember the widget, whose WM_SETFOCUS moves the focus back into
// the content when the form is activated again.
func webView2FocusOwner(hwnd win.HWND) win.HWND {
	for h := hwnd; h != 0; h = win.GetAncestor(h, win.GA_PARENT) {
		if w := windowFromHandle(h); w != nil {
			if wv, ok := w.(*WebView2); ok && h != hwnd {
				return wv.hWnd
			}
			return hwnd
		}
		if win.GetWindowLong(h, win.GWL_STYLE)&win.WS_CHILD == 0 {
			break
		}
	}
	return hwnd
}

// URL returns the URL of the current page, or the URL a navigation
// requested before the browser was ready will go to.
func (wv *WebView2) URL() string {
	return wv.url
}

// SetURL navigates to url. Before the browser is ready, the navigation is
// remembered and carried out when it is.
func (wv *WebView2) SetURL(url string) error {
	return wv.Navigate(url)
}

// Navigate navigates to url, which must be a complete URL such as
// https://example.com/ or file:///C:/page.html; an empty url is
// about:blank. Before the browser is
// ready, the navigation is remembered and carried out when it is.
func (wv *WebView2) Navigate(url string) error {
	if url == "" {
		url = "about:blank"
	}
	url16, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return wrapError(err)
	}
	nav := func() error {
		if hr := wv.webView.Navigate(url16); win.FAILED(hr) {
			return webView2Error("ICoreWebView2.Navigate", hr)
		}
		return nil
	}
	return wv.navigateOrQueue(nav, url)
}

// NavigateToString shows the HTML document html. Its URL is about:blank.
// Before the browser is ready, the navigation is remembered and carried
// out when it is.
func (wv *WebView2) NavigateToString(html string) error {
	html16, err := syscall.UTF16PtrFromString(html)
	if err != nil {
		return wrapError(err)
	}
	nav := func() error {
		if hr := wv.webView.NavigateToString(html16); win.FAILED(hr) {
			return webView2Error("ICoreWebView2.NavigateToString", hr)
		}
		return nil
	}
	return wv.navigateOrQueue(nav, "")
}

func (wv *WebView2) navigateOrQueue(nav func() error, url string) error {
	if wv.creationErr != nil {
		return ErrWebView2NotReady
	}
	if wv.webView == nil {
		wv.pendingNavigation = nav
		if url != "" && url != wv.url {
			wv.url = url
			wv.urlChangedPublisher.Publish()
		}
		return nil
	}
	return nav()
}

// withWebView calls f with the browser, or returns ErrWebView2NotReady.
func (wv *WebView2) withWebView(name string, f func(webView *win.ICoreWebView2) win.HRESULT) error {
	if wv.webView == nil {
		return ErrWebView2NotReady
	}
	if hr := f(wv.webView); win.FAILED(hr) {
		return webView2Error("ICoreWebView2."+name, hr)
	}
	return nil
}

// GoBack goes to the previous page of the history.
func (wv *WebView2) GoBack() error {
	return wv.withWebView("GoBack", (*win.ICoreWebView2).GoBack)
}

// GoForward goes to the next page of the history.
func (wv *WebView2) GoForward() error {
	return wv.withWebView("GoForward", (*win.ICoreWebView2).GoForward)
}

// Reload reloads the current page.
func (wv *WebView2) Reload() error {
	return wv.withWebView("Reload", (*win.ICoreWebView2).Reload)
}

// Stop stops all navigations and pending resource fetches.
func (wv *WebView2) Stop() error {
	return wv.withWebView("Stop", (*win.ICoreWebView2).Stop)
}

// CanGoBack reports whether there is a previous page in the history.
// [WebView2.HistoryChanged] is published when this may change.
func (wv *WebView2) CanGoBack() bool {
	var b win.BOOL
	if wv.webView != nil {
		wv.webView.Get_CanGoBack(&b)
	}
	return b != 0
}

// CanGoForward reports whether there is a next page in the history.
// [WebView2.HistoryChanged] is published when this may change.
func (wv *WebView2) CanGoForward() bool {
	var b win.BOOL
	if wv.webView != nil {
		wv.webView.Get_CanGoForward(&b)
	}
	return b != 0
}

// Title returns the title of the current document.
func (wv *WebView2) Title() string {
	return wv.title
}

// ExecuteScript runs the JavaScript code script in the top-level document
// and calls callback (if not nil) on the UI thread with the result of the
// last expression as JSON, or with an error. A result that cannot be
// converted to JSON, such as undefined or a function, is "null".
//
// It returns ErrWebView2NotReady before the browser is ready; callback is
// not called then.
func (wv *WebView2) ExecuteScript(script string, callback func(resultJSON string, err error)) error {
	script16, err := syscall.UTF16PtrFromString(script)
	if err != nil {
		return wrapError(err)
	}
	if wv.webView == nil {
		return ErrWebView2NotReady
	}
	handler := com.NewObject(&webView2ScriptCallback{wv, callback}, webView2ExecuteScriptCompletedVtbl)
	defer handler.Release()
	if hr := wv.webView.ExecuteScript(script16,
		(*win.ICoreWebView2ExecuteScriptCompletedHandler)(handler.Interface(0))); win.FAILED(hr) {
		return webView2Error("ICoreWebView2.ExecuteScript", hr)
	}
	return nil
}

// PostWebMessage posts message to the top-level document as a string.
// The page receives it with
//
//	window.chrome.webview.addEventListener("message", e => { /* e.data */ })
//
// where e.data is the string.
func (wv *WebView2) PostWebMessage(message string) error {
	message16, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return wrapError(err)
	}
	return wv.withWebView("PostWebMessageAsString", func(webView *win.ICoreWebView2) win.HRESULT {
		return webView.PostWebMessageAsString(message16)
	})
}

// PostWebMessageAsJSON posts the JSON value json to the top-level
// document; the page gets the parsed value as e.data.
func (wv *WebView2) PostWebMessageAsJSON(json string) error {
	json16, err := syscall.UTF16PtrFromString(json)
	if err != nil {
		return wrapError(err)
	}
	return wv.withWebView("PostWebMessageAsJson", func(webView *win.ICoreWebView2) win.HRESULT {
		return webView.PostWebMessageAsJson(json16)
	})
}

// NavigationStarting is published with the URL when a navigation of the
// top-level document starts.
func (wv *WebView2) NavigationStarting() *StringEvent {
	return wv.navigationStartingPublisher.Event()
}

// NavigationCompleted is published when a navigation of the top-level
// document has finished, successfully or not.
func (wv *WebView2) NavigationCompleted() *WebView2NavigationCompletedEvent {
	return wv.navigationCompletedPublisher.Event()
}

// URLChanged is published when [WebView2.URL] changes.
func (wv *WebView2) URLChanged() *Event {
	return wv.urlChangedPublisher.Event()
}

// TitleChanged is published when [WebView2.Title] changes.
func (wv *WebView2) TitleChanged() *Event {
	return wv.titleChangedPublisher.Event()
}

// HistoryChanged is published when the history changes, and with it
// possibly [WebView2.CanGoBack] and [WebView2.CanGoForward].
func (wv *WebView2) HistoryChanged() *Event {
	return wv.historyChangedPublisher.Event()
}

// WebMessageReceived is published with the message as JSON when the
// top-level document calls window.chrome.webview.postMessage(message).
// A string message arrives as a JSON string, for example "\"hello\"".
func (wv *WebView2) WebMessageReceived() *StringEvent {
	return wv.webMessageReceivedPublisher.Event()
}

func (wv *WebView2) onSourceChanged() {
	var uri *uint16
	if wv.webView == nil || win.FAILED(wv.webView.Get_Source(&uri)) {
		return
	}
	if url := win.CoTaskMemString(uri); url != wv.url {
		wv.url = url
		wv.urlChangedPublisher.Publish()
	}
}

func (wv *WebView2) onDocumentTitleChanged() {
	var title *uint16
	if wv.webView == nil || win.FAILED(wv.webView.Get_DocumentTitle(&title)) {
		return
	}
	if t := win.CoTaskMemString(title); t != wv.title {
		wv.title = t
		wv.titleChangedPublisher.Publish()
	}
}

func (wv *WebView2) WndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_WINDOWPOSCHANGED:
		// WindowBase.WndProc does not pass this message to DefWindowProc, so
		// the widget gets no WM_SIZE or WM_MOVE.
		wp := ptrFromUintptr[win.WINDOWPOS](lParam)
		if wp.Flags&win.SWP_NOSIZE == 0 {
			wv.updateBounds()
		}
		// Popups of the page, such as the list of a select element, are
		// positioned from the parent window.
		if wp.Flags&win.SWP_NOMOVE == 0 && wv.controller != nil {
			wv.controller.NotifyParentWindowPositionChanged()
		}

	case win.WM_SHOWWINDOW:
		if wv.controller != nil {
			wv.controller.Put_IsVisible(win.BoolToBOOL(wParam != 0))
		}

	case win.WM_SETFOCUS:
		if wv.controller != nil {
			reason := int32(win.COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC)
			if win.GetKeyState(win.VK_TAB) < 0 {
				if win.GetKeyState(win.VK_SHIFT) < 0 {
					reason = win.COREWEBVIEW2_MOVE_FOCUS_REASON_PREVIOUS
				} else {
					reason = win.COREWEBVIEW2_MOVE_FOCUS_REASON_NEXT
				}
			}
			wv.controller.MoveFocus(reason)
		}
	}

	return wv.WidgetBase.WndProc(hwnd, msg, wParam, lParam)
}

func (*WebView2) CreateLayoutItem(ctx *LayoutContext) LayoutItem {
	return NewGreedyLayoutItem()
}

// WebView2NavigationCompletedEventHandler is called when a navigation has
// finished. webErrorStatus tells why a navigation failed.
type WebView2NavigationCompletedEventHandler func(succeeded bool, webErrorStatus WebView2WebErrorStatus)

type webView2NavigationCompletedEventHandlerInfo struct {
	handler WebView2NavigationCompletedEventHandler
	once    bool
}

type WebView2NavigationCompletedEvent struct {
	handlers []webView2NavigationCompletedEventHandlerInfo
}

func (e *WebView2NavigationCompletedEvent) Attach(handler WebView2NavigationCompletedEventHandler) int {
	handlerInfo := webView2NavigationCompletedEventHandlerInfo{handler, false}

	for i, h := range e.handlers {
		if h.handler == nil {
			e.handlers[i] = handlerInfo
			return i
		}
	}

	e.handlers = append(e.handlers, handlerInfo)

	return len(e.handlers) - 1
}

func (e *WebView2NavigationCompletedEvent) Detach(handle int) {
	e.handlers[handle].handler = nil
}

func (e *WebView2NavigationCompletedEvent) Once(handler WebView2NavigationCompletedEventHandler) {
	i := e.Attach(handler)
	e.handlers[i].once = true
}

type WebView2NavigationCompletedEventPublisher struct {
	event WebView2NavigationCompletedEvent
}

func (p *WebView2NavigationCompletedEventPublisher) Event() *WebView2NavigationCompletedEvent {
	return &p.event
}

func (p *WebView2NavigationCompletedEventPublisher) Publish(succeeded bool, webErrorStatus WebView2WebErrorStatus) {
	for i, h := range p.event.handlers {
		if h.handler != nil {
			h.handler(succeeded, webErrorStatus)

			if h.once {
				p.event.Detach(i)
			}
		}
	}
}
