// Copyright 2010 The win Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build windows

package win

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// System commands
const (
	SC_SIZE         = 0xF000
	SC_MOVE         = 0xF010
	SC_MINIMIZE     = 0xF020
	SC_MAXIMIZE     = 0xF030
	SC_NEXTWINDOW   = 0xF040
	SC_PREVWINDOW   = 0xF050
	SC_VSCROLL      = 0xF070
	SC_HSCROLL      = 0xF080
	SC_MOUSEMENU    = 0xF090
	SC_KEYMENU      = 0xF100
	SC_ARRANGE      = 0xF110
	SC_RESTORE      = 0xF120
	SC_TASKLIST     = 0xF130
	SC_SCREENSAVE   = 0xF140
	SC_HOTKEY       = 0xF150
	SC_DEFAULT      = 0xF160
	SC_MONITORPOWER = 0xF170
	SC_CONTEXTHELP  = 0xF180
	SC_SEPARATOR    = 0xF00F
)

// Static control styles
const (
	SS_BITMAP          = 14
	SS_BLACKFRAME      = 7
	SS_BLACKRECT       = 4
	SS_CENTERIMAGE     = 512
	SS_EDITCONTROL     = 0x2000
	SS_ENHMETAFILE     = 15
	SS_ETCHEDFRAME     = 18
	SS_ETCHEDVERT      = 17
	SS_GRAYFRAME       = 8
	SS_GRAYRECT        = 5
	SS_ICON            = 3
	SS_LEFTNOWORDWRAP  = 0xc
	SS_OWNERDRAW       = 0xd
	SS_REALSIZECONTROL = 0x040
	SS_REALSIZEIMAGE   = 0x800
	SS_RIGHTJUST       = 0x400
	SS_SIMPLE          = 11
	SS_SUNKEN          = 4096
	SS_WHITEFRAME      = 9
	SS_WHITERECT       = 6
	SS_USERITEM        = 10
	SS_TYPEMASK        = 0x0000001F
	SS_WORDELLIPSIS    = 0x0000C000
	SS_ELLIPSISMASK    = 0x0000C000
)

// Button message constants
const (
	BM_CLICK    = 245
	BM_GETIMAGE = 246
	BM_GETSTATE = 242
	BM_SETSTATE = 243
	BM_SETSTYLE = 244
)

// Button notifications
const (
	BN_PAINT         = 1
	BN_HILITE        = 2
	BN_PUSHED        = BN_HILITE
	BN_UNHILITE      = 3
	BN_UNPUSHED      = BN_UNHILITE
	BN_DISABLE       = 4
	BN_DOUBLECLICKED = 5
	BN_DBLCLK        = BN_DOUBLECLICKED
	BN_SETFOCUS      = 6
	BN_KILLFOCUS     = 7
)

const (
	IMAGE_CURSOR      = 2
	IMAGE_ENHMETAFILE = 3
)

const (
	LR_DEFAULTCOLOR     = 0
	LR_MONOCHROME       = 1
	LR_COLOR            = 2
	LR_COPYRETURNORG    = 4
	LR_COPYDELETEORG    = 8
	LR_LOADFROMFILE     = 16
	LR_LOADTRANSPARENT  = 32
	LR_LOADREALSIZE     = 128
	LR_DEFAULTSIZE      = 0x0040
	LR_VGACOLOR         = 0x0080
	LR_LOADMAP3DCOLORS  = 4096
	LR_COPYFROMRESOURCE = 0x4000
	LR_SHARED           = 32768
)

// Button style constants
const (
	BS_3STATE      = 5
	BS_BOTTOM      = 0x800
	BS_CENTER      = 0x300
	BS_CHECKBOX    = 2
	BS_ICON        = 64
	BS_LEFT        = 256
	BS_MULTILINE   = 0x2000
	BS_NOTIFY      = 0x4000
	BS_OWNERDRAW   = 0xB
	BS_PUSHLIKE    = 4096
	BS_RADIOBUTTON = 4
	BS_RIGHT       = 512
	BS_RIGHTBUTTON = 32
	BS_TEXT        = 0
	BS_USERBUTTON  = 8
	BS_VCENTER     = 0xC00
	BS_FLAT        = 0x8000
)

const (
	PM_NOREMOVE = 0x0000
	PM_REMOVE   = 0x0001
	PM_NOYIELD  = 0x0002
)

// Button state constants
const (
	BST_FOCUS  = 8
	BST_PUSHED = 4
)

// Predefined brushes constants
const (
	COLOR_3DHILIGHT  = 20
	COLOR_BTNHILIGHT = 20
)

// GetAncestor flags
const (
	GA_ROOTOWNER = 3
)

// GetWindowLong and GetWindowLongPtr constants
const (
	GWL_WNDPROC     = -4
	GWL_HINSTANCE   = -6
	GWLP_HINSTANCE  = -6
	GWLP_HWNDPARENT = -8
	GWLP_ID         = -12
	GWL_USERDATA    = -21
)

// Predefined window handles
const (
	HWND_BROADCAST = HWND(0xFFFF)
	HWND_NOTOPMOST = ^HWND(1) // -2
	HWND_TOPMOST   = ^HWND(0) // -1
	HWND_DESKTOP   = HWND(0)
	HWND_MESSAGE   = ^HWND(2) // -3
)

// Predefined icon constants
const (
	IDI_HAND        = 32513
	IDI_EXCLAMATION = 32515
	IDI_ASTERISK    = 32516
)

// GetSystemMetrics constants
const (
	SM_CXSCREEN             = 0
	SM_CYSCREEN             = 1
	SM_CXBORDER             = 5
	SM_CYBORDER             = 6
	SM_CXDLGFRAME           = 7
	SM_CYDLGFRAME           = 8
	SM_CYVTHUMB             = 9
	SM_CXHTHUMB             = 10
	SM_CXICON               = 11
	SM_CXCURSOR             = 13
	SM_CYCURSOR             = 14
	SM_CYMENU               = 15
	SM_CXFULLSCREEN         = 16
	SM_CYFULLSCREEN         = 17
	SM_CYKANJIWINDOW        = 18
	SM_MOUSEPRESENT         = 19
	SM_CYVSCROLL            = 20
	SM_CXHSCROLL            = 21
	SM_DEBUG                = 22
	SM_SWAPBUTTON           = 23
	SM_RESERVED1            = 24
	SM_RESERVED2            = 25
	SM_RESERVED3            = 26
	SM_RESERVED4            = 27
	SM_CXMIN                = 28
	SM_CYMIN                = 29
	SM_CXSIZE               = 30
	SM_CYSIZE               = 31
	SM_CXFRAME              = 32
	SM_CYFRAME              = 33
	SM_CXMINTRACK           = 34
	SM_CYMINTRACK           = 35
	SM_CXDOUBLECLK          = 36
	SM_CYDOUBLECLK          = 37
	SM_CXICONSPACING        = 38
	SM_CYICONSPACING        = 39
	SM_MENUDROPALIGNMENT    = 40
	SM_PENWINDOWS           = 41
	SM_DBCSENABLED          = 42
	SM_CMOUSEBUTTONS        = 43
	SM_CXFIXEDFRAME         = SM_CXDLGFRAME
	SM_CYFIXEDFRAME         = SM_CYDLGFRAME
	SM_CXSIZEFRAME          = SM_CXFRAME
	SM_CYSIZEFRAME          = SM_CYFRAME
	SM_SECURE               = 44
	SM_CXEDGE               = 45
	SM_CYEDGE               = 46
	SM_CXMINSPACING         = 47
	SM_CYMINSPACING         = 48
	SM_CYSMCAPTION          = 51
	SM_CXSMSIZE             = 52
	SM_CYSMSIZE             = 53
	SM_CXMENUSIZE           = 54
	SM_CYMENUSIZE           = 55
	SM_ARRANGE              = 56
	SM_CXMINIMIZED          = 57
	SM_CYMINIMIZED          = 58
	SM_CXMAXTRACK           = 59
	SM_CYMAXTRACK           = 60
	SM_CXMAXIMIZED          = 61
	SM_CYMAXIMIZED          = 62
	SM_NETWORK              = 63
	SM_CLEANBOOT            = 67
	SM_CXDRAG               = 68
	SM_CYDRAG               = 69
	SM_SHOWSOUNDS           = 70
	SM_CXMENUCHECK          = 71
	SM_CYMENUCHECK          = 72
	SM_SLOWMACHINE          = 73
	SM_MIDEASTENABLED       = 74
	SM_MOUSEWHEELPRESENT    = 75
	SM_XVIRTUALSCREEN       = 76
	SM_YVIRTUALSCREEN       = 77
	SM_CXVIRTUALSCREEN      = 78
	SM_CYVIRTUALSCREEN      = 79
	SM_CMONITORS            = 80
	SM_SAMEDISPLAYFORMAT    = 81
	SM_IMMENABLED           = 82
	SM_CXFOCUSBORDER        = 83
	SM_CYFOCUSBORDER        = 84
	SM_TABLETPC             = 86
	SM_MEDIACENTER          = 87
	SM_STARTER              = 88
	SM_SERVERR2             = 89
	SM_CMETRICS             = 91
	SM_REMOTESESSION        = 0x1000
	SM_SHUTTINGDOWN         = 0x2000
	SM_REMOTECONTROL        = 0x2001
	SM_CARETBLINKINGENABLED = 0x2002
)

