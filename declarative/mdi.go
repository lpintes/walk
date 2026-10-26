// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package declarative

import (
	"maps"

	"github.com/lpintes/walk"
)

// MdiMainWindow declares a walk.MdiMainWindow, the frame of a
// multiple-document interface application. It has the fields of MainWindow
// except the widgets, which belong into the MdiChildren.
type MdiMainWindow struct {
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
	RightToLeftLayout  bool
	RightToLeftReading bool
	ToolTipText        Property
	Visible            Property

	// Form

	Icon  Property
	Size  Size
	Title Property

	// MainWindow

	Bounds            Rectangle
	Expressions       func() map[string]walk.Expression
	Functions         map[string]func(args ...any) (any, error)
	MenuItems         []MenuItem
	OnDropFiles       walk.DropFilesEventHandler
	StatusBarItems    []StatusBarItem
	SuspendedUntilRun bool
	ToolBar           ToolBar

	// MdiMainWindow

	AssignTo **walk.MdiMainWindow

	// WindowMenu points to the menu that lists the children, usually
	// filled by the AssignTo of a Menu in MenuItems (see
	// walk.MdiMainWindow.SetWindowMenu).
	WindowMenu **walk.Menu

	// MdiChildren are created and shown after the frame, in this order;
	// the last one is the active child.
	MdiChildren []MdiChildWindow

	OnActiveMdiChildChanged walk.EventHandler
}

// Create creates the frame and its children.
func (mw MdiMainWindow) Create() error {
	w, err := walk.NewMdiMainWindowWithCfg(&walk.MainWindowCfg{
		Name:   mw.Name,
		Bounds: mw.Bounds.toW(),
	})
	if err != nil {
		return err
	}

	if mw.AssignTo != nil {
		*mw.AssignTo = w
	}

	return MainWindow{
		Accessibility:      mw.Accessibility,
		Background:         mw.Background,
		ContextMenuItems:   mw.ContextMenuItems,
		DoubleBuffering:    mw.DoubleBuffering,
		Enabled:            mw.Enabled,
		Font:               mw.Font,
		MaxSize:            mw.MaxSize,
		MinSize:            mw.MinSize,
		Name:               mw.Name,
		OnBoundsChanged:    mw.OnBoundsChanged,
		OnKeyDown:          mw.OnKeyDown,
		OnKeyPress:         mw.OnKeyPress,
		OnKeyUp:            mw.OnKeyUp,
		OnMouseDown:        mw.OnMouseDown,
		OnMouseMove:        mw.OnMouseMove,
		OnMouseUp:          mw.OnMouseUp,
		OnSizeChanged:      mw.OnSizeChanged,
		Persistent:         mw.Persistent,
		RightToLeftLayout:  mw.RightToLeftLayout,
		RightToLeftReading: mw.RightToLeftReading,
		ToolTipText:        mw.ToolTipText,
		Visible:            mw.Visible,
		Icon:               mw.Icon,
		Size:               mw.Size,
		Title:              mw.Title,
		Bounds:             mw.Bounds,
		Expressions:        mw.Expressions,
		Functions:          mw.Functions,
		MenuItems:          mw.MenuItems,
		OnDropFiles:        mw.OnDropFiles,
		StatusBarItems:     mw.StatusBarItems,
		SuspendedUntilRun:  mw.SuspendedUntilRun,
		ToolBar:            mw.ToolBar,
	}.init(&w.MainWindow, w, func(builder *Builder) error {
		if mw.OnActiveMdiChildChanged != nil {
			w.ActiveMdiChildChanged().Attach(mw.OnActiveMdiChildChanged)
		}

		// The menus are built by an earlier deferred function, the frame
		// is shown by another one.
		builder.Defer(func() error {
			if mw.WindowMenu != nil {
				return w.SetWindowMenu(*mw.WindowMenu)
			}
			return nil
		})

		for _, child := range mw.MdiChildren {
			builder.Defer(func() error {
				return child.Create(w)
			})
		}

		return nil
	})
}

