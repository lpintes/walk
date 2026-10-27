// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"bytes"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lpintes/walk/internal/win"
)

// RichEdit is a multiline text editor with character and paragraph
// formatting, based on the RichEdit control of msftedit.dll
// (RICHEDIT50W).
//
// Character positions, as used by TextSelection, SetTextSelection,
// TextRange and Find, count a paragraph break as one character, while
// Text returns paragraph breaks as "\r\n".
//
// The control exposes its own UI Automation provider with the Text
// pattern, so screen readers can read the text and its formatting.
type RichEdit struct {
	WidgetBase
	readOnlyChangedPublisher  EventPublisher
	textChangedPublisher      EventPublisher
	selectionChangedPublisher EventPublisher
	linkClickedPublisher      StringEventPublisher
	settingText               int
	linkKeyDown               bool
	linkButtonDown            bool
	linkDownRange             win.CHARRANGE
	linkDownSelection         [2]int
}

const (
	// richEditFocusTimerID is far from the small IDs msftedit uses for
	// its own timers.
	richEditFocusTimerID = 0x57414C4B
	// richEditFocusDelay in milliseconds is after the late events of the
	// task switcher.
	richEditFocusDelay = 100
)

var (
	loadMsfteditOnce sync.Once
	loadMsfteditErr  error
)

// NewRichEdit creates a RichEdit with a vertical scroll bar.
func NewRichEdit(parent Container) (*RichEdit, error) {
	return NewRichEditWithStyle(parent, win.WS_VSCROLL)
}

// NewRichEditWithStyle creates a RichEdit with additional window styles,
// for example win.WS_HSCROLL, win.WS_VSCROLL or win.ES_NOHIDESEL.
func NewRichEditWithStyle(parent Container, style uint32) (*RichEdit, error) {
	loadMsfteditOnce.Do(func() {
		loadMsfteditErr = win.LoadMsftedit()
	})
	if loadMsfteditErr != nil {
		return nil, wrapError(loadMsfteditErr)
	}

	re := new(RichEdit)

	if err := InitWidget(
		re,
		parent,
		win.MSFTEDIT_CLASS,
		win.WS_TABSTOP|win.WS_VISIBLE|win.ES_MULTILINE|win.ES_WANTRETURN|win.ES_AUTOVSCROLL|style,
		win.WS_EX_CLIENTEDGE); err != nil {
		return nil, err
	}

	re.SendMessage(win.EM_SETEVENTMASK, 0, win.ENM_CHANGE|win.ENM_SELCHANGE|win.ENM_LINK)
	// Needed for justified paragraphs.
	re.SendMessage(win.EM_SETTYPOGRAPHYOPTIONS, win.TO_ADVANCEDTYPOGRAPHY, win.TO_ADVANCEDTYPOGRAPHY)
	// The default limit is 32767 characters.
	re.SendMessage(win.EM_EXLIMITTEXT, 0, 0x7FFFFFFF)

	re.GraphicsEffects().Add(InteractionEffect)
	re.GraphicsEffects().Add(FocusEffect)

	re.MustRegisterProperty("ReadOnly", NewProperty(
		func() any {
			return re.ReadOnly()
		},
		func(v any) error {
			return re.SetReadOnly(v.(bool))
		},
		re.readOnlyChangedPublisher.Event()))

	re.MustRegisterProperty("Text", NewProperty(
		func() any {
			return re.Text()
		},
		func(v any) error {
			return re.SetText(assertStringOr(v, ""))
		},
		re.textChangedPublisher.Event()))

	return re, nil
}

// Text returns the text without formatting, with "\r\n" between
// paragraphs.
func (re *RichEdit) Text() string {
	return re.text()
}

// SetText replaces the whole text and its formatting with text in the
// default format. It clears the undo buffer. Like WM_SETTEXT, it reads
// text starting with "{\rtf" as Rich Text Format.
func (re *RichEdit) SetText(text string) error {
	changed := text != re.Text()

	re.settingText++
	err := re.setText(text)
	re.settingText--

	if err == nil && changed {
		re.textChangedPublisher.Publish()
	}

	return err
}

