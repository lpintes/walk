// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"errors"
	"syscall"
	"testing"

	"github.com/lpintes/walk/internal/win"
)

func TestErrorFromHRESULTWrapsCode(t *testing.T) {
	err := errorFromHRESULT("Test", win.HRESULT(-2147024891)) // E_ACCESSDENIED, 0x80070005

	if !errors.Is(err, syscall.Errno(0x80070005)) {
		t.Fatalf("errors.Is did not find the HRESULT in %q", err.(*Error).Message())
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) || errno != 0x80070005 {
		t.Fatalf("errors.As: got %#x", uint32(errno))
	}
	if msg := err.(*Error).Message(); msg != "Test: Error -2147024891" {
		t.Fatalf("message changed: %q", msg)
	}
}

func TestNewErrWithCode(t *testing.T) {
	err := newErrWithCode("Test", syscall.Errno(5))
	if !errors.Is(err, syscall.Errno(5)) {
		t.Fatal("errors.Is did not find the code")
	}
	if inner := err.(*Error).Inner(); inner != syscall.Errno(5) {
		t.Fatalf("Inner: got %v", inner)
	}
	if err := newErrWithCode("Test", 0); err.(*Error).Inner() != nil {
		t.Fatal("code 0 was wrapped")
	}
}

func TestCodeErrorThroughWrapError(t *testing.T) {
	err := wrapErr(&codeError{message: "Test: HRESULT 0x8007000e", code: syscall.Errno(0x8007000E)})
	if !errors.Is(err, syscall.Errno(0x8007000E)) {
		t.Fatal("errors.Is did not find the code")
	}
	if msg := err.(*Error).Message(); msg != "Test: HRESULT 0x8007000e" {
		t.Fatalf("message changed: %q", msg)
	}
}