// ShowWindow constants
const (
	SW_NORMAL          = 1
	SW_SHOWNORMAL      = 1
	SW_SHOWMINIMIZED   = 2
	SW_MAXIMIZE        = 3
	SW_SHOWMAXIMIZED   = 3
	SW_SHOWNOACTIVATE  = 4
	SW_MINIMIZE        = 6
	SW_SHOWMINNOACTIVE = 7
	SW_RESTORE         = 9
	SW_SHOWDEFAULT     = 10
	SW_FORCEMINIMIZE   = 11
)

// SetWindowPos flags
const (
	SWP_DRAWFRAME      = 0x0020
	SWP_HIDEWINDOW     = 0x0080
	SWP_NOCOPYBITS     = 0x0100
	SWP_NOREDRAW       = 0x0008
	SWP_NOREPOSITION   = SWP_NOOWNERZORDER
	SWP_NOSENDCHANGING = 0x0400
	SWP_DEFERERASE     = 0x2000
	SWP_ASYNCWINDOWPOS = 0x4000
)

// UI state constants
const (
	UISF_HIDEACCEL = 0x2
	UISF_ACTIVE    = 0x4
)

// Virtual key codes
const (
	VK_HANGEUL = 0x15
)

// Window style constants
const (
	WS_OVERLAPPED  = 0x00000000
	WS_MINIMIZE    = 0x20000000
	WS_MAXIMIZE    = 0x01000000
	WS_DLGFRAME    = 0x00400000
	WS_MINIMIZEBOX = 0x00020000
	WS_MAXIMIZEBOX = 0x00010000
	WS_TILED       = 0x00000000
	WS_ICONIC      = 0x20000000
	WS_SIZEBOX     = 0x00040000
	WS_POPUPWINDOW = 0x80000000 | 0x00800000 | 0x00080000
	WS_CHILDWINDOW = 0x40000000
)

// Extended window style constants
const (
	WS_EX_DLGMODALFRAME    = 0x00000001
	WS_EX_NOPARENTNOTIFY   = 0x00000004
	WS_EX_TOPMOST          = 0x00000008
	WS_EX_ACCEPTFILES      = 0x00000010
	WS_EX_TRANSPARENT      = 0x00000020
	WS_EX_MDICHILD         = 0x00000040
	WS_EX_TOOLWINDOW       = 0x00000080
	WS_EX_WINDOWEDGE       = 0x00000100
	WS_EX_CONTEXTHELP      = 0x00000400
	WS_EX_RIGHT            = 0x00001000
	WS_EX_LEFT             = 0x00000000
	WS_EX_LTRREADING       = 0x00000000
	WS_EX_LEFTSCROLLBAR    = 0x00004000
	WS_EX_RIGHTSCROLLBAR   = 0x00000000
	WS_EX_STATICEDGE       = 0x00020000
	WS_EX_APPWINDOW        = 0x00040000
	WS_EX_OVERLAPPEDWINDOW = 0x00000100 | 0x00000200
	WS_EX_PALETTEWINDOW    = 0x00000100 | 0x00000080 | 0x00000008
	WS_EX_LAYERED          = 0x00080000
	WS_EX_NOINHERITLAYOUT  = 0x00100000
	WS_EX_NOACTIVATE       = 0x08000000
)

// Window message constants
const (
	WM_ACTIVATEAPP            = 28
	WM_AFXFIRST               = 864
	WM_AFXLAST                = 895
	WM_ASKCBFORMATNAME        = 780
	WM_CANCELJOURNAL          = 75
	WM_CANCELMODE             = 31
	WM_CAPTURECHANGED         = 533
	WM_CHANGECBCHAIN          = 781
	WM_CHARTOITEM             = 47
	WM_CHILDACTIVATE          = 34
	WM_CLEAR                  = 771
	WM_COMMNOTIFY             = 68 /* OBSOLETE */
	WM_COMPACTING             = 65
	WM_COMPAREITEM            = 57
	WM_COPY                   = 769
	WM_COPYDATA               = 74
	WM_CREATE                 = 1
	WM_CTLCOLORBTN            = 309
	WM_CTLCOLORDLG            = 310
	WM_CTLCOLORLISTBOX        = 308
	WM_CTLCOLORMSGBOX         = 306
	WM_CTLCOLORSCROLLBAR      = 311
	WM_CUT                    = 768
	WM_DEADCHAR               = 259
	WM_DELETEITEM             = 45
	WM_DESTROYCLIPBOARD       = 775
	WM_DEVICECHANGE           = 537
	WM_DEVMODECHANGE          = 27
	WM_DISPLAYCHANGE          = 126
	WM_DRAWCLIPBOARD          = 776
	WM_ENABLE                 = 10
	WM_ENDSESSION             = 22
	WM_ENTERIDLE              = 289
	WM_ENTERMENULOOP          = 529
	WM_EXITMENULOOP           = 530
	WM_FONTCHANGE             = 29
	WM_GETFONT                = 49
	WM_GETHOTKEY              = 51
	WM_GETICON                = 127
	WM_HANDHELDFIRST          = 856
	WM_HANDHELDLAST           = 863
	WM_HELP                   = 83
	WM_HOTKEY                 = 786
	WM_HSCROLLCLIPBOARD       = 782
	WM_ICONERASEBKGND         = 39
	WM_INITDIALOG             = 272
	WM_INITMENU               = 278
	WM_INPUT                  = 0x00FF
	WM_INPUTLANGCHANGE        = 81
	WM_INPUTLANGCHANGEREQUEST = 80
	WM_MDIACTIVATE            = 546
	WM_MDICASCADE             = 551
	WM_MDICREATE              = 544
	WM_MDIDESTROY             = 545
	WM_MDIGETACTIVE           = 553
	WM_MDIICONARRANGE         = 552
	WM_MDIMAXIMIZE            = 549
	WM_MDINEXT                = 548
	WM_MDIREFRESHMENU         = 564
	WM_MDIRESTORE             = 547
	WM_MDISETMENU             = 560
	WM_MDITILE                = 550
	WM_GETOBJECT              = 0x003D
	WM_QUERYUISTATE           = 0x0129
	WM_UNINITMENUPOPUP        = 0x0125
	WM_MENURBUTTONUP          = 290
	WM_MENUCOMMAND            = 0x0126
	WM_MENUGETOBJECT          = 0x0124
	WM_MENUDRAG               = 0x0123
	WM_APPCOMMAND             = 0x0319
	WM_MENUCHAR               = 288
	WM_MENUSELECT             = 287
	WM_MOVE                   = 3
	WM_MOVING                 = 534
	WM_NCACTIVATE             = 134
	WM_NCCREATE               = 129
	WM_NCDESTROY              = 130
	WM_NCLBUTTONDBLCLK        = 163
	WM_NCLBUTTONDOWN          = 161
	WM_NCLBUTTONUP            = 162
	WM_NCMBUTTONDBLCLK        = 169
	WM_NCMBUTTONDOWN          = 167
	WM_NCMBUTTONUP            = 168
	WM_NCXBUTTONDOWN          = 171
	WM_NCXBUTTONUP            = 172
	WM_NCXBUTTONDBLCLK        = 173
	WM_NCMOUSEHOVER           = 0x02A0
	WM_NCMOUSELEAVE           = 0x02A2
	WM_NCMOUSEMOVE            = 160
	WM_NCPAINT                = 133
	WM_NCRBUTTONDBLCLK        = 166
	WM_NCRBUTTONDOWN          = 164
	WM_NCRBUTTONUP            = 165
	WM_NEXTDLGCTL             = 40
	WM_NEXTMENU               = 531
	WM_NOTIFYFORMAT           = 85
	WM_NULL                   = 0
	WM_PAINTCLIPBOARD         = 777
	WM_PAINTICON              = 38
	WM_PALETTECHANGED         = 785
	WM_PALETTEISCHANGING      = 784
	WM_PARENTNOTIFY           = 528
	WM_PENWINFIRST            = 896
	WM_PENWINLAST             = 911
	WM_POWER                  = 72
	WM_POWERBROADCAST         = 536
	WM_QUERYDRAGICON          = 55
	WM_QUERYENDSESSION        = 17
	WM_QUERYNEWPALETTE        = 783
	WM_QUERYOPEN              = 19
	WM_QUEUESYNC              = 35
	WM_QUIT                   = 18
	WM_RENDERALLFORMATS       = 774
	WM_RENDERFORMAT           = 773
	WM_SETHOTKEY              = 50
	WM_SETTINGCHANGE          = 26
	WM_SHOWWINDOW             = 24
	WM_SIZECLIPBOARD          = 779
	WM_SIZING                 = 532
	WM_SPOOLERSTATUS          = 42
	WM_STYLECHANGED           = 125
	WM_STYLECHANGING          = 124
	WM_SYSCHAR                = 262
	WM_SYSDEADCHAR            = 263
	WM_SYSKEYDOWN             = 260
	WM_SYSKEYUP               = 261
	WM_TCARD                  = 82
	WM_TIMECHANGE             = 30
	WM_UNDO                   = 772
	WM_USERCHANGED            = 84
	WM_VKEYTOITEM             = 46
	WM_VSCROLLCLIPBOARD       = 778
	WM_WINDOWPOSCHANGING      = 70
	WM_WININICHANGE           = 26
	WM_KEYFIRST               = 256
	WM_KEYLAST                = 264
	WM_SYNCPAINT              = 136
	WM_MBUTTONDBLCLK          = 521
	WM_MOUSEFIRST             = 512
	WM_XBUTTONDOWN            = 523
	WM_XBUTTONUP              = 524
	WM_XBUTTONDBLCLK          = 525
	WM_MOUSELAST              = 525
	WM_MOUSEHOVER             = 0x2A1
	WM_UNICHAR                = 0x0109
)

