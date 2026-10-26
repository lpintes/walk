// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lpintes/walk/internal/clipfmt"
	"github.com/lpintes/walk/internal/win"
)

// ClipboardFormat identifies a clipboard format: one of the standard
// formats of Windows (CF_UNICODETEXT, CF_DIB, ...) or a format registered
// by name with RegisterClipboardFormat.
type ClipboardFormat uint32

// Standard clipboard formats.
const (
	// ClipboardFormatText is Unicode text (CF_UNICODETEXT), the format of
	// ClipboardService.Text and SetText.
	ClipboardFormatText ClipboardFormat = win.CF_UNICODETEXT

	// ClipboardFormatDIB is a device independent bitmap with a
	// BITMAPINFOHEADER (CF_DIB).
	ClipboardFormatDIB ClipboardFormat = win.CF_DIB

	// ClipboardFormatDIBV5 is a device independent bitmap with a
	// BITMAPV5HEADER (CF_DIBV5), which can carry an alpha channel.
	ClipboardFormatDIBV5 ClipboardFormat = win.CF_DIBV5

	// ClipboardFormatFiles is a list of files (CF_HDROP), as Explorer copies
	// them.
	ClipboardFormatFiles ClipboardFormat = win.CF_HDROP
)

// Registered clipboard formats used by ClipboardService. They are
// registered when the package is initialized; do not change them.
var (
	// ClipboardFormatHTML is "HTML Format", the HTML clipboard format
	// (CF_HTML) of browsers and word processors.
	ClipboardFormatHTML = mustRegisterClipboardFormat("HTML Format")

	// ClipboardFormatRTF is "Rich Text Format".
	ClipboardFormatRTF = mustRegisterClipboardFormat("Rich Text Format")

	// ClipboardFormatPNG is "PNG", an image in the PNG file format.
	ClipboardFormatPNG = mustRegisterClipboardFormat("PNG")

	// ClipboardFormatPreferredDropEffect is "Preferred DropEffect", a
	// DWORD with the DropEffect for a file list: DropEffectMove for cut,
	// DropEffectCopy for copy.
	ClipboardFormatPreferredDropEffect = mustRegisterClipboardFormat("Preferred DropEffect")

	// ClipboardFormatExcludeFromMonitorProcessing is
	// "ExcludeClipboardContentFromMonitorProcessing": clipboard monitors,
	// such as clipboard managers, should ignore content with this format.
	ClipboardFormatExcludeFromMonitorProcessing = mustRegisterClipboardFormat("ExcludeClipboardContentFromMonitorProcessing")

	// ClipboardFormatCanIncludeInClipboardHistory is
	// "CanIncludeInClipboardHistory": a DWORD, 0 keeps the content out of
	// the clipboard history of Windows (Windows+V).
	ClipboardFormatCanIncludeInClipboardHistory = mustRegisterClipboardFormat("CanIncludeInClipboardHistory")

	// ClipboardFormatCanUploadToCloudClipboard is
	// "CanUploadToCloudClipboard": a DWORD, 0 keeps the content from being
	// synchronized to other devices.
	ClipboardFormatCanUploadToCloudClipboard = mustRegisterClipboardFormat("CanUploadToCloudClipboard")
)

// ErrClipboardFormatNotAvailable is returned (possibly wrapped) when the
// clipboard does not contain the requested format.
var ErrClipboardFormatNotAvailable = errors.New("clipboard format not available")

func mustRegisterClipboardFormat(name string) ClipboardFormat {
	f, err := RegisterClipboardFormat(name)
	if err != nil {
		panic(err)
	}
	return f
}

// RegisterClipboardFormat registers a clipboard format by name and returns
// it. Registering the same name again, in any process, returns the same
// format.
func RegisterClipboardFormat(name string) (ClipboardFormat, error) {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	f := win.RegisterClipboardFormat(p)
	if f == 0 {
		return 0, lastError("RegisterClipboardFormat")
	}
	return ClipboardFormat(f), nil
}

