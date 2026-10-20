// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk/internal/win"
)

// TaskDialogIcon is the icon of a TaskDialog. Screen readers do not read the icon; say in the
// text what it means.
type TaskDialogIcon int

// Task dialog icons
const (
	TaskDialogNoIcon TaskDialogIcon = iota
	TaskDialogIconInformation
	TaskDialogIconWarning
	TaskDialogIconError
	TaskDialogIconShield
)

// TaskDialogCommonButtons is a set of the standard buttons of a TaskDialog, whose text
// Windows provides in the language of the system.
type TaskDialogCommonButtons uint32

// Standard task dialog buttons. Choosing one of them returns the DlgCmd value of the same
// name, for example DlgCmdOK.
const (
	TaskDialogOK     TaskDialogCommonButtons = win.TDCBF_OK_BUTTON
	TaskDialogYes    TaskDialogCommonButtons = win.TDCBF_YES_BUTTON
	TaskDialogNo     TaskDialogCommonButtons = win.TDCBF_NO_BUTTON
	TaskDialogCancel TaskDialogCommonButtons = win.TDCBF_CANCEL_BUTTON
	TaskDialogRetry  TaskDialogCommonButtons = win.TDCBF_RETRY_BUTTON
	TaskDialogClose  TaskDialogCommonButtons = win.TDCBF_CLOSE_BUTTON
)

// TaskDialogButton is a button or a radio button of a TaskDialog with its own text. Use IDs
// that differ from the DlgCmd values of the common buttons, for example 100 and above. An &
// in Text marks the access key.
type TaskDialogButton struct {
	ID   int
	Text string
}

// TaskDialog is a richer message box: the task dialog of the common controls (version 6),
// which screen readers know as a dialog and read when it opens. Fill in the fields and call
// Show. Only MainInstruction or Content is required; without any button the dialog has an OK
// button.
type TaskDialog struct {
	// Title is the window title; empty means the name of the executable.
	Title string

	// MainInstruction is the main message, in a larger font.
	MainInstruction string

	// Content is the text below the main instruction.
	Content string

	// Icon is the icon next to the main instruction.
	Icon TaskDialogIcon

	// CommonButtons are the standard buttons, for example TaskDialogYes | TaskDialogNo.
	CommonButtons TaskDialogCommonButtons

	// Buttons are buttons with their own text, shown before the common buttons. Text may have a
	// second line after "\n", which command links show smaller.
	Buttons []TaskDialogButton

	// CommandLinks shows Buttons as command links, large buttons on their own line, instead
	// of push buttons.
	CommandLinks bool

	// DefaultButton is the ID of the button that has the focus first; 0 means the first one.
	DefaultButton int

	// RadioButtons are choices shown below the content; Show returns the selected one.
	RadioButtons []TaskDialogButton

	// DefaultRadioButton is the ID of the radio button selected first; 0 means the first one.
	DefaultRadioButton int

	// NoDefaultRadioButton starts with no radio button selected.
	NoDefaultRadioButton bool

	// VerificationText is the text of a check box at the bottom, such as "Do not ask
	// again"; empty means no check box. Show returns its state.
	VerificationText string

	// VerificationChecked checks the check box first.
	VerificationChecked bool

	// ExpandedInformation is extra text shown when the user expands it with a button; the
	// button reads ExpandedControlText while it is shown and CollapsedControlText while it is
	// hidden (empty for the defaults of Windows).
	ExpandedInformation  string
	ExpandedControlText  string
	CollapsedControlText string

	// ExpandedByDefault shows ExpandedInformation first.
	ExpandedByDefault bool

	// Footer is a text at the bottom of the dialog.
	Footer string

	// AllowCancel lets Escape, Alt+F4 and the close button close the dialog, which returns
	// DlgCmdCancel, also without a Cancel button.
	AllowCancel bool

	// OnHyperlinkClicked, if set, makes links of the form <a href="target">text</a> in
	// Content, ExpandedInformation and Footer work: the function is called with the target
	// when the user follows a link; the dialog stays open. ShellOpen opens a web page.
	OnHyperlinkClicked func(href string)
}

// TaskDialogResult is the choice the user made in a TaskDialog.
type TaskDialogResult struct {
	// Button is the ID of the chosen button: a DlgCmd value for a common button, such as
	// DlgCmdOK, or the ID of a TaskDialogButton.
	Button int

	// RadioButton is the ID of the selected radio button, 0 if none.
	RadioButton int

	// VerificationChecked is the state of the verification check box.
	VerificationChecked bool
}

// TaskDialogAvailable reports whether task dialogs work in the program: they need version 6
// of the common controls, which the program asks for in its manifest
// (Microsoft.Windows.Common-Controls, as the manifests of the examples do).
func TaskDialogAvailable() bool {
	return win.TaskDialogIndirectAvailable()
}

var (
	taskDialogCallbackOnce sync.Once
	taskDialogCallbackPtr  uintptr
	taskDialogs            = make(map[uintptr]*TaskDialog)
	taskDialogsMu          sync.Mutex
	taskDialogNextID       uintptr
)