// TextLength returns the number of characters, counting a paragraph break
// as one.
func (re *RichEdit) TextLength() int {
	gtl := win.GETTEXTLENGTHEX{Flags: win.GTL_NUMCHARS | win.GTL_PRECISE, Codepage: 1200}
	return int(re.SendMessage(win.EM_GETTEXTLENGTHEX, uintptr(unsafe.Pointer(&gtl)), 0))
}

// RTF returns the whole text as Rich Text Format.
func (re *RichEdit) RTF() (string, error) {
	data, err := re.streamOut(win.SF_RTF)
	return string(data), err
}

// SetRTF replaces the whole text with rtf, which must be Rich Text Format
// (starting with "{\rtf"). It clears the undo buffer.
func (re *RichEdit) SetRTF(rtf string) error {
	return re.streamIn(win.SF_RTF, rtf)
}

// ReplaceSelectedRTF replaces the selection with rtf, which must be Rich
// Text Format.
func (re *RichEdit) ReplaceSelectedRTF(rtf string) error {
	return re.streamIn(win.SF_RTF|win.SFF_SELECTION, rtf)
}

// SelectedRTF returns the selection as Rich Text Format.
func (re *RichEdit) SelectedRTF() (string, error) {
	data, err := re.streamOut(win.SF_RTF | win.SFF_SELECTION)
	return string(data), err
}

// richEditStream is the state of one EM_STREAMIN or EM_STREAMOUT call. The
// calls run synchronously on the thread of the window, and the callback
// finds the stream by the cookie.
type richEditStream struct {
	// es is passed to RichEdit. It is here, on the heap, because RichEdit
	// keeps its address while the callbacks run, and they may move the
	// goroutine stack.
	es  win.EDITSTREAM
	in  []byte
	out bytes.Buffer
}

var (
	richEditStreamsMu   sync.Mutex
	richEditStreams     = make(map[uintptr]*richEditStream)
	richEditStreamNext  uintptr
	richEditStreamInCb  uintptr
	richEditStreamOutCb uintptr
)

func init() {
	AppendToWalkInit(func() {
		richEditStreamInCb = syscall.NewCallback(richEditStreamInCallback)
		richEditStreamOutCb = syscall.NewCallback(richEditStreamOutCallback)
	})
}

func newRichEditStream(s *richEditStream) uintptr {
	richEditStreamsMu.Lock()
	defer richEditStreamsMu.Unlock()
	richEditStreamNext++
	richEditStreams[richEditStreamNext] = s
	return richEditStreamNext
}

func richEditStreamFor(cookie uintptr) *richEditStream {
	richEditStreamsMu.Lock()
	defer richEditStreamsMu.Unlock()
	return richEditStreams[cookie]
}

func deleteRichEditStream(cookie uintptr) {
	richEditStreamsMu.Lock()
	defer richEditStreamsMu.Unlock()
	delete(richEditStreams, cookie)
}

// richEditStreamInCallback is the EDITSTREAMCALLBACK of EM_STREAMIN: it
// copies up to cb bytes to buf and stores their number in *pcb.
func richEditStreamInCallback(cookie, buf, cb, pcb uintptr) uintptr {
	s := richEditStreamFor(cookie)
	if s == nil {
		return 1
	}
	n := copy(unsafe.Slice(ptrFromUintptr[byte](buf), int(int32(cb))), s.in)
	s.in = s.in[n:]
	*ptrFromUintptr[int32](pcb) = int32(n)
	return 0
}

// richEditStreamOutCallback is the EDITSTREAMCALLBACK of EM_STREAMOUT: it
// takes cb bytes from buf.
func richEditStreamOutCallback(cookie, buf, cb, pcb uintptr) uintptr {
	s := richEditStreamFor(cookie)
	if s == nil {
		return 1
	}
	n := int(int32(cb))
	if n > 0 {
		s.out.Write(unsafe.Slice(ptrFromUintptr[byte](buf), n))
	}
	*ptrFromUintptr[int32](pcb) = int32(n)
	return 0
}

