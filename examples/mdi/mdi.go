// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This example demonstrates a multiple-document interface (MDI): a frame
// window with document windows inside it, a Window menu that lists them,
// and the standard MDI keyboard: Ctrl+F6 and Ctrl+Shift+F6 switch
// documents, Ctrl+F4 closes the active one, Alt+Hyphen opens its system
// menu. A document with changes asks before it is closed.
package main

import (
	"fmt"
	"log"

	"github.com/lpintes/walk"
	. "github.com/lpintes/walk/declarative"
)

type document struct {
	child *walk.MdiChildWindow
	text  *walk.TextEdit
	dirty bool
}

var (
	frame     *walk.MdiMainWindow
	status    *walk.StatusBarItem
	documents int
)

func newDocument() {
	documents++
	name := fmt.Sprintf("Document %d", documents)
	doc := new(document)

	if err := (MdiChildWindow{
		AssignTo: &doc.child,
		Title:    name,
		Size:     Size{Width: 400, Height: 250},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: "&Text:"},
			TextEdit{
				AssignTo:      &doc.text,
				Accessibility: Accessibility{Name: "Text"},
				VScroll:       true,
				OnTextChanged: func() {
					if !doc.dirty {
						doc.dirty = true
						doc.child.SetTitle(name + " (changed)")
					}
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "&Clear", OnClicked: func() { doc.text.SetText("") }},
				},
			},
		},
		OnClosing: func(canceled *bool, reason walk.CloseReason) {
			if !doc.dirty {
				return
			}
			if walk.MsgBox(frame, "Close", name+" has changes. Close it anyway?", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
				*canceled = true
			}
		},
	}).Create(frame); err != nil {
		log.Print(err)
	}
}

func main() {
	var windowMenu *walk.Menu

	if err := (MdiMainWindow{
		AssignTo:   &frame,
		Title:      "Walk MDI Example",
		Size:       Size{Width: 800, Height: 600},
		WindowMenu: &windowMenu,
		MenuItems: []MenuItem{
			Menu{
				Text: "&File",
				Items: []MenuItem{
					Action{
						Text:        "&New",
						Shortcut:    Shortcut{Modifiers: walk.ModControl, Key: walk.KeyN},
						OnTriggered: newDocument,
					},
					Action{
						Text: "&Close",
						OnTriggered: func() {
							if c := frame.ActiveMdiChild(); c != nil {
								c.Close()
							}
						},
					},
					Separator{},
					Action{
						Text:        "E&xit",
						OnTriggered: func() { frame.Close() },
					},
				},
			},
			Menu{
				AssignTo: &windowMenu,
				Text:     "&Window",
				Items: []MenuItem{
					Action{Text: "&Cascade", OnTriggered: func() { frame.Cascade() }},
					Action{Text: "Tile &Horizontally", OnTriggered: func() { frame.TileHorizontally() }},
					Action{Text: "Tile &Vertically", OnTriggered: func() { frame.TileVertically() }},
					Action{Text: "&Arrange Icons", OnTriggered: func() { frame.ArrangeIcons() }},
					Separator{},
					Action{Text: "Close A&ll", OnTriggered: func() { frame.CloseAllMdiChildren() }},
				},
			},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &status, Text: "No document", Width: 300},
		},
		OnActiveMdiChildChanged: func() {
			if c := frame.ActiveMdiChild(); c != nil {
				status.SetText("Active: " + c.Title())
			} else {
				status.SetText("No document")
			}
		},
	}).Create(); err != nil {
		log.Fatal(err)
	}

	for range 3 {
		newDocument()
	}

	frame.Run()
}
