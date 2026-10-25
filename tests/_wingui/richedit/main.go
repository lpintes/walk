// Command richedit checks on Windows the RichEdit widget (step 5):
//
//   - Text, SetText, TextLength, TextChanged, also with text that is not
//     ASCII and with more than the 32767 characters of the default limit.
//   - TextSelection, SetTextSelection, SelectedText, TextRange,
//     SelectionChanged, ReplaceSelectedText, Undo, Redo, Modified and Find.
//   - Character formatting (bold, italic, underline, font, size, colors)
//     of the selection, also a selection with mixed formatting, and the
//     default character formatting.
//   - Paragraph formatting (alignment, bullets, numbering, indentation).
//   - RTF, SetRTF, SelectedRTF and ReplaceSelectedRTF, with formatting and
//     text that is not ASCII surviving a round trip.
//   - AutoURLDetect: a URL in the text becomes a link, and a click and
//     Enter on it publish LinkClicked with the URL.
//   - ReadOnly and ReadOnlyChanged.
//   - UI Automation (package uia): the control type, the name set with
//     Accessibility, and the text through the Text pattern.
//
// The checks run on a goroutine while the window runs its message loop.
// It prints PASS and FAIL lines and exits with code 1 if a check failed.
//
// With the argument "manual" it skips the checks and shows a window with
// formatted text (a heading, bold, italic and underlined words, a plain
// word, two bullets and a link) for a test by hand with a screen reader:
// Ctrl+B, Ctrl+I and Ctrl+U (shortcut actions of this program) turn the
// effect of the selection on or off, the button "Read
// format" announces the formatting at the caret, and clicking a link or
// pressing Enter on it announces its URL.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
	"github.com/lpintes/walk/internal/win"
	"github.com/lpintes/walk/uia"
)

var failed bool

func check(name string, ok bool, format string, args ...any) {
	if ok {
		fmt.Println("PASS", name)
		return
	}
	failed = true
	fmt.Printf("FAIL %s: %s\n", name, fmt.Sprintf(format, args...))
}

var (
	mw *walk.MainWindow
	re *walk.RichEdit

	textChanged      int
	selectionChanged int
	readOnlyChanged  int
	linkCount        int
	links            = make(chan string, 10)
)

// onUI runs f on the goroutine of the window and waits for it.
func onUI(f func()) {
	done := make(chan struct{})
	mw.Synchronize(func() {
		f()
		close(done)
	})
	<-done
}

const sampleRTF = `{\rtf1\ansi\deff0{\fonttbl{\f0 Segoe UI;}}` +
	`{\pard\qc\b\fs32 RichEdit test\par}` +
	`{\pard This text has \b bold\b0 , \i italic\i0  and \ul underlined\ul0  words.\par}` +
	`{\pard This word is plain.\par}` +
	`{\pard{\pntext\'B7\tab}{\*\pn\pnlvlblt\pnf0\pnindent360{\pntxtb\'B7}}\fi-360\li360 First bullet\par}` +
	`{\pard{\pntext\'B7\tab}{\*\pn\pnlvlblt\pnf0\pnindent360{\pntxtb\'B7}}\fi-360\li360 Second bullet\par}` +
	`{\pard Visit https://github.com/lpintes/walk for the source.\par}}`

