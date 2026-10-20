// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package declarative

import (
	"github.com/lpintes/walk"
)

// WebView2 creates a walk.WebView2. The options (UserDataFolder,
// BrowserExecutableFolder, AdditionalBrowserArguments and Language) are
// those of walk.WebView2Options. HTML, if not empty, is shown with
// NavigateToString when URL is nil.
type WebView2 struct {
	// Window

	Accessibility      Accessibility
	Background         Brush
	ContextMenuItems   []MenuItem
	DoubleBuffering    bool
	Enabled            Property
	Font               Font
	MaxSize            Size
	MinSize            Size
	Name               string
	OnBoundsChanged    walk.EventHandler
	OnKeyDown          walk.KeyEventHandler
	OnKeyPress         walk.KeyEventHandler
	OnKeyUp            walk.KeyEventHandler
	OnMouseDown        walk.MouseEventHandler
	OnMouseMove        walk.MouseEventHandler
	OnMouseUp          walk.MouseEventHandler
	OnSizeChanged      walk.EventHandler
	Persistent         bool
	RightToLeftReading bool
	ToolTipText        Property
	Visible            Property

	// Widget

	Alignment          Alignment2D
	AlwaysConsumeSpace bool
	Column             int
	ColumnSpan         int
	GraphicsEffects    []walk.WidgetGraphicsEffect
	Row                int
	RowSpan            int
	StretchFactor      int

	// WebView2

	AdditionalBrowserArguments string
	AssignTo                   **walk.WebView2
	BrowserExecutableFolder    string
	HTML                       string
	Language                   string
	OnCreationFailed           walk.ErrorEventHandler
	OnHistoryChanged           walk.EventHandler
	OnNavigationCompleted      walk.WebView2NavigationCompletedEventHandler
	OnNavigationStarting       walk.StringEventHandler
	OnReady                    walk.EventHandler
	OnTitleChanged             walk.EventHandler
	OnURLChanged               walk.EventHandler
	OnWebMessageReceived       walk.StringEventHandler
	URL                        Property
	UserDataFolder             string
}

func (wv WebView2) Create(builder *Builder) error {
	w, err := walk.NewWebView2WithOptions(builder.Parent(), walk.WebView2Options{
		UserDataFolder:             wv.UserDataFolder,
		BrowserExecutableFolder:    wv.BrowserExecutableFolder,
		AdditionalBrowserArguments: wv.AdditionalBrowserArguments,
		Language:                   wv.Language,
	})
	if err != nil {
		return err
	}

	if wv.AssignTo != nil {
		*wv.AssignTo = w
	}

	return builder.InitWidget(wv, w, func() error {
		if wv.OnCreationFailed != nil {
			w.CreationFailed().Attach(wv.OnCreationFailed)
		}
		if wv.OnHistoryChanged != nil {
			w.HistoryChanged().Attach(wv.OnHistoryChanged)
		}
		if wv.OnNavigationCompleted != nil {
			w.NavigationCompleted().Attach(wv.OnNavigationCompleted)
		}
		if wv.OnNavigationStarting != nil {
			w.NavigationStarting().Attach(wv.OnNavigationStarting)
		}
		if wv.OnReady != nil {
			w.Ready().Attach(wv.OnReady)
		}
		if wv.OnTitleChanged != nil {
			w.TitleChanged().Attach(wv.OnTitleChanged)
		}
		if wv.OnURLChanged != nil {
			w.URLChanged().Attach(wv.OnURLChanged)
		}
		if wv.OnWebMessageReceived != nil {
			w.WebMessageReceived().Attach(wv.OnWebMessageReceived)
		}
		if wv.HTML != "" && wv.URL == nil {
			if err := w.NavigateToString(wv.HTML); err != nil {
				return err
			}
		}

		return nil
	})
}