const (
	INDEXID_OBJECT    = 0
	INDEXID_CONTAINER = 0

	OBJID_WINDOW            = int32(0x00000000)
	OBJID_SYSMENU           = int32(-((0xFFFFFFFF ^ 0xFFFFFFFF) + 1))
	OBJID_TITLEBAR          = int32(-((0xFFFFFFFE ^ 0xFFFFFFFF) + 1))
	OBJID_MENU              = int32(-((0xFFFFFFFD ^ 0xFFFFFFFF) + 1))
	OBJID_VSCROLL           = int32(-((0xFFFFFFFB ^ 0xFFFFFFFF) + 1))
	OBJID_HSCROLL           = int32(-((0xFFFFFFFA ^ 0xFFFFFFFF) + 1))
	OBJID_SIZEGRIP          = int32(-((0xFFFFFFF9 ^ 0xFFFFFFFF) + 1))
	OBJID_CARET             = int32(-((0xFFFFFFF8 ^ 0xFFFFFFFF) + 1))
	OBJID_CURSOR            = int32(-((0xFFFFFFF7 ^ 0xFFFFFFFF) + 1))
	OBJID_ALERT             = int32(-((0xFFFFFFF6 ^ 0xFFFFFFFF) + 1))
	OBJID_SOUND             = int32(-((0xFFFFFFF5 ^ 0xFFFFFFFF) + 1))
	OBJID_QUERYCLASSNAMEIDX = int32(-((0xFFFFFFF4 ^ 0xFFFFFFFF) + 1))
	OBJID_NATIVEOM          = int32(-((0xFFFFFFF0 ^ 0xFFFFFFFF) + 1))
)

// event constants
const (
	EVENT_MIN = 0x00000001
	EVENT_MAX = 0x7FFFFFFF

	EVENT_SYSTEM_SOUND                  = 0x0001
	EVENT_SYSTEM_ALERT                  = 0x0002
	EVENT_SYSTEM_FOREGROUND             = 0x0003
	EVENT_SYSTEM_MENUSTART              = 0x0004
	EVENT_SYSTEM_MENUEND                = 0x0005
	EVENT_SYSTEM_MENUPOPUPSTART         = 0x0006
	EVENT_SYSTEM_MENUPOPUPEND           = 0x0007
	EVENT_SYSTEM_CAPTURESTART           = 0x0008
	EVENT_SYSTEM_CAPTUREEND             = 0x0009
	EVENT_SYSTEM_MOVESIZESTART          = 0x000A
	EVENT_SYSTEM_MOVESIZEEND            = 0x000B
	EVENT_SYSTEM_CONTEXTHELPSTART       = 0x000C
	EVENT_SYSTEM_CONTEXTHELPEND         = 0x000D
	EVENT_SYSTEM_DRAGDROPSTART          = 0x000E
	EVENT_SYSTEM_DRAGDROPEND            = 0x000F
	EVENT_SYSTEM_DIALOGSTART            = 0x0010
	EVENT_SYSTEM_DIALOGEND              = 0x0011
	EVENT_SYSTEM_SCROLLINGSTART         = 0x0012
	EVENT_SYSTEM_SCROLLINGEND           = 0x0013
	EVENT_SYSTEM_SWITCHSTART            = 0x0014
	EVENT_SYSTEM_SWITCHEND              = 0x0015
	EVENT_SYSTEM_MINIMIZESTART          = 0x0016
	EVENT_SYSTEM_MINIMIZEEND            = 0x0017
	EVENT_SYSTEM_DESKTOPSWITCH          = 0x0020
	EVENT_SYSTEM_SWITCHER_APPGRABBED    = 0x0024
	EVENT_SYSTEM_SWITCHER_APPOVERTARGET = 0x0025
	EVENT_SYSTEM_SWITCHER_APPDROPPED    = 0x0026
	EVENT_SYSTEM_SWITCHER_CANCELLED     = 0x0027
	EVENT_SYSTEM_IME_KEY_NOTIFICATION   = 0x0029
	EVENT_SYSTEM_END                    = 0x00FF

	EVENT_OEM_DEFINED_START = 0x0101
	EVENT_OEM_DEFINED_END   = 0x01FF

	EVENT_CONSOLE_CARET             = 0x4001
	EVENT_CONSOLE_UPDATE_REGION     = 0x4002
	EVENT_CONSOLE_UPDATE_SIMPLE     = 0x4003
	EVENT_CONSOLE_UPDATE_SCROLL     = 0x4004
	EVENT_CONSOLE_LAYOUT            = 0x4005
	EVENT_CONSOLE_START_APPLICATION = 0x4006
	EVENT_CONSOLE_END_APPLICATION   = 0x4007
	EVENT_CONSOLE_END               = 0x40FF

	EVENT_UIA_EVENTID_START = 0x4E00
	EVENT_UIA_EVENTID_END   = 0x4EFF

	EVENT_UIA_PROPID_START = 0x7500
	EVENT_UIA_PROPID_END   = 0x75FF

	EVENT_OBJECT_DESTROY                          = 0x8001
	EVENT_OBJECT_SHOW                             = 0x8002
	EVENT_OBJECT_HIDE                             = 0x8003
	EVENT_OBJECT_REORDER                          = 0x8004
	EVENT_OBJECT_FOCUS                            = 0x8005
	EVENT_OBJECT_SELECTION                        = 0x8006
	EVENT_OBJECT_SELECTIONADD                     = 0x8007
	EVENT_OBJECT_SELECTIONREMOVE                  = 0x8008
	EVENT_OBJECT_SELECTIONWITHIN                  = 0x8009
	EVENT_OBJECT_LOCATIONCHANGE                   = 0x800B
	EVENT_OBJECT_VALUECHANGE                      = 0x800E
	EVENT_OBJECT_PARENTCHANGE                     = 0x800F
	EVENT_OBJECT_INVOKED                          = 0x8013
	EVENT_OBJECT_TEXTSELECTIONCHANGED             = 0x8014
	EVENT_OBJECT_CONTENTSCROLLED                  = 0x8015
	EVENT_SYSTEM_ARRANGMENTPREVIEW                = 0x8016
	EVENT_OBJECT_CLOAKED                          = 0x8017
	EVENT_OBJECT_UNCLOAKED                        = 0x8018
	EVENT_OBJECT_LIVEREGIONCHANGED                = 0x8019
	EVENT_OBJECT_HOSTEDOBJECTSINVALIDATED         = 0x8020
	EVENT_OBJECT_DRAGSTART                        = 0x8021
	EVENT_OBJECT_DRAGCANCEL                       = 0x8022
	EVENT_OBJECT_DRAGCOMPLETE                     = 0x8023
	EVENT_OBJECT_DRAGENTER                        = 0x8024
	EVENT_OBJECT_DRAGLEAVE                        = 0x8025
	EVENT_OBJECT_DRAGDROPPED                      = 0x8026
	EVENT_OBJECT_IME_SHOW                         = 0x8027
	EVENT_OBJECT_IME_HIDE                         = 0x8028
	EVENT_OBJECT_IME_CHANGE                       = 0x8029
	EVENT_OBJECT_TEXTEDIT_CONVERSIONTARGETCHANGED = 0x8030

	EVENT_AIA_START = 0xa000
	EVENT_AIA_END   = 0xafff

	WINEVENT_OUTOFCONTEXT   = 0x0000
	WINEVENT_SKIPOWNTHREAD  = 0x0001
	WINEVENT_SKIPOWNPROCESS = 0x0002
	WINEVENT_INCONTEXT      = 0x0004
)