func (re *RichEdit) streamIn(format uintptr, data string) error {
	s := &richEditStream{in: []byte(data)}
	cookie := newRichEditStream(s)
	defer deleteRichEditStream(cookie)

	es := &s.es
	es.SetCookie(cookie)
	es.SetCallback(richEditStreamInCb)

	re.settingText++
	re.SendMessage(win.EM_STREAMIN, format, uintptr(unsafe.Pointer(es)))
	re.settingText--

	if e := es.Error(); e != 0 {
		return newError("EM_STREAMIN failed")
	}

	re.textChangedPublisher.Publish()
	re.notifyLiveRegionChanged()

	return nil
}

func (re *RichEdit) streamOut(format uintptr) ([]byte, error) {
	s := new(richEditStream)
	cookie := newRichEditStream(s)
	defer deleteRichEditStream(cookie)

	es := &s.es
	es.SetCookie(cookie)
	es.SetCallback(richEditStreamOutCb)

	re.SendMessage(win.EM_STREAMOUT, format, uintptr(unsafe.Pointer(es)))

	if e := es.Error(); e != 0 {
		return nil, newError("EM_STREAMOUT failed")
	}
	// msftedit ends Rich Text Format with a NUL, which is not part of the
	// text and must not get into a Go string.
	return bytes.TrimRight(s.out.Bytes(), "\x00"), nil
}

// TextSelection returns the selection. end is the position after the
// last selected character; start equals end if nothing is selected.
func (re *RichEdit) TextSelection() (start, end int) {
	var cr win.CHARRANGE
	re.SendMessage(win.EM_EXGETSEL, 0, uintptr(unsafe.Pointer(&cr)))
	return int(cr.CpMin), int(cr.CpMax)
}

// SetTextSelection selects the characters from start to end. start 0 and
// end -1 select everything.
func (re *RichEdit) SetTextSelection(start, end int) {
	cr := win.CHARRANGE{CpMin: int32(start), CpMax: int32(end)}
	re.SendMessage(win.EM_EXSETSEL, 0, uintptr(unsafe.Pointer(&cr)))
}

// SelectedText returns the selected text, with "\r" between paragraphs.
func (re *RichEdit) SelectedText() string {
	start, end := re.TextSelection()
	if end <= start {
		return ""
	}
	buf := make([]uint16, end-start+1)
	n := re.SendMessage(win.EM_GETSELTEXT, 0, uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf[:min(int(n), len(buf))])
}

// TextRange returns the text from start to end, with "\r" between
// paragraphs. end -1 means the end of the text.
func (re *RichEdit) TextRange(start, end int) string {
	if end < 0 {
		end = re.TextLength()
	}
	if end <= start {
		return ""
	}
	buf := make([]uint16, end-start+1)
	tr := win.TEXTRANGE{
		Chrg:      win.CHARRANGE{CpMin: int32(start), CpMax: int32(end)},
		LpstrText: &buf[0],
	}
	n := re.SendMessage(win.EM_GETTEXTRANGE, 0, uintptr(unsafe.Pointer(&tr)))
	return syscall.UTF16ToString(buf[:min(int(n), len(buf))])
}

// ReplaceSelectedText replaces the selection with text, which gets the
// formatting of the selection.
func (re *RichEdit) ReplaceSelectedText(text string, canUndo bool) {
	re.SendMessage(win.EM_REPLACESEL,
		uintptr(win.BoolToBOOL(canUndo)),
		uintptr(unsafe.Pointer(win.StringToUTF16Ptr(text))))
	re.notifyLiveRegionChanged()
}

// AppendText adds text at the end and keeps the selection.
func (re *RichEdit) AppendText(text string) {
	s, e := re.TextSelection()
	l := re.TextLength()
	re.SetTextSelection(l, l)
	re.ReplaceSelectedText(text, false)
	re.SetTextSelection(s, e)
}

// FindFlags are options of Find.
type FindFlags uint32

const (
	FindMatchCase FindFlags = 1 << iota
	FindWholeWord
	FindBackward
)