// taskDialogCallback is the PFTASKDIALOGCALLBACK of every task dialog; refData identifies the
// TaskDialog in taskDialogs.
func taskDialogCallback(hwnd win.HWND, msg uint32, wParam, lParam, refData uintptr) uintptr {
	if msg == win.TDN_HYPERLINK_CLICKED {
		taskDialogsMu.Lock()
		td := taskDialogs[refData]
		taskDialogsMu.Unlock()
		if td != nil && td.OnHyperlinkClicked != nil {
			td.OnHyperlinkClicked(windows.UTF16PtrToString(ptrFromUintptr[uint16](lParam)))
		}
	}
	return win.S_OK
}

// utf16Ptr converts s for a TaskDialog field, nil for an empty string. A NUL in s is shown
// as the symbol for it, as MsgBox does.
func utf16Ptr(s string) *uint16 {
	if s == "" {
		return nil
	}
	return win.StringToUTF16Ptr(strings.ReplaceAll(s, "\x00", "␀"))
}

func taskDialogButtons(buttons []TaskDialogButton) []win.TASKDIALOG_BUTTON {
	var bs []win.TASKDIALOG_BUTTON
	for _, b := range buttons {
		bs = append(bs, win.TASKDIALOG_BUTTON{NButtonID: int32(b.ID), PszButtonText: utf16Ptr(b.Text)})
	}
	return bs
}

// Show shows the task dialog, modal to owner if owner is not nil, and returns when the user
// has chosen a button. It returns an error if task dialogs are not available (see
// TaskDialogAvailable) or the dialog cannot be created, for example for a DefaultButton that
// does not exist.
func (td *TaskDialog) Show(owner Form) (TaskDialogResult, error) {
	if !win.TaskDialogIndirectAvailable() {
		return TaskDialogResult{}, newError("TaskDialogIndirect not available (common controls version 6 needed)")
	}

	config := win.TASKDIALOGCONFIG{
		PszWindowTitle:          utf16Ptr(td.Title),
		PszMainInstruction:      utf16Ptr(td.MainInstruction),
		PszContent:              utf16Ptr(td.Content),
		DwCommonButtons:         uint32(td.CommonButtons),
		Buttons:                 taskDialogButtons(td.Buttons),
		NDefaultButton:          int32(td.DefaultButton),
		RadioButtons:            taskDialogButtons(td.RadioButtons),
		NDefaultRadioButton:     int32(td.DefaultRadioButton),
		PszVerificationText:     utf16Ptr(td.VerificationText),
		PszExpandedInformation:  utf16Ptr(td.ExpandedInformation),
		PszExpandedControlText:  utf16Ptr(td.ExpandedControlText),
		PszCollapsedControlText: utf16Ptr(td.CollapsedControlText),
		PszFooter:               utf16Ptr(td.Footer),
	}
	if owner != nil {
		config.HwndParent = owner.Handle()
		config.DwFlags |= win.TDF_POSITION_RELATIVE_TO_WINDOW
	}
	switch td.Icon {
	case TaskDialogIconInformation:
		config.MainIcon = win.TD_INFORMATION_ICON
	case TaskDialogIconWarning:
		config.MainIcon = win.TD_WARNING_ICON
	case TaskDialogIconError:
		config.MainIcon = win.TD_ERROR_ICON
	case TaskDialogIconShield:
		config.MainIcon = win.TD_SHIELD_ICON
	}
	if td.CommandLinks {
		config.DwFlags |= win.TDF_USE_COMMAND_LINKS
	}
	if td.NoDefaultRadioButton {
		config.DwFlags |= win.TDF_NO_DEFAULT_RADIO_BUTTON
	}
	if td.VerificationChecked {
		config.DwFlags |= win.TDF_VERIFICATION_FLAG_CHECKED
	}
	if td.ExpandedByDefault {
		config.DwFlags |= win.TDF_EXPANDED_BY_DEFAULT
	}
	if td.AllowCancel {
		config.DwFlags |= win.TDF_ALLOW_DIALOG_CANCELLATION
	}
	if td.OnHyperlinkClicked != nil {
		config.DwFlags |= win.TDF_ENABLE_HYPERLINKS

		// syscall.NewCallback callbacks are never freed, so one is shared by all dialogs.
		taskDialogCallbackOnce.Do(func() {
			taskDialogCallbackPtr = syscall.NewCallback(taskDialogCallback)
		})
		taskDialogsMu.Lock()
		taskDialogNextID++
		id := taskDialogNextID
		taskDialogs[id] = td
		taskDialogsMu.Unlock()
		defer func() {
			taskDialogsMu.Lock()
			delete(taskDialogs, id)
			taskDialogsMu.Unlock()
		}()
		config.PfCallback = taskDialogCallbackPtr
		config.LpCallbackData = id
	}

	var button, radio int32
	var verification win.BOOL
	if hr := win.TaskDialogIndirect(&config, &button, &radio, &verification); win.FAILED(hr) {
		return TaskDialogResult{}, errorFromHRESULT("TaskDialogIndirect", hr)
	}
	return TaskDialogResult{
		Button:              int(button),
		RadioButton:         int(radio),
		VerificationChecked: verification != win.FALSE,
	}, nil
}
