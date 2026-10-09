// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
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
func GetLocaleInfo Locale:LCID result=int32 entry=GetLocaleInfoW
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
	if f := s.functions[1]; f.params["Locale"] != "LCID" || f.result != "int32" || f.entry != "GetLocaleInfoW" {
		t.Errorf("GetLocaleInfo = %+v", f)
	}
}

func TestParseSpecErrors(t *testing.T) {
	for _, tc := range []struct{ spec, err string }{
		{"const A\n", "missing metadata"},
		{"metadata 1 sha256:x\nconst A\nconst A\n", "already declared on line 2"},
		{"metadata 1 sha256:x\nfunc F bogus\n", "unknown option"},
		{"metadata 1 sha256:x\ntype T\n", "unknown directive"},
		{"metadata 1\n", "want: metadata"},
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
		params: []param{
			{"hWnd", goType{name: "HWND"}},
			{"bEnable", goType{name: "bool"}},
			{"lpRect", goType{name: "*RECT"}},
			{"pv", goType{name: "unsafe.Pointer"}},
			{"lParam", goType{name: "uintptr"}},
		},
		shapes: []shape{shapeScalar, shapeBool, shapePointer, shapePointer, shapeScalar},
		result: "bool",
		rshape: shapeBool,
	}
	var b bytes.Buffer
	(&generator{}).writeFunc(&b, f)
	want := `func EnableWindow(hWnd HWND, bEnable bool, lpRect *RECT, pv unsafe.Pointer, lParam uintptr) bool {
	r1, _, _ := syscall.SyscallN(procEnableWindow.Addr(), uintptr(hWnd), uintptr(BoolToBOOL(bEnable)), uintptr(unsafe.Pointer(lpRect)), uintptr(pv), lParam)
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
func TestGenerated(t *testing.T) {
	const winDir = "../../internal/win"
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
