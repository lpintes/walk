// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/microsoft/go-winmd/flags"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseSpec(t *testing.T) {
	path := writeFile(t, t.TempDir(), "winmd.txt", `
# comment
metadata 1.2.3 sha256:abcd
dll USER32.dll libuser32
const WS_CHILD
const LOCALE_USER_DEFAULT LCID # typed
func GetMessage rawbool
func GetLocaleInfo Locale:LCID result=int32 entry=GetLocaleInfoW fallback=GetLocaleInfoA
struct MSG hwnd=HWnd lParam:LPARAM
struct NMHDR2 entry=NMHDR
guid IID_IUnknown
guid PROPID_ACC_NAME MSAAPROPID
interface IUnknown
interface IFoo entry=IBar get_Name=GetName
`)
	s, err := parseSpec(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.metaVersion != "1.2.3" || s.metaSHA256 != "abcd" {
		t.Errorf("metadata = %q %q", s.metaVersion, s.metaSHA256)
	}
	if len(s.dlls) != 1 || *s.dlls[0] != (dllSpec{"user32.dll", "libuser32"}) {
		t.Errorf("dlls = %+v", s.dlls)
	}
	if len(s.constants) != 2 || s.constants[0].goType != "" || s.constants[1].goType != "LCID" {
		t.Errorf("constants = %+v %+v", s.constants[0], s.constants[1])
	}
	if len(s.functions) != 2 {
		t.Fatalf("functions = %+v", s.functions)
	}
	if f := s.functions[0]; !f.rawBool {
		t.Errorf("GetMessage = %+v", f)
	}
	if f := s.functions[1]; f.params["Locale"] != "LCID" || f.result != "int32" || f.entry != "GetLocaleInfoW" || f.fallback != "GetLocaleInfoA" {
		t.Errorf("GetLocaleInfo = %+v", f)
	}
	if len(s.structs) != 2 {
		t.Fatalf("structs = %+v", s.structs)
	}
	if st := s.structs[0]; st.names["hwnd"] != "HWnd" || st.types["lParam"] != "LPARAM" || len(st.names) != 1 || len(st.types) != 1 {
		t.Errorf("MSG = %+v", st)
	}
	if got := s.structs[0].metaNames(); strings.Join(got, " ") != "MSGW MSG" {
		t.Errorf("MSG metadata names = %q", got)
	}
	if got := s.structs[1].metaNames(); strings.Join(got, " ") != "NMHDR" {
		t.Errorf("NMHDR2 metadata names = %q", got)
	}
	if len(s.guids) != 2 || s.guids[0].guidType() != "IID" || s.guids[1].guidType() != "MSAAPROPID" {
		t.Errorf("guids = %+v", s.guids)
	}
	if len(s.interfaces) != 2 {
		t.Fatalf("interfaces = %+v", s.interfaces)
	}
	if is := s.interfaces[0]; is.metaName() != "IUnknown" || len(is.names) != 0 {
		t.Errorf("IUnknown = %+v", is)
	}
	if is := s.interfaces[1]; is.metaName() != "IBar" || is.names["get_Name"] != "GetName" || len(is.names) != 1 {
		t.Errorf("IFoo = %+v", is)
	}
}

func TestGUID(t *testing.T) {
	g := [16]byte{0x61, 0xf9, 0x56, 0x88, 0x0a, 0x34, 0xd0, 0x11, 0xa9, 0x6b, 0x00, 0xc0, 0x4f, 0xd7, 0x05, 0xa2}
	want := "CLSID{0x8856f961, 0x340a, 0x11d0, [8]byte{0xa9, 0x6b, 0x00, 0xc0, 0x4f, 0xd7, 0x05, 0xa2}}"
	if got := goGUID("CLSID", &g); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	for _, tc := range []struct{ name, typ string }{
		{"IID_IUnknown", "IID"},
		{"DIID_DWebBrowserEvents2", "IID"},
		{"CLSID_WebBrowser", "CLSID"},
		{"FOLDERID_Desktop", "syscall.GUID"},
	} {
		if got := (&guidSpec{name: tc.name}).guidType(); got != tc.typ {
			t.Errorf("type of %s = %s, want %s", tc.name, got, tc.typ)
		}
	}
}

