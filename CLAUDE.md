# CLAUDE.md

Project memory for Claude Code. It is loaded at the start of every session.
At the end of each step, update the "Status" section in the same pull request.

## Project

Fork of walk (Windows Application Library Kit for Go), originally
`github.com/lxn/walk`, which is no longer maintained. License: BSD 3-clause.
The `LICENSE` and `AUTHORS` files must be preserved.

- Module: `github.com/lpintes/walk`, Go 1.23 or later.
- Main package in the repository root, declarative API in `declarative/`,
  examples in `examples/`, tool in `tools/ui2walk`.

## User and communication

- Talk to the user in Slovak. Everything committed to the repository (code,
  comments, docs, commit messages, pull requests) is in English.
- The user uses a screen reader: no visualizations, diagrams or tables;
  describe everything with plain text and lists.
- Accessibility (UI Automation, MSAA) matters for this project; take extra
  care with any change touching it.

## Working in the cloud

- The container runs Linux. Code can only be built for Windows, not run.
  The user does GUI testing on Windows.
- Checks before committing:
  - `GOOS=windows GOARCH=amd64 go build ./...`
  - `GOOS=windows GOARCH=386 go build ./...`
  - `GOOS=windows GOARCH=arm64 go build . ./declarative ./tools/...`
    (examples do not build on arm64: their `rsrc.syso` files are 386 COFF
    objects)
  - `gofmt -l .` must print nothing
  - `GOOS=windows go vet ./...` (has known warnings; do not add new ones)
  - `go test ./tools/...`
  - after changing `internal/win/winmd.txt` or the generator:
    `go generate ./internal/win` and commit the regenerated files
- If a newer dependency requires Go newer than 1.23, pick an older compatible
  version (e.g. `golang.org/x/sys` v0.35.0, `github.com/microsoft/go-winmd`
  v0.0.0-20260113112744-dbc8a42468d0, the last one for Go 1.18; later
  versions need Go 1.24 or newer).
- Workflow: one logical step = one session = one pull request into `master`.

## Modernization plan

1. Go module and syntax modernization (`go.mod`, imports, `//go:build`, `any`).
2. Bring `github.com/lxn/win` into the repository as the internal package
   `internal/win` (BSD license) and drop the external dependency. Behavior
   must not change.
3. Generator for Win32 declarations from the `Windows.Win32.winmd` metadata
   (NuGet package `Microsoft.Windows.SDK.Win32Metadata`) using the parser
   `github.com/microsoft/go-winmd`. Generate only the symbols walk uses
   (about 1435) and gradually replace the hand-written code in
   `internal/win`. Macros such as `LOWORD`, `MAKEINTRESOURCE` or `FAILED` are
   not in the metadata and stay hand-written. Watch out for
   architecture-specific structs and for COM interfaces that walk implements
   itself (WebView, OLE hosting).
4. Dialogs recognized by screen readers: option A first, then option B if
   needed (see "Real dialogs for screen readers" below).
5. `RichEdit` widget.
6. Internal COM helper package (`internal/com`); move the existing WebView
   site implementations onto it.
7. `WebView2` widget; deprecate the old `WebView`.
8. Internal code modernization without public API changes.
9. Generic public API (events, models); breaking, needs a separate decision
   before it starts.

Details and rationale for steps 4 to 9 are in "Candidate next steps" below.

## Status

- Step 1: done, pull request lpintes/walk#1.
- Step 2: done, pull request lpintes/walk#2. `github.com/lxn/win` (version
  v0.0.0-20210218163916-a377121e959e) lives in `internal/win`; see its
  `README.md`.
