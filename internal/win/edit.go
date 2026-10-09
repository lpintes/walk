// Copyright 2010 The win Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package win

// Edit styles
const (
	ES_PASSWORD    = 0x0020
	ES_AUTOVSCROLL = 0x0040
	ES_NOHIDESEL   = 0x0100
	ES_OEMCONVERT  = 0x0400
	ES_NUMBER      = 0x2000
)

// Edit notifications
const (
	EN_SETFOCUS     = 0x0100
	EN_KILLFOCUS    = 0x0200
	EN_UPDATE       = 0x0400
	EN_ERRSPACE     = 0x0500
	EN_MAXTEXT      = 0x0501
	EN_HSCROLL      = 0x0601
	EN_VSCROLL      = 0x0602
	EN_ALIGN_LTR_EC = 0x0700
	EN_ALIGN_RTL_EC = 0x0701
)

// Edit messages
const (
	EM_SETRECT             = 0x00B3
	EM_SETRECTNP           = 0x00B4
	EM_SCROLL              = 0x00B5
	EM_LINESCROLL          = 0x00B6
	EM_GETMODIFY           = 0x00B8
	EM_SETMODIFY           = 0x00B9
	EM_LINEINDEX           = 0x00BB
	EM_SETHANDLE           = 0x00BC
	EM_GETHANDLE           = 0x00BD
	EM_GETTHUMB            = 0x00BE
	EM_LINELENGTH          = 0x00C1
	EM_GETLINE             = 0x00C4
	EM_CANUNDO             = 0x00C6
	EM_UNDO                = 0x00C7
	EM_FMTLINES            = 0x00C8
	EM_LINEFROMCHAR        = 0x00C9
	EM_SETTABSTOPS         = 0x00CB
	EM_EMPTYUNDOBUFFER     = 0x00CD
	EM_GETFIRSTVISIBLELINE = 0x00CE
	EM_SETMARGINS          = 0x00D3
	EM_GETMARGINS          = 0x00D4
	EM_SETIMESTATUS        = 0x00D8
	EM_GETIMESTATUS        = 0x00D9
	EM_SETCARETINDEX       = 0x1511
)
