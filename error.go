// Copyright 2011 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package walk

import (
	"fmt"
	"log"
	"runtime/debug"
	"syscall"
)

import (
	"github.com/lpintes/walk/internal/win"
)

var (
	logErrors    bool
	panicOnError bool
)

// Error is the error type of walk. When a Windows function fails, the error
// wraps the Windows error code as a syscall.Errno: a Win32 error code such as
// ERROR_ACCESS_DENIED (5), or the 32-bit value of an HRESULT such as
// E_OUTOFMEMORY (0x8007000E). Test for a code with errors.Is, for example
// errors.Is(err, syscall.Errno(5)), or get it with errors.As.
type Error struct {
	inner   error
	message string
	stack   []byte
}

// Inner returns the wrapped error: the error that caused this one, or the
// Windows error code as a syscall.Errno.
func (err *Error) Inner() error {
	return err.inner
}

// Unwrap returns the wrapped error, so that errors.Is and errors.As look
// through walk errors.
func (err *Error) Unwrap() error {
	return err.inner
}

func (err *Error) Message() string {
	if err.message != "" {
		return err.message
	}

	if err.inner != nil {
		if walkErr, ok := err.inner.(*Error); ok {
			return walkErr.Message()
		} else {
			return err.inner.Error()
		}
	}

	return ""
}

func (err *Error) Stack() []byte {
	return err.stack
}

func (err *Error) Error() string {
	return fmt.Sprintf("%s\n\nStack:\n%s", err.Message(), err.stack)
}

func processErrorNoPanic(err error) error {
	if logErrors {
		if walkErr, ok := err.(*Error); ok {
			log.Print(walkErr.Error())
		} else {
			log.Printf("%s\n\nStack:\n%s", err, debug.Stack())
		}
	}

	return err
}

func processError(err error) error {
	processErrorNoPanic(err)

	if panicOnError {
		panic(err)
	}

	return err
}

func newErr(message string) error {
	return &Error{message: message, stack: debug.Stack()}
}

// newErrWithCode returns an *Error with message whose inner error is the
// Windows error code, so that errors.Is and errors.As find the code.
// A code of 0 is not wrapped.
func newErrWithCode(message string, code syscall.Errno) error {
	err := &Error{message: message, stack: debug.Stack()}
	if code != 0 {
		err.inner = code
	}
	return err
}

func newError(message string) error {
	return processError(newErr(message))
}

func newErrorNoPanic(message string) error {
	return processErrorNoPanic(newErr(message))
}

// lastError returns an error for a failed Win32 function. If GetLastError
// reports an error, the error wraps it as a syscall.Errno.
func lastError(win32FuncName string) error {
	if errno := win.GetLastError(); errno != win.ERROR_SUCCESS {
		return processError(newErrWithCode(fmt.Sprintf("%s: Error %d", win32FuncName, errno), syscall.Errno(errno)))
	}

	return newError(win32FuncName)
}

// errorFromHRESULT returns an error for a failed function returning an
// HRESULT. The error wraps the HRESULT as a syscall.Errno holding its 32-bit
// value, for example syscall.Errno(0x80070005) for E_ACCESSDENIED.
func errorFromHRESULT(funcName string, hr win.HRESULT) error {
	return processError(newErrWithCode(fmt.Sprintf("%s: Error %d", funcName, hr), hresultErrno(hr)))
}

// hresultErrno converts an HRESULT to the syscall.Errno that errors from
// errorFromHRESULT wrap.
func hresultErrno(hr win.HRESULT) syscall.Errno {
	return syscall.Errno(uint32(hr))
}

// codeError is an error with a fixed text that wraps a Windows error code,
// for errors that are not *Error.
type codeError struct {
	message string
	code    syscall.Errno
}

func (err *codeError) Error() string {
	return err.message
}

func (err *codeError) Unwrap() error {
	return err.code
}

func wrapErr(err error) error {
	if _, ok := err.(*Error); ok {
		return err
	}

	return &Error{inner: err, stack: debug.Stack()}
}

func wrapErrorNoPanic(err error) error {
	return processErrorNoPanic(wrapErr(err))
}

func wrapError(err error) error {
	return processError(wrapErr(err))
}