func TestCOM(t *testing.T) {
	skipWithoutMetadata(t)
	_, m, err := load(filepath.Join(winDir, "winmd.txt"), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, guid string }{
		// Interface.
		{"IID_IUnknown", "IID{0x00000000, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}"},
		// Coclass.
		{"CLSID_WebBrowser", "IID{0x8856f961, 0x340a, 0x11d0, [8]byte{0xa9, 0x6b, 0x00, 0xc0, 0x4f, 0xd7, 0x05, 0xa2}}"},
		// Constant.
		{"CLSID_AccPropServices", "IID{0xb5f8350b, 0x0548, 0x48b1, [8]byte{0xa6, 0xee, 0x88, 0xbd, 0x00, 0xb4, 0xa5, 0xe7}}"},
	} {
		g, _, err := m.lookupGUID(tc.name)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got := goGUID("IID", g); got != tc.guid {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.guid)
		}
	}
	if _, _, err := m.lookupGUID("IID_NoSuchInterface"); err == nil {
		t.Error("IID_NoSuchInterface: no error")
	}

	td, err := m.lookupInterface("IOleInPlaceSite")
	if err != nil {
		t.Fatal(err)
	}
	methods, err := m.vtable(td)
	if err != nil {
		t.Fatal(err)
	}
	// IUnknown, then IOleWindow, then IOleInPlaceSite.
	want := "QueryInterface AddRef Release GetWindow ContextSensitiveHelp CanInPlaceActivate OnInPlaceActivate " +
		"OnUIActivate GetWindowContext Scroll OnUIDeactivate OnInPlaceDeactivate DiscardUndoState " +
		"DeactivateAndUndo OnPosRectChange"
	if got := strings.Join(methods, " "); got != want {
		t.Errorf("IOleInPlaceSite vtable:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseSpecErrors(t *testing.T) {
	for _, tc := range []struct{ spec, err string }{
		{"const A\n", "missing metadata"},
		{"metadata 1 sha256:x\nconst A\nconst A\n", "already declared on line 2"},
		{"metadata 1 sha256:x\nfunc F bogus\n", "unknown option"},
		{"metadata 1 sha256:x\ntype T\n", "unknown directive"},
		{"metadata 1\n", "want: metadata"},
		{"metadata 1 sha256:x\nguid A B C\n", "want: guid"},
		{"metadata 1 sha256:x\ninterface I x:y\n", "unknown option"},
	} {
		path := writeFile(t, t.TempDir(), "winmd.txt", tc.spec)
		_, err := parseSpec(path)
		if err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("%q: got error %v, want %q", tc.spec, err, tc.err)
		}
	}
}

func TestGoValue(t *testing.T) {
	for _, tc := range []struct {
		c    constant
		want string
	}{
		{constant{kind: flags.ElementType_I4, ival: -4}, "-4"},
		{constant{kind: flags.ElementType_I4, ival: 7}, "7"},
		{constant{kind: flags.ElementType_I4, ival: 1024}, "0x400"},
		{constant{kind: flags.ElementType_U4, uval: 0x40000000}, "0x40000000"},
		{constant{kind: flags.ElementType_U2, uval: 0}, "0"},
		{constant{kind: flags.ElementType_STRING, sval: "SysListView32"}, `"SysListView32"`},
		{constant{kind: flags.ElementType_R4, fval: 0.5}, "0.5"},
	} {
		got, err := tc.c.goValue()
		if err != nil || got != tc.want {
			t.Errorf("%+v: got %q, %v; want %q", tc.c, got, err, tc.want)
		}
	}
	if _, err := (&constant{name: "IID_X", guid: new([16]byte)}).goValue(); err == nil {
		t.Error("GUID constant: no error")
	}
}

