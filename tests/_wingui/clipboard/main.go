// Command clipboard checks on Windows the clipboard formats of
// walk.ClipboardService (step 14 part 3):
//
//   - text with the existing Text and SetText;
//   - HTML: the header offsets are checked byte by byte in the raw data,
//     HTML returns the fragment, the document and the source URL, and the
//     plain text alternative is set;
//   - Rich Text Format, also from and into a RichEdit;
//   - file lists: the paths are read back with DragQueryFileW as the shell
//     reads them, and the "Preferred DropEffect" for cut and copy;
//   - images: PNG, CF_DIBV5 and CF_DIB, sizes and every pixel, a walk Bitmap
//     in and out;
//   - several formats at once with ClipboardData, the list of formats and
//     the ContainsX methods;
//   - sensitive data (clipboard history and cloud exclusion formats);
//   - errors: a failed conversion leaves the clipboard unchanged, missing
//     formats return ErrClipboardFormatNotAvailable;
//   - the ContentsChanged event.
//
// The test raw-reads the clipboard with its own Win32 calls, so it checks
// the bytes other programs see, not only walk's own round trip. It saves the
// text on the clipboard first and puts it back at the end; other formats
// cannot be restored, which an INFO line says. It prints PASS, FAIL and INFO
// lines and exits with code 1 if a check failed.
//
// With the argument "manual" it shows a window with buttons that copy HTML,
// RTF, files, an image and a password, and a button that reads what is on
// the clipboard, for pasting into Word, WordPad or Explorer and for a test
// with a screen reader. Results go to a polite live region and standard
// output.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
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
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	shell32          = windows.NewLazySystemDLL("shell32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	globalSize       = kernel32.NewProc("GlobalSize")
	moveMemory       = kernel32.NewProc("RtlMoveMemory")
	dragQueryFileW   = shell32.NewProc("DragQueryFileW")
)

const cfHDROP = 15