// mouse button constants
const (
	MK_CONTROL  = 0x0008
	MK_SHIFT    = 0x0004
	MK_XBUTTON1 = 0x0020
	MK_XBUTTON2 = 0x0040
)

// TrackPopupMenu[Ex] flags
const (
	TPM_CENTERALIGN     = 0x0004
	TPM_LEFTALIGN       = 0x0000
	TPM_RIGHTALIGN      = 0x0008
	TPM_BOTTOMALIGN     = 0x0020
	TPM_TOPALIGN        = 0x0000
	TPM_VCENTERALIGN    = 0x0010
	TPM_NONOTIFY        = 0x0080
	TPM_LEFTBUTTON      = 0x0000
	TPM_RIGHTBUTTON     = 0x0002
	TPM_HORNEGANIMATION = 0x0800
	TPM_HORPOSANIMATION = 0x0400
	TPM_VERNEGANIMATION = 0x2000
	TPM_VERPOSANIMATION = 0x1000
	TPM_HORIZONTAL      = 0x0000
	TPM_VERTICAL        = 0x0040
)

// WINDOWPLACEMENT flags
const (
	WPF_ASYNCWINDOWPLACEMENT = 0x0004
	WPF_RESTORETOMAXIMIZED   = 0x0002
	WPF_SETMINPOSITION       = 0x0001
)

// Window class styles
const (
	CS_VREDRAW         = 0x00000001
	CS_HREDRAW         = 0x00000002
	CS_KEYCVTWINDOW    = 0x00000004
	CS_DBLCLKS         = 0x00000008
	CS_OWNDC           = 0x00000020
	CS_CLASSDC         = 0x00000040
	CS_PARENTDC        = 0x00000080
	CS_NOKEYCVT        = 0x00000100
	CS_NOCLOSE         = 0x00000200
	CS_SAVEBITS        = 0x00000800
	CS_BYTEALIGNCLIENT = 0x00001000
	CS_BYTEALIGNWINDOW = 0x00002000
	CS_GLOBALCLASS     = 0x00004000
	CS_IME             = 0x00010000
	CS_DROPSHADOW      = 0x00020000
)

// SystemParametersInfo actions
const (
	SPI_GETNONCLIENTMETRICS = 0x0029
)

// Dialog styles
const (
	DS_ABSALIGN      = 0x0001
	DS_SYSMODAL      = 0x0002
	DS_3DLOOK        = 0x0004
	DS_FIXEDSYS      = 0x0008
	DS_NOFAILCREATE  = 0x0010
	DS_LOCALEDIT     = 0x0020
	DS_SETFONT       = 0x0040
	DS_MODALFRAME    = 0x0080
	DS_NOIDLEMSG     = 0x0100
	DS_SETFOREGROUND = 0x0200
	DS_CONTROL       = 0x0400
	DS_CENTER        = 0x0800
	DS_CENTERMOUSE   = 0x1000
	DS_CONTEXTHELP   = 0x2000
	DS_USEPIXELS     = 0x8000
	DS_SHELLFONT     = (DS_SETFONT | DS_FIXEDSYS)
)

// WM_GETDLGCODE return values
const (
	DLGC_RADIOBUTTON     = 0x0040
	DLGC_STATIC          = 0x0100
	DLGC_UNDEFPUSHBUTTON = 0x0020
	DLGC_WANTMESSAGE     = 0x0004
	DLGC_WANTTAB         = 0x0002
)

// Owner drawing actions
const (
	ODA_FOCUS  = 0x0002
	ODA_SELECT = 0x0004
)

// Owner drawing states
const (
	ODS_CHECKED      = 0x0001
	ODS_COMBOBOXEDIT = 0x0002
	ODS_DEFAULT      = 0x0004
	ODS_DISABLED     = 0x0008
	ODS_FOCUS        = 0x0010
	ODS_GRAYED       = 0x0020
	ODS_SELECTED     = 0x0040
)

// Raw input device flags
const (
	RIDEV_APPKEYS      = 0x00000400
	RIDEV_CAPTUREMOUSE = 0x00000200
	RIDEV_DEVNOTIFY    = 0x00002000
	RIDEV_EXCLUDE      = 0x00000010
	RIDEV_EXINPUTSINK  = 0x00001000
	RIDEV_INPUTSINK    = 0x00000100
	RIDEV_NOHOTKEYS    = 0x00000200
	RIDEV_NOLEGACY     = 0x00000030
	RIDEV_PAGEONLY     = 0x00000020
	RIDEV_REMOVE       = 0x00000001
)

// Raw input device command flags
const (
	RID_HEADER = 0x10000005
	RID_INPUT  = 0x10000003
)

// Raw input type
const (
	RIM_TYPEHID      = 2
	RIM_TYPEKEYBOARD = 1
	RIM_TYPEMOUSE    = 0
)

// Raw input scan code information
const (
	RI_KEY_MAKE  = 0
	RI_KEY_BREAK = 1
	RI_KEY_E0    = 2
	RI_KEY_E1    = 4
)

// Raw input mouse state
const (
	MOUSE_MOVE_RELATIVE      = 0x00
	MOUSE_MOVE_ABSOLUTE      = 0x01
	MOUSE_VIRTUAL_DESKTOP    = 0x02
	MOUSE_ATTRIBUTES_CHANGED = 0x04
)