var standardClipboardFormatNames = map[ClipboardFormat]string{
	win.CF_TEXT:         "CF_TEXT",
	win.CF_BITMAP:       "CF_BITMAP",
	win.CF_METAFILEPICT: "CF_METAFILEPICT",
	win.CF_SYLK:         "CF_SYLK",
	win.CF_DIF:          "CF_DIF",
	win.CF_TIFF:         "CF_TIFF",
	win.CF_OEMTEXT:      "CF_OEMTEXT",
	win.CF_DIB:          "CF_DIB",
	win.CF_PALETTE:      "CF_PALETTE",
	win.CF_PENDATA:      "CF_PENDATA",
	win.CF_RIFF:         "CF_RIFF",
	win.CF_WAVE:         "CF_WAVE",
	win.CF_UNICODETEXT:  "CF_UNICODETEXT",
	win.CF_ENHMETAFILE:  "CF_ENHMETAFILE",
	win.CF_HDROP:        "CF_HDROP",
	win.CF_LOCALE:       "CF_LOCALE",
	win.CF_DIBV5:        "CF_DIBV5",
}

// Name returns the name of a registered format, the name of the constant
// for a standard format (for example "CF_UNICODETEXT"), or the number in
// hexadecimal for other formats.
func (f ClipboardFormat) Name() string {
	if name, ok := standardClipboardFormatNames[f]; ok {
		return name
	}
	var buf [256]uint16
	if n := win.GetClipboardFormatName(uint32(f), &buf[0], int32(len(buf))); n > 0 {
		return syscall.UTF16ToString(buf[:n])
	}
	return fmt.Sprintf("0x%04X", uint32(f))
}

// String returns the same as Name.
func (f ClipboardFormat) String() string {
	return f.Name()
}

// DropEffect is the effect of a drag and drop or paste operation with
// files. As the "Preferred DropEffect" of a file list on the clipboard,
// Explorer moves the files for DropEffectMove (cut) and copies them
// otherwise.
type DropEffect uint32

// The drop effects.
const (
	DropEffectNone DropEffect = win.DROPEFFECT_NONE
	DropEffectCopy DropEffect = win.DROPEFFECT_COPY
	DropEffectMove DropEffect = win.DROPEFFECT_MOVE
	DropEffectLink DropEffect = win.DROPEFFECT_LINK
)

// ClipboardHTML is HTML read from the clipboard.
type ClipboardHTML struct {
	// Fragment is the copied part of the document.
	Fragment string

	// Document is the whole HTML document that contains the fragment. Its
	// head may carry styles the fragment needs.
	Document string

	// SourceURL is the URL of the page the HTML was copied from, if the
	// source program gave one.
	SourceURL string
}

type clipboardItem struct {
	format ClipboardFormat
	encode func() ([]byte, error)
}

// ClipboardData collects data in several formats, which
// ClipboardService.SetData puts on the clipboard at once, so that each
// program that pastes can take the format it understands best. The zero
// value is empty and ready to use.
//
// The Set methods only record the data; it is converted, and errors are
// returned, by SetData. Setting a format again replaces it.
type ClipboardData struct {
	items []clipboardItem
}

func (d *ClipboardData) add(format ClipboardFormat, encode func() ([]byte, error)) {
	for i := range d.items {
		if d.items[i].format == format {
			d.items[i].encode = encode
			return
		}
	}
	d.items = append(d.items, clipboardItem{format, encode})
}

// SetText sets Unicode text (ClipboardFormatText). The text must not
// contain NUL.
func (d *ClipboardData) SetText(s string) {
	d.add(ClipboardFormatText, func() ([]byte, error) { return clipfmt.UTF16(s) })
}

// SetHTML sets an HTML fragment (ClipboardFormatHTML). It is wrapped in a
// minimal document, and the header gives the byte offsets of the document
// and the fragment. sourceURL is optional and must not contain line breaks.
func (d *ClipboardData) SetHTML(fragment, sourceURL string) {
	d.add(ClipboardFormatHTML, func() ([]byte, error) { return clipfmt.EncodeHTML(fragment, sourceURL) })
}