// withRawClipboard opens the clipboard without a window, trying again for a
// while if a clipboard monitor has it open.
func withRawClipboard(f func() error) error {
	for i := 0; ; i++ {
		if r, _, _ := openClipboard.Call(0); r != 0 {
			break
		}
		if i == 50 {
			return errors.New("OpenClipboard failed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer closeClipboard.Call()
	return f()
}

// rawData reads a clipboard format with plain Win32 calls.
func rawData(format walk.ClipboardFormat) ([]byte, error) {
	var data []byte
	err := withRawClipboard(func() error {
		h, _, _ := getClipboardData.Call(uintptr(format))
		if h == 0 {
			return fmt.Errorf("GetClipboardData(%s) failed", format)
		}
		size, _, _ := globalSize.Call(h)
		p, _, _ := globalLock.Call(h)
		if p == 0 {
			return errors.New("GlobalLock failed")
		}
		defer globalUnlock.Call(h)
		data = make([]byte, size)
		if size > 0 {
			// The memory belongs to Windows; copy it with a Windows
			// function rather than converting the address to a Go pointer.
			moveMemory.Call(uintptr(unsafe.Pointer(&data[0])), p, size)
		}
		return nil
	})
	return data, err
}

// shellFiles reads the CF_HDROP of the clipboard with DragQueryFileW, as
// Explorer does.
func shellFiles() ([]string, error) {
	var paths []string
	err := withRawClipboard(func() error {
		h, _, _ := getClipboardData.Call(cfHDROP)
		if h == 0 {
			return errors.New("GetClipboardData(CF_HDROP) failed")
		}
		n, _, _ := dragQueryFileW.Call(h, 0xFFFFFFFF, 0, 0)
		for i := range n {
			size, _, _ := dragQueryFileW.Call(h, i, 0, 0)
			buf := make([]uint16, size+1)
			dragQueryFileW.Call(h, i, uintptr(unsafe.Pointer(&buf[0])), size+1)
			paths = append(paths, windows.UTF16ToString(buf))
		}
		return nil
	})
	return paths, err
}

var cb = walk.Clipboard()

// headerValue returns the number after "key:" in the CF_HTML header.
func headerValue(data []byte, key string) int {
	i := bytes.Index(data, []byte("\r\n"+key+":"))
	if i < 0 {
		return -2
	}
	rest := data[i+len(key)+3:]
	end := bytes.Index(rest, []byte("\r\n"))
	if end < 0 {
		return -2
	}
	n, err := strconv.Atoi(string(rest[:end]))
	if err != nil {
		return -2
	}
	return n
}

func testText() {
	const s = "walk clipboard test: ľščťžýáíé 日本 😀"
	err := cb.SetText(s)
	got, err2 := cb.Text()
	check("SetText and Text", err == nil && err2 == nil && got == s, "%q, %v, %v", got, err, err2)
	ok, err := cb.ContainsText()
	check("ContainsText", ok && err == nil, "%v, %v", ok, err)
}

func testHTML() {
	const fragment = `<h1>Title</h1><p>ľščť <b>bold</b> 日本 😀 <a href="https://example.com/">link</a></p>`
	const plain = "Title\r\nľščť bold 日本 😀 link"
	err := cb.SetHTML(fragment, plain)
	check("SetHTML", err == nil, "%v", err)

	raw, err := rawData(walk.ClipboardFormatHTML)
	if err != nil {
		check("HTML raw data", false, "%v", err)
		return
	}
	nul := bytes.IndexByte(raw, 0)
	check("HTML raw data terminated with NUL", nul > 0, "no NUL in %d bytes", len(raw))
	if nul < 0 {
		nul = len(raw)
	}
	check("HTML header starts with Version", bytes.HasPrefix(raw, []byte("Version:")), "%q", raw[:min(len(raw), 20)])
	sh := headerValue(raw, "StartHTML")
	eh := headerValue(raw, "EndHTML")
	sf := headerValue(raw, "StartFragment")
	ef := headerValue(raw, "EndFragment")
	fmt.Printf("INFO HTML offsets StartHTML %d EndHTML %d StartFragment %d EndFragment %d, NUL at %d\n", sh, eh, sf, ef, nul)
	if sh < 0 || eh < sh || sf < sh || ef < sf || eh > nul {
		check("HTML offsets", false, "invalid offsets in %q", raw[:nul])
		return
	}
	check("HTML EndHTML is the end of the data", eh == nul, "EndHTML %d, NUL at %d", eh, nul)
	check("HTML StartHTML points to <html>", bytes.HasPrefix(raw[sh:], []byte("<html")), "%q", raw[sh:min(sh+10, nul)])
	check("HTML fragment bytes", bytes.Equal(raw[sf:ef], []byte(fragment)), "%q", raw[sf:ef])
	check("HTML StartFragment after the comment", bytes.HasSuffix(raw[:sf], []byte("<!--StartFragment-->")), "%q", raw[max(0, sf-20):sf])
	check("HTML EndFragment at the comment", bytes.HasPrefix(raw[ef:], []byte("<!--EndFragment-->")), "%q", raw[ef:min(ef+18, nul)])
	check("HTML header is ASCII", !slices.ContainsFunc(raw[:sh], func(b byte) bool { return b >= 0x80 }), "%q", raw[:sh])

	h, err := cb.HTML()
	if err != nil {
		check("HTML", false, "%v", err)
		return
	}
	check("HTML Fragment", h.Fragment == fragment, "%q", h.Fragment)
	check("HTML Document", h.Document == string(raw[sh:eh]), "%q", h.Document)
	check("HTML SourceURL empty", h.SourceURL == "", "%q", h.SourceURL)
	text, err := cb.Text()
	check("HTML plain text alternative", err == nil && text == plain, "%q, %v", text, err)
	ok, err := cb.ContainsHTML()
	check("ContainsHTML", ok && err == nil, "%v, %v", ok, err)

	var d walk.ClipboardData
	d.SetHTML("<i>x</i>", "https://example.com/page?a=1")
	err = cb.SetData(&d)
	h, err2 := cb.HTML()
	check("HTML with SourceURL", err == nil && err2 == nil && h.Fragment == "<i>x</i>" && h.SourceURL == "https://example.com/page?a=1",
		"%+v, %v, %v", h, err, err2)
	ok, _ = cb.ContainsText()
	check("SetData with HTML only has no text", !ok, "text available")
}

const sampleRTF = `{\rtf1\ansi\deff0{\fonttbl{\f0 Arial;}}\f0\fs24 Plain \b bold\b0  and \i italic\i0 .\par}`

func testRTF(re1, re2 *walk.RichEdit) {
	err := cb.SetRTF(sampleRTF, "Plain bold and italic.")
	check("SetRTF", err == nil, "%v", err)
	got, err := cb.RTF()
	check("RTF", err == nil && got == sampleRTF, "%q, %v", got, err)
	raw, err := rawData(walk.ClipboardFormatRTF)
	check("RTF raw data", err == nil && bytes.HasPrefix(raw, append([]byte(sampleRTF), 0)), "%q, %v", raw, err)
	text, _ := cb.Text()
	check("RTF plain text alternative", text == "Plain bold and italic.", "%q", text)
	ok, err := cb.ContainsRTF()
	check("ContainsRTF", ok && err == nil, "%v, %v", ok, err)

	// From one RichEdit through the clipboard into another.
	if err := re1.SetRTF(sampleRTF); err != nil {
		check("RichEdit.SetRTF", false, "%v", err)
		return
	}
	rtf, err := re1.RTF()
	if err != nil {
		check("RichEdit.RTF", false, "%v", err)
		return
	}
	err = cb.SetRTF(rtf, re1.Text())
	got, err2 := cb.RTF()
	if err == nil && err2 == nil {
		err = re2.SetRTF(got)
	}
	check("RTF from RichEdit to RichEdit", err == nil && err2 == nil && re2.Text() == re1.Text(), "%q, %q, %v, %v", re2.Text(), re1.Text(), err, err2)
}

func testFiles() {
	dir, err := os.MkdirTemp("", "walk-clipboard-test-")
	if err != nil {
		check("files setup", false, "%v", err)
		return
	}
	defer os.RemoveAll(dir)
	paths := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "ľščť 日本.txt")}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("walk clipboard test"), 0o644); err != nil {
			check("files setup", false, "%v", err)
			return
		}
	}

	err = cb.SetFiles(walk.DropEffectMove, paths...)
	check("SetFiles", err == nil, "%v", err)
	got, err := shellFiles()
	check("files read with DragQueryFileW", err == nil && slices.Equal(got, paths), "%q, %v", got, err)
	got, err = cb.Files()
	check("Files", err == nil && slices.Equal(got, paths), "%q, %v", got, err)
	effect, err := cb.PreferredDropEffect()
	check("PreferredDropEffect move", err == nil && effect == walk.DropEffectMove, "%d, %v", effect, err)
	raw, err := rawData(walk.ClipboardFormatPreferredDropEffect)
	check("Preferred DropEffect raw DWORD", err == nil && len(raw) >= 4 && binary.LittleEndian.Uint32(raw) == 2, "%x, %v", raw, err)
	ok, err := cb.ContainsFiles()
	check("ContainsFiles", ok && err == nil, "%v, %v", ok, err)

	err = cb.SetFiles(walk.DropEffectNone, paths[1])
	got, err2 := cb.Files()
	effect, err3 := cb.PreferredDropEffect()
	ok, _ = cb.ContainsFormat(walk.ClipboardFormatPreferredDropEffect)
	check("SetFiles without drop effect", err == nil && err2 == nil && err3 == nil && slices.Equal(got, paths[1:]) && effect == walk.DropEffectNone && !ok,
		"%q, %d, %v, %v %v %v", got, effect, ok, err, err2, err3)

	err = cb.SetFiles(walk.DropEffectCopy)
	check("SetFiles without paths fails", err != nil, "no error")
}