// Raw input transistion state of mouse buttons
const (
	RI_MOUSE_LEFT_BUTTON_DOWN   = 0x0001
	RI_MOUSE_LEFT_BUTTON_UP     = 0x0002
	RI_MOUSE_MIDDLE_BUTTON_DOWN = 0x0010
	RI_MOUSE_MIDDLE_BUTTON_UP   = 0x0020
	RI_MOUSE_RIGHT_BUTTON_DOWN  = 0x0004
	RI_MOUSE_RIGHT_BUTTON_UP    = 0x0008
	RI_MOUSE_BUTTON_1_DOWN      = 0x0001
	RI_MOUSE_BUTTON_1_UP        = 0x0002
	RI_MOUSE_BUTTON_2_DOWN      = 0x0004
	RI_MOUSE_BUTTON_2_UP        = 0x0008
	RI_MOUSE_BUTTON_3_DOWN      = 0x0010
	RI_MOUSE_BUTTON_3_UP        = 0x0020
	RI_MOUSE_BUTTON_4_DOWN      = 0x0040
	RI_MOUSE_BUTTON_4_UP        = 0x0080
	RI_MOUSE_BUTTON_5_DOWN      = 0x0100
	RI_MOUSE_BUTTON_5_UP        = 0x0200
	RI_MOUSE_WHEEL              = 0x0400
)

// Multi monitor constants
const (
	MONITOR_DEFAULTTONULL    = 0x0
	MONITOR_DEFAULTTONEAREST = 0x2
)

// MONITORINFO flags
const (
	MONITORINFOF_PRIMARY = 0x1
)

// INPUT Type
const (
	INPUT_MOUSE    = 0
	INPUT_KEYBOARD = 1
	INPUT_HARDWARE = 2
)

// MOUSEINPUT MouseData
const (
	XBUTTON1 = 0x0001
	XBUTTON2 = 0x0002
)

// MOUSEINPUT DwFlags
const (
	MOUSEEVENTF_ABSOLUTE        = 0x8000
	MOUSEEVENTF_HWHEEL          = 0x1000
	MOUSEEVENTF_MOVE            = 0x0001
	MOUSEEVENTF_MOVE_NOCOALESCE = 0x2000
	MOUSEEVENTF_LEFTDOWN        = 0x0002
	MOUSEEVENTF_LEFTUP          = 0x0004
	MOUSEEVENTF_RIGHTDOWN       = 0x0008
	MOUSEEVENTF_RIGHTUP         = 0x0010
	MOUSEEVENTF_MIDDLEDOWN      = 0x0020
	MOUSEEVENTF_MIDDLEUP        = 0x0040
	MOUSEEVENTF_VIRTUALDESK     = 0x4000
	MOUSEEVENTF_WHEEL           = 0x0800
	MOUSEEVENTF_XDOWN           = 0x0080
	MOUSEEVENTF_XUP             = 0x0100
)

// KEYBDINPUT DwFlags
const (
	KEYEVENTF_EXTENDEDKEY = 0x0001
	KEYEVENTF_KEYUP       = 0x0002
	KEYEVENTF_SCANCODE    = 0x0008
	KEYEVENTF_UNICODE     = 0x0004
)

// GetWindow uCmd constants
const (
	GW_ENABLEDPOPUP = 6
	GW_HWNDFIRST    = 0
	GW_HWNDLAST     = 1
	GW_HWNDNEXT     = 2
	GW_HWNDPREV     = 3
	GW_OWNER        = 4
)

// Standard clipboard formats
const (
	CF_BITMAP          = 2
	CF_DIB             = 8
	CF_DIBV5           = 17
	CF_DIF             = 5
	CF_DSPBITMAP       = 0x0082
	CF_DSPENHMETAFILE  = 0x008E
	CF_DSPMETAFILEPICT = 0x0083
	CF_DSPTEXT         = 0x0081
	CF_ENHMETAFILE     = 14
	CF_GDIOBJFIRST     = 0x0300
	CF_GDIOBJLAST      = 0x03FF
	CF_HDROP           = 15
	CF_LOCALE          = 16
	CF_METAFILEPICT    = 3
	CF_OEMTEXT         = 7
	CF_OWNERDISPLAY    = 0x0080
	CF_PALETTE         = 9
	CF_PENDATA         = 10
	CF_PRIVATEFIRST    = 0x0200
	CF_PRIVATELAST     = 0x02FF
	CF_RIFF            = 11
	CF_SYLK            = 4
	CF_TEXT            = 1
	CF_TIFF            = 6
	CF_WAVE            = 12
)

// ScrollBar constants
const (
	SB_CTL  = 2
	SB_BOTH = 3
)

// ScrollBar commands
const (
	SB_TOP    = 6
	SB_LEFT   = 6
	SB_BOTTOM = 7
	SB_RIGHT  = 7
)

// [Get|Set]ScrollInfo mask constants
const (
	SIF_DISABLENOSCROLL = 8
	SIF_ALL             = SIF_RANGE + SIF_PAGE + SIF_POS + SIF_TRACKPOS
)

// DrawIconEx flags
const (
	DI_COMPAT      = 0x0004
	DI_DEFAULTSIZE = 0x0008
	DI_IMAGE       = 0x0002
	DI_MASK        = 0x0001
	DI_NOMIRROR    = 0x0010
)

// WM_NCHITTEST constants
const (
	HTBORDER      = 18
	HTBOTTOM      = 15
	HTBOTTOMLEFT  = 16
	HTBOTTOMRIGHT = 17
	HTCAPTION     = 2
	HTCLOSE       = 20
	HTERROR       = -2
	HTGROWBOX     = 4
	HTHELP        = 21
	HTHSCROLL     = 6
	HTLEFT        = 10
	HTMENU        = 5
	HTMAXBUTTON   = 9
	HTMINBUTTON   = 8
	HTNOWHERE     = 0
	HTREDUCE      = 8
	HTRIGHT       = 11
	HTSIZE        = 4
	HTSYSMENU     = 3
	HTTOP         = 12
	HTTOPLEFT     = 13
	HTTOPRIGHT    = 14
	HTTRANSPARENT = -1
	HTVSCROLL     = 7
	HTZOOM        = 9
)

// AnimateWindow flags
const (
	AW_ACTIVATE     = 0x00020000
	AW_BLEND        = 0x00080000
	AW_CENTER       = 0x00000010
	AW_HIDE         = 0x00010000
	AW_HOR_POSITIVE = 0x00000001
	AW_HOR_NEGATIVE = 0x00000002
	AW_SLIDE        = 0x00040000
	AW_VER_POSITIVE = 0x00000004
	AW_VER_NEGATIVE = 0x00000008
)

// Session ending constants
const (
	ENDSESSION_CLOSEAPP = 0x00000001
	ENDSESSION_CRITICAL = 0x40000000
	ENDSESSION_LOGOFF   = 0x80000000
)

// ChangeWindowMessageFilterEx constants
const (
	MSGFLT_RESET    = 0
	MSGFLT_DISALLOW = 2

	MSGFLTINFO_NONE                     = 0
	MSGFLTINFO_ALREADYALLOWED_FORWND    = 1
	MSGFLTINFO_ALREADYDISALLOWED_FORWND = 2
	MSGFLTINFO_ALLOWED_HIGHER           = 3
)

// TRACKMOUSEEVENT flags
const (
	TME_CANCEL    = 0x80000000
	TME_HOVER     = 0x00000001
	TME_NONCLIENT = 0x00000010
	TME_QUERY     = 0x40000000
)

// HIGHCONTRAST flags
const (
	HCF_AVAILABLE       = 0x00000002
	HCF_HOTKEYACTIVE    = 0x00000004
	HCF_CONFIRMHOTKEY   = 0x00000008
	HCF_HOTKEYSOUND     = 0x00000010
	HCF_INDICATOR       = 0x00000020
	HCF_HOTKEYAVAILABLE = 0x00000040
)