// SetRTF sets Rich Text Format (ClipboardFormatRTF), such as RichEdit.RTF
// returns. RTF is 7-bit text; the string is stored as it is, followed by
// NUL.
func (d *ClipboardData) SetRTF(rtf string) {
	d.add(ClipboardFormatRTF, func() ([]byte, error) {
		if strings.IndexByte(rtf, 0) >= 0 {
			return nil, errors.New("RTF contains NUL")
		}
		return append([]byte(rtf), 0), nil
	})
}

// SetFiles sets a list of files (ClipboardFormatFiles), which Explorer can
// paste. The paths should be absolute. With effect DropEffectMove, Explorer
// moves the files when pasting (cut); with DropEffectCopy it copies them.
// DropEffectNone sets no ClipboardFormatPreferredDropEffect, which Explorer
// treats as copy.
func (d *ClipboardData) SetFiles(effect DropEffect, paths ...string) {
	paths = append([]string(nil), paths...)
	d.add(ClipboardFormatFiles, func() ([]byte, error) { return clipfmt.EncodeFileList(paths) })
	if effect != DropEffectNone {
		d.add(ClipboardFormatPreferredDropEffect, func() ([]byte, error) {
			return binary.LittleEndian.AppendUint32(nil, uint32(effect)), nil
		})
	}
}

// SetImage sets an image in three formats: ClipboardFormatPNG and
// ClipboardFormatDIBV5 with transparency, and ClipboardFormatDIB (24 bits
// per pixel) for older programs, with transparent pixels blended onto
// white. The image is read when SetData is called.
func (d *ClipboardData) SetImage(im image.Image) {
	d.add(ClipboardFormatPNG, func() ([]byte, error) {
		var buf bytes.Buffer
		if err := png.Encode(&buf, im); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	})
	d.add(ClipboardFormatDIBV5, func() ([]byte, error) { return clipfmt.EncodeDIBV5(im) })
	d.add(ClipboardFormatDIB, func() ([]byte, error) { return clipfmt.EncodeDIB(im) })
}

// SetFormat sets data in any format, as raw bytes. The bytes are copied.
// Use it only for formats stored in global memory (not CF_BITMAP,
// CF_ENHMETAFILE or CF_PALETTE, which are GDI handles).
func (d *ClipboardData) SetFormat(format ClipboardFormat, data []byte) {
	data = append([]byte(nil), data...)
	d.add(format, func() ([]byte, error) { return data, nil })
}

// SetSensitive marks the data as sensitive, for example a password: it
// sets ClipboardFormatExcludeFromMonitorProcessing, and 0 for
// ClipboardFormatCanIncludeInClipboardHistory and
// ClipboardFormatCanUploadToCloudClipboard, so that the clipboard history
// of Windows, the cloud clipboard and clipboard managers that follow the
// convention do not keep it. Other programs can still paste it.
func (d *ClipboardData) SetSensitive() {
	zero := func() ([]byte, error) { return make([]byte, 4), nil }
	d.add(ClipboardFormatExcludeFromMonitorProcessing, zero)
	d.add(ClipboardFormatCanIncludeInClipboardHistory, zero)
	d.add(ClipboardFormatCanUploadToCloudClipboard, zero)
}

// SetData replaces the contents of the clipboard with all formats of data
// at once. All data is converted first; if a conversion fails, the
// clipboard is not changed. If Windows refuses a format, the clipboard is
// emptied and the error returned.
func (c *ClipboardService) SetData(data *ClipboardData) error {
	type encoded struct {
		format ClipboardFormat
		bytes  []byte
	}
	var all []encoded
	for _, item := range data.items {
		b, err := item.encode()
		if err != nil {
			return wrapError(fmt.Errorf("clipboard format %s: %w", item.format.Name(), err))
		}
		all = append(all, encoded{item.format, b})
	}

	return c.withOpenClipboardRetry(func() error {
		if !win.EmptyClipboard() {
			return lastError("EmptyClipboard")
		}
		for _, e := range all {
			if err := setClipboardBytes(e.format, e.bytes); err != nil {
				win.EmptyClipboard()
				return err
			}
		}
		return nil
	})
}