func main() {
	manual := len(os.Args) > 1 && os.Args[1] == "manual"

	var status *walk.Label
	children := []Widget{
		Label{Text: "Document:"},
		RichEdit{
			AssignTo:           &re,
			AutoURLDetect:      true,
			Accessibility:      Accessibility{Name: "Document"},
			OnTextChanged:      func() { textChanged++ },
			OnSelectionChanged: func() { selectionChanged++ },
			OnLinkClicked: func(url string) {
				if manual {
					// The status is a live region, so this is announced.
					// The count makes each activation change the text.
					linkCount++
					status.SetText(fmt.Sprintf("Link %d %s", linkCount, url))
				}
				select {
				case links <- url:
				default:
				}
			},
		},
		LineEdit{Accessibility: Accessibility{Name: "Other field"}},
	}
	if manual {
		children = append(children,
			PushButton{Text: "Read format", OnClicked: readFormat},
			Label{AssignTo: &status, Text: "Ready", Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite}},
		)
	}

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "walk richedit test",
		Size:     Size{Width: 500, Height: 500},
		Layout:   VBox{},
		Children: children,
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}
	re.ReadOnlyChanged().Attach(func() { readOnlyChanged++ })

	if manual {
		if err := re.SetRTF(sampleRTF); err != nil {
			fmt.Println("SetRTF:", err)
		}
		// msftedit has no shortcuts for character formatting, so the
		// test adds them as an application would. Shortcut actions run
		// before the key reaches the control, so Ctrl+I does not also
		// insert a tab character.
		for _, s := range []struct {
			key   walk.Key
			name  string
			m     walk.CharFormatMask
			field func(*walk.CharFormat) *bool
		}{
			{walk.KeyB, "bold", walk.CharFormatBold, func(cf *walk.CharFormat) *bool { return &cf.Bold }},
			{walk.KeyI, "italic", walk.CharFormatItalic, func(cf *walk.CharFormat) *bool { return &cf.Italic }},
			{walk.KeyU, "underlined", walk.CharFormatUnderline, func(cf *walk.CharFormat) *bool { return &cf.Underline }},
		} {
			a := walk.NewAction()
			a.SetShortcut(walk.Shortcut{Modifiers: walk.ModControl, Key: s.key})
			a.Triggered().Attach(func() { toggleFormat(s.name, s.m, s.field) })
			re.ShortcutActions().Add(a)
		}
		// After the window has shown and focused its first widget.
		mw.Synchronize(func() { re.SetFocus() })
		mw.Run()
		return
	}

	hwnd := uintptr(mw.Handle())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100 && !win.IsWindowVisible(win.HWND(hwnd)); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		onUI(checkText)
		onUI(checkSelection)
		onUI(checkCharFormat)
		onUI(checkParaFormat)
		onUI(checkRTF)
		checkLinks()
		onUI(checkReadOnly)
		checkUIA()
		mw.Synchronize(func() { mw.Close() })
	}()
	mw.Run()
	<-done

	if failed {
		os.Exit(1)
	}
}

// toggleFormat turns an effect of the selection on, or off if the whole
// selection has it, and announces the result in the manual test.
func toggleFormat(name string, m walk.CharFormatMask, field func(*walk.CharFormat) *bool) {
	cf, mask := re.SelectionCharFormat()
	on := !(mask&m != 0 && *field(&cf))
	var f walk.CharFormat
	*field(&f) = on
	if err := re.SetSelectionCharFormat(f, m); err != nil {
		walk.Announce(mw, err.Error(), walk.AnnounceImportantAll)
		return
	}
	state := "off"
	if on {
		state = "on"
	}
	walk.Announce(mw, name+" "+state, walk.AnnounceImportantAll)
}

// readFormat announces the formatting at the caret in the manual test.
func readFormat() {
	cf, mask := re.SelectionCharFormat()
	pf, _ := re.SelectionParaFormat()
	var parts []string
	if mask&walk.CharFormatFontFamily != 0 {
		parts = append(parts, cf.FontFamily)
	}
	if mask&walk.CharFormatPointSize != 0 {
		parts = append(parts, fmt.Sprintf("%g points", cf.PointSize))
	}
	for _, e := range []struct {
		on   bool
		m    walk.CharFormatMask
		name string
	}{
		{cf.Bold, walk.CharFormatBold, "bold"},
		{cf.Italic, walk.CharFormatItalic, "italic"},
		{cf.Underline, walk.CharFormatUnderline, "underlined"},
		{cf.Link, walk.CharFormatLink, "link"},
	} {
		if mask&e.m != 0 && e.on {
			parts = append(parts, e.name)
		}
	}
	switch pf.Alignment {
	case walk.ParaAlignCenter:
		parts = append(parts, "centered")
	case walk.ParaAlignRight:
		parts = append(parts, "right aligned")
	case walk.ParaAlignJustify:
		parts = append(parts, "justified")
	}
	if pf.Numbering == walk.ParaNumberingBullet {
		parts = append(parts, "bullet")
	}
	text := strings.Join(parts, ", ")
	walk.Announce(mw, text, walk.AnnounceImportantAll)
}