// Run creates the frame and its children and runs the message loop until
// the frame is closed.
func (mw MdiMainWindow) Run() (int, error) {
	var w *walk.MdiMainWindow

	if mw.AssignTo == nil {
		mw.AssignTo = &w
	}

	if err := mw.Create(); err != nil {
		return 0, err
	}

	return (*mw.AssignTo).Run(), nil
}

// MdiChildWindow declares a walk.MdiChildWindow, a document window inside
// an MDI frame, with a layout and widgets like a Dialog.
type MdiChildWindow struct {
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
	RightToLeftLayout  bool
	RightToLeftReading bool

	// Visible false creates the child hidden; Show shows it.
	Visible Property

	// Container

	Children   []Widget
	DataBinder DataBinder
	Layout     Layout

	// Form

	Expressions func() map[string]walk.Expression
	Functions   map[string]func(args ...any) (any, error)
	Icon        Property
	Size        Size
	Title       Property

	// MdiChildWindow

	AssignTo **walk.MdiChildWindow

	// Bounds are relative to the client area of the MDI client. Without
	// them (and without Size) Windows places the child.
	Bounds Rectangle

	// Maximized maximizes the child when it is shown.
	Maximized bool

	OnActivating   walk.EventHandler
	OnDeactivating walk.EventHandler
	OnClosing      walk.CloseEventHandler
}

// Create creates the child in frame and shows it, unless Visible is false.
func (c MdiChildWindow) Create(frame *walk.MdiMainWindow) error {
	w, err := walk.NewMdiChildWindow(frame)
	if err != nil {
		return err
	}

	if c.AssignTo != nil {
		*c.AssignTo = w
	}

	fi := formInfo{
		// Window
		Accessibility:      c.Accessibility,
		Background:         c.Background,
		ContextMenuItems:   c.ContextMenuItems,
		DoubleBuffering:    c.DoubleBuffering,
		Enabled:            c.Enabled,
		Font:               c.Font,
		MaxSize:            c.MaxSize,
		MinSize:            c.MinSize,
		Name:               c.Name,
		OnBoundsChanged:    c.OnBoundsChanged,
		OnKeyDown:          c.OnKeyDown,
		OnKeyPress:         c.OnKeyPress,
		OnKeyUp:            c.OnKeyUp,
		OnMouseDown:        c.OnMouseDown,
		OnMouseMove:        c.OnMouseMove,
		OnMouseUp:          c.OnMouseUp,
		OnSizeChanged:      c.OnSizeChanged,
		RightToLeftReading: c.RightToLeftReading,
		Visible:            c.Visible,

		// Container
		Children:   c.Children,
		DataBinder: c.DataBinder,
		Layout:     c.Layout,

		// Form
		Icon:  c.Icon,
		Title: c.Title,
	}

	builder := NewBuilder(nil)

	w.SetSuspended(true)
	builder.Defer(func() error {
		w.SetSuspended(false)
		return nil
	})

	if err := w.SetRightToLeftLayout(c.RightToLeftLayout); err != nil {
		return err
	}

	return builder.InitWidget(fi, w, func() error {
		if !c.Bounds.toW().IsZero() {
			if err := w.SetBounds(c.Bounds.toW()); err != nil {
				return err
			}
		} else if c.Size.Width > 0 && c.Size.Height > 0 {
			if err := w.SetSize(c.Size.toW()); err != nil {
				return err
			}
		}

		if c.OnActivating != nil {
			w.Activating().Attach(c.OnActivating)
		}
		if c.OnDeactivating != nil {
			w.Deactivating().Attach(c.OnDeactivating)
		}
		if c.OnClosing != nil {
			w.Closing().Attach(c.OnClosing)
		}

		if c.Expressions != nil {
			maps.Copy(builder.expressions, c.Expressions())
		}
		if c.Functions != nil {
			for name, fn := range c.Functions {
				builder.functions[name] = fn
			}
		}

		builder.Defer(func() error {
			if c.Visible != false {
				w.Show()

				if c.Maximized {
					w.Maximize()
				}
			}

			return nil
		})

		return nil
	})
}
