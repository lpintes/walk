// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"slices"
	"strings"

	"github.com/lpintes/walk/internal/win"
)

// The system accessibility proxies name an edit field, combo box or similar
// control that has no text of its own from the window before it in z-order,
// but only if that window has the class Static. A Label is a window of its
// own class with the Static inside, so walk names the field itself: the
// Label sets the accessible name (PROPID_ACC_NAME) of its field through
// Dynamic Annotation and updates it when its text changes.

// labelTarget is implemented by the widgets that a Label right before them
// names automatically. labelHWnds returns the windows to name: the widget
// window and the windows inside it that get the focus. It returns nil when
// the widget has a name of its own (a check box with text).
type labelTarget interface {
	labelHWnds() []win.HWND
}

func (le *LineEdit) labelHWnds() []win.HWND    { return []win.HWND{le.hWnd} }
func (te *TextEdit) labelHWnds() []win.HWND    { return []win.HWND{te.hWnd} }
func (re *RichEdit) labelHWnds() []win.HWND    { return []win.HWND{re.hWnd} }
func (de *DateEdit) labelHWnds() []win.HWND    { return []win.HWND{de.hWnd} }
func (s *Slider) labelHWnds() []win.HWND       { return []win.HWND{s.hWnd} }
func (pb *ProgressBar) labelHWnds() []win.HWND { return []win.HWND{pb.hWnd} }
func (lb *ListBox) labelHWnds() []win.HWND     { return []win.HWND{lb.hWnd} }
func (tv *TreeView) labelHWnds() []win.HWND    { return []win.HWND{tv.hWnd} }

func (tv *TableView) labelHWnds() []win.HWND {
	hwnds := []win.HWND{tv.hWnd, tv.hwndNormalLV}
	if tv.hwndFrozenLV != 0 {
		hwnds = append(hwnds, tv.hwndFrozenLV)
	}
	return hwnds
}

func (cb *ComboBox) labelHWnds() []win.HWND {
	hwnds := []win.HWND{cb.hWnd}
	if cb.Editable() {
		if edit := win.GetWindow(cb.hWnd, win.GW_CHILD); edit != 0 {
			hwnds = append(hwnds, edit)
		}
	}
	return hwnds
}

func (ne *NumberEdit) labelHWnds() []win.HWND {
	if ne.edit == nil {
		return []win.HWND{ne.hWnd}
	}
	return []win.HWND{ne.hWnd, ne.edit.hWnd}
}

func (cb *CheckBox) labelHWnds() []win.HWND {
	if cb.Text() != "" {
		return nil
	}
	return []win.HWND{cb.hWnd}
}

func (rb *RadioButton) labelHWnds() []win.HWND {
	if rb.Text() != "" {
		return nil
	}
	return []win.HWND{rb.hWnd}
}

// LabelFor returns the widget this Label names for screen readers, or nil.
func (l *Label) LabelFor() Widget {
	return l.labelFor
}

// SetLabelFor makes the text of the Label the accessible name of widget, so
// that screen readers read it when widget gets the focus. The name follows
// later changes of the text. It does not replace a name set with
// Accessibility.SetName.
//
// By default a Label names the widget inserted right after it into the same
// container if that is a field without text of its own (an edit field,
// combo box, date edit, number edit, slider, progress bar, list, tree or
// table, or a check box or radio button without text). SetLabelFor replaces
// that choice; SetLabelFor(nil) turns it off.
func (l *Label) SetLabelFor(widget Widget) error {
	l.labelForExplicit = true
	return l.setLabelFor(widget)
}

// autoLabelFor makes the Label name widget if no other choice was made.
func (l *Label) autoLabelFor(widget Widget) {
	if l.labelForExplicit || l.labelFor != nil || widget.AsWidgetBase().labeledBy != nil {
		return
	}
	if _, ok := widget.(labelTarget); !ok {
		return
	}
	// The widget is inserted while it is created, before it has created
	// the windows inside it and before the program could name it; name it
	// once the message loop runs.
	l.attachLabelFor(widget)
	l.Synchronize(func() {
		if l.labelFor == widget {
			l.applyLabelFor()
		}
	})
}