func checkText() {
	before := textChanged
	err := re.SetText("Hello world\r\nSecond line")
	check("SetText", err == nil, "%v", err)
	check("Text", re.Text() == "Hello world\r\nSecond line", "got %q", re.Text())
	check("TextLength", re.TextLength() == 23, "got %d, want 23", re.TextLength())
	check("TextChanged after SetText", textChanged == before+1, "published %d times, want 1", textChanged-before)

	before = textChanged
	re.SetText("Hello world\r\nSecond line")
	check("SetText with the same text", textChanged == before, "TextChanged published %d times, want 0", textChanged-before)

	const unicode = "Žltý kôň úpel ďábelské ódy 😀 中文"
	re.SetText(unicode)
	check("Text not ASCII", re.Text() == unicode, "got %q", re.Text())

	long := strings.Repeat("0123456789", 10000)
	re.SetText(long)
	check("text above 32767 characters", re.TextLength() == len(long), "TextLength %d, want %d", re.TextLength(), len(long))

	re.SetText("one\ntwo")
	check("SetText with LF", re.Text() == "one\r\ntwo", "got %q", re.Text())

	re.SetText("Hello world\r\nSecond line")
}

func checkSelection() {
	before := selectionChanged
	re.SetTextSelection(0, 5)
	s, e := re.TextSelection()
	check("TextSelection", s == 0 && e == 5, "got %d, %d", s, e)
	check("SelectedText", re.SelectedText() == "Hello", "got %q", re.SelectedText())
	check("SelectionChanged", selectionChanged > before, "not published")
	check("TextRange", re.TextRange(6, 11) == "world", "got %q", re.TextRange(6, 11))
	check("TextRange over a paragraph break", re.TextRange(6, 18) == "world\rSecond", "got %q", re.TextRange(6, 18))
	check("TextRange to the end", re.TextRange(19, -1) == "line", "got %q", re.TextRange(19, -1))

	ms, me, ok := re.Find("second", 0, 0)
	check("Find", ok && ms == 12 && me == 18, "got %d, %d, %v", ms, me, ok)
	_, _, ok = re.Find("second", 0, walk.FindMatchCase)
	check("Find FindMatchCase", !ok, "found")
	_, _, ok = re.Find("Hell", 0, walk.FindWholeWord)
	check("Find FindWholeWord", !ok, "found")
	ms, _, ok = re.Find("o", 23, walk.FindBackward)
	check("Find FindBackward", ok && ms == 15, "got %d, %v", ms, ok)

	re.SetModified(false)
	before = textChanged
	re.SetTextSelection(0, 5)
	re.ReplaceSelectedText("Goodbye", true)
	check("ReplaceSelectedText", strings.HasPrefix(re.Text(), "Goodbye world"), "got %q", re.Text())
	check("TextChanged after ReplaceSelectedText", textChanged > before, "not published")
	check("Modified", re.Modified(), "false after ReplaceSelectedText")
	check("CanUndo", re.CanUndo(), "false")
	re.Undo()
	check("Undo", strings.HasPrefix(re.Text(), "Hello world"), "got %q", re.Text())
	check("CanRedo", re.CanRedo(), "false")
	re.Redo()
	check("Redo", strings.HasPrefix(re.Text(), "Goodbye world"), "got %q", re.Text())

	re.AppendText("\r\nEnd")
	check("AppendText", strings.HasSuffix(re.Text(), "line\r\nEnd"), "got %q", re.Text())

	re.SetText("Hello world\r\nSecond line")
}