// Find searches for text from position start to the end of the text, or
// to the beginning with FindBackward. It returns the range of the first
// match, or ok false.
func (re *RichEdit) Find(text string, start int, flags FindFlags) (matchStart, matchEnd int, ok bool) {
	s, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return 0, 0, false
	}
	var opts uintptr
	if flags&FindBackward == 0 {
		opts |= win.FR_DOWN
	}
	if flags&FindMatchCase != 0 {
		opts |= win.FR_MATCHCASE
	}
	if flags&FindWholeWord != 0 {
		opts |= win.FR_WHOLEWORD
	}
	end := -1
	if flags&FindBackward != 0 {
		end = 0
	}
	ft := win.FINDTEXTEX{
		Chrg:      win.CHARRANGE{CpMin: int32(start), CpMax: int32(end)},
		LpstrText: s,
	}
	if int32(re.SendMessage(win.EM_FINDTEXTEXW, opts, uintptr(unsafe.Pointer(&ft)))) < 0 {
		return 0, 0, false
	}
	return int(ft.ChrgText.CpMin), int(ft.ChrgText.CpMax), true
}

// ScrollToCaret scrolls the caret into view.
func (re *RichEdit) ScrollToCaret() {
	re.SendMessage(win.EM_SCROLLCARET, 0, 0)
}

func (re *RichEdit) ReadOnly() bool {
	// RichEdit keeps the read-only state in its options, not necessarily
	// in the window style.
	return re.SendMessage(win.EM_GETOPTIONS, 0, 0)&win.ECO_READONLY != 0
}

func (re *RichEdit) SetReadOnly(readOnly bool) error {
	if re.SendMessage(win.EM_SETREADONLY, uintptr(win.BoolToBOOL(readOnly)), 0) == 0 {
		return newError("SendMessage(EM_SETREADONLY)")
	}

	re.readOnlyChangedPublisher.Publish()

	return nil
}

// MaxLength returns the maximum number of characters the user can enter.
func (re *RichEdit) MaxLength() int {
	return int(re.SendMessage(win.EM_GETLIMITTEXT, 0, 0))
}

// SetMaxLength sets the maximum number of characters the user can enter.
func (re *RichEdit) SetMaxLength(value int) {
	re.SendMessage(win.EM_EXLIMITTEXT, 0, uintptr(value))
}

// Modified reports whether the text was changed since the control was
// created or since SetModified(false).
func (re *RichEdit) Modified() bool {
	return re.SendMessage(win.EM_GETMODIFY, 0, 0) != 0
}

func (re *RichEdit) SetModified(modified bool) {
	re.SendMessage(win.EM_SETMODIFY, uintptr(win.BoolToBOOL(modified)), 0)
}

func (re *RichEdit) CanUndo() bool {
	return re.SendMessage(win.EM_CANUNDO, 0, 0) != 0
}

func (re *RichEdit) Undo() {
	re.SendMessage(win.EM_UNDO, 0, 0)
}

func (re *RichEdit) CanRedo() bool {
	return re.SendMessage(win.EM_CANREDO, 0, 0) != 0
}

func (re *RichEdit) Redo() {
	re.SendMessage(win.EM_REDO, 0, 0)
}

// Cut moves the selection to the clipboard.
func (re *RichEdit) Cut() {
	re.SendMessage(win.WM_CUT, 0, 0)
}

// Copy copies the selection to the clipboard.
func (re *RichEdit) Copy() {
	re.SendMessage(win.WM_COPY, 0, 0)
}

// Paste replaces the selection with the clipboard contents.
func (re *RichEdit) Paste() {
	re.SendMessage(win.WM_PASTE, 0, 0)
}

// AutoURLDetect reports whether URLs in the text are detected and shown as
// links.
func (re *RichEdit) AutoURLDetect() bool {
	return re.SendMessage(win.EM_GETAUTOURLDETECT, 0, 0) != 0
}

// SetAutoURLDetect turns the detection of URLs on or off. Detected URLs are
// shown as links, and clicking one or pressing Enter on it publishes
// LinkClicked.
func (re *RichEdit) SetAutoURLDetect(enabled bool) error {
	var v uintptr
	if enabled {
		v = win.AURL_ENABLEURL
	}
	if re.SendMessage(win.EM_AUTOURLDETECT, v, 0) != 0 {
		return newError("SendMessage(EM_AUTOURLDETECT)")
	}
	return nil
}

// CharFormatMask says which fields of a CharFormat are used.
type CharFormatMask uint32