func TestShape(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package win

import "unsafe"

type (
	HANDLE  uintptr
	HWND    HANDLE
	BOOL    int32
	REFIID  *IID
	IID     struct{}
	PVOID   unsafe.Pointer
)
`)
	writeFile(t, dir, outputPrefix+"skipped.go", "package win\n\ntype SKIPPED int\n")
	p, err := parseGoPackage(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.hasType("SKIPPED") {
		t.Error("generated file was parsed")
	}
	for _, tc := range []struct {
		expr  string
		shape shape
		basic string
	}{
		{"HWND", shapeScalar, "uintptr"},
		{"BOOL", shapeScalar, "int32"},
		{"REFIID", shapePointer, ""},
		{"PVOID", shapePointer, ""},
		{"*RECT", shapePointer, ""},
		{"unsafe.Pointer", shapePointer, ""},
		{"bool", shapeBool, "bool"},
		{"IID", shapeOther, ""},
		{"float32", shapeFloat, "float32"},
		{"int", shapeScalar, "int"},
	} {
		if got := p.shapeOf(tc.expr); got != tc.shape {
			t.Errorf("shapeOf(%s) = %v, want %v", tc.expr, got, tc.shape)
		}
		if got := p.basic(tc.expr); got != tc.basic {
			t.Errorf("basic(%s) = %q, want %q", tc.expr, got, tc.basic)
		}
	}
}

func TestWriteFunc(t *testing.T) {
	f := &genFunc{
		spec:   &funcSpec{name: "EnableWindow"},
		method: &method{entry: "EnableWindow"},
		proc:   "procEnableWindow",
		params: []param{
			{name: "hWnd", typ: goType{name: "HWND"}},
			{name: "bEnable", typ: goType{name: "bool"}},
			{name: "lpRect", typ: goType{name: "*RECT"}},
			{name: "pv", typ: goType{name: "unsafe.Pointer"}},
			{name: "lParam", typ: goType{name: "uintptr"}},
			{name: "ftn", typ: goType{name: "BLENDFUNCTION"}},
		},
		shapes:  []shape{shapeScalar, shapeBool, shapePointer, shapePointer, shapeScalar, shapeOther},
		byValue: []int{0, 0, 0, 0, 0, 4},
		result:  "bool",
		rshape:  shapeBool,
	}
	var b bytes.Buffer
	(&generator{}).writeFunc(&b, f)
	want := `func EnableWindow(hWnd HWND, bEnable bool, lpRect *RECT, pv unsafe.Pointer, lParam uintptr, ftn BLENDFUNCTION) bool {
	r1, _, _ := syscall.SyscallN(procEnableWindow.Addr(), uintptr(hWnd), uintptr(BoolToBOOL(bEnable)), uintptr(unsafe.Pointer(lpRect)), uintptr(pv), lParam, uintptr(*(*uint32)(unsafe.Pointer(&ftn))))
	return r1 != 0
}

`
	if got := b.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestGenerated checks that the generated files in internal/win are up to
// date. It needs the metadata package in the cache directory, which go
// generate downloads.
const winDir = "../../internal/win"

// skipWithoutMetadata skips a test if the metadata package of the
// specification of internal/win has not been downloaded.
func skipWithoutMetadata(t *testing.T) *spec {
	t.Helper()
	s, err := parseSpec(filepath.Join(winDir, "winmd.txt"))
	if err != nil {
		t.Fatal(err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	nupkg := filepath.Join(cache, "walk-winmdgen", strings.ToLower(nugetPackage+"."+s.metaVersion)+".nupkg")
	if _, err := os.Stat(nupkg); err != nil {
		t.Skipf("metadata not downloaded: %v", err)
	}
	return s
}

func TestStructLayout(t *testing.T) {
	skipWithoutMetadata(t)
	_, m, err := load(filepath.Join(winDir, "winmd.txt"), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		arch    arch
		rules   layoutRules
		size    int
		align   int
		offsets []int
	}{
		{"MSG", archAMD64, cRules, 48, 8, []int{0, 8, 16, 24, 32, 36}},
		{"MSG", arch386, cRules, 28, 4, []int{0, 4, 8, 12, 16, 20}},
		{"NMHDR", archARM64, cRules, 24, 8, []int{0, 8, 16}},
		// Declared with #pragma pack(1).
		{"NMTVKEYDOWN", arch386, cRules, 18, 1, []int{0, 12, 14}},
		{"NMTVKEYDOWN", arch386, goRules, 20, 4, []int{0, 12, 16}},
		{"NMTVKEYDOWN", archAMD64, cRules, 30, 1, []int{0, 24, 26}},
		// 64-bit fields are aligned to 8 bytes in C but to 4 bytes in Go
		// on 386.
		{"MEMORYSTATUSEX", arch386, cRules, 64, 8, []int{0, 4, 8, 16, 24, 32, 40, 48, 56}},
		{"MEMORYSTATUSEX", arch386, goRules, 64, 4, []int{0, 4, 8, 16, 24, 32, 40, 48, 56}},
	} {
		td, err := m.lookupStruct([]string{tc.name + "W", tc.name}, tc.arch)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		l, err := m.structLayout(td, tc.arch, tc.rules)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if l.size != tc.size || l.align != tc.align || fmt.Sprint(l.offsets) != fmt.Sprint(tc.offsets) {
			t.Errorf("%s on %v with rules %d: got size %d, align %d, offsets %v; want %d, %d, %v",
				tc.name, tc.arch, tc.rules, l.size, l.align, l.offsets, tc.size, tc.align, tc.offsets)
		}
	}
}

func TestGenerated(t *testing.T) {
	skipWithoutMetadata(t)

	out := t.TempDir()
	// The generator reads the hand-written types from the output
	// directory.
	files, err := filepath.Glob(filepath.Join(winDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasPrefix(filepath.Base(f), outputPrefix) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, out, filepath.Base(f), string(data))
	}
	if err := run(filepath.Join(winDir, "winmd.txt"), "", out); err != nil {
		t.Fatal(err)
	}
	generated, _ := filepath.Glob(filepath.Join(out, outputPrefix+"*.go"))
	committed, _ := filepath.Glob(filepath.Join(winDir, outputPrefix+"*.go"))
	if len(generated) != len(committed) {
		t.Errorf("generated %d files, %d committed", len(generated), len(committed))
	}
	for _, g := range generated {
		got, _ := os.ReadFile(g)
		want, err := os.ReadFile(filepath.Join(winDir, filepath.Base(g)))
		if err != nil {
			t.Error(err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date; run go generate ./internal/win", filepath.Base(g))
		}
	}
}