func checkCharFormat() {
	re.SetTextSelection(0, 5)
	err := re.SetSelectionCharFormat(walk.CharFormat{Bold: true}, walk.CharFormatBold)
	check("SetSelectionCharFormat bold", err == nil, "%v", err)
	cf, mask := re.SelectionCharFormat()
	check("SelectionCharFormat bold", cf.Bold && mask&walk.CharFormatBold != 0, "Bold %v, mask %#x", cf.Bold, mask)

	re.SetTextSelection(0, 11)
	cf, mask = re.SelectionCharFormat()
	check("SelectionCharFormat mixed", mask&walk.CharFormatBold == 0, "mask %#x has Bold for a mixed selection", mask)

	re.SetTextSelection(6, 11)
	f := walk.CharFormat{
		Italic:          true,
		Underline:       true,
		Strikeout:       true,
		FontFamily:      "Courier New",
		PointSize:       14,
		TextColor:       walk.RGB(200, 0, 0),
		BackgroundColor: walk.RGB(255, 255, 0),
	}
	m := walk.CharFormatItalic | walk.CharFormatUnderline | walk.CharFormatStrikeout |
		walk.CharFormatFontFamily | walk.CharFormatPointSize | walk.CharFormatTextColor | walk.CharFormatBackgroundColor
	err = re.SetSelectionCharFormat(f, m)
	check("SetSelectionCharFormat", err == nil, "%v", err)
	cf, mask = re.SelectionCharFormat()
	check("SelectionCharFormat mask", mask&m == m, "mask %#x, want %#x", mask, m)
	check("SelectionCharFormat italic underline strikeout", cf.Italic && cf.Underline && cf.Strikeout && !cf.Bold, "%+v", cf)
	check("SelectionCharFormat font", cf.FontFamily == "Courier New" && cf.PointSize == 14, "font %q, size %g", cf.FontFamily, cf.PointSize)
	check("SelectionCharFormat colors", cf.TextColor == f.TextColor && !cf.DefaultTextColor &&
		cf.BackgroundColor == f.BackgroundColor && !cf.DefaultBackgroundColor,
		"text %#x (default %v), background %#x (default %v)", cf.TextColor, cf.DefaultTextColor, cf.BackgroundColor, cf.DefaultBackgroundColor)

	err = re.SetSelectionCharFormat(walk.CharFormat{DefaultTextColor: true, DefaultBackgroundColor: true},
		walk.CharFormatTextColor|walk.CharFormatBackgroundColor)
	check("SetSelectionCharFormat default colors", err == nil, "%v", err)
	cf, _ = re.SelectionCharFormat()
	check("SelectionCharFormat default colors", cf.DefaultTextColor && cf.DefaultBackgroundColor, "%+v", cf)

	err = re.SetSelectionCharFormat(walk.CharFormat{FontFamily: strings.Repeat("x", 40)}, walk.CharFormatFontFamily)
	check("SetSelectionCharFormat long font name", err != nil, "no error")

	df := re.DefaultCharFormat()
	check("DefaultCharFormat", df.FontFamily != "" && df.PointSize > 0 && !df.Bold, "%+v", df)
	err = re.SetDefaultCharFormat(walk.CharFormat{PointSize: 12}, walk.CharFormatPointSize)
	check("SetDefaultCharFormat", err == nil && re.DefaultCharFormat().PointSize == 12, "%v, size %g", err, re.DefaultCharFormat().PointSize)

	re.SetText("Hello world\r\nSecond line")
}