// setClipboardBytes copies b into movable global memory and gives it to the
// clipboard, which must be open. Windows keeps only that memory, never a
// pointer to Go memory.
func setClipboardBytes(format ClipboardFormat, b []byte) error {
	size := max(len(b), 1)
	hMem := win.GlobalAlloc(win.GMEM_MOVEABLE, uintptr(size))
	if hMem == 0 {
		return lastError("GlobalAlloc")
	}
	p := win.GlobalLock(hMem)
	if p == nil {
		err := lastError("GlobalLock")
		win.GlobalFree(hMem)
		return err
	}
	dst := unsafe.Slice((*byte)(p), size)
	clear(dst)
	copy(dst, b)
	win.GlobalUnlock(hMem)

	if win.SetClipboardData(uint32(format), win.HANDLE(hMem)) == 0 {
		err := lastError("SetClipboardData")
		win.GlobalFree(hMem)
		return err
	}
	// The system owns hMem now.
	return nil
}

// clipboardBytes returns a copy of the global memory of a format; the
// clipboard must be open. The copy may be longer than the data, because
// GlobalSize can round the size up.
func clipboardBytes(format ClipboardFormat) ([]byte, error) {
	if !win.IsClipboardFormatAvailable(uint32(format)) {
		return nil, fmt.Errorf("%w: %s", ErrClipboardFormatNotAvailable, format.Name())
	}
	hMem := win.HGLOBAL(win.GetClipboardData(uint32(format)))
	if hMem == 0 {
		return nil, lastError("GetClipboardData")
	}
	size := win.GlobalSize(hMem)
	p := win.GlobalLock(hMem)
	if p == nil {
		return nil, lastError("GlobalLock")
	}
	defer win.GlobalUnlock(hMem)
	return bytes.Clone(unsafe.Slice((*byte)(p), size)), nil
}

