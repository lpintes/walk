// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

// Package uia is a client of Microsoft UI Automation: it reads and controls
// the user interface of other programs, the way screen readers and
// automated tests do.
//
// New connects to UI Automation. From the returned Automation, Root,
// ElementFromHandle, ElementFromPoint and FocusedElement return an
// Element, an object in the user interface tree: a window, a button, an
// edit field, a list item and so on. Its methods read the properties
// (Name, ControlType, AutomationID, IsEnabled, BoundingRectangle, ...),
// find other elements below it (FindFirst and FindAll with a Condition)
// and move the keyboard focus to it (SetFocus). A TreeWalker moves through
// the tree from element to element.
//
// The package works in any Go program, with or without walk windows.
//
// # Threads
//
// All calls to UI Automation run on one operating system thread that
// belongs to the Automation, in the multithreaded COM apartment, as
// Microsoft recommends for UI Automation clients. Methods may therefore be
// called from any goroutine, also concurrently.
//
// UI Automation gets the information about a window from the thread that
// created it. A call about a window of the calling program therefore
// waits for that window's thread to process messages. Do not call the
// methods of this package from the goroutine that runs the message loop of
// the windows they ask about, for example from a walk event handler about
// windows of the same program: the call cannot finish until UI Automation
// gives up after a timeout. Start a goroutine instead and use
// walk.Synchronize to bring the results back to the user interface.
//
// # Releasing elements
//
// Every Element holds a reference to a COM object, possibly in another
// process. Release frees it at once; otherwise the garbage collector frees
// it some time after the Element becomes unreachable. Close the Automation
// when it is no longer needed; the elements it returned stop working then.
//
// Methods of an element that no longer exists, for example because its
// window was closed, return an error wrapping ErrElementNotAvailable.
package uia