func checkParaFormat() {
	re.SetTextSelection(0, 0)
	err := re.SetSelectionParaFormat(walk.ParaFormat{Alignment: walk.ParaAlignCenter}, walk.ParaFormatAlignment)
	check("SetSelectionParaFormat center", err == nil, "%v", err)
	pf, mask := re.SelectionParaFormat()
	check("SelectionParaFormat center", pf.Alignment == walk.ParaAlignCenter && mask&walk.ParaFormatAlignment != 0, "%+v, mask %#x", pf, mask)

	err = re.SetSelectionParaFormat(walk.ParaFormat{Alignment: walk.ParaAlignJustify}, walk.ParaFormatAlignment)
	pf, _ = re.SelectionParaFormat()
	check("justify", err == nil && pf.Alignment == walk.ParaAlignJustify, "%v, %+v", err, pf)

	re.SetTextSelection(0, -1)
	pf, mask = re.SelectionParaFormat()
	check("SelectionParaFormat mixed", mask&walk.ParaFormatAlignment == 0, "mask %#x has Alignment for paragraphs with different alignments", mask)

	re.SetTextSelection(13, 13)
	f := walk.ParaFormat{Numbering: walk.ParaNumberingBullet, StartIndent: 18, Offset: 18, SpaceBefore: 6, SpaceAfter: 3}
	m := walk.ParaFormatNumbering | walk.ParaFormatStartIndent | walk.ParaFormatOffset | walk.ParaFormatSpaceBefore | walk.ParaFormatSpaceAfter
	err = re.SetSelectionParaFormat(f, m)
	check("SetSelectionParaFormat bullet", err == nil, "%v", err)
	pf, _ = re.SelectionParaFormat()
	check("SelectionParaFormat bullet", pf.Numbering == f.Numbering && pf.StartIndent == 18 && pf.Offset == 18 &&
		pf.SpaceBefore == 6 && pf.SpaceAfter == 3, "%+v", pf)

	err = re.SetSelectionParaFormat(walk.ParaFormat{Numbering: walk.ParaNumberingArabic}, walk.ParaFormatNumbering)
	pf, _ = re.SelectionParaFormat()
	check("numbering", err == nil && pf.Numbering == walk.ParaNumberingArabic, "%v, %+v", err, pf)
	check("text unchanged by the paragraph format", re.Text() == "Hello world\r\nSecond line", "got %q", re.Text())

	re.SetText("Hello world\r\nSecond line")
}

func checkRTF() {
	re.SetText("Hello world\r\nŽltý kôň")
	re.SetTextSelection(0, 5)
	re.SetSelectionCharFormat(walk.CharFormat{Bold: true}, walk.CharFormatBold)

	rtf, err := re.RTF()
	check("RTF", err == nil && strings.HasPrefix(rtf, `{\rtf`) && strings.Contains(rtf, `\b`), "%v, %.200q", err, rtf)

	re.SetText("")
	before := textChanged
	err = re.SetRTF(rtf)
	check("SetRTF", err == nil, "%v", err)
	check("SetRTF text", re.Text() == "Hello world\r\nŽltý kôň", "got %q", re.Text())
	check("TextChanged after SetRTF", textChanged == before+1, "published %d times, want 1", textChanged-before)
	re.SetTextSelection(0, 5)
	cf, mask := re.SelectionCharFormat()
	check("SetRTF keeps bold", cf.Bold && mask&walk.CharFormatBold != 0, "%+v", cf)
	re.SetTextSelection(6, 11)
	cf, _ = re.SelectionCharFormat()
	check("SetRTF keeps plain text plain", !cf.Bold, "bold")

	err = re.SetRTF(`{\rtf1\ansi {\i italic} plain\par\qc centered}`)
	check("SetRTF italic", err == nil && re.Text() == "italic plain\r\ncentered", "%v, %q", err, re.Text())
	re.SetTextSelection(0, 6)
	cf, _ = re.SelectionCharFormat()
	check("SetRTF italic format", cf.Italic, "not italic")
	re.SetTextSelection(14, 14)
	pf, _ := re.SelectionParaFormat()
	check("SetRTF centered paragraph", pf.Alignment == walk.ParaAlignCenter, "alignment %d", pf.Alignment)

	re.SetText("Hello world")
	re.SetTextSelection(6, 11)
	err = re.ReplaceSelectedRTF(`{\rtf1\ansi {\b there}}`)
	check("ReplaceSelectedRTF", err == nil && re.Text() == "Hello there", "%v, %q", err, re.Text())
	re.SetTextSelection(6, 11)
	cf, _ = re.SelectionCharFormat()
	check("ReplaceSelectedRTF bold", cf.Bold, "not bold")
	sel, err := re.SelectedRTF()
	check("SelectedRTF", err == nil && strings.Contains(sel, "there") && !strings.Contains(sel, "Hello"), "%v, %.200q", err, sel)

	re.SetText("Hello world\r\nSecond line")
}