- Step 3: in progress, done in parts.
  - Part 1: pull request lpintes/walk#4.
  - Part 1 contents: generator `tools/winmdgen` (see its package documentation
    and `internal/win/README.md`), specification `internal/win/winmd.txt`,
    generated `internal/win/zwinmd_constants.go` and
    `zwinmd_functions.go`. Metadata version 71.0.26-preview. Replaced 1035
    constants, 189 functions with their `LazyProc` variables and the 12
    `LazyDLL` variables. The Go API of `internal/win` stayed identical on
    386, amd64 and arm64 (checked by dumping every constant with type and
    value, every signature and type, and for every plain syscall wrapper the
    DLL, entry point and argument and result conversions, before and
    after).
  - Part 2: structs, pull request lpintes/walk#5. It targets the branch
    of part 1, so merge lpintes/walk#4 first (with a merge commit, not
    squash), delete its branch so that GitHub retargets lpintes/walk#5 to
    `master`, then merge lpintes/walk#5. New `struct` directive in `winmd.txt`, generated
    `internal/win/zwinmd_structs.go` with 62 structs and
    `zwinmd_layout_{386,amd64,arm64}.go`, which make the build fail if a
    generated struct's size, alignment or field offsets differ from the
    metadata. Field names and types follow lxn/win through overrides
    (for example `MSG` `hwnd=HWnd`, `SIZE` `cx=CX`). The generator computes
    the C layout from the metadata (including `#pragma pack`) and rejects
    structs Go would lay out differently. `-suggest` also proposes
    structs and checks the hand-written layout on every architecture;
    `removedecl` removes struct types. The Go API of `internal/win` stayed
    identical on 386, amd64 and arm64.
  - Structs still hand-written, with the reason: COM interfaces and
    vtables (not structs in the metadata); `NOTIFYICONDATA`,
    `TVINSERTSTRUCT`, `VARIANT` (anonymous unions); `OPENFILENAME`,
    `SHFILEINFO`, `NMTVKEYDOWN` (`#pragma pack(1)`, which Go cannot
    express); `TBBUTTON` (differs between architectures); `HDITEM`,
    `LVITEM`, `LVCOLUMN`, `NMLVDISPINFO` (lxn/win has older, shorter
    versions); `BITMAPINFO` (see "Known issues"); `BITMAPV4HEADER`,
    `BITMAPV5HEADER`, `VARIANTARG` (embedded fields); `BROWSEINFO`,
    `ENHMETAHEADER` (refer to `ITEMIDLIST` and `RECTL`, which the package
    does not define); `TOOLINFO` (the metadata calls it `TTTOOLINFOW`;
    try `struct TOOLINFO entry=TTTOOLINFOW` in a later part).
  - Next parts: GUID variables (`IID_*`, `CLSID_*`), COM interface
    vtables, functions the generator rejects today (structs or floats by
    value, 64-bit parameters on 386, architecture-specific functions such
    as `GetWindowLongPtr`), then removal of hand-written declarations walk
    does not use.
  - Procedure used for part 1, repeat it for later parts (all from the
    repository root):
    1. `go run ./tools/winmigrate/apidump internal/win ARCH > before_ARCH.txt`
       for 386, amd64 and arm64 (keep the files outside the repository).
    2. `go run ./tools/winmdgen -spec internal/win/winmd.txt -suggest .`
       prints spec lines for used symbols that can be generated without
       any change; skipped symbols and reasons go to standard error. Add
       the lines to `internal/win/winmd.txt`. The generator itself must be
       extended first for new kinds of symbols (structs, GUIDs, ...), and
       `-suggest` together with it.
    3. `go generate ./internal/win`.
    4. `go run ./tools/winmigrate/removedecl internal/win/winmd.txt internal/win`
       removes the replaced hand-written declarations; then look for
       orphaned comments (part 1 left `// Library` comments, removed by
       hand).
    5. Dump again and `diff` against step 1: there must be no difference
       on any architecture. Then run the checks above.
    `apidump` uses the sizes of the target architecture since part 2;
    dumps made by the older version differ on 386 for constants such as
    `^uintptr(0)`, so make both dumps with the same version.
- Bugs found in part 1 (see "Known issues"): the user decided to record
  them; fixing is optional. Proposed: fix `DragFinish` in a separate small
  pull request (a memory leak, low risk, test that dropping files still
  works); the `HDN_*` and `ODA_*`/`ODS_*` bugs are visual only and hard to
  test with a screen reader, so they stay recorded until someone can check
  the appearance. No GitHub issues were created for them.
- `TESTING_ON_WINDOWS.md` describes tasks for an agent on a Windows
  machine: a smoke test of step 3 parts 1 and 2 and tests for these
  bugs. Finding while writing it: the `HDN_*` and `ODS_*` bugs can be fixed without any
  change of behavior (walk effectively reacts to `HDN_ITEMCHANGEDW` and
  tests the real `ODS_SELECTED` bit; the `ODA_FOCUS` check is dead code).
  Only the `DragFinish` fix changes behavior.
- Steps 4 to 9: not started.

## Candidate next steps (analysis for steps 4 to 9)

Each item is its own step and pull request, in the order given in the
modernization plan. Step 3 (generator) goes first, because RichEdit, WebView2
and real dialogs all need new Win32/COM declarations.

### RichEdit widget

- Today there is no RichEdit widget. `TextEdit` and `LineEdit` use the plain
  `EDIT` class. `internal/win` already has `richedit.go` (messages, styles,
  `CHARFORMAT`, `PARAFORMAT`) and `richole.go` (`IRichEditOle`), unused by walk.
- Plan: new `RichEdit` widget (plus declarative counterpart) on the
  `RICHEDIT50W` class from `msftedit.dll` (load the DLL before creating the
  window). Start with plain/RTF text get and set, selection, character and
  paragraph formatting, read-only mode, change and selection events.