// testPicture is 7 x 5 pixels with different colors, a half transparent
// and a fully transparent pixel.
func testPicture() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 7, 5))
	for y := range 5 {
		for x := range 7 {
			im.SetNRGBA(x, y, color.NRGBA{uint8(30 * x), uint8(50 * y), uint8(255 - 20*x - 10*y), 255})
		}
	}
	im.SetNRGBA(1, 1, color.NRGBA{200, 0, 0, 128})
	im.SetNRGBA(2, 1, color.NRGBA{0, 0, 0, 0})
	return im
}

func onWhite(c color.NRGBA) color.NRGBA {
	f := func(v uint8) uint8 { return uint8((uint32(v)*uint32(c.A) + 255*(255-uint32(c.A)) + 127) / 255) }
	return color.NRGBA{f(c.R), f(c.G), f(c.B), 255}
}

func comparePixels(name string, got image.Image, want *image.NRGBA, conv func(color.NRGBA) color.NRGBA) {
	if got == nil {
		check(name, false, "no image")
		return
	}
	if got.Bounds().Size() != want.Bounds().Size() {
		check(name+" size", false, "%v, want %v", got.Bounds().Size(), want.Bounds().Size())
		return
	}
	check(name+" size", true, "")
	bad := 0
	first := ""
	gb := got.Bounds()
	for y := range want.Bounds().Dy() {
		for x := range want.Bounds().Dx() {
			w := want.NRGBAAt(x, y)
			if conv != nil {
				w = conv(w)
			}
			g := color.NRGBAModel.Convert(got.At(gb.Min.X+x, gb.Min.Y+y)).(color.NRGBA)
			if w.A == 0 && g.A == 0 {
				continue
			}
			if g != w {
				if bad == 0 {
					first = fmt.Sprintf("pixel %d,%d is %v, want %v", x, y, g, w)
				}
				bad++
			}
		}
	}
	check(name+" pixels", bad == 0, "%d wrong, first %s", bad, first)
}