type (
	HACCEL    HANDLE
	HCURSOR   HANDLE
	HDWP      HANDLE
	HICON     HANDLE
	HMENU     HANDLE
	HMONITOR  HANDLE
	HRAWINPUT HANDLE
	HWND      HANDLE
)

type RAWINPUTDEVICE struct {
	UsUsagePage uint16
	UsUsage     uint16
	DwFlags     uint32
	HwndTarget  HWND
}

type RAWINPUTHEADER struct {
	DwType  uint32
	DwSize  uint32
	HDevice HANDLE
	WParam  uintptr
}

type RAWINPUTMOUSE struct {
	Header RAWINPUTHEADER
	Data   RAWMOUSE
}

type RAWINPUTKEYBOARD struct {
	Header RAWINPUTHEADER
	Data   RAWKEYBOARD
}

type RAWINPUTHID struct {
	Header RAWINPUTHEADER
	Data   RAWHID
}

type RAWMOUSE struct {
	UsFlags            uint16
	UsButtonFlags      uint16
	UsButtonData       uint16
	Pad_cgo_0          [2]byte
	UlRawButtons       uint32
	LLastX             int32
	LLastY             int32
	UlExtraInformation uint32
}

type RAWKEYBOARD struct {
	MakeCode         uint16
	Flags            uint16
	Reserved         int16
	VKey             uint16
	Message          uint32
	ExtraInformation uint32
}

type RAWHID struct {
	DwSizeHid uint32
	DwCount   uint32
	BRawData  [1]byte
}

type CREATESTRUCT struct {
	CreateParams    uintptr
	Instance        HINSTANCE
	Menu            HMENU
	Parent          HWND
	Cy              int32
	Cx              int32
	Y               int32
	X               int32
	Style           int32
	Name, ClassName uintptr
	ExStyle         uint32
}

type CHANGEFILTERSTRUCT struct {
	size      uint32
	extStatus uint32
}

type TPMPARAMS struct {
	CbSize    uint32
	RcExclude RECT
}

type NONCLIENTMETRICS struct {
	CbSize           uint32
	IBorderWidth     int32
	IScrollWidth     int32
	IScrollHeight    int32
	ICaptionWidth    int32
	ICaptionHeight   int32
	LfCaptionFont    LOGFONT
	ISmCaptionWidth  int32
	ISmCaptionHeight int32
	LfSmCaptionFont  LOGFONT
	IMenuWidth       int32
	IMenuHeight      int32
	LfMenuFont       LOGFONT
	LfStatusFont     LOGFONT
	LfMessageFont    LOGFONT
}

type MOUSE_INPUT struct {
	Type uint32
	Mi   MOUSEINPUT
}

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBD_INPUT struct {
	Type uint32
	Ki   KEYBDINPUT
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
	Unused      [8]byte
}

type HARDWARE_INPUT struct {
	Type uint32
	Hi   HARDWAREINPUT
}

type HARDWAREINPUT struct {
	UMsg    uint32
	WParamL uint16
	WParamH uint16
	Unused  [16]byte
}

func GET_X_LPARAM(lp uintptr) int32 {
	return int32(int16(LOWORD(uint32(lp))))
}

func GET_Y_LPARAM(lp uintptr) int32 {
	return int32(int16(HIWORD(uint32(lp))))
}

var (

	// Functions
	addClipboardFormatListener *windows.LazyProc
	adjustWindowRect           *windows.LazyProc
	attachThreadInput          *windows.LazyProc
	animateWindow              *windows.LazyProc
	bringWindowToTop           *windows.LazyProc
	createDialogParam          *windows.LazyProc
	deleteMenu                 *windows.LazyProc
	dialogBoxParam             *windows.LazyProc
	drawFocusRect              *windows.LazyProc
	enableMenuItem             *windows.LazyProc
	endDialog                  *windows.LazyProc
	getActiveWindow            *windows.LazyProc
	getClassName               *windows.LazyProc
	getDesktopWindow           *windows.LazyProc
	getDpiForWindow            *windows.LazyProc
	getMenuCheckMarkDimensions *windows.LazyProc
	getMenuItemCount           *windows.LazyProc
	getMenuItemID              *windows.LazyProc
	getMenuItemInfo            *windows.LazyProc
	getRawInputData            *windows.LazyProc
	getSubMenu                 *windows.LazyProc
	getSystemMenu              *windows.LazyProc
	getSystemMetrics           *windows.LazyProc
	getSystemMetricsForDpi     *windows.LazyProc
	getWindowThreadProcessId   *windows.LazyProc
	isIconic                   *windows.LazyProc
	isZoomed                   *windows.LazyProc
	loadMenu                   *windows.LazyProc
	loadString                 *windows.LazyProc
	messageBeep                *windows.LazyProc
	unregisterClass            *windows.LazyProc
	peekMessage                *windows.LazyProc
	redrawWindow               *windows.LazyProc
	registerRawInputDevices    *windows.LazyProc
	sendDlgItemMessage         *windows.LazyProc
	sendInput                  *windows.LazyProc
	setCursorPos               *windows.LazyProc
	setMenuItemBitmaps         *windows.LazyProc
	setRect                    *windows.LazyProc
	setWinEventHook            *windows.LazyProc
	trackPopupMenu             *windows.LazyProc
	unhookWinEvent             *windows.LazyProc
	windowFromPoint            *windows.LazyProc
)

func init() {
	// Functions
	addClipboardFormatListener = libuser32.NewProc("AddClipboardFormatListener")
	adjustWindowRect = libuser32.NewProc("AdjustWindowRect")
	attachThreadInput = libuser32.NewProc("AttachThreadInput")
	animateWindow = libuser32.NewProc("AnimateWindow")
	bringWindowToTop = libuser32.NewProc("BringWindowToTop")
	createDialogParam = libuser32.NewProc("CreateDialogParamW")
	deleteMenu = libuser32.NewProc("DeleteMenu")
	dialogBoxParam = libuser32.NewProc("DialogBoxParamW")
	drawFocusRect = libuser32.NewProc("DrawFocusRect")
	enableMenuItem = libuser32.NewProc("EnableMenuItem")
	endDialog = libuser32.NewProc("EndDialog")
	getActiveWindow = libuser32.NewProc("GetActiveWindow")
	getClassName = libuser32.NewProc("GetClassNameW")
	getDesktopWindow = libuser32.NewProc("GetDesktopWindow")
	getDpiForWindow = libuser32.NewProc("GetDpiForWindow")
	getMenuCheckMarkDimensions = libuser32.NewProc("GetMenuCheckMarkDimensions")
	getMenuItemCount = libuser32.NewProc("GetMenuItemCount")
	getMenuItemID = libuser32.NewProc("GetMenuItemID")
	getMenuItemInfo = libuser32.NewProc("GetMenuItemInfoW")
	getRawInputData = libuser32.NewProc("GetRawInputData")
	getSubMenu = libuser32.NewProc("GetSubMenu")
	getSystemMenu = libuser32.NewProc("GetSystemMenu")
	getSystemMetrics = libuser32.NewProc("GetSystemMetrics")
	getSystemMetricsForDpi = libuser32.NewProc("GetSystemMetricsForDpi")
	getWindowThreadProcessId = libuser32.NewProc("GetWindowThreadProcessId")
	isIconic = libuser32.NewProc("IsIconic")
	isZoomed = libuser32.NewProc("IsZoomed")
	loadMenu = libuser32.NewProc("LoadMenuW")
	loadString = libuser32.NewProc("LoadStringW")
	messageBeep = libuser32.NewProc("MessageBeep")
	unregisterClass = libuser32.NewProc("UnregisterClassW")
	peekMessage = libuser32.NewProc("PeekMessageW")
	redrawWindow = libuser32.NewProc("RedrawWindow")
	registerRawInputDevices = libuser32.NewProc("RegisterRawInputDevices")
	sendDlgItemMessage = libuser32.NewProc("SendDlgItemMessageW")
	sendInput = libuser32.NewProc("SendInput")
	setCursorPos = libuser32.NewProc("SetCursorPos")
	setMenuItemBitmaps = libuser32.NewProc("SetMenuItemBitmaps")
	setRect = libuser32.NewProc("SetRect")
	setWinEventHook = libuser32.NewProc("SetWinEventHook")
	trackPopupMenu = libuser32.NewProc("TrackPopupMenu")
	unhookWinEvent = libuser32.NewProc("UnhookWinEvent")
	windowFromPoint = libuser32.NewProc("WindowFromPoint")
}

