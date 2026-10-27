// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package clipfmt encodes and decodes the bytes of clipboard formats: the
// HTML clipboard format (CF_HTML), device independent bitmaps (CF_DIB and
// CF_DIBV5) and file lists (the DROPFILES structure of CF_HDROP).
//
// It works on byte slices only and calls no Windows function, so walk can
// copy the result into memory from GlobalAlloc and the tests run on every
// platform.
package clipfmt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math/bits"
	"strconv"
	"strings"
	"unicode/utf16"
)

var le = binary.LittleEndian

// HTML is the content of the HTML clipboard format.
type HTML struct {
	// Fragment is the part of the document that was copied, between
	// StartFragment and EndFragment.
	Fragment string

	// Document is the whole HTML document, between StartHTML and EndHTML.
	// It contains Fragment.
	Document string

	// SourceURL is the URL of the document the fragment comes from, or the
	// empty string.
	SourceURL string
}

const (
	htmlHeader = "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
	htmlPrefix = "<html>\r\n<body>\r\n<!--StartFragment-->"
	htmlSuffix = "<!--EndFragment-->\r\n</body>\r\n</html>"

	startFragmentComment = "<!--StartFragment-->"
	endFragmentComment   = "<!--EndFragment-->"
)

// EncodeHTML returns the HTML clipboard format for an HTML fragment: the
// header with the byte offsets (in UTF-8) of the document and of the
// fragment, a document around the fragment and a terminating NUL, which the
// offsets do not include.
//
// sourceURL is optional; it must not contain line breaks. The fragment must
// not contain NUL.
func EncodeHTML(fragment, sourceURL string) ([]byte, error) {
	if strings.IndexByte(fragment, 0) >= 0 {
		return nil, errors.New("HTML fragment contains NUL")
	}
	if strings.ContainsAny(sourceURL, "\r\n\x00") {
		return nil, errors.New("HTML source URL contains a line break or NUL")
	}
	src := ""
	if sourceURL != "" {
		src = "SourceURL:" + sourceURL + "\r\n"
	}
	startHTML := len(fmt.Sprintf(htmlHeader, 0, 0, 0, 0)) + len(src)
	startFragment := startHTML + len(htmlPrefix)
	endFragment := startFragment + len(fragment)
	endHTML := endFragment + len(htmlSuffix)

	b := make([]byte, 0, endHTML+1)
	b = fmt.Appendf(b, htmlHeader, startHTML, endHTML, startFragment, endFragment)
	b = append(b, src...)
	b = append(b, htmlPrefix...)
	b = append(b, fragment...)
	b = append(b, htmlSuffix...)
	b = append(b, 0)
	return b, nil
}

// DecodeHTML parses the HTML clipboard format. data may contain trailing
// bytes after a NUL, as memory from GlobalAlloc does.
//
// When the fragment offsets are missing or out of range, the fragment is
// taken from between the StartFragment and EndFragment comments of the
// document.
func DecodeHTML(data []byte) (HTML, error) {
	if i := indexNUL(data); i >= 0 {
		data = data[:i]
	}

	offsets := map[string]int{"StartHTML": -1, "EndHTML": -1, "StartFragment": -1, "EndFragment": -1}
	var h HTML
	seenVersion := false
	pos := 0
	for pos < len(data) {
		if start := offsets["StartHTML"]; start >= 0 && pos >= start {
			break
		}
		end := pos
		for end < len(data) && data[end] != '\r' && data[end] != '\n' {
			end++
		}
		line := string(data[pos:end])
		if strings.HasPrefix(line, "<") {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			break
		}
		switch key {
		case "Version":
			seenVersion = true
		case "SourceURL":
			h.SourceURL = value
		default:
			if _, isOffset := offsets[key]; isOffset {
				n, err := strconv.Atoi(strings.TrimSpace(value))
				if err != nil {
					return HTML{}, fmt.Errorf("HTML clipboard format: invalid %s %q", key, value)
				}
				offsets[key] = n
			}
		}
		pos = end
		if pos < len(data) && data[pos] == '\r' {
			pos++
		}
		if pos < len(data) && data[pos] == '\n' {
			pos++
		}
	}
	if !seenVersion {
		return HTML{}, errors.New("HTML clipboard format: no Version in the header")
	}

	startHTML, endHTML := offsets["StartHTML"], offsets["EndHTML"]
	if startHTML < 0 || startHTML > len(data) {
		startHTML = pos
	}
	if endHTML < startHTML || endHTML > len(data) {
		endHTML = len(data)
	}
	h.Document = string(data[startHTML:endHTML])

	startFragment, endFragment := offsets["StartFragment"], offsets["EndFragment"]
	if startFragment >= 0 && startFragment <= endFragment && endFragment <= len(data) {
		h.Fragment = string(data[startFragment:endFragment])
		return h, nil
	}
	i := strings.Index(h.Document, startFragmentComment)
	j := strings.LastIndex(h.Document, endFragmentComment)
	if i < 0 || j < i+len(startFragmentComment) {
		return HTML{}, errors.New("HTML clipboard format: no valid fragment")
	}
	h.Fragment = h.Document[i+len(startFragmentComment) : j]
	return h, nil
}

