// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package clipfmt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"testing"
)

func headerOffset(t *testing.T, data []byte, key string) int {
	t.Helper()
	i := bytes.Index(data, []byte(key+":"))
	if i < 0 {
		t.Fatalf("no %s in %q", key, data)
	}
	rest := data[i+len(key)+1:]
	end := bytes.IndexByte(rest, '\r')
	n, err := strconv.Atoi(string(rest[:end]))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestHTMLOffsets(t *testing.T) {
	for _, src := range []string{"", "https://example.com/a?b=c"} {
		fragment := "<p>ľščťž <b>bold</b> 日本 😀</p>"
		data, err := EncodeHTML(fragment, src)
		if err != nil {
			t.Fatal(err)
		}
		if data[len(data)-1] != 0 {
			t.Fatal("no terminating NUL")
		}
		text := data[:len(data)-1]
		sh := headerOffset(t, text, "StartHTML")
		eh := headerOffset(t, text, "EndHTML")
		sf := headerOffset(t, text, "StartFragment")
		ef := headerOffset(t, text, "EndFragment")
		if eh != len(text) {
			t.Errorf("EndHTML %d, length %d", eh, len(text))
		}
		if !bytes.HasPrefix(text[sh:], []byte("<html>")) {
			t.Errorf("StartHTML %d points to %q", sh, text[sh:])
		}
		if got := string(text[sf:ef]); got != fragment {
			t.Errorf("fragment %q", got)
		}
		if !bytes.HasSuffix(text[:sf], []byte(startFragmentComment)) || !bytes.HasPrefix(text[ef:], []byte(endFragmentComment)) {
			t.Error("fragment is not between the comments")
		}
		if src != "" && !bytes.Contains(text[:sh], []byte("SourceURL:"+src+"\r\n")) {
			t.Error("no SourceURL")
		}

		h, err := DecodeHTML(append(data, 0xAA, 0xBB))
		if err != nil {
			t.Fatal(err)
		}
		if h.Fragment != fragment || h.SourceURL != src || h.Document != string(text[sh:eh]) {
			t.Errorf("decoded %+v", h)
		}
	}
}

func TestHTMLErrors(t *testing.T) {
	if _, err := EncodeHTML("a\x00b", ""); err == nil {
		t.Error("NUL accepted")
	}
	if _, err := EncodeHTML("a", "http://x\r\nEvil:1"); err == nil {
		t.Error("line break in the source URL accepted")
	}
	if _, err := DecodeHTML([]byte("<html>no header</html>")); err == nil {
		t.Error("no header accepted")
	}
}

// TestDecodeHTMLForeign decodes the format as other programs write it:
// Version 1.0, -1 for a missing StartHTML, LF line ends and wrong fragment
// offsets.
func TestDecodeHTMLForeign(t *testing.T) {
	doc := "<html><body><!--StartFragment--><i>x</i><!--EndFragment--></body></html>"
	header := "Version:1.0\nStartHTML:-1\nEndHTML:-1\nStartFragment:999999\nEndFragment:999999\n"
	h, err := DecodeHTML([]byte(header + doc))
	if err != nil {
		t.Fatal(err)
	}
	if h.Fragment != "<i>x</i>" || h.Document != doc {
		t.Errorf("decoded %+v", h)
	}

	// Offsets with fewer digits, as some programs write them.
	body := "<html><body><!--StartFragment-->ab<!--EndFragment--></body></html>"
	tmpl := "Version:0.9\r\nStartHTML:%d\r\nEndHTML:%d\r\nStartFragment:%d\r\nEndFragment:%d\r\n"
	n := 0
	var s string
	for range 3 { // until the header length is stable
		s = fmt.Sprintf(tmpl, n, n+len(body), n+32, n+34)
		n = len(s)
	}
	s += body
	h, err = DecodeHTML([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	if h.Fragment != "ab" || h.Document != body {
		t.Errorf("decoded %+v", h)
	}
}

func TestFileList(t *testing.T) {
	paths := []string{`C:\temp\a.txt`, `C:\Používatelia\ľščť\日本.txt`}
	data, err := EncodeFileList(paths)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(data) != 20 || binary.LittleEndian.Uint32(data[16:]) != 1 {
		t.Error("wrong DROPFILES header")
	}
	if !bytes.HasSuffix(data, []byte{0, 0, 0, 0}) {
		t.Error("list not terminated with two NULs")
	}
	got, err := DecodeFileList(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != strings.Join(paths, "|") {
		t.Errorf("got %q", got)
	}
	if _, err := EncodeFileList(nil); err == nil {
		t.Error("empty list accepted")
	}
	if _, err := EncodeFileList([]string{""}); err == nil {
		t.Error("empty path accepted")
	}

	ansi := append(make([]byte, 20), "C:\\a\x00C:\\b\x00\x00"...)
	binary.LittleEndian.PutUint32(ansi, 20)
	got, err = DecodeFileList(ansi)
	if err != nil || strings.Join(got, "|") != `C:\a|C:\b` {
		t.Errorf("ANSI list: %q, %v", got, err)
	}
}

func testImage() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(10, 20, 15, 23)) // 5 x 3, not at the origin
	for y := range 3 {
		for x := range 5 {
			im.SetNRGBA(10+x, 20+y, color.NRGBA{uint8(40 * x), uint8(80 * y), uint8(10*x + y), 255})
		}
	}
	im.SetNRGBA(10, 20, color.NRGBA{255, 0, 0, 128})
	im.SetNRGBA(11, 20, color.NRGBA{0, 0, 0, 0})
	return im
}

func TestDIBV5RoundTrip(t *testing.T) {
	src := testImage()
	data, err := EncodeDIBV5(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 124+5*3*4 {
		t.Fatalf("length %d", len(data))
	}
	got, err := DecodeDIB(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 5 || got.Bounds().Dy() != 3 {
		t.Fatalf("size %v", got.Bounds())
	}
	for y := range 3 {
		for x := range 5 {
			if a, b := src.NRGBAAt(10+x, 20+y), got.NRGBAAt(x, y); a != b {
				t.Errorf("pixel %d,%d: %v, want %v", x, y, b, a)
			}
		}
	}
}

func TestDIBRoundTrip(t *testing.T) {
	src := testImage()
	data, err := EncodeDIB(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 40+16*3 { // rows of 15 bytes padded to 16
		t.Fatalf("length %d", len(data))
	}
	got, err := DecodeDIB(data)
	if err != nil {
		t.Fatal(err)
	}
	for y := range 3 {
		for x := range 5 {
			want := src.NRGBAAt(10+x, 20+y)
			switch {
			case x == 0 && y == 0:
				want = color.NRGBA{255, 127, 127, 255} // half red on white
			case x == 1 && y == 0:
				want = color.NRGBA{255, 255, 255, 255} // transparent: white
			}
			if b := got.NRGBAAt(x, y); b != want {
				t.Errorf("pixel %d,%d: %v, want %v", x, y, b, want)
			}
		}
	}
}

func dibHeader(w, h int32, bpp uint16, compression uint32, clrUsed uint32) []byte {
	b := make([]byte, 40)
	binary.LittleEndian.PutUint32(b, 40)
	binary.LittleEndian.PutUint32(b[4:], uint32(w))
	binary.LittleEndian.PutUint32(b[8:], uint32(h))
	binary.LittleEndian.PutUint16(b[12:], 1)
	binary.LittleEndian.PutUint16(b[14:], bpp)
	binary.LittleEndian.PutUint32(b[16:], compression)
	binary.LittleEndian.PutUint32(b[32:], clrUsed)
	return b
}

func TestDecodeDIBVariants(t *testing.T) {
	// 32-bit BI_RGB, top-down, alpha all zero: opaque.
	d := dibHeader(2, -1, 32, 0, 0)
	d = append(d, 1, 2, 3, 0, 4, 5, 6, 0)
	im, err := DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	if im.NRGBAAt(0, 0) != (color.NRGBA{3, 2, 1, 255}) || im.NRGBAAt(1, 0) != (color.NRGBA{6, 5, 4, 255}) {
		t.Errorf("32-bit: %v", im.Pix)
	}

	// 16-bit BI_BITFIELDS 565 with the masks after BITMAPINFOHEADER, bottom-up.
	d = dibHeader(1, 2, 16, 3, 0)
	d = binary.LittleEndian.AppendUint32(d, 0xF800)
	d = binary.LittleEndian.AppendUint32(d, 0x07E0)
	d = binary.LittleEndian.AppendUint32(d, 0x001F)
	d = append(d, 0x1F, 0x00, 0, 0) // bottom row: blue
	d = append(d, 0x00, 0xF8, 0, 0) // top row: red
	im, err = DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	if im.NRGBAAt(0, 0) != (color.NRGBA{255, 0, 0, 255}) || im.NRGBAAt(0, 1) != (color.NRGBA{0, 0, 255, 255}) {
		t.Errorf("16-bit: %v", im.Pix)
	}

	// 1-bit with a two color palette.
	d = dibHeader(9, 1, 1, 0, 2)
	d = append(d, 0, 0, 0, 0, 255, 255, 255, 0)
	d = append(d, 0b10000000, 0b10000000, 0, 0)
	im, err = DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	for x := range 9 {
		want := uint8(0)
		if x == 0 || x == 8 {
			want = 255
		}
		if im.NRGBAAt(x, 0).R != want {
			t.Errorf("1-bit pixel %d: %v", x, im.NRGBAAt(x, 0))
		}
	}

	// 8-bit with 3 palette entries.
	d = dibHeader(3, 1, 8, 0, 3)
	d = append(d, 0, 0, 255, 0, 0, 255, 0, 0, 255, 0, 0, 0)
	d = append(d, 2, 1, 0, 0)
	im, err = DecodeDIB(d)
	if err != nil {
		t.Fatal(err)
	}
	if im.NRGBAAt(0, 0) != (color.NRGBA{0, 0, 255, 255}) || im.NRGBAAt(2, 0) != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("8-bit: %v", im.Pix)
	}

	for name, d := range map[string][]byte{
		"short":      {40, 0},
		"truncated":  dibHeader(100, 100, 24, 0, 0),
		"rle":        append(dibHeader(1, 1, 8, 1, 0), make([]byte, 1024)...),
		"zero width": append(dibHeader(0, 1, 24, 0, 0), 0, 0, 0, 0),
		"huge":       dibHeader(1<<20, 1<<20, 24, 0, 0),
		"overflow":   dibHeader(1<<16+1, 1<<16+1, 32, 0, 0),
		"wide":       dibHeader(1<<30, 1, 32, 0, 0),
		"min height": dibHeader(1, -1<<31, 32, 0, 0),
		"colors":     append(dibHeader(1, 1, 8, 0, 0xFFFFFFFF), make([]byte, 64)...),
	} {
		if _, err := DecodeDIB(d); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestUTF16(t *testing.T) {
	b, err := UTF16("aľ😀")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{'a', 0, 0x3E, 0x01, 0x3D, 0xD8, 0x00, 0xDE, 0, 0}
	if !bytes.Equal(b, want) {
		t.Errorf("%x", b)
	}
	if _, err := UTF16("a\x00"); err == nil {
		t.Error("NUL accepted")
	}
	if String([]byte("{\\rtf1 x}\x00junk")) != "{\\rtf1 x}" {
		t.Error("String")
	}
}

func TestDIBV5ExtraMasks(t *testing.T) {
	src := testImage()
	data, err := EncodeDIBV5(src)
	if err != nil {
		t.Fatal(err)
	}
	// The masks once more after the header, as some programs write them.
	extra := append(append(bytes.Clone(data[:124]), data[40:52]...), data[124:]...)
	for name, d := range map[string][]byte{"plain": data, "extra masks": extra} {
		got, err := DecodeDIB(d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for y := range 3 {
			for x := range 5 {
				if a, b := src.NRGBAAt(10+x, 20+y), got.NRGBAAt(x, y); a != b {
					t.Errorf("%s: pixel %d,%d: %v, want %v", name, x, y, b, a)
				}
			}
		}
	}
}

func TestEncodeLargeImage(t *testing.T) {
	// A huge image is refused before any pixel is read; on 386 the size
	// would overflow int.
	im := image.NewNRGBA(image.Rect(0, 0, 0, 0))
	im.Rect = image.Rect(0, 0, 1<<16+1, 1<<16+1)
	if _, err := EncodeDIBV5(im); err == nil {
		t.Error("huge image accepted")
	}
}

func TestFastPaths(t *testing.T) {
	// RGBA (premultiplied) and NRGBA must give the same pixels as the
	// generic conversion.
	rgba := image.NewRGBA(image.Rect(3, 4, 259, 260))
	nrgba := image.NewNRGBA(rgba.Rect)
	for y := range 256 {
		for x := range 256 {
			a := uint8(y)
			v := uint8(x)
			if v > a {
				v = a // valid premultiplied color
			}
			rgba.SetRGBA(3+x, 4+y, color.RGBA{v, v / 2, a - v, a})
			nrgba.SetNRGBA(3+x, 4+y, color.NRGBA{uint8(x), uint8(y), uint8(x ^ y), uint8(x + y)})
		}
	}
	for _, im := range []image.Image{rgba, nrgba} {
		_, _, pix, err := nrgbaPixels(im)
		if err != nil {
			t.Fatal(err)
		}
		i := 0
		for y := im.Bounds().Min.Y; y < im.Bounds().Max.Y; y++ {
			for x := im.Bounds().Min.X; x < im.Bounds().Max.X; x++ {
				if want := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA); pix[i] != want {
					t.Fatalf("%T pixel %d,%d: %v, want %v", im, x, y, pix[i], want)
				}
				i++
			}
		}
	}
}