const (
	CharFormatBold            CharFormatMask = win.CFM_BOLD
	CharFormatItalic          CharFormatMask = win.CFM_ITALIC
	CharFormatUnderline       CharFormatMask = win.CFM_UNDERLINE
	CharFormatStrikeout       CharFormatMask = win.CFM_STRIKEOUT
	CharFormatLink            CharFormatMask = win.CFM_LINK
	CharFormatFontFamily      CharFormatMask = win.CFM_FACE
	CharFormatPointSize       CharFormatMask = win.CFM_SIZE
	CharFormatTextColor       CharFormatMask = win.CFM_COLOR
	CharFormatBackgroundColor CharFormatMask = win.CFM_BACKCOLOR
)

// CharFormat is the formatting of characters.
type CharFormat struct {
	Bold      bool
	Italic    bool
	Underline bool
	Strikeout bool
	// Link marks the text as a link. URLs detected by AutoURLDetect are
	// links too.
	Link       bool
	FontFamily string
	PointSize  float64
	// TextColor is ignored if DefaultTextColor is true, which means the
	// system text color.
	TextColor        Color
	DefaultTextColor bool
	// BackgroundColor is ignored if DefaultBackgroundColor is true, which
	// means the background of the control.
	BackgroundColor        Color
	DefaultBackgroundColor bool
}

func (f *CharFormat) toCHARFORMAT2(mask CharFormatMask) (*win.CHARFORMAT2, error) {
	cf := &win.CHARFORMAT2{
		CbSize: uint32(unsafe.Sizeof(win.CHARFORMAT2{})),
		DwMask: uint32(mask),
	}
	setEffect := func(on bool, effect uint32) {
		if on {
			cf.DwEffects |= effect
		}
	}
	setEffect(f.Bold, win.CFE_BOLD)
	setEffect(f.Italic, win.CFE_ITALIC)
	setEffect(f.Underline, win.CFE_UNDERLINE)
	setEffect(f.Strikeout, win.CFE_STRIKEOUT)
	setEffect(f.Link, win.CFE_LINK)
	if mask&CharFormatFontFamily != 0 {
		name, err := syscall.UTF16FromString(f.FontFamily)
		if err != nil {
			return nil, wrapError(err)
		}
		if len(name) > len(cf.SzFaceName) {
			return nil, newError("font family name too long")
		}
		copy(cf.SzFaceName[:], name)
	}
	if mask&CharFormatPointSize != 0 {
		if f.PointSize <= 0 {
			return nil, newError("invalid point size")
		}
		cf.YHeight = int32(f.PointSize*20 + 0.5)
	}
	if mask&CharFormatTextColor != 0 {
		if f.DefaultTextColor {
			cf.DwEffects |= win.CFE_AUTOCOLOR
		} else {
			cf.CrTextColor = win.COLORREF(f.TextColor)
		}
	}
	if mask&CharFormatBackgroundColor != 0 {
		if f.DefaultBackgroundColor {
			cf.DwEffects |= win.CFE_AUTOBACKCOLOR
		} else {
			cf.CrBackColor = win.COLORREF(f.BackgroundColor)
		}
	}
	return cf, nil
}

func charFormatFromCHARFORMAT2(cf *win.CHARFORMAT2) (CharFormat, CharFormatMask) {
	return CharFormat{
		Bold:                   cf.DwEffects&win.CFE_BOLD != 0,
		Italic:                 cf.DwEffects&win.CFE_ITALIC != 0,
		Underline:              cf.DwEffects&win.CFE_UNDERLINE != 0,
		Strikeout:              cf.DwEffects&win.CFE_STRIKEOUT != 0,
		Link:                   cf.DwEffects&win.CFE_LINK != 0,
		FontFamily:             syscall.UTF16ToString(cf.SzFaceName[:]),
		PointSize:              float64(cf.YHeight) / 20,
		TextColor:              Color(cf.CrTextColor),
		DefaultTextColor:       cf.DwEffects&win.CFE_AUTOCOLOR != 0,
		BackgroundColor:        Color(cf.CrBackColor),
		DefaultBackgroundColor: cf.DwEffects&win.CFE_AUTOBACKCOLOR != 0,
	}, CharFormatMask(cf.DwMask) & charFormatMaskAll
}

const charFormatMaskAll = CharFormatBold | CharFormatItalic | CharFormatUnderline |
	CharFormatStrikeout | CharFormatLink | CharFormatFontFamily | CharFormatPointSize |
	CharFormatTextColor | CharFormatBackgroundColor