func indexNUL(data []byte) int {
	for i, b := range data {
		if b == 0 {
			return i
		}
	}
	return -1
}

// String returns the bytes of data up to the first NUL, for text formats
// such as Rich Text Format.
func String(data []byte) string {
	if i := indexNUL(data); i >= 0 {
		data = data[:i]
	}
	return string(data)
}

// UTF16 returns s as UTF-16 with a terminating NUL, in little-endian bytes,
// as CF_UNICODETEXT stores it. s must not contain NUL.
func UTF16(s string) ([]byte, error) {
	if strings.IndexByte(s, 0) >= 0 {
		return nil, errors.New("text contains NUL")
	}
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u)+2)
	for i, c := range u {
		le.PutUint16(b[2*i:], c)
	}
	return b, nil
}

// dropFilesSize is the size of the DROPFILES structure: pFiles (DWORD), pt
// (POINT, two LONGs), fNC (BOOL) and fWide (BOOL).
const dropFilesSize = 20

// EncodeFileList returns a DROPFILES structure followed by the paths in
// UTF-16, each terminated by NUL, and a second NUL after the last one, as
// CF_HDROP stores it. There must be at least one path, and paths must not be
// empty or contain NUL.
func EncodeFileList(paths []string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, errors.New("no file paths")
	}
	var u []uint16
	for _, p := range paths {
		if p == "" || strings.IndexByte(p, 0) >= 0 {
			return nil, fmt.Errorf("invalid file path %q", p)
		}
		u = append(u, utf16.Encode([]rune(p))...)
		u = append(u, 0)
	}
	u = append(u, 0)
	b := make([]byte, dropFilesSize+2*len(u))
	le.PutUint32(b[0:], dropFilesSize) // pFiles
	le.PutUint32(b[16:], 1)            // fWide
	for i, c := range u {
		le.PutUint16(b[dropFilesSize+2*i:], c)
	}
	return b, nil
}

// DecodeFileList parses a DROPFILES structure with its paths, wide or ANSI
// (ANSI paths are decoded as Latin-1, which is right only for ASCII).
func DecodeFileList(data []byte) ([]string, error) {
	if len(data) < dropFilesSize {
		return nil, errors.New("DROPFILES too short")
	}
	off := int(le.Uint32(data))
	wide := le.Uint32(data[16:]) != 0
	if off < dropFilesSize || off > len(data) {
		return nil, errors.New("DROPFILES: invalid pFiles")
	}
	var paths []string
	if wide {
		var cur []uint16
		for i := off; i+1 < len(data); i += 2 {
			c := le.Uint16(data[i:])
			if c != 0 {
				cur = append(cur, c)
				continue
			}
			if len(cur) == 0 {
				return paths, nil
			}
			paths = append(paths, string(utf16.Decode(cur)))
			cur = cur[:0]
		}
	} else {
		var cur []rune
		for _, c := range data[off:] {
			if c != 0 {
				cur = append(cur, rune(c))
				continue
			}
			if len(cur) == 0 {
				return paths, nil
			}
			paths = append(paths, string(cur))
			cur = cur[:0]
		}
	}
	return nil, errors.New("DROPFILES: the list is not terminated")
}