func testImage() {
	pic := testPicture()
	err := cb.SetImage(pic)
	check("SetImage", err == nil, "%v", err)
	formats, _ := cb.Formats()
	for _, f := range []walk.ClipboardFormat{walk.ClipboardFormatPNG, walk.ClipboardFormatDIBV5, walk.ClipboardFormatDIB} {
		check("SetImage sets "+f.Name(), slices.Contains(formats, f), "formats %v", formats)
	}
	fmt.Printf("INFO formats after SetImage: %v\n", formats)
	ok, err := cb.ContainsImage()
	check("ContainsImage", ok && err == nil, "%v, %v", ok, err)

	raw, err := rawData(walk.ClipboardFormatDIBV5)
	if err == nil && len(raw) >= 124 {
		le := binary.LittleEndian
		check("CF_DIBV5 header", le.Uint32(raw) == 124 && le.Uint32(raw[4:]) == 7 && le.Uint32(raw[8:]) == 5 && le.Uint16(raw[14:]) == 32 && le.Uint32(raw[52:]) == 0xFF000000,
			"size %d, %d x %d, %d bits, alpha mask %x", le.Uint32(raw), int32(le.Uint32(raw[4:])), int32(le.Uint32(raw[8:])), le.Uint16(raw[14:]), le.Uint32(raw[52:]))
	} else {
		check("CF_DIBV5 raw data", false, "%d bytes, %v", len(raw), err)
	}
	raw, err = rawData(walk.ClipboardFormatPNG)
	check("PNG raw data has the PNG signature", err == nil && bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")), "%q, %v", raw[:min(len(raw), 8)], err)

	im, err := cb.ImageOfFormat(walk.ClipboardFormatPNG)
	check("ImageOfFormat PNG", err == nil, "%v", err)
	comparePixels("PNG", im, pic, nil)
	im, err = cb.ImageOfFormat(walk.ClipboardFormatDIBV5)
	check("ImageOfFormat CF_DIBV5", err == nil, "%v", err)
	comparePixels("CF_DIBV5", im, pic, nil)
	im, err = cb.ImageOfFormat(walk.ClipboardFormatDIB)
	check("ImageOfFormat CF_DIB", err == nil, "%v", err)
	comparePixels("CF_DIB", im, pic, onWhite)
	im, err = cb.Image()
	check("Image", err == nil, "%v", err)
	comparePixels("Image", im, pic, nil)

	// A walk Bitmap in and out. Bitmaps store premultiplied alpha, so an
	// opaque picture keeps exact pixels.
	opaque := testPicture()
	for i := 3; i < len(opaque.Pix); i += 4 {
		opaque.Pix[i] = 255
	}
	bmp, err := walk.NewBitmapFromImageForDPI(opaque, 96)
	if err != nil {
		check("NewBitmapFromImageForDPI", false, "%v", err)
		return
	}
	defer bmp.Dispose()
	err = cb.SetBitmap(bmp)
	check("SetBitmap", err == nil, "%v", err)
	im, err = cb.Image()
	check("Image after SetBitmap", err == nil, "%v", err)
	comparePixels("SetBitmap", im, opaque, nil)
	bmp2, err := cb.Bitmap(96)
	if err == nil {
		check("Bitmap size", bmp2.Size() == walk.Size{Width: 7, Height: 5}, "%v", bmp2.Size())
		back, err := bmp2.ToImage()
		if err == nil {
			comparePixels("Bitmap", back, opaque, nil)
		} else {
			check("Bitmap ToImage", false, "%v", err)
		}
		bmp2.Dispose()
	} else {
		check("Bitmap", false, "%v", err)
	}

	_, err = cb.ImageOfFormat(walk.ClipboardFormatText)
	check("ImageOfFormat with a text format fails", err != nil, "no error")
}

func testMultiple(customFormat walk.ClipboardFormat) {
	pic := testPicture()
	exe, _ := os.Executable()
	var d walk.ClipboardData
	d.SetText("multi text")
	d.SetHTML("<b>multi</b>", "")
	d.SetRTF(sampleRTF)
	d.SetFiles(walk.DropEffectCopy, exe)
	d.SetImage(pic)
	d.SetFormat(customFormat, []byte{1, 2, 3, 0, 4})
	d.SetText("multi text, replaced") // replaces the first text
	err := cb.SetData(&d)
	check("SetData with several formats", err == nil, "%v", err)

	formats, err := cb.Formats()
	names := make([]string, len(formats))
	for i, f := range formats {
		names[i] = f.Name()
	}
	fmt.Printf("INFO formats: %s\n", strings.Join(names, ", "))
	want := []walk.ClipboardFormat{walk.ClipboardFormatText, walk.ClipboardFormatHTML, walk.ClipboardFormatRTF,
		walk.ClipboardFormatFiles, walk.ClipboardFormatPreferredDropEffect, walk.ClipboardFormatPNG,
		walk.ClipboardFormatDIBV5, walk.ClipboardFormatDIB, customFormat}
	last := -1
	inOrder := true
	for _, f := range want {
		i := slices.Index(formats, f)
		if i < 0 {
			check("Formats contains "+f.Name(), false, "%v", names)
			inOrder = false
			continue
		}
		if i < last {
			inOrder = false
		}
		last = i
	}
	check("Formats in the order of setting", err == nil && inOrder, "%v, %v", names, err)
	check("Formats name of the custom format", customFormat.Name() == "walk clipboard test format", "%q", customFormat.Name())

	text, err1 := cb.Text()
	h, err2 := cb.HTML()
	rtf, err3 := cb.RTF()
	files, err4 := cb.Files()
	im, err5 := cb.ImageOfFormat(walk.ClipboardFormatDIBV5)
	custom, err6 := cb.Data(customFormat)
	check("SetData text", err1 == nil && text == "multi text, replaced", "%q, %v", text, err1)
	check("SetData HTML", err2 == nil && h.Fragment == "<b>multi</b>", "%v, %v", h, err2)
	check("SetData RTF", err3 == nil && rtf == sampleRTF, "%q, %v", rtf, err3)
	check("SetData files", err4 == nil && slices.Equal(files, []string{exe}), "%q, %v", files, err4)
	check("SetData image", err5 == nil, "%v", err5)
	comparePixels("SetData image", im, pic, nil)
	check("SetData custom format", err6 == nil && bytes.HasPrefix(custom, []byte{1, 2, 3, 0, 4}), "%x, %v", custom, err6)

	for name, f := range map[string]func() (bool, error){
		"ContainsText": cb.ContainsText, "ContainsHTML": cb.ContainsHTML, "ContainsRTF": cb.ContainsRTF,
		"ContainsFiles": cb.ContainsFiles, "ContainsImage": cb.ContainsImage,
	} {
		ok, err := f()
		check(name+" with several formats", ok && err == nil, "%v, %v", ok, err)
	}
	sensitive, err := cb.IsSensitive()
	check("IsSensitive false", !sensitive && err == nil, "%v, %v", sensitive, err)

	// A conversion that fails leaves the clipboard unchanged.
	var bad walk.ClipboardData
	bad.SetText("must not appear")
	bad.SetHTML("a\x00b", "")
	err = cb.SetData(&bad)
	text, _ = cb.Text()
	check("SetData with an invalid item fails and changes nothing", err != nil && text == "multi text, replaced", "%v, %q", err, text)

	if err := cb.Clear(); err != nil {
		check("Clear", false, "%v", err)
	}
	formats, _ = cb.Formats()
	check("Clear empties the clipboard", len(formats) == 0, "%v", formats)
	for name, f := range map[string]func() (bool, error){
		"ContainsText": cb.ContainsText, "ContainsHTML": cb.ContainsHTML, "ContainsRTF": cb.ContainsRTF,
		"ContainsFiles": cb.ContainsFiles, "ContainsImage": cb.ContainsImage,
	} {
		ok, err := f()
		check(name+" after Clear", !ok && err == nil, "%v, %v", ok, err)
	}
	_, err = cb.HTML()
	check("HTML on an empty clipboard", errors.Is(err, walk.ErrClipboardFormatNotAvailable), "%v", err)
	_, err = cb.Files()
	check("Files on an empty clipboard", errors.Is(err, walk.ErrClipboardFormatNotAvailable), "%v", err)
	_, err = cb.Image()
	check("Image on an empty clipboard", errors.Is(err, walk.ErrClipboardFormatNotAvailable), "%v", err)
	effect, err := cb.PreferredDropEffect()
	check("PreferredDropEffect on an empty clipboard", effect == walk.DropEffectNone && err == nil, "%d, %v", effect, err)
}

func testSensitive() {
	err := cb.SetSensitiveText("secret password")
	check("SetSensitiveText", err == nil, "%v", err)
	text, err := cb.Text()
	check("sensitive text can be pasted", err == nil && text == "secret password", "%q, %v", text, err)
	for _, f := range []walk.ClipboardFormat{walk.ClipboardFormatExcludeFromMonitorProcessing,
		walk.ClipboardFormatCanIncludeInClipboardHistory, walk.ClipboardFormatCanUploadToCloudClipboard} {
		ok, _ := cb.ContainsFormat(f)
		check("sensitive text has "+f.Name(), ok, "missing")
	}
	for _, f := range []walk.ClipboardFormat{walk.ClipboardFormatCanIncludeInClipboardHistory, walk.ClipboardFormatCanUploadToCloudClipboard} {
		raw, err := rawData(f)
		check(f.Name()+" is 0", err == nil && len(raw) >= 4 && binary.LittleEndian.Uint32(raw) == 0, "%x, %v", raw, err)
	}
	sensitive, err := cb.IsSensitive()
	check("IsSensitive", sensitive && err == nil, "%v, %v", sensitive, err)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "manual" {
		manual()
		return
	}

	var mw *walk.MainWindow
	var re1, re2 *walk.RichEdit
	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "walk clipboard test",
		Size:     Size{Width: 400, Height: 300},
		Layout:   VBox{},
		Children: []Widget{
			RichEdit{AssignTo: &re1, Accessibility: Accessibility{Name: "Source"}},
			RichEdit{AssignTo: &re2, Accessibility: Accessibility{Name: "Target"}},
		},
	}).Create(); err != nil {
		fmt.Println("FAIL create window:", err)
		os.Exit(1)
	}

	customFormat, err := walk.RegisterClipboardFormat("walk clipboard test format")
	check("RegisterClipboardFormat", err == nil && customFormat >= 0xC000, "%d, %v", customFormat, err)
	again, _ := walk.RegisterClipboardFormat("walk clipboard test format")
	check("RegisterClipboardFormat again gives the same format", again == customFormat, "%d and %d", again, customFormat)

	// Save the user's clipboard: only text can be put back.
	savedFormats, _ := cb.Formats()
	hadText, _ := cb.ContainsText()
	var savedText string
	if hadText {
		savedText, _ = cb.Text()
	}
	textFormats := []walk.ClipboardFormat{walk.ClipboardFormatText, 1, 7, 16} // CF_TEXT, CF_OEMTEXT, CF_LOCALE
	var lost []string
	for _, f := range savedFormats {
		if !slices.Contains(textFormats, f) {
			lost = append(lost, f.Name())
		}
	}
	fmt.Printf("INFO clipboard before the test: %d formats, text saved: %v\n", len(savedFormats), hadText)
	if len(lost) > 0 {
		fmt.Printf("INFO these formats of the clipboard cannot be restored after the test: %s\n", strings.Join(lost, ", "))
	}

	changed := 0
	cb.ContentsChanged().Attach(func() { changed++ })

	mw.Synchronize(func() {
		testText()
		testHTML()
		testRTF(re1, re2)
		testFiles()
		testImage()
		testMultiple(customFormat)
		testSensitive()

		// WM_CLIPBOARDUPDATE is posted; let the message loop deliver it.
		go func() {
			time.Sleep(time.Second)
			mw.Synchronize(func() {
				check("ContentsChanged", changed > 0, "no event")
				fmt.Printf("INFO ContentsChanged published %d times\n", changed)

				var err error
				if hadText {
					var d walk.ClipboardData
					d.SetText(savedText)
					err = cb.SetData(&d)
				} else {
					err = cb.Clear()
				}
				fmt.Printf("INFO clipboard restored (text: %v), error: %v\n", hadText, err)
				mw.Close()
			})
		}()
	})
	mw.Run()
	if failed {
		os.Exit(1)
	}
}

