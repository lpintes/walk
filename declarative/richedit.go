// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package declarative

import (
	"github.com/lpintes/walk"
	"github.com/lpintes/walk/internal/win"
)

type RichEdit struct {
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

	// RichEdit

	AssignTo      **walk.RichEdit
	AutoURLDetect bool
	HScroll       bool
	// NoVScroll removes the vertical scroll bar.
	NoVScroll bool
	// HideSelection hides the selection when the control does not have
	// the focus.
	HideSelection      bool
	MaxLength          int
	OnLinkClicked      walk.StringEventHandler
	OnSelectionChanged walk.EventHandler
	OnTextChanged      walk.EventHandler
	ReadOnly           Property
	// RTF sets the initial text as Rich Text Format; it is used only if
	// Text is not set.
	RTF  string
	Text Property
}

func (re RichEdit) Create(builder *Builder) error {
	var style uint32
	if re.HScroll {
		style |= win.WS_HSCROLL
	}
	if !re.NoVScroll {
		style |= win.WS_VSCROLL
	}
	if !re.HideSelection {
		style |= win.ES_NOHIDESEL
	}

	w, err := walk.NewRichEditWithStyle(builder.Parent(), style)
	if err != nil {
		return err
	}

	if re.AssignTo != nil {
		*re.AssignTo = w
	}

	return builder.InitWidget(re, w, func() error {
		if re.MaxLength > 0 {
			w.SetMaxLength(re.MaxLength)
		}

		if re.AutoURLDetect {
			if err := w.SetAutoURLDetect(true); err != nil {
				return err
			}
		}

		if re.RTF != "" && re.Text == nil {
			if err := w.SetRTF(re.RTF); err != nil {
				return err
			}
		}

		if re.OnTextChanged != nil {
			w.TextChanged().Attach(re.OnTextChanged)
		}

		if re.OnSelectionChanged != nil {
			w.SelectionChanged().Attach(re.OnSelectionChanged)
		}

		if re.OnLinkClicked != nil {
			w.LinkClicked().Attach(re.OnLinkClicked)
		}

		return nil
	})
}