func AddClipboardFormatListener(hwnd HWND) bool {
	if addClipboardFormatListener.Find() != nil {
		return false
	}

	ret, _, _ := syscall.Syscall(addClipboardFormatListener.Addr(), 1,
		uintptr(hwnd),
		0,
		0)

	return ret != 0
}

func AdjustWindowRect(lpRect *RECT, dwStyle uint32, bMenu bool) bool {
	ret, _, _ := syscall.Syscall(adjustWindowRect.Addr(), 3,
		uintptr(unsafe.Pointer(lpRect)),
		uintptr(dwStyle),
		uintptr(BoolToBOOL(bMenu)))

	return ret != 0
}

func AttachThreadInput(idAttach int32, idAttachTo int32, fAttach bool) bool {
	ret, _, _ := syscall.Syscall(attachThreadInput.Addr(), 3,
		uintptr(idAttach),
		uintptr(idAttachTo),
		uintptr(BoolToBOOL(fAttach)))

	return ret != 0
}

func AnimateWindow(hwnd HWND, dwTime, dwFlags uint32) bool {
	ret, _, _ := syscall.Syscall(animateWindow.Addr(), 3,
		uintptr(hwnd),
		uintptr(dwTime),
		uintptr(dwFlags))

	return ret != 0
}

func GetWindowThreadProcessId(hwnd HWND, processId *uint32) uint32 {
	ret, _, _ := syscall.Syscall(getWindowThreadProcessId.Addr(), 2,
		uintptr(hwnd),
		uintptr(unsafe.Pointer(processId)),
		0)

	return uint32(ret)
}

func BringWindowToTop(hwnd HWND) bool {
	ret, _, _ := syscall.Syscall(bringWindowToTop.Addr(), 1,
		uintptr(hwnd),
		0,
		0)
	return ret != 0
}

func CreateDialogParam(instRes HINSTANCE, name *uint16, parent HWND,
	proc, param uintptr) HWND {
	ret, _, _ := syscall.Syscall6(createDialogParam.Addr(), 5,
		uintptr(instRes),
		uintptr(unsafe.Pointer(name)),
		uintptr(parent),
		proc,
		param,
		0)

	return HWND(ret)
}

func DeleteMenu(hMenu HMENU, uPosition uint32, uFlags uint32) bool {
	ret, _, _ := syscall.Syscall(deleteMenu.Addr(), 3,
		uintptr(hMenu),
		uintptr(uPosition),
		uintptr(uFlags))

	return ret != 0
}

func DialogBoxParam(instRes HINSTANCE, name *uint16, parent HWND, proc, param uintptr) int {
	ret, _, _ := syscall.Syscall6(dialogBoxParam.Addr(), 5,
		uintptr(instRes),
		uintptr(unsafe.Pointer(name)),
		uintptr(parent),
		proc,
		param,
		0)

	return int(ret)
}

func DrawFocusRect(hDC HDC, lprc *RECT) bool {
	ret, _, _ := syscall.Syscall(drawFocusRect.Addr(), 2,
		uintptr(hDC),
		uintptr(unsafe.Pointer(lprc)),
		0)

	return ret != 0
}

func EnableMenuItem(hMenu HMENU, uIDEnableItem uint32, uEnable uint32) bool {
	ret, _, _ := syscall.Syscall(enableMenuItem.Addr(), 3,
		uintptr(hMenu),
		uintptr(uIDEnableItem),
		uintptr(uEnable))

	return ret != 0
}

func EndDialog(hwnd HWND, result int) bool {
	ret, _, _ := syscall.Syscall(endDialog.Addr(), 2,
		uintptr(hwnd),
		uintptr(result),
		0)

	return ret != 0
}

func GetActiveWindow() HWND {
	ret, _, _ := syscall.Syscall(getActiveWindow.Addr(), 0,
		0,
		0,
		0)

	return HWND(ret)
}

func GetClassName(hWnd HWND, className *uint16, maxCount int) (int, error) {
	ret, _, e := syscall.Syscall(getClassName.Addr(), 3,
		uintptr(hWnd),
		uintptr(unsafe.Pointer(className)),
		uintptr(maxCount))
	if ret == 0 {
		return 0, e
	}
	return int(ret), nil
}

func GetDesktopWindow() HWND {
	ret, _, _ := syscall.Syscall(getDesktopWindow.Addr(), 0,
		0,
		0,
		0)

	return HWND(ret)
}

func GetDpiForWindow(hwnd HWND) uint32 {
	if getDpiForWindow.Find() != nil {
		hdc := GetDC(hwnd)
		defer ReleaseDC(hwnd, hdc)

		return uint32(GetDeviceCaps(hdc, LOGPIXELSY))
	}

	ret, _, _ := syscall.Syscall(getDpiForWindow.Addr(), 1,
		uintptr(hwnd),
		0,
		0)

	return uint32(ret)
}

func GetMenuCheckMarkDimensions() int32 {
	ret, _, _ := syscall.Syscall(getMenuCheckMarkDimensions.Addr(), 0,
		0,
		0,
		0)

	return int32(ret)
}

func GetMenuItemCount(hMenu HMENU) int32 {
	ret, _, _ := syscall.Syscall(getMenuItemCount.Addr(), 1,
		uintptr(hMenu),
		0,
		0)

	return int32(ret)
}

func GetMenuItemID(hMenu HMENU, nPos int32) uint32 {
	ret, _, _ := syscall.Syscall(getMenuItemID.Addr(), 2,
		uintptr(hMenu),
		uintptr(nPos),
		0)

	return uint32(ret)
}

func GetMenuItemInfo(hmenu HMENU, item uint32, fByPosition BOOL, lpmii *MENUITEMINFO) bool {
	ret, _, _ := syscall.Syscall6(getMenuItemInfo.Addr(), 4,
		uintptr(hmenu),
		uintptr(item),
		uintptr(fByPosition),
		uintptr(unsafe.Pointer(lpmii)),
		0,
		0)

	return ret != 0
}

func GetRawInputData(hRawInput HRAWINPUT, uiCommand uint32, pData unsafe.Pointer, pcbSize *uint32, cBSizeHeader uint32) uint32 {
	ret, _, _ := syscall.Syscall6(getRawInputData.Addr(), 5,
		uintptr(hRawInput),
		uintptr(uiCommand),
		uintptr(pData),
		uintptr(unsafe.Pointer(pcbSize)),
		uintptr(cBSizeHeader),
		0)

	return uint32(ret)
}

func GetSubMenu(hMenu HMENU, nPos int32) HMENU {
	ret, _, _ := syscall.Syscall(getSubMenu.Addr(), 2,
		uintptr(hMenu),
		uintptr(nPos),
		0)

	return HMENU(ret)
}

func GetSystemMenu(hWnd HWND, revert bool) HMENU {
	ret, _, _ := syscall.Syscall(getSystemMenu.Addr(), 2,
		uintptr(hWnd),
		uintptr(BoolToBOOL(revert)),
		0)
	return HMENU(ret)
}

func GetSystemMetrics(nIndex int32) int32 {
	ret, _, _ := syscall.Syscall(getSystemMetrics.Addr(), 1,
		uintptr(nIndex),
		0,
		0)

	return int32(ret)
}

func GetSystemMetricsForDpi(nIndex int32, dpi uint32) int32 {
	if getSystemMetricsForDpi.Find() != nil {
		return GetSystemMetrics(nIndex)
	}

	ret, _, _ := syscall.Syscall(getSystemMetricsForDpi.Addr(), 2,
		uintptr(nIndex),
		uintptr(dpi),
		0)

	return int32(ret)
}