const manualHTML = `<h2>walk clipboard test</h2><p>This is <b>bold</b>, <i>italic</i> and a <a href="https://example.com/">link</a>.</p><ul><li>First item</li><li>Second item, ľščťžýáíé</li></ul>`

func manualImage() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := range 100 {
		for x := range 200 {
			c := color.NRGBA{uint8(x * 255 / 199), uint8(y * 255 / 99), 128, 255}
			if x >= 150 {
				// The right quarter fades out, to see transparency.
				c.A = uint8(255 - (x-150)*5)
			}
			im.SetNRGBA(x, y, c)
		}
	}
	return im
}

func describeClipboard() string {
	var parts []string
	formats, err := cb.Formats()
	if err != nil {
		return "Formats: " + err.Error()
	}
	if len(formats) == 0 {
		return "The clipboard is empty."
	}
	names := make([]string, len(formats))
	for i, f := range formats {
		names[i] = f.Name()
	}
	parts = append(parts, fmt.Sprintf("%d formats: %s.", len(formats), strings.Join(names, ", ")))
	if s, _ := cb.IsSensitive(); s {
		parts = append(parts, "Marked as sensitive.")
	}
	if ok, _ := cb.ContainsText(); ok {
		text, _ := cb.Text()
		if len([]rune(text)) > 100 {
			text = string([]rune(text)[:100]) + "..."
		}
		parts = append(parts, fmt.Sprintf("Text: %s", text))
	}
	if ok, _ := cb.ContainsHTML(); ok {
		if h, err := cb.HTML(); err == nil {
			parts = append(parts, fmt.Sprintf("HTML fragment of %d bytes, document of %d bytes, source URL %q.", len(h.Fragment), len(h.Document), h.SourceURL))
		} else {
			parts = append(parts, "HTML: "+err.Error())
		}
	}
	if ok, _ := cb.ContainsRTF(); ok {
		rtf, _ := cb.RTF()
		parts = append(parts, fmt.Sprintf("Rich Text Format of %d bytes.", len(rtf)))
	}
	if ok, _ := cb.ContainsFiles(); ok {
		files, _ := cb.Files()
		effect, _ := cb.PreferredDropEffect()
		op := "copied"
		if effect&walk.DropEffectMove != 0 {
			op = "cut"
		}
		parts = append(parts, fmt.Sprintf("%d files %s: %s.", len(files), op, strings.Join(files, "; ")))
	}
	if ok, _ := cb.ContainsImage(); ok {
		if im, err := cb.Image(); err == nil {
			parts = append(parts, fmt.Sprintf("Image of %d by %d pixels.", im.Bounds().Dx(), im.Bounds().Dy()))
		} else {
			parts = append(parts, "Image: "+err.Error())
		}
	}
	return strings.Join(parts, " ")
}