- Optional later: Text Object Model (`ITextDocument`, `ITextRange`) for richer
  editing; declarations should come from the generator.
- Accessibility: msftedit exposes native UI Automation including the Text
  pattern, which screen readers handle well. Verify with NVDA, JAWS and
  Narrator that caret, selection and formatting are reported.

### Modern browser widget

- `WebView` hosts the old Internet Explorer `WebBrowser` ActiveX control
  (`CLSID_WebBrowser`, MSHTML) and implements `IOleClientSite`,
  `IOleInPlaceSite`, `IOleInPlaceFrame`, `IDocHostUIHandler` and
  `DWebBrowserEvents2` itself. MSHTML still ships with Windows 10 and 11, but
  without the `FEATURE_BROWSER_EMULATION` registry setting it renders in IE7
  mode, and modern sites mostly break. It is effectively dead technology.
- Plan: new `WebView2` widget based on Microsoft Edge WebView2 (Chromium).
  Needs the WebView2 runtime (part of Windows 11, installed on most Windows 10
  machines). Environment creation is asynchronous through COM completion
  handlers that walk must implement. Avoid shipping `WebView2Loader.dll` if
  possible: either reimplement the small loader logic (find the runtime,
  call `CreateWebViewEnvironmentWithOptionsInternal` in
  `EmbeddedBrowserWebView.dll`), or evaluate `github.com/jchv/go-webview2`
  (pure Go) as a reference or dependency.
- Keep the old `WebView` for compatibility at first, mark it deprecated.
- Accessibility: WebView2 exposes Chromium's UI Automation tree; check that
  focus moves into and out of the web content with the keyboard.

### COM support and go-ole

- `github.com/go-ole/go-ole` mainly helps as a COM client: `IUnknown`,
  `IDispatch`, `VARIANT`, `SAFEARRAY` and late-bound automation calls
  (`oleutil.CallMethod`). Walk mostly needs the opposite direction:
  implementing COM interfaces in Go (OLE hosting sites, event sinks, WebView2
  handlers), where go-ole offers little. Its types would also duplicate the
  ones in `internal/win` and the generator, and the project is barely
  maintained.
- `github.com/AndyBalholm/com-and-go` (last change 2018) was reviewed too.
  Version 1 generates client call wrappers from COM interfaces written as Go
  interfaces in comments (tool `mkcomcall`); version 2 relies on C files
  compiled into the Go runtime, which stopped working with Go 1.5. It is
  client-only as well, so nothing to reuse except the idea of generated
  wrappers that return `error` instead of a raw `HRESULT`, worth considering
  for the generator in step 3.
- Recommendation: do not depend on go-ole. Instead add a small internal
  helper (for example `internal/com`) for implementing COM objects: building
  vtables with `syscall.NewCallback`, reference counting, `QueryInterface`
  dispatch, and keeping Go objects reachable while COM holds pointers to
  them. Use it for the existing WebView sites and for WebView2.

### Real dialogs for screen readers

- Today `Dialog` is an ordinary top-level window of the custom class
  `\o/ Walk_Dialog_Class \o/` with `WS_CAPTION|WS_SYSMENU` (and
  `WS_THICKFRAME` unless fixed size). Modality is simulated in
  `FormBase.Run`: the owner is disabled with `EnableWindow` and a nested walk
  message loop runs. Keyboard handling (Tab, Enter, Escape, default button)
  is walk's own code in `FormBase.handleKeyDown` and `Dialog.WndProc`.
- Screen readers recognize dialogs by the `#32770` window class, by the MSAA
  role `ROLE_SYSTEM_DIALOG` on the window object, or by the UI Automation
  `IsDialog` property. Walk dialogs have none of these, so for example NVDA
  does not announce "dialog" and does not read the dialog text when it opens.
- Option A (cheap, try first): set `ROLE_SYSTEM_DIALOG` through Dynamic
  Annotation on `OBJID_WINDOW` (the existing `Accessibility.SetRole`
  annotates `OBJID_CLIENT` only) and add `WS_EX_DLGMODALFRAME` and
  `WS_EX_CONTROLPARENT`. Must be tested with NVDA, JAWS and Narrator; UI
  Automation may still not report `IsDialog`.
