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
- If a newer dependency requires Go newer than 1.23, pick an older compatible
  version (e.g. `golang.org/x/sys` v0.35.0).
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

## Status

- Step 1: done, pull request lpintes/walk#1.
- Step 2: done, pull request lpintes/walk#2. `github.com/lxn/win` (version
  v0.0.0-20210218163916-a377121e959e) lives in `internal/win`; see its
  `README.md`.
- Step 3: not started.

## Candidate next steps (analysis, not yet scheduled)

Order is open; each item would be its own step and pull request. Step 3
(generator) should go first, because RichEdit, WebView2 and real dialogs all
need new Win32/COM declarations.

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
  and 3 in `internal/win` (`kernel32.go`, `oleaut32.go`, `win.go`). Review
  them in step 3.
- The examples have no `rsrc.syso` for arm64.