func manual() {
	dir := filepath.Join(os.TempDir(), "walk-clipboard-manual")
	files := []string{filepath.Join(dir, "first.txt"), filepath.Join(dir, "druhý súbor.txt")}
	if err := os.MkdirAll(dir, 0o755); err == nil {
		for _, f := range files {
			os.WriteFile(f, []byte("walk clipboard manual test\r\n"), 0o644)
		}
	}

	var status *walk.Label
	report := func(what string, err error) {
		text := what
		if err != nil {
			text = what + " failed: " + err.Error()
		}
		fmt.Println(text)
		status.SetText(text)
	}
	if _, err := (MainWindow{
		Title:  "walk clipboard manual test",
		Layout: VBox{},
		Children: []Widget{
			PushButton{Text: "Copy &HTML", OnClicked: func() {
				report("Copied HTML with a heading, a link and a list.", cb.SetHTML(manualHTML, "walk clipboard test\r\nThis is bold, italic and a link.\r\nFirst item\r\nSecond item, ľščťžýáíé"))
			}},
			PushButton{Text: "Copy &RTF", OnClicked: func() {
				report("Copied Rich Text Format.", cb.SetRTF(sampleRTF, "Plain bold and italic."))
			}},
			PushButton{Text: "Copy &files", OnClicked: func() {
				report("Copied 2 files from "+dir+".", cb.SetFiles(walk.DropEffectCopy, files...))
			}},
			PushButton{Text: "Cu&t files", OnClicked: func() {
				report("Cut 2 files from "+dir+".", cb.SetFiles(walk.DropEffectMove, files...))
			}},
			PushButton{Text: "Copy &image", OnClicked: func() {
				report("Copied an image of 200 by 100 pixels with a transparent right edge.", cb.SetImage(manualImage()))
			}},
			PushButton{Text: "Copy &password (sensitive)", OnClicked: func() {
				report("Copied the password secret123, excluded from the clipboard history.", cb.SetSensitiveText("secret123"))
			}},
			PushButton{Text: "Re&ad clipboard", OnClicked: func() { report(describeClipboard(), nil) }},
			Label{AssignTo: &status, Text: "Ready", Accessibility: Accessibility{LiveSetting: AccLiveSettingPolite}},
		},
	}).Run(); err != nil {
		fmt.Println("FAIL", err)
		os.Exit(1)
	}
}