- Option B (proper): create the dialog window with
  `CreateDialogIndirectParam` from an empty in-memory `DLGTEMPLATE`, so it is
  a real `#32770` window, keep walk's child widgets and layout, and keep the
  modal loop modeless-style (disabled owner plus walk's loop) so that
  `Synchronize` and walk's key handling keep working. Watch for conflicts
  between `IsDialogMessage` and `handleKeyDown`.
- Also consider wrapping `TaskDialogIndirect` (comctl32 v6) as a native,
  accessible replacement for richer message boxes. `MsgBox` already uses the
  real `MessageBox`, and the common dialogs are the real system ones.

### Code modernization

- Worth doing, but internally first and without breaking the public API in
  the same step. Candidates:
  - `unsafe.Slice` and `unsafe.String` instead of `(*[1 << 30]T)` array
    casts, and cleaner `lParam` handling; this ties in with the `go vet`
    warnings below.
  - `errors.Is`, `errors.As` and `%w` wrapping for walk errors.
  - `slices`, `maps`, built-in `min` and `max` instead of hand-written
    helpers.
  - Remove workarounds for Windows versions older than 10, if any remain.
- Public API candidates (breaking, decide separately): the eleven event files
  (`event.go`, `intevent.go`, `stringevent.go`, `keyevent.go`, ...) are nearly
  identical copies and could become a generic `Event[T]`; models and data
  binding could use generics instead of `any` and reflection.

## Known issues

- `go vet` reports 72 "possible misuse of unsafe.Pointer" warnings: 69 in
  walk, mostly `lParam` to struct pointer conversions in window procedures,
  and 3 in `internal/win` (`GlobalLock`, `SysAllocString`,
  `MAKEINTRESOURCE`). The 3 in `internal/win` were reviewed in step 3: they
  convert memory allocated by Windows or integer resource IDs, not Go
  memory, and stay as they are. `go vet` also reports "struct literal
  uses unkeyed fields" for walk and the examples (for example unkeyed
  `win.RECT` and `win.POINT` literals); these are known too.
- Bugs inherited from lxn/win, found by comparing with the metadata in
  step 3 and kept hand-written so that behavior does not change. Fixing
  them changes behavior and needs GUI testing:
  - `HDN_FIRST` is `^uint32(300)` (that is -301) instead of -300, so every
    `HDN_*` value is off by one. `HDN_ITEMCHANGING` is really
    `HDN_ITEMCHANGEDW`; `TableView` relies on it.
  - `ODA_FOCUS` is 2 (really `ODA_SELECT`, `ODA_FOCUS` is 4) and
    `ODS_CHECKED` is 1 (really `ODS_SELECTED`, `ODS_CHECKED` is 8); used by
    `ListBox` owner drawing and `models.go`.
  - `DragFinish` calls the `DragAcceptFiles` entry point, so the `HDROP` of
    dropped files is never freed.
  - `SetViewportOrgEx` returns `COLORREF` and `PostMessage` returns
    `uintptr` instead of a BOOL; `DragAcceptFiles` returns a result although
    the function has none.
- Hand-written structs whose layout differs from the metadata, found in
  step 3 part 2. None of them breaks walk today:
  - `NMTVKEYDOWN` and `NMTCKEYDOWN` are declared with `#pragma pack(1)`,
    so `Flags` is at offset 14 (386) or 26 (64-bit), not 16 or 28 as in
    Go. Walk reads only `WVKey`, which is at the right offset.
  - `BITMAPINFO.BmiColors` is a pointer in Go but an inline array of one
    `RGBQUAD` in C, so the struct is 4 bytes larger on 64-bit. Walk uses
    it with `GetDIBits` for 32-bit bitmaps without a color table; for a
    bitmap with a color table, `GetDIBits` would write past the struct.
  - `HDITEM`, `LVITEM`, `LVCOLUMN` and `NMLVDISPINFO` lack the fields of
    newer Windows versions. This is safe as long as walk does not set
    masks for the missing fields.
  - The RichEdit structs in `richedit.go` (for example `ENLINK`,
    `EDITSTREAM`, `GETTEXTEX`, `MSGFILTER`, `SELCHANGE`) are wrong on
    64-bit: `richedit.h` uses `#pragma pack(4)` there, so pointer-sized
    fields can sit at offsets that are not a multiple of 8, which Go
    cannot express with plain fields. Walk does not use them yet; step 5
    (RichEdit) must handle this, for example with byte arrays and
    accessors. `TABLEROWPARMS` and `TABLECELLPARMS` are wrong on every
    architecture.
- Constants whose value differs from the metadata only in representation
  (for example `E_NOTIMPL` as a positive untyped constant, `HWND_TOPMOST`,
  `TVI_ROOT`, `CB_ERR`) or that the metadata does not have
  (`LPSTR_TEXTCALLBACK`, `SB_SETTIPTEXT`, `TBM_GETPOS`) stay hand-written.
  `STATE_SYSTEM_VALID` is 0x7fffffff as in the current SDK headers; the
  metadata has the older value 0x3fffffff.
- The examples have no `rsrc.syso` for arm64.