// SelectionCharFormat returns the formatting of the selection, or of the
// text that would be typed at the caret if nothing is selected. The mask
// says which fields are the same in the whole selection.
func (re *RichEdit) SelectionCharFormat() (CharFormat, CharFormatMask) {
	return re.charFormat(win.SCF_SELECTION)
}

// DefaultCharFormat returns the formatting of text without other
// formatting.
func (re *RichEdit) DefaultCharFormat() CharFormat {
	f, _ := re.charFormat(win.SCF_DEFAULT)
	return f
}

func (re *RichEdit) charFormat(scope uintptr) (CharFormat, CharFormatMask) {
	cf := win.CHARFORMAT2{CbSize: uint32(unsafe.Sizeof(win.CHARFORMAT2{}))}
	re.SendMessage(win.EM_GETCHARFORMAT, scope, uintptr(unsafe.Pointer(&cf)))
	return charFormatFromCHARFORMAT2(&cf)
}

// SetSelectionCharFormat applies the fields of f in mask to the
// selection, or to the text typed next at the caret if nothing is
// selected.
func (re *RichEdit) SetSelectionCharFormat(f CharFormat, mask CharFormatMask) error {
	return re.setCharFormat(win.SCF_SELECTION, f, mask)
}

// SetDefaultCharFormat applies the fields of f in mask to the text
// without other formatting.
func (re *RichEdit) SetDefaultCharFormat(f CharFormat, mask CharFormatMask) error {
	return re.setCharFormat(win.SCF_DEFAULT, f, mask)
}

func (re *RichEdit) setCharFormat(scope uintptr, f CharFormat, mask CharFormatMask) error {
	cf, err := f.toCHARFORMAT2(mask)
	if err != nil {
		return err
	}
	if re.SendMessage(win.EM_SETCHARFORMAT, scope, uintptr(unsafe.Pointer(cf))) == 0 {
		return newError("SendMessage(EM_SETCHARFORMAT)")
	}
	return nil
}

// ParaAlignment is the horizontal alignment of a paragraph.
type ParaAlignment uint16

const (
	ParaAlignLeft    ParaAlignment = win.PFA_LEFT
	ParaAlignRight   ParaAlignment = win.PFA_RIGHT
	ParaAlignCenter  ParaAlignment = win.PFA_CENTER
	ParaAlignJustify ParaAlignment = win.PFA_JUSTIFY
)

// ParaNumbering is the kind of bullets or numbers of a paragraph.
type ParaNumbering uint16

const (
	ParaNumberingNone        ParaNumbering = 0
	ParaNumberingBullet      ParaNumbering = win.PFN_BULLET
	ParaNumberingArabic      ParaNumbering = win.PFN_ARABIC
	ParaNumberingLowerLetter ParaNumbering = win.PFN_LCLETTER
	ParaNumberingUpperLetter ParaNumbering = win.PFN_UCLETTER
	ParaNumberingLowerRoman  ParaNumbering = win.PFN_LCROMAN
	ParaNumberingUpperRoman  ParaNumbering = win.PFN_UCROMAN
)

// ParaFormatMask says which fields of a ParaFormat are used.
type ParaFormatMask uint32

const (
	ParaFormatAlignment   ParaFormatMask = win.PFM_ALIGNMENT
	ParaFormatNumbering   ParaFormatMask = win.PFM_NUMBERING
	ParaFormatStartIndent ParaFormatMask = win.PFM_STARTINDENT
	ParaFormatRightIndent ParaFormatMask = win.PFM_RIGHTINDENT
	ParaFormatOffset      ParaFormatMask = win.PFM_OFFSET
	ParaFormatSpaceBefore ParaFormatMask = win.PFM_SPACEBEFORE
	ParaFormatSpaceAfter  ParaFormatMask = win.PFM_SPACEAFTER
)

const paraFormatMaskAll = ParaFormatAlignment | ParaFormatNumbering | ParaFormatStartIndent |
	ParaFormatRightIndent | ParaFormatOffset | ParaFormatSpaceBefore | ParaFormatSpaceAfter