func checkLinks() {
	var start int
	onUI(func() {
		re.SetFocus()
		re.SetText("See https://example.com/walk now")
		start = 4
		// A range: the format of a caret is the insertion format, which
		// does not carry the link effect.
		re.SetTextSelection(start+2, start+6)
	})
	var cf walk.CharFormat
	var mask walk.CharFormatMask
	for range 20 {
		onUI(func() { cf, mask = re.SelectionCharFormat() })
		if cf.Link {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	check("AutoURLDetect link", cf.Link && mask&walk.CharFormatLink != 0, "%+v", cf)

	var auto bool
	onUI(func() { auto = re.AutoURLDetect() })
	check("AutoURLDetect", auto, "false")

	// A click on the link: the button messages at its position.
	onUI(func() {
		var pt win.POINT
		re.SendMessage(win.EM_POSFROMCHAR, uintptr(unsafe.Pointer(&pt)), uintptr(start+3))
		lp := uintptr(uint32(pt.X+2)&0xFFFF | uint32(pt.Y+4)<<16)
		re.SendMessage(win.WM_LBUTTONDOWN, win.MK_LBUTTON, lp)
		re.SendMessage(win.WM_LBUTTONUP, 0, lp)
	})
	select {
	case url := <-links:
		check("LinkClicked by click", url == "https://example.com/walk", "URL %q", url)
	case <-time.After(2 * time.Second):
		check("LinkClicked by click", false, "not published")
	}

	// Enter on the link in a read-only control.
	onUI(func() {
		re.SetReadOnly(true)
		re.SetTextSelection(start+3, start+3)
		re.SendMessage(win.WM_KEYDOWN, win.VK_RETURN, 0x001C0001)
		re.SendMessage(win.WM_CHAR, win.VK_RETURN, 0x001C0001)
		re.SendMessage(win.WM_KEYUP, win.VK_RETURN, 0xC01C0001)
	})
	select {
	case url := <-links:
		check("LinkClicked by Enter", url == "https://example.com/walk", "URL %q", url)
	case <-time.After(2 * time.Second):
		check("LinkClicked by Enter", false, "not published")
	}
	select {
	case url := <-links:
		check("LinkClicked once per Enter", false, "published again: %q", url)
	case <-time.After(500 * time.Millisecond):
		check("LinkClicked once per Enter", true, "")
	}
	onUI(func() {
		re.SetReadOnly(false)
		re.SetAutoURLDetect(false)
		re.SetText("Hello world\r\nSecond line")
	})
}

func checkReadOnly() {
	before := readOnlyChanged
	err := re.SetReadOnly(true)
	check("SetReadOnly", err == nil && re.ReadOnly(), "%v, ReadOnly %v", err, re.ReadOnly())
	check("ReadOnlyChanged", readOnlyChanged == before+1, "published %d times", readOnlyChanged-before)
	re.SetReadOnly(false)
	check("SetReadOnly false", !re.ReadOnly(), "still read-only")

	re.SetMaxLength(100)
	check("MaxLength", re.MaxLength() == 100, "got %d", re.MaxLength())
	re.SetMaxLength(0x7FFFFFFF)
}

func checkUIA() {
	a, err := uia.New()
	if err != nil {
		check("uia.New", false, "%v", err)
		return
	}
	defer a.Close()
	var hwnd uintptr
	onUI(func() { hwnd = uintptr(re.Handle()) })
	e, err := a.ElementFromHandle(hwnd)
	if err != nil {
		check("ElementFromHandle", false, "%v", err)
		return
	}
	defer e.Release()
	ct, _ := e.ControlType()
	check("UIA control type", ct == uia.ControlTypeDocument || ct == uia.ControlTypeEdit, "got %v", ct)
	name, _ := e.Name()
	check("UIA name", name == "Document", "got %q", name)
	text, err := e.Text()
	check("UIA Text pattern", err == nil && strings.Contains(text, "Hello world") && strings.Contains(text, "Second line"), "%v, %q", err, text)
}