// withOpenClipboardRetry is withOpenClipboard, but tries again for a short
// time if another program has the clipboard open. Clipboard monitors, such
// as the clipboard history of Windows, open it right after every change.
func (c *ClipboardService) withOpenClipboardRetry(f func() error) error {
	for i := 0; ; i++ {
		if win.OpenClipboard(c.hwnd) {
			break
		}
		if i == 10 {
			return lastError("OpenClipboard")
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer win.CloseClipboard()

	return f()
}

// Data returns the raw bytes of a format stored in global memory. The
// slice may be longer than the data, because Windows can round the size of
// the memory up. It returns an error wrapping
// ErrClipboardFormatNotAvailable if the clipboard has no such format.
func (c *ClipboardService) Data(format ClipboardFormat) (data []byte, err error) {
	err = c.withOpenClipboardRetry(func() error {
		data, err = clipboardBytes(format)
		return err
	})
	return
}

// Formats returns the formats on the clipboard, in the order the program
// that set them gave them, including the formats Windows can convert to
// (for example CF_TEXT for CF_UNICODETEXT).
func (c *ClipboardService) Formats() (formats []ClipboardFormat, err error) {
	err = c.withOpenClipboardRetry(func() error {
		for f := win.EnumClipboardFormats(0); f != 0; f = win.EnumClipboardFormats(f) {
			formats = append(formats, ClipboardFormat(f))
		}
		return nil
	})
	return
}

// ContainsFormat returns whether the clipboard contains a format.
func (c *ClipboardService) ContainsFormat(format ClipboardFormat) (available bool, err error) {
	err = c.withOpenClipboardRetry(func() error {
		available = win.IsClipboardFormatAvailable(uint32(format))
		return nil
	})
	return
}

func (c *ClipboardService) containsAny(formats ...ClipboardFormat) (available bool, err error) {
	err = c.withOpenClipboardRetry(func() error {
		for _, f := range formats {
			if win.IsClipboardFormatAvailable(uint32(f)) {
				available = true
				break
			}
		}
		return nil
	})
	return
}

// ContainsHTML returns whether the clipboard contains HTML.
func (c *ClipboardService) ContainsHTML() (bool, error) {
	return c.ContainsFormat(ClipboardFormatHTML)
}

// ContainsRTF returns whether the clipboard contains Rich Text Format.
func (c *ClipboardService) ContainsRTF() (bool, error) {
	return c.ContainsFormat(ClipboardFormatRTF)
}

// ContainsFiles returns whether the clipboard contains a list of files.
func (c *ClipboardService) ContainsFiles() (bool, error) {
	return c.ContainsFormat(ClipboardFormatFiles)
}

// ContainsImage returns whether the clipboard contains an image that Image
// can read: PNG or a device independent bitmap (Windows converts CF_BITMAP
// to CF_DIB, so it counts too).
func (c *ClipboardService) ContainsImage() (bool, error) {
	return c.containsAny(ClipboardFormatPNG, ClipboardFormatDIBV5, ClipboardFormatDIB)
}

// IsSensitive returns whether the data on the clipboard is marked as
// sensitive (see ClipboardData.SetSensitive): it has
// ClipboardFormatExcludeFromMonitorProcessing, or 0 for
// ClipboardFormatCanIncludeInClipboardHistory.
func (c *ClipboardService) IsSensitive() (sensitive bool, err error) {
	err = c.withOpenClipboardRetry(func() error {
		if win.IsClipboardFormatAvailable(uint32(ClipboardFormatExcludeFromMonitorProcessing)) {
			sensitive = true
			return nil
		}
		if b, err := clipboardBytes(ClipboardFormatCanIncludeInClipboardHistory); err == nil && len(b) >= 4 {
			sensitive = binary.LittleEndian.Uint32(b) == 0
		}
		return nil
	})
	return
}

// SetSensitiveText sets text marked as sensitive, for example a password:
// the clipboard history of Windows, the cloud clipboard and clipboard
// managers that follow the convention do not keep it. See
// ClipboardData.SetSensitive.
func (c *ClipboardService) SetSensitiveText(s string) error {
	var d ClipboardData
	d.SetText(s)
	d.SetSensitive()
	return c.SetData(&d)
}

// HTML returns the HTML on the clipboard: the copied fragment, the whole
// document and the source URL.
func (c *ClipboardService) HTML() (*ClipboardHTML, error) {
	b, err := c.Data(ClipboardFormatHTML)
	if err != nil {
		return nil, err
	}
	h, err := clipfmt.DecodeHTML(b)
	if err != nil {
		return nil, wrapError(err)
	}
	return &ClipboardHTML{Fragment: h.Fragment, Document: h.Document, SourceURL: h.SourceURL}, nil
}

// SetHTML replaces the contents of the clipboard with an HTML fragment
// and, if plainText is not empty, the same content as plain text for
// programs that do not understand HTML. See ClipboardData.SetHTML.
func (c *ClipboardService) SetHTML(fragment, plainText string) error {
	var d ClipboardData
	d.SetHTML(fragment, "")
	if plainText != "" {
		d.SetText(plainText)
	}
	return c.SetData(&d)
}

// RTF returns the Rich Text Format on the clipboard.
func (c *ClipboardService) RTF() (string, error) {
	b, err := c.Data(ClipboardFormatRTF)
	if err != nil {
		return "", err
	}
	return clipfmt.String(b), nil
}

// SetRTF replaces the contents of the clipboard with Rich Text Format and,
// if plainText is not empty, the same content as plain text.
func (c *ClipboardService) SetRTF(rtf, plainText string) error {
	var d ClipboardData
	d.SetRTF(rtf)
	if plainText != "" {
		d.SetText(plainText)
	}
	return c.SetData(&d)
}

// Files returns the paths of the list of files on the clipboard, for
// example files copied or cut in Explorer.
func (c *ClipboardService) Files() (paths []string, err error) {
	err = c.withOpenClipboardRetry(func() error {
		if !win.IsClipboardFormatAvailable(win.CF_HDROP) {
			return fmt.Errorf("%w: %s", ErrClipboardFormatNotAvailable, ClipboardFormatFiles.Name())
		}
		// The clipboard owns the HDROP: no DragFinish.
		hDrop := win.HDROP(win.GetClipboardData(win.CF_HDROP))
		if hDrop == 0 {
			return lastError("GetClipboardData")
		}
		n := win.DragQueryFile(hDrop, 0xFFFFFFFF, nil, 0)
		for i := range n {
			size := win.DragQueryFile(hDrop, i, nil, 0) + 1
			buf := make([]uint16, size)
			win.DragQueryFile(hDrop, i, &buf[0], size)
			paths = append(paths, syscall.UTF16ToString(buf))
		}
		return nil
	})
	return
}

// PreferredDropEffect returns the ClipboardFormatPreferredDropEffect of the
// clipboard: DropEffectMove if files were cut, DropEffectCopy (or a
// combination with DropEffectLink) if they were copied, and DropEffectNone
// if the format is missing, which Explorer treats as copy.
func (c *ClipboardService) PreferredDropEffect() (DropEffect, error) {
	b, err := c.Data(ClipboardFormatPreferredDropEffect)
	if errors.Is(err, ErrClipboardFormatNotAvailable) {
		return DropEffectNone, nil
	}
	if err != nil {
		return DropEffectNone, err
	}
	if len(b) < 4 {
		return DropEffectNone, newError("Preferred DropEffect: too short")
	}
	return DropEffect(binary.LittleEndian.Uint32(b)), nil
}

// SetFiles replaces the contents of the clipboard with a list of files.
// See ClipboardData.SetFiles.
func (c *ClipboardService) SetFiles(effect DropEffect, paths ...string) error {
	var d ClipboardData
	d.SetFiles(effect, paths...)
	return c.SetData(&d)
}

// Image returns the image on the clipboard, read from the first available
// of ClipboardFormatPNG, ClipboardFormatDIBV5 and ClipboardFormatDIB (into
// which Windows converts CF_BITMAP). A bitmap is returned as *image.NRGBA.
func (c *ClipboardService) Image() (image.Image, error) {
	for _, f := range []ClipboardFormat{ClipboardFormatPNG, ClipboardFormatDIBV5, ClipboardFormatDIB} {
		im, err := c.ImageOfFormat(f)
		if errors.Is(err, ErrClipboardFormatNotAvailable) {
			continue
		}
		return im, err
	}
	return nil, fmt.Errorf("%w: no image", ErrClipboardFormatNotAvailable)
}

// ImageOfFormat returns the image on the clipboard in one format:
// ClipboardFormatPNG, ClipboardFormatDIBV5 or ClipboardFormatDIB.
//
// The alpha channel of a 32-bit bitmap is read as straight (not
// premultiplied) alpha; a 32-bit bitmap whose alpha values are all zero is
// read as opaque, because many programs leave them zero. Compressed bitmaps
// are not supported.
func (c *ClipboardService) ImageOfFormat(format ClipboardFormat) (image.Image, error) {
	switch format {
	case ClipboardFormatPNG, ClipboardFormatDIBV5, ClipboardFormatDIB:
	default:
		return nil, newError(fmt.Sprintf("clipboard format %s is not an image format", format.Name()))
	}
	b, err := c.Data(format)
	if err != nil {
		return nil, err
	}
	var im image.Image
	if format == ClipboardFormatPNG {
		im, err = png.Decode(bytes.NewReader(b))
	} else {
		im, err = clipfmt.DecodeDIB(b)
	}
	if err != nil {
		return nil, wrapError(err)
	}
	return im, nil
}

// SetImage replaces the contents of the clipboard with an image. See
// ClipboardData.SetImage.
func (c *ClipboardService) SetImage(im image.Image) error {
	var d ClipboardData
	d.SetImage(im)
	return c.SetData(&d)
}

// Bitmap returns the image on the clipboard (see Image) as a Bitmap for
// the given DPI.
func (c *ClipboardService) Bitmap(dpi int) (*Bitmap, error) {
	im, err := c.Image()
	if err != nil {
		return nil, err
	}
	return NewBitmapFromImageForDPI(im, dpi)
}

// SetBitmap replaces the contents of the clipboard with the pixels of a
// Bitmap. A bitmap drawn with GDI functions may have only zero alpha
// values; it is copied as opaque. See ClipboardData.SetImage.
func (c *ClipboardService) SetBitmap(bmp *Bitmap) error {
	im, err := bmp.ToImage()
	if err != nil {
		return err
	}
	allZero := true
	for i := 3; i < len(im.Pix); i += 4 {
		if im.Pix[i] != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		for i := 3; i < len(im.Pix); i += 4 {
			im.Pix[i] = 0xFF
		}
	}
	return c.SetImage(im)
}