// ParaFormat is the formatting of paragraphs. Distances are in points.
type ParaFormat struct {
	Alignment ParaAlignment
	// Numbering other than bullets starts at 1 and is followed by a
	// period.
	Numbering ParaNumbering
	// StartIndent is the indentation of the first line from the left
	// margin.
	StartIndent float64
	RightIndent float64
	// Offset is the indentation of the other lines relative to the first
	// line.
	Offset      float64
	SpaceBefore float64
	SpaceAfter  float64
}

func twips(points float64) int32 {
	if points < 0 {
		return int32(points*20 - 0.5)
	}
	return int32(points*20 + 0.5)
}

// SelectionParaFormat returns the formatting of the paragraphs of the
// selection. The mask says which fields are the same in all of them.
func (re *RichEdit) SelectionParaFormat() (ParaFormat, ParaFormatMask) {
	pf := win.PARAFORMAT2{CbSize: uint32(unsafe.Sizeof(win.PARAFORMAT2{}))}
	re.SendMessage(win.EM_GETPARAFORMAT, 0, uintptr(unsafe.Pointer(&pf)))
	return ParaFormat{
		Alignment:   ParaAlignment(pf.WAlignment),
		Numbering:   ParaNumbering(pf.WNumbering),
		StartIndent: float64(pf.DxStartIndent) / 20,
		RightIndent: float64(pf.DxRightIndent) / 20,
		Offset:      float64(pf.DxOffset) / 20,
		SpaceBefore: float64(pf.DySpaceBefore) / 20,
		SpaceAfter:  float64(pf.DySpaceAfter) / 20,
	}, ParaFormatMask(pf.DwMask) & paraFormatMaskAll
}

// SetSelectionParaFormat applies the fields of f in mask to the
// paragraphs of the selection.
func (re *RichEdit) SetSelectionParaFormat(f ParaFormat, mask ParaFormatMask) error {
	pf := win.PARAFORMAT2{
		CbSize:        uint32(unsafe.Sizeof(win.PARAFORMAT2{})),
		DwMask:        uint32(mask),
		WAlignment:    uint16(f.Alignment),
		WNumbering:    uint16(f.Numbering),
		DxStartIndent: twips(f.StartIndent),
		DxRightIndent: twips(f.RightIndent),
		DxOffset:      twips(f.Offset),
		DySpaceBefore: twips(f.SpaceBefore),
		DySpaceAfter:  twips(f.SpaceAfter),
	}
	if mask&ParaFormatNumbering != 0 && f.Numbering != ParaNumberingNone && f.Numbering != ParaNumberingBullet {
		pf.DwMask |= win.PFM_NUMBERINGSTART | win.PFM_NUMBERINGSTYLE
		pf.WNumberingStart = 1
		pf.WNumberingStyle = win.PFNS_PERIOD
	}
	if re.SendMessage(win.EM_SETPARAFORMAT, 0, uintptr(unsafe.Pointer(&pf))) == 0 {
		return newError("SendMessage(EM_SETPARAFORMAT)")
	}
	return nil
}

func (re *RichEdit) ReadOnlyChanged() *Event {
	return re.readOnlyChangedPublisher.Event()
}

// TextChanged is published when the text changes, by the user or by a
// method.
func (re *RichEdit) TextChanged() *Event {
	return re.textChangedPublisher.Event()
}

// SelectionChanged is published when the selection or the caret position
// changes.
func (re *RichEdit) SelectionChanged() *Event {
	return re.selectionChangedPublisher.Event()
}

// LinkClicked is published with the text of a link when the user clicks
// it or presses Enter on it.
func (re *RichEdit) LinkClicked() *StringEvent {
	return re.linkClickedPublisher.Event()
}

// ContextMenuLocation returns the caret position in screen coordinates in
// native pixels.
func (re *RichEdit) ContextMenuLocation() Point {
	_, end := re.TextSelection()
	var pt win.POINT
	re.SendMessage(win.EM_POSFROMCHAR, uintptr(unsafe.Pointer(&pt)), uintptr(end))
	windowTrimToClientBounds(re.hWnd, &pt)
	return pointPixelsFromPOINT(pt)
}

func (*RichEdit) NeedsWmSize() bool {
	return true
}