// Bitmap header and compression values of wingdi.h.
const (
	bitmapInfoHeaderSize = 40
	bitmapV5HeaderSize   = 124

	biRGB            = 0
	biBitfields      = 3
	biAlphaBitfields = 6

	lcsSRGB     = 0x73524742 // 'sRGB'
	lcsGMImages = 4

	pelsPerMeter96DPI = 3780
)

// maxPixels limits the size of decoded and encoded bitmaps (8192 x 8192),
// so that a corrupt header cannot make the decoder allocate gigabytes,
// also in the address space of a 32-bit process.
const maxPixels = 1 << 26

// validSize reports whether w x h is a positive size of at most maxPixels.
// It does not multiply, so it cannot overflow a 32-bit int.
func validSize(w, h int) bool {
	return w > 0 && h > 0 && w <= maxPixels/h
}

func nrgbaPixels(im image.Image) (w, h int, pix []color.NRGBA, err error) {
	b := im.Bounds()
	w, h = b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0, nil, errors.New("empty image")
	}
	if !validSize(w, h) {
		return 0, 0, nil, errors.New("image too large")
	}
	pix = make([]color.NRGBA, 0, w*h)
	switch im := im.(type) {
	case *image.NRGBA:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			row := im.Pix[im.PixOffset(b.Min.X, y):]
			for x := range w {
				q := row[4*x:]
				pix = append(pix, color.NRGBA{q[0], q[1], q[2], q[3]})
			}
		}
		return w, h, pix, nil
	case *image.RGBA:
		// Premultiplied alpha: divide the colors by alpha.
		for y := b.Min.Y; y < b.Max.Y; y++ {
			row := im.Pix[im.PixOffset(b.Min.X, y):]
			for x := range w {
				q := row[4*x:]
				pix = append(pix, unpremultiply(q[0], q[1], q[2], q[3]))
			}
		}
		return w, h, pix, nil
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			pix = append(pix, color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA))
		}
	}
	return w, h, pix, nil
}

// EncodeDIBV5 returns a CF_DIBV5 bitmap of im: a BITMAPV5HEADER with
// BI_BITFIELDS (the masks are part of the header) and the sRGB color space,
// followed by 32-bit bottom-up rows with straight (not premultiplied) alpha.
func EncodeDIBV5(im image.Image) ([]byte, error) {
	w, h, pix, err := nrgbaPixels(im)
	if err != nil {
		return nil, err
	}
	size := 4 * w * h
	b := make([]byte, bitmapV5HeaderSize+size)
	le.PutUint32(b[0:], bitmapV5HeaderSize)
	le.PutUint32(b[4:], uint32(w))
	le.PutUint32(b[8:], uint32(h))
	le.PutUint16(b[12:], 1)  // planes
	le.PutUint16(b[14:], 32) // bit count
	le.PutUint32(b[16:], biBitfields)
	le.PutUint32(b[20:], uint32(size))
	le.PutUint32(b[24:], pelsPerMeter96DPI)
	le.PutUint32(b[28:], pelsPerMeter96DPI)
	le.PutUint32(b[40:], 0x00FF0000) // red mask
	le.PutUint32(b[44:], 0x0000FF00) // green mask
	le.PutUint32(b[48:], 0x000000FF) // blue mask
	le.PutUint32(b[52:], 0xFF000000) // alpha mask
	le.PutUint32(b[56:], lcsSRGB)
	// Endpoints (36 bytes) and gamma (12 bytes) stay zero.
	le.PutUint32(b[108:], lcsGMImages)
	// Profile data, profile size and reserved stay zero.
	p := b[bitmapV5HeaderSize:]
	for y := range h {
		row := pix[(h-1-y)*w : (h-y)*w]
		for _, c := range row {
			p[0], p[1], p[2], p[3] = c.B, c.G, c.R, c.A
			p = p[4:]
		}
	}
	return b, nil
}