func IsIconic(hWnd HWND) bool {
	ret, _, _ := syscall.Syscall(isIconic.Addr(), 1,
		uintptr(hWnd),
		0,
		0)

	return ret != 0
}

func IsZoomed(hWnd HWND) bool {
	ret, _, _ := syscall.Syscall(isZoomed.Addr(), 1,
		uintptr(hWnd),
		0,
		0)

	return ret != 0
}

func LoadMenu(hinst HINSTANCE, name *uint16) HMENU {
	ret, _, _ := syscall.Syscall(loadMenu.Addr(), 2,
		uintptr(hinst),
		uintptr(unsafe.Pointer(name)),
		0)

	return HMENU(ret)
}

func LoadString(instRes HINSTANCE, id uint32, buf *uint16, length int32) int32 {
	ret, _, _ := syscall.Syscall6(loadString.Addr(), 4,
		uintptr(instRes),
		uintptr(id),
		uintptr(unsafe.Pointer(buf)),
		uintptr(length),
		0,
		0)

	return int32(ret)
}

// Plays a waveform sound. uType is the sound to be played. The sounds are set by the user through the Sound control panel application.
// The following values can be used as a sound:
//
//	MB_ICONASTERISK (see MB_ICONINFORMATION)
//	MB_ICONEXCLAMATION (see MB_ICONWARNING)
//	MB_ICONERROR (The sound specified as the Windows Critical Stop sound)
//	MB_ICONHAND (See MB_ICONERROR)
//	MB_ICONINFORMATION (The sounds specified as the Windows Asterisk sound)
//	MB_ICONQUESTION (The sound specified as the Windows Question sound)
//	MB_ICONSTOP (See MB_ICONERROR)
//	MB_ICONWARNING (The sounds specified as the Windows Exclamation sound)
//	MB_OK (The sound specified as the Windows Default Beep sound)
//
// The function will return true if the function succeeds, false if otherwise.
func MessageBeep(uType uint32) bool {
	ret, _, _ := syscall.Syscall(messageBeep.Addr(), 2,
		uintptr(uType),
		0,
		0)

	return ret != 0
}

func UnregisterClass(name *uint16) bool {
	ret, _, _ := syscall.Syscall(unregisterClass.Addr(), 1,
		uintptr(unsafe.Pointer(name)),
		0,
		0)

	return ret != 0
}

func PeekMessage(lpMsg *MSG, hWnd HWND, wMsgFilterMin, wMsgFilterMax, wRemoveMsg uint32) bool {
	ret, _, _ := syscall.Syscall6(peekMessage.Addr(), 5,
		uintptr(unsafe.Pointer(lpMsg)),
		uintptr(hWnd),
		uintptr(wMsgFilterMin),
		uintptr(wMsgFilterMax),
		uintptr(wRemoveMsg),
		0)

	return ret != 0
}

const (
	// RedrawWindow() flags
	RDW_INVALIDATE    = 0x0001
	RDW_INTERNALPAINT = 0x0002
	RDW_ERASE         = 0x0004

	RDW_VALIDATE        = 0x0008
	RDW_NOINTERNALPAINT = 0x0010
	RDW_NOERASE         = 0x0020

	RDW_NOCHILDREN  = 0x0040
	RDW_ALLCHILDREN = 0x0080

	RDW_UPDATENOW = 0x0100
	RDW_ERASENOW  = 0x0200

	RDW_FRAME   = 0x0400
	RDW_NOFRAME = 0x0800
)

func RedrawWindow(hWnd HWND, lprcUpdate *RECT, hrgnUpdate HRGN, flags uint32) bool {
	ret, _, _ := syscall.Syscall6(redrawWindow.Addr(), 4,
		uintptr(hWnd),
		uintptr(unsafe.Pointer(lprcUpdate)),
		uintptr(hrgnUpdate),
		uintptr(flags),
		0,
		0)

	return ret != 0
}

func RegisterRawInputDevices(pRawInputDevices *RAWINPUTDEVICE, uiNumDevices uint32, cbSize uint32) bool {
	ret, _, _ := syscall.Syscall(registerRawInputDevices.Addr(), 3,
		uintptr(unsafe.Pointer(pRawInputDevices)),
		uintptr(uiNumDevices),
		uintptr(cbSize))

	return ret != 0
}

func SendDlgItemMessage(hWnd HWND, id int32, msg uint32, wParam, lParam uintptr) uintptr {
	ret, _, _ := syscall.Syscall6(sendDlgItemMessage.Addr(), 5,
		uintptr(hWnd),
		uintptr(id),
		uintptr(msg),
		wParam,
		lParam,
		0)

	return ret
}

// pInputs expects a unsafe.Pointer to a slice of MOUSE_INPUT or KEYBD_INPUT or HARDWARE_INPUT structs.
func SendInput(nInputs uint32, pInputs unsafe.Pointer, cbSize int32) uint32 {
	ret, _, _ := syscall.Syscall(sendInput.Addr(), 3,
		uintptr(nInputs),
		uintptr(pInputs),
		uintptr(cbSize))

	return uint32(ret)
}

func SetCursorPos(X, Y int32) bool {
	ret, _, _ := syscall.Syscall(setCursorPos.Addr(), 2,
		uintptr(X),
		uintptr(Y),
		0)

	return ret != 0
}

func SetMenuItemBitmaps(hMenu HMENU, uPosition uint32, uFlags uint32, hBitmapUnchecked HBITMAP, hBitmapChecked HBITMAP) bool {
	ret, _, _ := syscall.Syscall6(setMenuItemBitmaps.Addr(), 5,
		uintptr(hMenu),
		uintptr(uPosition),
		uintptr(uFlags),
		uintptr(hBitmapUnchecked),
		uintptr(hBitmapChecked),
		0)

	return ret != 0
}

func SetRect(lprc *RECT, xLeft, yTop, xRight, yBottom uint32) BOOL {
	ret, _, _ := syscall.Syscall6(setRect.Addr(), 5,
		uintptr(unsafe.Pointer(lprc)),
		uintptr(xLeft),
		uintptr(yTop),
		uintptr(xRight),
		uintptr(yBottom),
		0)

	return BOOL(ret)
}

type WINEVENTPROC func(hWinEventHook HWINEVENTHOOK, event uint32, hwnd HWND, idObject int32, idChild int32, idEventThread uint32, dwmsEventTime uint32) uintptr

func SetWinEventHook(eventMin uint32, eventMax uint32, hmodWinEventProc HMODULE, callbackFunction WINEVENTPROC, idProcess uint32, idThread uint32, dwFlags uint32) (HWINEVENTHOOK, error) {
	ret, _, err := syscall.Syscall9(setWinEventHook.Addr(), 7,
		uintptr(eventMin),
		uintptr(eventMax),
		uintptr(hmodWinEventProc),
		windows.NewCallback(callbackFunction),
		uintptr(idProcess),
		uintptr(idThread),
		uintptr(dwFlags),
		0, 0)

	if ret == 0 {
		return 0, err
	}

	return HWINEVENTHOOK(ret), nil
}

func TrackPopupMenu(hMenu HMENU, uFlags uint32, x, y int32, nReserved int32, hWnd HWND, prcRect *RECT) uint32 {
	ret, _, _ := syscall.Syscall9(trackPopupMenu.Addr(), 7,
		uintptr(hMenu),
		uintptr(uFlags),
		uintptr(x),
		uintptr(y),
		uintptr(nReserved),
		uintptr(hWnd),
		uintptr(unsafe.Pointer(prcRect)),
		0,
		0)

	return uint32(ret)
}

func UnhookWinEvent(hWinHookEvent HWINEVENTHOOK) bool {
	ret, _, _ := syscall.Syscall(unhookWinEvent.Addr(), 1, uintptr(hWinHookEvent), 0, 0)
	return ret != 0
}

func WindowFromPoint(Point POINT) HWND {
	ret, _, _ := syscall.Syscall(windowFromPoint.Addr(), 2,
		uintptr(Point.X),
		uintptr(Point.Y),
		0)

	return HWND(ret)
}