func (re *RichEdit) WndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_COMMAND:
		switch win.HIWORD(uint32(wParam)) {
		case win.EN_CHANGE:
			if re.settingText == 0 {
				re.textChangedPublisher.Publish()
			}
		}

	case win.WM_NOTIFY:
		nmh := ptrFromUintptr[win.NMHDR](lParam)
		switch nmh.Code {
		case win.EN_SELCHANGE:
			re.selectionChangedPublisher.Publish()

		case win.EN_LINK:
			enl := ptrFromUintptr[win.ENLINK](lParam)
			var activated bool
			switch enl.Msg() {
			case win.WM_LBUTTONDOWN:
				// A click on a link does not move the selection, so the
				// selection at the button up only tells a click from a
				// drag that selects text.
				start, end := re.TextSelection()
				re.linkButtonDown = true
				re.linkDownRange = enl.Chrg()
				re.linkDownSelection = [2]int{start, end}

			case win.WM_LBUTTONUP:
				// msftedit also reports Enter on a link as a
				// WM_LBUTTONDOWN and WM_LBUTTONUP pair, in read-only and
				// editable controls.
				start, end := re.TextSelection()
				activated = re.linkButtonDown &&
					re.linkDownRange == enl.Chrg() &&
					re.linkDownSelection == [2]int{start, end}
				re.linkButtonDown = false

			case win.WM_KEYDOWN:
				activated = enl.WParam() == win.VK_RETURN
				re.linkKeyDown = activated

			case win.WM_CHAR:
				// One Enter may report both WM_KEYDOWN and WM_CHAR.
				activated = enl.WParam() == '\r' && !re.linkKeyDown
				re.linkKeyDown = false
			}
			if chrg := enl.Chrg(); activated && chrg.CpMin < chrg.CpMax {
				re.linkClickedPublisher.Publish(re.TextRange(int(chrg.CpMin), int(chrg.CpMax)))
			}
		}

	case win.WM_SETFOCUS:
		// The focus comes back with the activation of the form, for
		// example after Alt+Tab. msftedit raises a second focus event of
		// its own, and with the late events of the Windows 11 task
		// switcher NVDA can lose the focus event of the control: it keeps
		// only the last four focus and foreground events while it waits
		// for the foreground to change, and then announces only the
		// title of the form. A focus event a little later repeats it;
		// NVDA drops it as a duplicate if it has the focus already.
		if windowFromHandle(win.HWND(wParam)) == nil {
			win.SetTimer(re.hWnd, richEditFocusTimerID, richEditFocusDelay, 0)
		}

	case win.WM_TIMER:
		if wParam == richEditFocusTimerID {
			win.KillTimer(re.hWnd, richEditFocusTimerID)
			if win.GetFocus() == re.hWnd {
				win.NotifyWinEvent(win.EVENT_OBJECT_FOCUS, re.hWnd, win.OBJID_CLIENT, win.CHILDID_SELF)
			}
			return 0
		}

	case win.WM_GETDLGCODE:
		if wParam == win.VK_RETURN {
			return win.DLGC_WANTALLKEYS
		}

		// Tab moves the focus, as in TextEdit. Without DLGC_HASSETSEL the
		// dialog manager does not select the whole document when the
		// focus comes with Tab; the caret stays where it was.
		return win.DLGC_WANTARROWS | win.DLGC_WANTCHARS
	}

	return re.WidgetBase.WndProc(hwnd, msg, wParam, lParam)
}

func (re *RichEdit) CreateLayoutItem(ctx *LayoutContext) LayoutItem {
	return &richEditLayoutItem{
		idealSize: SizeFrom96DPI(Size{200, 150}, ctx.dpi),
		minSize:   re.dialogBaseUnitsToPixels(Size{20, 12}),
	}
}

type richEditLayoutItem struct {
	LayoutItemBase
	idealSize Size // in native pixels
	minSize   Size // in native pixels
}

func (*richEditLayoutItem) LayoutFlags() LayoutFlags {
	return ShrinkableHorz | GrowableHorz | GreedyHorz | ShrinkableVert | GrowableVert | GreedyVert
}

func (li *richEditLayoutItem) IdealSize() Size {
	return li.idealSize
}

func (li *richEditLayoutItem) MinSize() Size {
	return li.minSize
}