// EncodeDIB returns a CF_DIB bitmap of im: a BITMAPINFOHEADER followed by
// 24-bit bottom-up rows. Programs differ in how they read the fourth byte of
// a 32-bit CF_DIB, so transparency is not stored: pixels that are not opaque
// are blended onto white.
func EncodeDIB(im image.Image) ([]byte, error) {
	w, h, pix, err := nrgbaPixels(im)
	if err != nil {
		return nil, err
	}
	stride := (w*3 + 3) &^ 3
	size := stride * h
	b := make([]byte, bitmapInfoHeaderSize+size)
	le.PutUint32(b[0:], bitmapInfoHeaderSize)
	le.PutUint32(b[4:], uint32(w))
	le.PutUint32(b[8:], uint32(h))
	le.PutUint16(b[12:], 1)  // planes
	le.PutUint16(b[14:], 24) // bit count
	le.PutUint32(b[16:], biRGB)
	le.PutUint32(b[20:], uint32(size))
	le.PutUint32(b[24:], pelsPerMeter96DPI)
	le.PutUint32(b[28:], pelsPerMeter96DPI)
	onWhite := func(v, a uint8) byte {
		return byte((uint32(v)*uint32(a) + 255*(255-uint32(a)) + 127) / 255)
	}
	for y := range h {
		row := pix[(h-1-y)*w : (h-y)*w]
		p := b[bitmapInfoHeaderSize+y*stride:]
		for _, c := range row {
			p[0], p[1], p[2] = onWhite(c.B, c.A), onWhite(c.G, c.A), onWhite(c.R, c.A)
			p = p[3:]
		}
	}
	return b, nil
}

// unpremultiply converts a premultiplied color to straight alpha, as
// color.NRGBAModel does.
func unpremultiply(r, g, b, a uint8) color.NRGBA {
	switch a {
	case 0:
		return color.NRGBA{}
	case 255:
		return color.NRGBA{r, g, b, 255}
	}
	// 255 * 0x101 * 0xffff fits into uint32.
	f := func(v uint8) uint8 { return uint8(uint32(v) * 0x101 * 0xffff / (uint32(a) * 0x101) >> 8) }
	return color.NRGBA{f(r), f(g), f(b), a}
}

type channel struct {
	mask  uint32
	shift int
	max   uint32
}

func newChannel(mask uint32) channel {
	if mask == 0 {
		return channel{}
	}
	shift := bits.TrailingZeros32(mask)
	return channel{mask, shift, mask >> shift}
}

func (c channel) value(v uint32) uint8 {
	if c.mask == 0 {
		return 0
	}
	return uint8((uint64((v&c.mask)>>c.shift)*255 + uint64(c.max)/2) / uint64(c.max))
}