func (l *Label) setLabelFor(widget Widget) error {
	if widget != l.labelFor {
		l.attachLabelFor(widget)
	}
	return l.applyLabelFor()
}

// attachLabelFor makes widget the widget of the Label, without naming it.
func (l *Label) attachLabelFor(widget Widget) {
	l.dropLabelFor()

	if widget == nil {
		return
	}

	wb := widget.AsWidgetBase()
	if other := wb.labeledBy; other != nil {
		other.dropLabelFor()
	}
	wb.labeledBy = l
	l.labelFor = widget
	l.labelForDisposingHandle = wb.Disposing().Attach(func() {
		if l.labelFor == widget {
			l.dropLabelFor()
		}
	})
}

// dropLabelFor removes the name the Label gave its widget and forgets the
// widget.
func (l *Label) dropLabelFor() {
	if l.labelFor == nil {
		return
	}
	wb := l.labelFor.AsWidgetBase()
	wb.Disposing().Detach(l.labelForDisposingHandle)
	if wb.labeledBy == l {
		wb.labeledBy = nil
	}
	l.clearLabelForHWnds(nil)
	l.labelFor = nil
}

// applyLabelFor sets the text of the Label as the name of the windows of its
// widget.
func (l *Label) applyLabelFor() error {
	if l.labelFor == nil {
		return nil
	}
	wb := l.labelFor.AsWidgetBase()

	var hwnds []win.HWND
	if t, ok := l.labelFor.(labelTarget); ok {
		hwnds = t.labelHWnds()
	} else if wb.hWnd != 0 {
		hwnds = []win.HWND{wb.hWnd}
	}
	if wb.acc != nil && wb.acc.nameSet {
		// The program named the widget: its name wins.
		l.clearLabelForHWnds(nil)
		return nil
	}

	name := l.Text()
	if win.GetWindowLong(l.hwndStatic, win.GWL_STYLE)&win.SS_NOPREFIX == 0 {
		name = stripMnemonic(name)
	}
	if name == "" {
		l.clearLabelForHWnds(nil)
		return nil
	}

	l.clearLabelForHWnds(hwnds)

	var firstErr error
	for _, h := range hwnds {
		if h == 0 {
			continue
		}
		if err := l.Accessibility().accSetPropertyStr(h, &win.PROPID_ACC_NAME, win.EVENT_OBJECT_NAMECHANGE, name); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !slices.Contains(l.labelForHWnds, h) {
			l.labelForHWnds = append(l.labelForHWnds, h)
		}
	}
	return firstErr
}

// clearLabelForHWnds removes the name from the windows the Label named,
// except for those in keep. The widget window keeps a name the program set
// with Accessibility.SetName, which replaced the Label's name there.
func (l *Label) clearLabelForHWnds(keep []win.HWND) {
	var named win.HWND
	if l.labelFor != nil {
		if wb := l.labelFor.AsWidgetBase(); wb.acc != nil && wb.acc.nameSet {
			named = wb.hWnd
		}
	}
	services := l.group.accessibilityServices()
	kept := l.labelForHWnds[:0]
	for _, h := range l.labelForHWnds {
		if slices.Contains(keep, h) {
			kept = append(kept, h)
			continue
		}
		if services != nil && h != named && win.IsWindow(h) {
			services.ClearHwndProps(h, win.OBJID_CLIENT, win.CHILDID_SELF, []win.MSAAPROPID{win.PROPID_ACC_NAME})
			win.NotifyWinEvent(win.EVENT_OBJECT_NAMECHANGE, h, win.OBJID_CLIENT, win.CHILDID_SELF)
		}
	}
	l.labelForHWnds = kept
}

// stripMnemonic removes the mnemonic prefixes from the text of a Static
// control: "&&" stands for "&", a single "&" marks the access key.
func stripMnemonic(text string) string {
	if !strings.Contains(text, "&") {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '&' {
			if i+1 < len(text) && text[i+1] == '&' {
				b.WriteByte('&')
				i++
			}
			continue
		}
		b.WriteByte(text[i])
	}
	return b.String()
}