// DecodeDIB decodes a CF_DIB or CF_DIBV5 bitmap: a BITMAPINFOHEADER,
// BITMAPV4HEADER, BITMAPV5HEADER or one of the other header versions,
// optional color masks and color table, and uncompressed pixels with 1, 4,
// 8, 16, 24 or 32 bits per pixel, bottom-up or top-down. Compressed bitmaps
// (RLE, JPEG, PNG) are not supported.
//
// The alpha channel of 32-bit bitmaps is taken as straight alpha. Many
// programs leave the fourth byte zero, so a 32-bit bitmap whose alpha values
// are all zero is decoded as opaque. Color space and gamma are ignored.
func DecodeDIB(data []byte) (*image.NRGBA, error) {
	if len(data) < 4 {
		return nil, errors.New("DIB too short")
	}
	hdr := int(le.Uint32(data))
	if hdr == 12 {
		return nil, errors.New("DIB with BITMAPCOREHEADER is not supported")
	}
	if hdr < bitmapInfoHeaderSize || hdr > len(data) {
		return nil, fmt.Errorf("DIB: invalid header size %d", hdr)
	}
	w := int(int32(le.Uint32(data[4:])))
	h := int(int32(le.Uint32(data[8:])))
	bpp := int(le.Uint16(data[14:]))
	compression := le.Uint32(data[16:])
	sizeImage := le.Uint32(data[20:])
	clrUsed32 := le.Uint32(data[32:])
	if clrUsed32 > 1<<16 {
		return nil, fmt.Errorf("DIB: invalid color count %d", clrUsed32)
	}
	clrUsed := int(clrUsed32)
	topDown := h < 0
	if topDown {
		h = -h
	}
	if !validSize(w, h) {
		return nil, fmt.Errorf("DIB: invalid size %d x %d", w, h)
	}

	offset := hdr
	var red, green, blue, alpha channel
	switch compression {
	case biRGB:
		switch bpp {
		case 16:
			red, green, blue = newChannel(0x7C00), newChannel(0x03E0), newChannel(0x001F)
		case 24, 32:
			red, green, blue = newChannel(0x00FF0000), newChannel(0x0000FF00), newChannel(0x000000FF)
			if bpp == 32 {
				alpha = newChannel(0xFF000000)
			}
		case 1, 4, 8:
		default:
			return nil, fmt.Errorf("DIB: unsupported bit count %d", bpp)
		}
	case biBitfields, biAlphaBitfields:
		if bpp != 16 && bpp != 32 {
			return nil, fmt.Errorf("DIB: bit fields with bit count %d", bpp)
		}
		var masks [4]uint32
		n := 3
		if compression == biAlphaBitfields {
			n = 4
		}
		if hdr >= 52 {
			// BITMAPV2INFOHEADER and later have the masks in the header.
			for i := range 3 {
				masks[i] = le.Uint32(data[40+4*i:])
			}
			if hdr >= 56 {
				masks[3] = le.Uint32(data[52:])
			}
		} else {
			if len(data) < offset+4*n {
				return nil, errors.New("DIB: color masks missing")
			}
			for i := range n {
				masks[i] = le.Uint32(data[offset+4*i:])
			}
			offset += 4 * n
		}
		red, green, blue, alpha = newChannel(masks[0]), newChannel(masks[1]), newChannel(masks[2]), newChannel(masks[3])
	default:
		return nil, fmt.Errorf("DIB: unsupported compression %d", compression)
	}

	var palette []color.NRGBA
	if bpp <= 8 {
		n := clrUsed
		if n == 0 {
			n = 1 << bpp
		}
		if n > 1<<bpp {
			return nil, fmt.Errorf("DIB: %d colors for %d bits per pixel", n, bpp)
		}
		if len(data) < offset+4*n {
			return nil, errors.New("DIB: color table missing")
		}
		for i := range n {
			q := data[offset+4*i:]
			palette = append(palette, color.NRGBA{q[2], q[1], q[0], 255})
		}
		offset += 4 * n
	} else if clrUsed > 0 {
		// An optional color table for palette devices precedes the pixels.
		offset += 4 * clrUsed
	}

	// At most 32 bits per pixel and maxPixels pixels: no overflow.
	stride := (w*bpp + 31) / 32 * 4
	need := stride * h
	if offset > len(data) || len(data)-offset < need {
		return nil, errors.New("DIB: pixel data truncated")
	}
	if hdr > bitmapInfoHeaderSize && (compression == biBitfields || compression == biAlphaBitfields) &&
		int64(sizeImage) == int64(need) && len(data)-offset == need+12 {
		// Some programs write the three masks after a BITMAPV4HEADER or
		// BITMAPV5HEADER too, although they are part of it.
		offset += 12
	}

	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	allAlphaZero := true
	for y := range h {
		src := data[offset+y*stride:]
		dy := h - 1 - y
		if topDown {
			dy = y
		}
		dst := im.Pix[dy*im.Stride:]
		for x := range w {
			var c color.NRGBA
			switch bpp {
			case 1, 4, 8:
				bit := x * bpp
				idx := int(src[bit/8]>>(8-bpp-bit%8)) & (1<<bpp - 1)
				if idx < len(palette) {
					c = palette[idx]
				} else {
					c = color.NRGBA{A: 255}
				}
			default:
				var v uint32
				switch bpp {
				case 16:
					v = uint32(le.Uint16(src[2*x:]))
				case 24:
					v = uint32(src[3*x]) | uint32(src[3*x+1])<<8 | uint32(src[3*x+2])<<16
				case 32:
					v = le.Uint32(src[4*x:])
				}
				c = color.NRGBA{red.value(v), green.value(v), blue.value(v), 255}
				if alpha.mask != 0 {
					c.A = alpha.value(v)
					if c.A != 0 {
						allAlphaZero = false
					}
				}
			}
			dst[4*x], dst[4*x+1], dst[4*x+2], dst[4*x+3] = c.R, c.G, c.B, c.A
		}
	}
	if alpha.mask != 0 && allAlphaZero {
		for i := 3; i < len(im.Pix); i += 4 {
			im.Pix[i] = 255
		}
	}
	return im, nil
}
