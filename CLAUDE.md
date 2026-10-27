# CLAUDE.md

Project memory for Claude Code. It is loaded at the start of every session.
At the end of each step, update the "Status" section in the same pull request.

## Project

Fork of walk (Windows Application Library Kit for Go), originally
`github.com/lxn/walk`, which is no longer maintained. License: BSD 3-clause.
The `LICENSE` and `AUTHORS` files must be preserved.

- Module: `github.com/lpintes/walk`, Go 1.23 or later.
- Main package in the repository root, declarative API in `declarative/`,
  examples in `examples/`, tool in `tools/ui2walk`. Internal packages:
  `internal/win` (Win32 declarations) and `internal/com` (COM objects
  implemented in Go).

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
  - `GOOS=windows GOARCH=arm64 go build ./...`
  - `gofmt -l .` must print nothing
  - `GOOS=windows go vet ./...` (only the 3 known warnings in
    `internal/win`; do not add new ones)
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
10. Live regions and screen reader announcements (queued 2026-10-10).
11. MDI windows (queued 2026-10-10).
12. Shell integration (queued 2026-10-13): the folder picker of the
    system file dialog first, then known folders, Recycle Bin, shortcuts
    and the taskbar. Needs COM client methods from the generator.
13. UI Automation client as a public package `uia` (started 2026-10-18).
14. Small system features (queued 2026-10-26): global hotkeys, single
    instance, power and session events, system settings (high contrast,
    screen reader flag, dark mode), more clipboard formats.

Steps 12 and 13 come from the review of Windows API areas outside the
GUI, `windows-api-oblasti.md` in the project files (in Slovak).

Details and rationale for steps 4 to 11 are in "Candidate next steps" below.

## Status

- Step 1: done, pull request lpintes/walk#1.
- Step 2: done, pull request lpintes/walk#2. `github.com/lxn/win` (version
  v0.0.0-20210218163916-a377121e959e) lives in `internal/win`; see its
  `README.md`.
- Step 3: done in three parts, merged and tested on Windows.
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
  - Structs still hand-written, with the reason: `NOTIFYICONDATA`,
    `TVINSERTSTRUCT`, `VARIANT` (anonymous unions); `OPENFILENAME`,
    `SHFILEINFO`, `NMTVKEYDOWN` (`#pragma pack(1)`, which Go cannot
    express); `TBBUTTON` (differs between architectures); `HDITEM`,
    `LVITEM`, `LVCOLUMN`, `NMLVDISPINFO` (lxn/win has older, shorter
    versions); `BITMAPINFO` (see "Known issues"); `BITMAPV4HEADER`,
    `BITMAPV5HEADER`, `VARIANTARG` (embedded fields); `ENHMETAHEADER`
    (refers to `RECTL`, which the package does not define).
  - Part 3: pull request lpintes/walk#6. New directives `guid`
    (26 GUID variables in `zwinmd_guids.go`) and `interface` (17 COM
    vtable structs and interface pointer structs in
    `zwinmd_interfaces.go`; the methods calling through the vtables stay
    hand-written). Functions may take generated structs of 1, 2 or 4
    bytes by value (`AlphaBlend`) and may name a `fallback` function for
    architectures where they do not exist (`GetWindowLongPtr` and
    `SetWindowLongPtr` call `GetWindowLongW` and `SetWindowLongW` on 386;
    their `LazyProc` variables are in `zwinmd_functions_GOARCH.go`).
    Pointers to undefined structs such as `ITEMIDLIST` work with a type
    override (`SHBrowseForFolder`, `SHGetPathFromIDList`, `BROWSEINFO`).
    `TOOLINFO` is generated from `TTTOOLINFOW`. The Go API stayed
    identical on 386, amd64 and arm64. Then `tools/winmigrate/prune`
    removed the 4883 hand-written declarations no package of the module
    uses on any architecture (16 files, among them `richedit.go`,
    `richole.go` and `tom.go`); the hand-written code of `internal/win`
    went from about 11300 to about 2000 lines, and `apidump` only lost
    lines.
  - Still hand-written by design: COM interface methods (except for
    interfaces generated with `methods` since step 12), macros and
    helpers (`LOWORD`, `MAKEINTRESOURCE`, `BoolToBOOL`, ...), functions
    with a fallback for older Windows versions (`GetDpiForWindow`, ...),
    functions returning pointers (`GlobalLock`, `LockResource`), the
    function with a known bug (`DragAcceptFiles`) and
    deliberately different bodies (`OleInitialize`, `GdiplusStartup`;
    `SHParseDisplayName` was one until step 12 removed it), and the
    constants listed under "Known issues". New Win32 symbols go into
    `winmd.txt`, not into hand-written code.
  - Procedure used for parts 1 to 3, repeat it when replacing more
    hand-written declarations (all from the repository root):
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
    `apidump` uses the sizes of the target architecture since part 2 and
    prints GUID values and drops conversions that do not change a value
    since part 3; make both dumps with the same version.
    To remove declarations walk no longer uses:
    `go run ./tools/winmigrate/prune internal/win`; the dump after it may
    only lose lines.
- Bugs found in part 1 (see "Known issues"): the user decided to record
  them; fixing is optional. `DragFinish` is fixed: it is generated now and
  calls `shell32!DragFinish` instead of `DragAcceptFiles`, so the `HDROP`
  of dropped files is freed. Tested on Windows 11 (386 and amd64) with a
  posted `WM_DROPFILES` (`GlobalFlags` reports the handle freed) and with
  a real OLE drag of shell files from another process (amd64). The `HDN_*`
  bug is fixed without a change of behavior: `HDN_FIRST` and
  `HDN_ITEMCHANGING` are gone, `HDN_ITEMCHANGED` (0xFFFFFEBF, the value
  `TableView` always reacted to) is generated, and `TableView.lvWndProc`
  uses it. Tested on Windows 11 (386 and amd64) with a `TableView` with a
  frozen column: for `LVM_SETCOLUMNWIDTH` and a header divider drag (mouse
  messages sent to the header window) on both headers, the header sends
  `HDN_ITEMCHANGINGW` (0xFFFFFEC0) and `HDN_ITEMCHANGEDW`, `updateLVSizes`
  runs once right after the latter, before and after the fix, and the
  frozen and normal list views stay adjacent. The `ODS_*`/`ODA_*` bug is
  fixed without a change of behavior: `ODS_CHECKED` and `ODA_FOCUS` are
  gone, `ODS_SELECTED` (1, the bit walk always tested) is generated,
  `ListBox` and `ListItemStyle.stateID` use it, and the dead `ODA_FOCUS`
  check in `ListBox.WndProc` is removed. Tested on Windows 11 (386 and
  amd64) with an owner drawn `ListBox` (selection by mouse and keyboard
  messages, focus moved away and back, pixels read with `GetPixel`):
  the selected item gets `LISS_SELECTED` with focus and
  `LISS_SELECTEDNOTFOCUS` without, the others `LISS_NORMAL`, with the
  same results before and after the fix. No GitHub issues were created
  for these bugs.
- Task 1 of `TESTING_ON_WINDOWS.md` (smoke test of step 3, 386 and amd64,
  also with NVDA on amd64) passed apart from the inherited problems listed
  under "Known issues".
- `TESTING_ON_WINDOWS.md` describes tasks for an agent on a Windows
  machine: a smoke test of step 3 (all three parts) and tests for these
  bugs. Finding while writing it: the `HDN_*` and `ODS_*` bugs can be fixed without any
  change of behavior (walk effectively reacts to `HDN_ITEMCHANGEDW` and
  tests the real `ODS_SELECTED` bit; the `ODA_FOCUS` check is dead code).
  Only the `DragFinish` fix changed behavior.
- The test programs of these tasks are kept in `tests/_wingui` with
  PowerShell scripts that build and run them and smoke test every example
  (see its `README.md`); rerun them on Windows after changes to
  `internal/win`, `internal/com` or the affected widgets. The leading underscore keeps
  them out of `./...`. Run the automated tests in Windows Sandbox with
  `tests/_wingui/sandbox/sandbox.ps1 -Minimize` (they steal the focus of
  the host desktop); only the manual screen reader tests run on the host.
- Step 6: done, pull request lpintes/walk#7. Done before steps 4 and 5
  because it needs no live testing while it is written.
  - Task 5 of `TESTING_ON_WINDOWS.md` passed on Windows 11 (386 and
    amd64): the `com` unit tests and all checks of `tests/_wingui/webview`
    pass; `webview_events` logs its events, opens the native context menu
    and exits with code 0 when closed with Alt+F4 after loading or with
    `WM_CLOSE` during loading. NVDA showed only the known problems. The
    webview test first timed out because the `WebBrowser` control reports
    the local path, not the `file:///` URL, in `Navigating`, `Navigated`
    and `DocumentCompleted`; the test now accepts both.
  - New package `internal/com` (see its package documentation):
    `NewVTable` takes a generated `win.*Vtbl` struct with the method
    callbacks (fields by name, so the order cannot be wrong; a missing
    method panics) and the interface IDs `QueryInterface` answers with
    it; `NewObject` builds an object from vtables and a Go value that
    implements it. Methods take `this *com.This` as first parameter and
    reach the Go value with `this.Object().Impl()`. Reference counting
    is real; while the count is above zero the memory COM points to is
    pinned with `runtime.Pinner` and the object stays reachable through
    a package map, so COM never holds pointers to memory the garbage
    collector may free. Unit tests in `com_test.go` run only on Windows.
  - `WebView` uses it: one site object (`IOleClientSite`,
    `IOleInPlaceSite`, `IDocHostUIHandler`, `DWebBrowserEvents2` sink)
    and one frame object (`IOleInPlaceFrame`). Gone are the structs
    embedded in `WebView`, the pointer arithmetic that found the
    `WebView` from an interface pointer, and the rule that the site must
    be the first field after `WidgetBase`. Intended behavior changes:
    `Dispose` unadvises the events sink and releases the objects;
    `GetWindowContext` adds a reference to the frame it returns; the
    frame answers `QueryInterface` for `IUnknown`, `IOleWindow`,
    `IOleInPlaceUIWindow` and `IOleInPlaceFrame` instead of
    `E_NOTIMPL`; the sink's `GetTypeInfoCount`, `GetTypeInfo` and
    `GetIDsOfNames` take their real parameters (they took one, which
    would unbalance the stack on 386). `go vet` lost 6 "possible misuse
    of unsafe.Pointer" warnings (now 63 in walk).
  - New in `internal/win`: GUIDs `IID_IOleWindow`, `IID_IOleInPlaceUIWindow`
    and `IID_IOleInPlaceFrame`, the hand-written
    `IConnectionPoint.Unadvise` and `E_POINTER`.
  - Test programs: `tests/_wingui/webview` and the `com` entry of the
    scripts (unit tests of `internal/com`).
  - Next users of `internal/com`: WebView2 handlers (step 7) and the
    `IRichEditOleCallback` of RichEdit (step 5) if needed.
- Step 8, part 1: pull request lpintes/walk#9, mechanical and
  behavior-preserving, checked with `go vet`, staticcheck 2025.1.1 and
  the modernize analyzer of gopls v0.18.1 (run them with
  `GOOS=windows`; install them with the Go 1.24 toolchain).
  - Removed unused and unreachable code; kept on purpose: the stopwatch
    debugging helpers, `ImageView.applyDPI` and
    `WebView.inPlaceActiveObjectSetFocus` (look like unfinished
    features; the latter may help with the WebView focus problem) and
    the `setValueFunc` methods (needed by unexported interface
    methods).
  - Field names in all struct literals of types from other packages;
    `0 == x` comparisons turned around; range over int, `slices`,
    `maps`, built-in `min` and `max` (the helpers `maxi` and `mini` are
    gone).
  - Hand-written `syscall.Syscall*` calls in `internal/win` use
    `syscall.SyscallN` with exactly the old argument count (apidump
    unchanged). `win.StringToUTF16` and `win.StringToUTF16Ptr` replace
    the deprecated syscall functions with identical behavior, including
    the panic for strings containing NUL.
  - `ptrFromUintptr[T]` in `util.go` converts `lParam` and
    `GetWindowLongPtr` values to pointers in one documented place; huge
    array casts became `unsafe.Slice` of the real length. `go vet` now
    reports only the 3 reviewed warnings in `internal/win`.
  - Fixed on the way: `TableView.RestoreState` parsed `LastSeenDate`
    with the wrong layout (column state was not pruned as intended);
    the `LoadIconWithScaleDown` failure check in `Icon` never fired; the
    `listbox_ownerdrawing` example goroutine never stopped; a
    transparent bitmap above 4 megapixels and a TableView or TreeView
    text above 264 characters with a larger buffer panicked.
  - The trace patches in `tests/_wingui/hdn` and `ods` were updated.
  - Tested on Windows 11 (386 and amd64) with `tests/_wingui`: `build.ps1`,
    `run.ps1` (all tests but `olednd`) and `examples.ps1` pass, with only
    the known problems (`SetRole` on 386, `ListBox` `hoverIndex`,
    `progressindicator` nil layout).
  - Left for later (each changes behavior or public text): the
    deprecated 96 DPI APIs still used by `declarative` (`NewCustomWidget`,
    `NewImageList`, `Resources.Bitmap`) and the examples; capitalized
    error strings in walk and `internal/win`; dot imports in the
    examples; call sites that could return an error instead of
    panicking on strings with NUL.
- Step 8, part 2: pull request lpintes/walk#10, errors.
  - `*Error` has an `Unwrap` method returning the wrapped error, so
    `errors.Is` and `errors.As` look through walk errors (a public
    addition, nothing removed or changed). `ToolTipErrorPresenter`
    finds a `ValidationError` with `errors.As`, so a wrapped one still
    shows its title and message; unwrapped ones behave as before.
  - `%w` instead of `%s`/`%v` for wrapped causes (`declarative`
    invalid expression panic, the tools), `errors.Is(err, io.EOF)` in
    `prune`, and log calls pass the error instead of `err.Error()`;
    all texts are unchanged.
  - Kept on purpose: the exact type assertions in `error.go`
    (`wrapErr` must not unwrap an `*Error` wrapped by someone else).
    `lastError` and `errorFromHRESULT` now carry the Windows code
    (step 8, part 3).
  - Cloud checks only; no Windows test needed beyond a smoke run, as
    the only runtime change is the `errors.As` in the presenter.
- Step 8, part 3: pull request lpintes/walk#26, merged; `go test .`
  passed on Windows 11 (386 and amd64). Windows error codes in walk
  errors (the user approved
  the change of `Inner` on 2026-10-26). `lastError` and
  `errorFromHRESULT` return an `*Error` whose inner error is the code as
  a `syscall.Errno` (`newErrWithCode`): the `GetLastError` value, or the
  32-bit value of the HRESULT (`hresultErrno`, for example
  `syscall.Errno(0x80070005)`). The same for the `WebView2` HRESULT
  errors (the loader uses the unexported `codeError`) and `ShellOpen`.
  Message texts are unchanged; `Inner` returns the code instead of nil,
  and a code of 0 is not wrapped. `CommDlgExtendedError` codes are not
  Win32 codes and stay text only. Unit tests in `error_test.go` (run
  `go test .` on Windows).
- Step 10: draft pull request lpintes/walk#11. Task 6 of
  `TESTING_ON_WINDOWS.md` passed on Windows 11 with NVDA 2026.2:
  `run.ps1 -Only part3,liveregion` on 386 and amd64, and the manual
  test by ear on amd64 (the `LineEdit` also on 386). Narrator and JAWS
  not tested yet. Findings fixed during the test:
  - NVDA handles `EVENT_OBJECT_LIVEREGIONCHANGED` through MSAA and
    speaks the MSAA name of the object, not its text or value, with
    normal priority (it ignores the UIA `LiveSetting`).
  - `StatusBar`: the window has no name, so NVDA said nothing.
    `StatusBarItem.SetText` raises the event for the part (MSAA child
    index+1, whose name is the text); UI Automation delivers it on the
    part element. `updateText` no longer raises it, because it also
    runs for every item on a DPI change.
  - `LineEdit` and `TextEdit`: NVDA read only the name (the label),
    and UI Automation does not pass the event on for edit controls at
    all. `NotifyLiveRegionChanged` announces their text with
    `Announce` instead (assertive: `AnnounceImportantMostRecent`,
    polite: `AnnounceAll`) and raises the event only if that fails.
  - Several quick `Announce` calls: NVDA spoke only the first two
    (its UIA event limiter coalesces notifications with the same
    sender, kind, processing and activity ID and keeps the oldest
    queued one); fixed with a sequence number in the activity ID.
  - A notification raised with the host provider from
    `UiaHostProviderFromHwnd` returns `S_OK` but reaches no client, so
    the own provider is needed.
  - Fixed the inherited 386 bug: `SetHwndProp` passes the 16-byte
    `VARIANT` by value as four stack words (`oleacc_32.go`), so
    `SetRole`, `SetState` and everything through `accSetPropertyInt`
    work on 386. `part3` lost its KNOWN case; `run.ps1` checks the
    annotated MSAA role too.
  - Part A: `AccLiveSetting`, `Accessibility.SetLiveSetting`,
    `LiveSetting` and `NotifyLiveRegionChanged`; declarative
    `Accessibility.LiveSetting`. `WindowBase.setText` (every `SetText`
    of a window; `static` raises it after updating its inner control)
    and `StatusBarItem.SetText` raise `EVENT_OBJECT_LIVEREGIONCHANGED`
    when a live setting is set. The property is cleared on dispose.
  - Part B: `walk.Announce(window, text, processing)` with
    `AnnouncementProcessing` (the `NotificationProcessing` values). It
    raises `UiaRaiseNotificationEvent` (kind Other, activity ID
    `Walk.Announce.<n>` with a sequence number, because NVDA coalesces
    quick notifications with the same activity ID and then speaks only
    the first two) from a minimal `IRawElementProviderSimple` made
    with `internal/com` (server side, COM threading, host provider from
    `UiaHostProviderFromHwnd`); it returns an error before Windows 10
    1709 and does nothing when `UiaClientsAreListening` is false.
  - New in `internal/win` (generated): `uiautomationcore.dll`,
    `UiaRaiseNotificationEvent`, `UiaHostProviderFromHwnd`,
    `UiaClientsAreListening`, `IRawElementProviderSimple` and its IID,
    `LiveSetting_Property_GUID`, `EVENT_OBJECT_LIVEREGIONCHANGED` and
    the `NotificationKind`, `NotificationProcessing` and
    `ProviderOptions` constants; hand-written
    `IRawElementProviderSimple.Release` and
    `UiaRaiseNotificationEventAvailable`.
  - Test programs: `tests/_wingui/liveregion` (`app` and a UI
    Automation client `client` that plays the screen reader).
  - Not done yet: using `Announce` for validation errors (behavior
    change, decide after the screen reader test).
- Review of the 27 open upstream lxn/walk pull requests (2026-10-11):
  `opened-prs.md` (in Slovak). Recommended: #838 (file dialog filter
  `|` bug), #771 (TreeView panic on root item insert), #552 (NotifyIcon
  double click, extended with keyboard activation `NIN_SELECT` and
  `NIN_KEYSELECT`), #393 (`PasswordMode` property) and #810 (README
  grammar).
- Ported from open lxn/walk pull requests, pull request lpintes/walk#13
  (one commit each; task 7 of `TESTING_ON_WINDOWS.md`, test program
  `tests/_wingui/upstream`). Task 7 passed on Windows 11 (386 and amd64):
  `run.ps1` and `examples.ps1` with only the known problems, and the
  manual test with NVDA on amd64. Enter and Space on the icon after
  Windows+B each publish one `Activated`, Shift+F10 and the application
  key open the menu; NVDA does not read the polite live region of the
  test window then, because that window is in the background, so the
  events were checked in the program's standard output. After a menu
  command focus goes to the application's window (`SetForegroundWindow`
  before the menu, unchanged from before).
  - #838: `FileDialog` filter compared only the low byte of each UTF-16
    unit with `|` (broke "ż" U+017C, Korean).
  - #771: `TreeView` panicked on `ItemInserted` of a root item; root items
    now go to their model position (`RootCount`/`RootAt`).
  - #552 extended: `NotifyIcon.Activated` (`NIN_SELECT` and
    `NIN_KEYSELECT`, so keyboard users can activate the icon),
    `NotifyIcon.MouseDoubleClick`; a keyboard context menu (shell
    `WM_CONTEXTMENU`, sent only for Shift+F10 and the application key with
    `NOTIFYICON_VERSION` 3) opens next to the icon
    (`Shell_NotifyIconGetRect`), and `WM_NULL` is posted after
    `TrackPopupMenuEx`. New in `internal/win`: `NIN_SELECT`, `NINF_KEY`,
    `TPM_VERTICAL`, `WM_NULL`, `Shell_NotifyIconGetRect` (generated) and
    hand-written `NOTIFYICONIDENTIFIER` (`#pragma pack(1)` on 386).
  - #393: `LineEdit.PasswordMode` is a bindable property (declarative
    field is now `Property`); a change redraws and raises
    `EVENT_OBJECT_STATECHANGE`.
  - Three doc comments naming the wrong method (found by #456).
  - #810: README grammar.
- Step 12, part 1: pull request lpintes/walk#14. Task 8 of
  `TESTING_ON_WINDOWS.md` passed on Windows 11 (386 and amd64):
  `run.ps1` (`part3` with `browse`) and `examples.ps1` with only the
  known problems, and the manual test with NVDA on amd64 (the dialog
  behaves like the standard one, typing a path in the address bar
  navigates, Select Folder returns the folder, Escape returns `false`
  with no error). Without `InitialDirPath` the dialog starts in the
  folder the shell chooses (Documents on the test machine).
  - Generator: the `methods` option of the `interface` directive
    generates Go methods calling through the vtable (`-METHOD` skips a
    method, `METHOD.PARAM:TYPE` overrides a parameter type); interfaces
    the Go package does not define need a type override, like undefined
    structs. Generated for `IFileDialog` and `IShellItem`; the other
    interfaces keep their hand-written methods.
  - `FileDialog.ShowBrowseFolder` uses `IFileDialog` with
    `FOS_PICKFOLDERS | FOS_FORCEFILESYSTEM` instead of
    `SHBrowseForFolder`. Intended changes: `InitialDirPath` is the start
    folder, not the root of the tree; `Title` is the window caption.
    `SHBrowseForFolder`, `SHGetPathFromIDList`, `SHParseDisplayName` and
    `BROWSEINFO` are gone. New hand-written macro `HRESULT_FROM_WIN32`.
  - Open and save already show the modern dialog: `GetOpenFileName`
    without hooks or templates uses it since Windows Vista.
- Fixed inherited problem (2026-10-15, committed to `master`): a
  `MainWindow` or dialog without a layout no longer panics in
  `SetVisible`. `FormBase.startLayout` skips the layout and only sizes
  the client composite to the client area, and `WM_WINDOWPOSCHANGED`
  does the same on resize; the children keep the bounds the program
  gave them. Tested on Windows 11: `examples/progressindicator` (386)
  passes `examples.ps1` (composite 598 x 300 like the client area), and
  a `MainWindow` without a layout (amd64) shows its button and keeps
  the composite equal to the client area after a resize.
- Fixed inherited problem (2026-10-15, committed to `master`): `ListBox`
  starts with `style.hoverIndex` -1, so item 0 is no longer drawn in the
  hot state until the mouse leaves the list once. The `ods` test lost
  its KNOWN case. Tested on Windows 11 (386 and amd64): `run.ps1` and
  `examples.ps1` pass with no KNOWN line.
- Step 12, part 2: pull request lpintes/walk#15. Task 9 of
  `TESTING_ON_WINDOWS.md` (test program `tests/_wingui/shell`) passed
  on Windows 11 (386 and amd64): `run.ps1 -Only shell` passes every
  check (no known folder is redirected on the test machine;
  `ProgramFiles` is `Program Files (x86)` on 386), and the test items
  were in the Recycle Bin with their original location and could be
  restored (checked with `Shell.Application`, namespace 10). The
  optional test with an item that cannot be recycled was not run.
  - `KnownFolderPath(KnownFolder)` with 15 known folders
    (`SHGetKnownFolderPath`); `AppDataPath` and the other CSIDL
    functions in `path.go` are unchanged.
  - `MoveToRecycleBin(owner, paths...)` with `IFileOperation` and
    `FOF_ALLOWUNDO | FOFX_RECYCLEONDELETE | FOF_NOCONFIRMATION |
    FOF_WANTNUKEWARNING | FOF_NOERRORUI | FOF_SILENT`: no dialogs except
    the question before a permanent delete; aborting returns an error.
  - `ShellLink` with `LoadShellLink` and `Save` (`IShellLinkW`,
    `IPersistFile`); `Hotkey` is a `Shortcut`, whose modifiers have the
    `HOTKEYF_*` values.
  - COM calls run in `withCOM` (`shell.go`): locked OS thread and
    `OleInitialize`, so the functions work from any goroutine.
  - Generator fix: a pointer to `BOOL` is `*BOOL`, not `*bool` (Windows
    writes 4 bytes); no existing declaration was affected. GUID
    variables of type `syscall.GUID` get keyed literals, as `go vet`
    wants for types of other packages.
- Step 12, part 3 (last part of step 12): pull request
  lpintes/walk#18, merged. Task 10 of `TESTING_ON_WINDOWS.md` passed on Windows
  11: `run.ps1 -Only shell,part3` and `examples.ps1` on 386 and amd64,
  and the manual test with NVDA on amd64 (`shell.exe manual`: the text
  file opens in its editor, the web page in the browser, and Explorer
  opens the folder with the file selected). The flashing cannot be
  judged by ear, so a throwaway program checked it on 386 and amd64: a
  hidden window registered with `RegisterShellHookWindow` got
  `HSHELL_FLASH` (0x8006) for a minimized form after `RequestAttention`
  (5 messages in 5 seconds) and none without the call.
  - `ShellOpen(owner, target)`: `ShellExecuteEx` with the default verb
    for a file, folder or URL, with `SEE_MASK_NOASYNC` (the launch is
    finished before `withCOM` uninitializes COM, also on a goroutine
    without a message loop) and `SEE_MASK_FLAG_NO_UI` (errors such as a
    missing file are returned, not shown in a message box; without it
    the test hung on that message box). `SE_ERR_NOASSOC` gets its own
    message, other failures carry the `GetLastError` text.
    `SHELLEXECUTEINFO` is hand-written (anonymous union of `hIcon` and
    `hMonitor`); 60 bytes on 386 and 112 on amd64, as in the SDK.
    `ShowInFolder(path)`: `SHParseDisplayName` and
    `SHOpenFolderAndSelectItems` (generated again, with `uintptr` for
    the `ITEMIDLIST` pointers).
  - Taskbar: the taskbar features walk needs were already there
    (`ProgressIndicator` with progress state, value and overlay icon).
    Fixed: `ITaskbarList3.HrInit` was never called (the documentation
    requires it first) and the object was never released; it is now
    released in `FormBase.Dispose`; after that `SetState`,
    `SetCompleted` and `SetOverlayIcon` return an error instead of
    calling through a nil pointer. New `FormBase.RequestAttention`
    flashes the taskbar button until the form is activated
    (`FlashWindowEx`); not added to the `Form` interface, so it is not a
    breaking change.
  - Not done: thumbnail toolbar buttons and jump lists (larger, little
    use for screen reader users).
- Step 13, part 1: pull request lpintes/walk#19. Task 11 of
  `TESTING_ON_WINDOWS.md` (test program `tests/_wingui/uia` with its
  `dump` mode) passed on Windows 11: `run.ps1 -Only uia` on 386 and
  amd64 with every check PASS and none skipped (the names set with
  `Accessibility.SetName` are visible through UI Automation), and the
  manual test on amd64: `uia.exe dump` of a foreign MDI application
  printed a readable control view tree whose names and control types
  match what NVDA says.
  - New public package `uia`: `New` connects (`CUIAutomation8`, Windows 8
    or later); `Root`, `ElementFromHandle`, `ElementFromPoint`,
    `FocusedElement`; `Element` with typed getters for the common
    properties, `LabeledBy`, `SetFocus`, `Equal`, `FindFirst` and
    `FindAll`; `Condition` values (`True`, `False`, `Property`,
    `PropertyMatch`, `ByName`, `ByAutomationID`, `ByClassName`,
    `ByControlType`, `And`, `Or`, `Not`), built into COM conditions only
    for the call, so they need no release; `TreeWalker` for the
    control, content and raw views and for a condition.
  - Threads: every call runs on one locked OS thread of the
    `Automation` in the multithreaded apartment (a channel of
    functions), so any goroutine may call, also walk's UI goroutine,
    except about its own windows (the call waits for that thread's
    messages; documented). Elements are released with `Release` or, as
    a fallback, by a finalizer that queues the release for the
    Automation thread. Errors wrap `ErrElementNotAvailable`
    (`UIA_E_ELEMENTNOTAVAILABLE`), `ErrReleased` or `ErrClosed`; other
    HRESULTs become `syscall.Errno`.
  - New in `internal/win` (generated): `IUIAutomation`,
    `IUIAutomationElement`, `IUIAutomationElementArray`,
    `IUIAutomationTreeWalker` and `IUIAutomationCondition` with
    `methods` (methods needing cache requests, event handlers,
    `SAFEARRAY` conversions or by-value structs skipped),
    `CLSID_CUIAutomation8`, `IID_IUIAutomation`, the control type,
    property, `TreeScope` and `PropertyConditionFlags` constants,
    `CoInitializeEx`, `CoUninitialize` and `VariantClear`.
    Hand-written: `ElementFromPoint`, `CreatePropertyCondition` and
    `CreatePropertyConditionEx` (`POINT` and `VARIANT` by value; split
    into stack words on 386, a register or a pointer to a copy on
    64-bit) and `VARIANT.SetBSTR` and `SetBool`.
  - Planned next parts: control patterns (Invoke, Value, Toggle,
    Selection, ExpandCollapse, Text), then events (focus, property and
    structure changes, notifications) with handlers made with
    `internal/com`; then the liveregion test client can use the package.
- Examples on every architecture (2026-10-18, committed to `master`):
  each example has `rsrc_windows_{386,amd64,arm64}.syso` instead of the
  386-only `rsrc.syso`, so `go build ./...` works on arm64 and amd64
  executables embed the manifest too; `build.ps1` no longer needs an
  overlay. All examples share one manifest. Regenerate with
  `go run github.com/akavel/rsrc@v0.10.2 -manifest <name>.exe.manifest
  -arch <arch> -o rsrc_windows_<arch>.syso` in the example directory.
- Step 13, part 2: pull request lpintes/walk#20. Task 12 of
  `TESTING_ON_WINDOWS.md` (the same test program `tests/_wingui/uia`
  with more widgets) passed on Windows 11: `run.ps1 -Only uia` on 386
  and amd64 with every check PASS (`SetRangeValue` included, so the
  `float64` passing works), and the manual test on amd64: `uia.exe dump`
  of the mouse properties dialog (`main.cpl`) printed the toggle,
  expand and collapse, selection and range values NVDA reports. The
  test first failed: UI Automation exposes a multiline `EDIT` (walk
  `TextEdit`) as ControlType Document, not Edit; the test expects that
  now.
  - Control patterns as methods of `Element`, each getting the pattern
    with `GetCurrentPatternAs` for the call and releasing it; an
    element without it returns an error wrapping
    `ErrPatternNotSupported`. Invoke, Value, RangeValue (with the struct
    `RangeValue`), Toggle (`ToggleState`), ExpandCollapse
    (`ExpandCollapseState`), SelectionItem, Selection, Text (`Text`,
    `SelectedText`), Window (`CloseWindow`, `WindowVisualState`,
    `SetWindowVisualState`, `IsModal`), ScrollItem and the MSAA
    properties of LegacyIAccessible (`DefaultAction`,
    `DoDefaultAction`, `Description`).
  - New in `internal/win` (generated): the eleven pattern interfaces
    with `methods`, `IUIAutomationTextRange` and
    `IUIAutomationTextRangeArray`, their IIDs, the pattern IDs and the
    `ToggleState`, `ExpandCollapseState` and `WindowVisualState`
    constants. Hand-written: `IUIAutomationRangeValuePattern.SetValue`
    (`float64` by value: two stack words on 386, a register on amd64,
    where `syscall.SyscallN` copies the arguments to the XMM registers
    too; `E_NOTIMPL` on arm64, where `SyscallN` sets no floating-point
    registers).
- Step 13, part 3: pull request lpintes/walk#21, first stacked on
  part 2. Deleting the branch of part 2 with
  `gh pr merge --delete-branch` closed this pull request instead of
  retargeting it; it had to be reopened with the base branch pushed
  again, retargeted to `master` with `gh pr edit --base`, and the base
  branch deleted afterwards. For stacked pull requests, retarget the
  next one to `master` before deleting the merged branch.
  Task 13 of `TESTING_ON_WINDOWS.md` (`tests/_wingui/uia`) passed on
  Windows 11: `run.ps1 -Only uia` on 386 and amd64 with every check
  PASS and no hang, twice per architecture. The focus checks used to
  print SKIP when the window got the foreground only a moment after
  the check; the test now waits up to three seconds for it (asking once
  with `SetForegroundWindow`) and fails without it, and no run skipped
  them any more. Two expectations of the test were
  wrong and were fixed; both are Windows behavior:
  - The proxies of Win32 controls raise no IsEnabled property change
    for `EnableWindow` (not even with the root as scope). The test
    checks the ToggleState change of a check box instead.
  - They send the new value as `VT_EMPTY` for most properties
    (ToggleState, Value, RangeValue; Name is an exception), so the Go
    value is nil; `OnPropertyChanged` documents this.
  - The live region changed event of a walk `Label` comes from the
    label window (`Walk_Static_Class`, a Pane), which has the live
    setting, not from the Static control (Text) inside it.
  - Events: `Automation.OnFocusChanged`, `Element.OnEvent` (18
    `EventID` values, among them `EventInvoked`, `EventWindowOpened`,
    `EventTextChanged` and `EventLiveRegionChanged`),
    `OnPropertyChanged` (new value converted to the Go type of the
    property), `OnStructureChanged` (`StructureChange`) and
    `OnNotification` (`Notification`, needs `IUIAutomation5`, Windows 10
    1709). Each returns a `Subscription`.
  - Threads: the handlers are COM objects made with `internal/com`
    (vtables created once); UI Automation calls them on its own threads.
    They only wrap the sender in an `Element` (with `AddRef`) and queue
    the call; a goroutine per subscription calls the Go function, so the
    function may call the package and even `Close` its own
    subscription, and `Remove*EventHandler` on the package thread never
    waits for a handler that waits for that thread. `Close` of the
    Automation removes every handler still registered.
  - New in `internal/win`: the five handler interfaces (vtables only)
    and `IUIAutomation5` (vtable only) with their IIDs, the
    `Add*`/`Remove*` handler methods of `IUIAutomation` (generated, with
    `unsafe.Pointer` for the cache request), the event IDs, the
    `StructureChangeType` and remaining `NotificationKind` constants,
    `VT_R8` and other `VARTYPE` values. Hand-written:
    `IUIAutomation5.AddNotificationEventHandler`,
    `RemoveNotificationEventHandler` and `Release`, and
    `VARIANT.Double`. In `uia`, `HandlePropertyChangedEvent` takes the
    `VARIANT` as four stack words on 386 and as a pointer on 64-bit.
- Step 5: pull request lpintes/walk#22, merged. Task 14 of
  `TESTING_ON_WINDOWS.md` (test program `tests/_wingui/richedit` with a
  `manual` mode) passed on Windows 11 after the fixes below:
  `run.ps1 -Only richedit` on 386 and amd64 with every check PASS, and
  the manual test with NVDA 2026.2 on amd64 (text by arrows and say all,
  formatting with NVDA+F, Enter and a real mouse click on a link, Tab
  and Shift+Tab, Ctrl+B, Ctrl+I and Ctrl+U of the test program). NVDA
  does not say the bullets (it reads the control through TOM). Narrator
  and JAWS not tested. Findings fixed during the test:
  - `LinkClicked` by mouse never fired when text was selected: a click
    on a link does not change the selection, so the old check "empty
    selection at `WM_LBUTTONUP`" was wrong. The widget now remembers the
    link and the selection at `WM_LBUTTONDOWN` and fires at
    `WM_LBUTTONUP` if both are unchanged (a drag that selects text does
    not fire). msftedit reports Enter on a link as the same button pair,
    in read-only and editable controls; no `WM_KEYDOWN` or `WM_CHAR`
    came on Windows 11 (those cases are kept for other versions).
  - Tab into the control selected the whole document (`DLGC_HASSETSEL`
    copied from `TextEdit`); removed, the caret stays where it was.
  - After Alt+Tab with the Windows 11 task switcher NVDA said only the
    form title and ignored the arrows when a `RichEdit` had the focus
    (an `EDIT` or a button was fine). Cause, from the NVDA source
    (`IAccessibleHandler/orderedWinEventLimiter.py`) and a WinEvent log:
    while NVDA waits for the foreground to change it keeps only the last
    four focus and foreground events, the switcher fires a late
    foreground and two focus events of its own, and msftedit raises a
    second focus event (positive object ID, its UI Automation proxy),
    which pushes the control's real focus event out. Workaround:
    `WM_SETFOCUS` from a window that is not walk's starts a 100 ms timer
    that raises `EVENT_OBJECT_FOCUS` again; NVDA drops it as a duplicate
    when it has the focus already. Tested by the user with the switcher,
    several times on every control.
  - msftedit has no Ctrl+B, Ctrl+I or Ctrl+U; the widget adds no
    shortcuts (as the system control), applications add them. The test
    program uses shortcut actions (`ShortcutActions`), which run before
    the key reaches the control; a `KeyDown` handler is not enough,
    because Ctrl+I then also inserts a tab character.
  - Test program: the focus is set after the window is shown, a link
    is announced once through the live status label, with a counter.
  - New widget `RichEdit` and `declarative.RichEdit` on the RichEdit
    control of `msftedit.dll` (`RICHEDIT50W`, loaded on the first
    `NewRichEdit`). Plain text (`Text`, `SetText`, `TextLength`; the
    limit is raised from 32767 characters to the maximum), Rich Text
    Format (`RTF`, `SetRTF`, `SelectedRTF`, `ReplaceSelectedRTF`, with
    `EM_STREAMIN` and `EM_STREAMOUT` and a callback that finds its
    buffer by the cookie), selection (`TextSelection`,
    `SetTextSelection`, `SelectedText`, `TextRange`,
    `ReplaceSelectedText`, `AppendText`), `Find` (`FindFlags`), undo
    and redo, clipboard, `Modified`, `MaxLength`, `ReadOnly`,
    `AutoURLDetect`.
  - Formatting of the selection: `CharFormat` with `CharFormatMask`
    (bold, italic, underline, strikeout, link, font family, size in
    points, text and background color or the default colors; the mask
    returned says which fields are the same in the whole selection),
    `SetDefaultCharFormat`, and `ParaFormat` with `ParaFormatMask`
    (alignment including justified, which needs
    `TO_ADVANCEDTYPOGRAPHY`, bullets and numbering starting at 1 with a
    period, indents and spacing in points).
  - Events: `TextChanged` (once per method call; `EN_CHANGE` during
    `SetText` and `SetRTF` is ignored), `SelectionChanged`
    (`EN_SELCHANGE`), `ReadOnlyChanged` and `LinkClicked` (a
    `StringEvent` with the link text, for `EN_LINK` with
    `WM_LBUTTONDOWN` and `WM_LBUTTONUP` on the same link with the same
    selection, or `WM_CHAR` or `WM_KEYDOWN` with Enter).
  - Keyboard: Tab moves the focus, as in `TextEdit`, without selecting
    the whole text; a live setting
    announces the text with `Announce`, as for `LineEdit` and
    `TextEdit`.
  - New in `internal/win`: the `EM_*`, `EN_*`, `ENM_*`, `CFM_*`,
    `CFE_*`, `PFM_*`, `PFA_*`, `PFN_*`, `SF_*`, `SCF_*` and `FR_*`
    constants, `EVENT_OBJECT_FOCUS` and `CHARRANGE` and `GETTEXTLENGTHEX` (generated).
    Hand-written in `richedit.go`: `MSFTEDIT_CLASS`, `LoadMsftedit`,
    `CHARFORMAT2` and `PARAFORMAT2` (anonymous unions; the build fails
    if their sizes differ from the SDK), `TEXTRANGE` and `FINDTEXTEX`
    (only the alignment differs from `#pragma pack(4)`), and
    `EDITSTREAM` and `ENLINK` as arrays of 4-byte words with accessors,
    because `#pragma pack(4)` puts their pointer-sized fields at
    offsets Go cannot express on 64-bit.
  - Not done: the Text Object Model (`ITextDocument`), OLE objects
    (`IRichEditOleCallback`), printing, zoom and background color.
- Step 13, part 4: pull request lpintes/walk#23, merged. Task 15 of
  `TESTING_ON_WINDOWS.md` passed on Windows 11: `run.ps1 -Only
  uia,liveregion` on 386 and amd64 with every check PASS, and the
  manual test on amd64: `uia.exe dump` read the tree with one cache
  request (no fallback) and printed grid sizes and cell positions of
  the Explorer details view ("14 rows, 4 columns", "row 1, column 1").
  The test first failed: a `TableView` sorts a slice model by the
  first column, so "Item 10" came right after "Item 1"; the test rows
  now have two-digit numbers.
  - Cache requests: `CacheRequest` is a Go value built per call, like
    `Condition` (`Properties`, `Scope`, where 0 means `ScopeElement`,
    and `Filter`, where nil keeps the control view).
    `FindFirstBuildCache`, `FindAllBuildCache`, `BuildUpdatedCache`;
    `CachedName`, `CachedControlType`, `CachedAutomationID`,
    `CachedClassName`, `CachedIsEnabled`, `CachedBoundingRectangle`,
    `CachedPropertyValue`, `CachedChildren` and `CachedParent`.
    `PropertyValue` reads any property by ID (same Go types as
    `OnPropertyChanged`); new constant `PropertyLiveSetting`. Not done:
    cached patterns (`AddPattern`, `GetCachedPatternAs`), the element
    mode without a reference (`AutomationElementMode_None`) and the
    `BuildCache` variants of `Root`, `ElementFromHandle` and the tree
    walker (`BuildUpdatedCache` covers them).
  - Patterns: Grid (`GridSize`, `GridItem`), GridItem (`GridPosition`,
    `ContainingGrid`), Table (`ColumnHeaders`, `RowHeaders`,
    `TableOrder`), TableItem (`ColumnHeaderItems`, `RowHeaderItems`)
    and Scroll (`ScrollInfo`, `Scroll`, `SetScrollPercent`, `NoScroll`).
    Skipped: Transform, MultipleView and Dock (little use for a screen
    reader style client).
  - New in `internal/win`: `IUIAutomationCacheRequest` and the five
    pattern interfaces (generated with `methods`), their IIDs and
    pattern IDs, the `RowOrColumnMajor` and `ScrollAmount` constants,
    `UIA_LiveSettingPropertyId`, and the generated `CreateCacheRequest`,
    `FindFirstBuildCache`, `FindAllBuildCache` and `BuildUpdatedCache`
    (skipped before because the cache request interface was not
    defined). Hand-written: `IUIAutomationScrollPattern.SetScrollPercent`
    (two `float64`, like `SetRangeValue`; `E_NOTIMPL` on arm64).
  - The client of `tests/_wingui/liveregion` uses `uia` (`PropertyValue`,
    `OnEvent`, `OnNotification`) instead of its own COM code, with the
    same checks; the sender's handle and name are now read on the
    subscription goroutine, after the event.
  - `tests/_wingui/uia` has a `TableView` (30 rows, 2 columns) for the
    grid checks; `dump` reads the tree with one cache request and falls
    back to the tree walker if that fails.
- Step 4: pull request lpintes/walk#24, merged. Task 16 of
  `TESTING_ON_WINDOWS.md` (test program `tests/_wingui/dialogs`) passed
  on Windows 11 (2026-10-26): `run.ps1` and `examples.ps1` on 386 and
  amd64 on the host and in Windows Sandbox, and the manual test with
  NVDA on amd64. Option A only; option B (`#32770` through
  `CreateDialogIndirectParam`) is not implemented and not needed for
  NVDA.
  - Results: UI Automation passes the annotation on; the walk dialog,
    the message box and the task dialog all have ControlType Window,
    Role 18, `IsDialog` true and `IsModal` true. Only the localized
    control type differs: "okno" (window) for the walk dialog,
    "dialógové okno" (dialog window) for the other two, so Narrator may
    still say "window"; not tested with Narrator or JAWS yet. NVDA says
    the title, "dialog" and the static texts of a walk dialog, like for
    the message box; Tab, Enter and Escape work. The task dialog reads
    and works with the keyboard (radio buttons, check box, expand
    button, footer link, buttons; Escape and Alt+F4 return Cancel);
    command links are announced as buttons with their second line read
    after the first, as in system task dialogs.
  - The footer link fires `OnHyperlinkClicked` on every Enter or Space,
    but UI Automation `Invoke` on the link returns `S_OK` and sends no
    `TDN_HYPERLINK_CLICKED` (`DoDefaultAction` fails with
    `DISP_E_EXCEPTION`), so the automated test reports that as INFO.
  - Every `Dialog` gets, through Dynamic Annotation when it is created,
    the MSAA role `ROLE_SYSTEM_DIALOG` on its client object and the UI
    Automation property `IsDialog` (`IsDialog_Property_GUID`, Windows 10
    1809). The client object, not `OBJID_WINDOW`, because that is where
    oleacc reports the dialog role for `#32770` windows (their window
    object has `ROLE_SYSTEM_WINDOW`) and the object NVDA reads: NVDA
    uses `OBJID_CLIENT` for windows and maps `(any class,
    ROLE_SYSTEM_DIALOG)` to its `Dialog` behavior, which says "dialog"
    and reads the static texts (`getDialogText` in
    `NVDAObjects/behaviors.py`), so option A should be enough for NVDA.
    Annotating the window object too could make clients that show window
    objects report two dialogs. NVDA reads `IsDialog` only for windows it
    handles through UI Automation (Narrator does). A program can still
    set another role with `SetRole` or the declarative `Accessibility`.
  - `Dialog.Show` raises `EVENT_SYSTEM_DIALOGSTART` the first time, and
    `WM_DESTROY` raises `EVENT_SYSTEM_DIALOGEND`, as the dialog manager
    does for `#32770` dialogs (NVDA ignores them; JAWS may use them).
  - Dialogs get `WS_EX_CONTROLPARENT` like `MainWindow` (no change for
    their own tab navigation, `IsDialogMessage` already recursed into the
    client composite). Not added: `WS_EX_DLGMODALFRAME`; it only changes
    the frame and hides the caption icon, and no screen reader uses it.
  - New public API: `Accessibility.SetIsDialog`; `TaskDialog` with
    `Show(owner)` returning `TaskDialogResult` (button, radio button,
    verification check box), `TaskDialogButton`, `TaskDialogIcon`,
    `TaskDialogCommonButtons` and `TaskDialogAvailable`. Fields: title,
    main instruction, content, icon, common buttons, custom buttons
    (optionally command links), default button, radio buttons with a
    default or none, verification text and state, expanded information
    with the control texts and the initial state, footer, `AllowCancel`
    and `OnHyperlinkClicked` (enables links; one shared
    `syscall.NewCallback` finds the dialog through its callback data).
    Common buttons return the `DlgCmd` values. Without common controls 6
    `Show` returns an error; all examples have the manifest.
  - In `uia`: `Element.IsDialog`, `Element.Role` (MSAA role through
    LegacyIAccessible) and `PropertyIsDialog`.
  - New in `internal/win` (generated): `taskDialogIndirect` (unexported,
    `unsafe.Pointer` config), the `TDF_`, `TDCBF_`, `TDN_` constants
    used and `TDM_CLICK_BUTTON`, `EVENT_SYSTEM_DIALOGSTART`,
    `EVENT_SYSTEM_DIALOGEND`, `OBJID_WINDOW`, `UIA_IsDialogPropertyId`,
    `IsDialog_Property_GUID`. Hand-written: `TASKDIALOGCONFIG` and
    `TASKDIALOG_BUTTON` (`#pragma pack(1)`: on 64-bit `hwndParent` is at
    offset 4), as Go structs with natural fields that the hand-written
    `TaskDialogIndirect` packs into pinned memory laid out as in C; the
    build checks the packed sizes against the SDK (96 and 8 bytes on 386,
    160 and 12 on 64-bit). Also `TD_*_ICON` (`MAKEINTRESOURCEW(-1)` to
    `(-4)`) and `TaskDialogIndirectAvailable`.
  - A concrete reason for option B would still be a screen reader that
    ignores the role and looks for the `#32770` class, or Narrator
    saying "window" because of the localized control type.
  - Not done: `Announce` for validation errors (step 10 leftover), a
    declarative counterpart of `TaskDialog` (it is a plain struct
    already), progress bars and timers in task dialogs.
- Step 7: pull request lpintes/walk#25, merged. Task 17 of
  `TESTING_ON_WINDOWS.md` (`tests/_wingui/webview2`) passed on Windows 11
  (2026-10-26) after the fix below: `run.ps1 -Only webview2` on 386 and
  amd64 with every check PASS (INFO: a script exception gives the result
  `null` and no error; after `SetFocus` the focus is inside the widget),
  and the manual test with NVDA on amd64 (Tab and Shift+Tab into, through
  and out of the page, browse mode, messages both ways), the keyboard
  part also on 386. F6 in the page does nothing. Closing with
  `WM_CLOSE` while the page loads and after it has loaded exits with
  code 0 on both architectures.
  - Fixed during the test: the browser kept its first size when the form
    was resized. `WindowBase.WndProc` does not pass
    `WM_WINDOWPOSCHANGED` to `DefWindowProc` (unless the widget has
    `NeedsWmSize`), so the widget never got `WM_SIZE` or `WM_MOVE`;
    `WebView2.WndProc` handles `WM_WINDOWPOSCHANGED` now, like the old
    `WebView`. The test checks that the browser window fills the widget
    before and after a resize. At 150 % WebView2 makes its window one
    pixel larger when a size is not a multiple of 1.5 (it rounds to
    device independent pixels); the parent clips it, and the test
    allows one pixel.
  - Widget `WebView2` (`webview2.go`): `NewWebView2`,
    `NewWebView2WithOptions` (`WebView2Options`: `UserDataFolder`,
    default `%LOCALAPPDATA%\<exe name>\WebView2`,
    `BrowserExecutableFolder` for a fixed version runtime,
    `AdditionalBrowserArguments`, `Language`), `URL`, `SetURL`,
    `Navigate`, `NavigateToString`, `GoBack`, `GoForward`, `Reload`,
    `Stop`, `CanGoBack`, `CanGoForward`, `Title`, `ExecuteScript(script,
    callback)` (result as JSON), `PostWebMessage` (string) and
    `PostWebMessageAsJSON`; events `Ready`, `CreationFailed` (with
    `CreationError`), `NavigationStarting`, `NavigationCompleted`
    (`WebView2NavigationCompletedEvent`: succeeded and web error
    status), `URLChanged`, `TitleChanged`, `HistoryChanged` and
    `WebMessageReceived` (the message as JSON). Creation is
    asynchronous: navigations requested before `Ready` are remembered
    (the last one wins) and carried out then; the other calls return
    `ErrWebView2NotReady`. Properties `URL` and `Title` (read only).
    Declarative `WebView2` with the options, `URL`, `HTML` and the
    event handlers. The old `WebView` and its declarative counterpart
    are marked `// Deprecated: use WebView2.`
  - Keyboard: `WM_SETFOCUS` calls `MoveFocus` (next, previous or
    programmatic, from the Tab and Shift state), so Tab moves into the
    page; `MoveFocusRequested` at the end of the page moves the focus
    to the next or previous tab stop with `GetNextDlgTabItem` and marks
    it handled. `WM_WINDOWPOSCHANGED` sets the bounds and, when the
    widget moves, calls `NotifyParentWindowPositionChanged`, which the
    form's `BoundsChanged` calls too; `WM_SHOWWINDOW` sets the
    visibility.
  - Loader without `WebView2Loader.dll` (`webview2_loader.go`, after
    OpenWebView2Loader and the Go port in `wailsapp/go-webview2`, ISC
    license, credited in the file): for the stable, beta, dev and canary
    channels, HKLM before HKCU, the 32-bit registry view
    (`KEY_WOW64_32KEY`), the value `EBWebView` of
    `SOFTWARE\Microsoft\EdgeUpdate\ClientState\{GUID}` or else `pv`
    of `...\Clients\{GUID}` with the default install folder; then
    `EBWebView\<x86|x64|arm64>\EmbeddedBrowserWebView.dll` and
    `CreateWebViewEnvironmentWithOptionsInternal(1, runtimeType,
    userDataFolder, options, handler)`. Fallback: `WebView2Loader.dll`
    next to the exe (`CreateCoreWebView2EnvironmentWithOptions`).
    Without a runtime `NewWebView2` returns an error wrapping
    `ErrWebView2RuntimeNotFound`.
  - COM objects with `internal/com` (`webview2_handlers.go`): the
    environment options (`ICoreWebView2EnvironmentOptions`, strings
    allocated with `SHStrDup`, target version 86.0.616.0), the
    environment, controller and `ExecuteScript` completion handlers
    and one object per widget with the seven event handlers.
  - Declarations hand-written in `internal/win/webview2.go`: the
    interfaces are not in the Win32 metadata. Interface IDs and vtable
    order were taken from `WebView2.h` of SDK 1.0.1150.38 (the copy
    vendored in `github.com/webview/webview_go`, fetched through the Go
    module proxy because nuget.org was blocked); every vtable lists all
    methods of its interface. `Put_Bounds` takes a `RECT` by value: four
    stack words on 386, a pointer to a copy on amd64, two registers on
    arm64 (`webview2_32.go`, `_amd64.go`, `_arm64.go`). Generated for
    it: `KEY_WOW64_32KEY`, `REG_SZ`, `ERROR_FILE_NOT_FOUND`,
    `WM_SHOWWINDOW`, `GetNextDlgTabItem` and `SHStrDup`
    (`shlwapi.dll`).
  - Not done: F6 and the `AcceleratorKeyPressed` event (keys the page
    does not handle), settings (`ICoreWebView2Settings`), new
    window requests, permissions, the cancel of `NavigationStarting`,
    and an example.
- Step 14, part 1: pull request lpintes/walk#28, merged. Task 19 of
  `TESTING_ON_WINDOWS.md` (test program `tests/_wingui/hotkeys` with a
  `manual` mode) passed on Windows 11 (2026-10-27) after the fix below:
  `run.ps1` and `examples.ps1` in Windows Sandbox on 386 and amd64, and
  the manual test with NVDA on amd64 (the hotkey brings the form to the
  foreground, also from minimized, no autorepeat, unregister and
  register again, a second instance hands over its arguments and the
  first window gets the foreground, a restart after Alt+F4 is the first
  instance again). The test first failed: a taken combination reported
  error 0, because `GetLastError` always returns 0 (see "Known issues");
  fixed with the generator option `lasterror`. Global hotkeys and
  single instance; upstream lxn/walk #228 was only the model (rewritten,
  see `opened-prs.md`).
  - `GlobalHotkey` (`globalhotkey.go`): `NewGlobalHotkey(owner,
    shortcut)` and `NewGlobalHotkeyWithOptions` (`GlobalHotkeyOptions`
    with `Repeat`; by default `MOD_NOREPEAT`), `Activated`, `Shortcut`,
    `Owner`, `ID`, `Repeat`, `IsDisposed`, `Dispose`. `RegisterHotKey`
    for the owner window (usually a form) on its thread (another thread
    is an error); IDs are the smallest unused per window, from 1.
    `WindowBase.WndProc` publishes `Activated` for `WM_HOTKEY` with the
    ID; `WindowBase.Dispose` unregisters the window's hotkeys before
    `DestroyWindow`; `Dispose` from another goroutine goes through
    `Synchronize`. A taken combination returns an error wrapping
    `ErrHotkeyAlreadyRegistered` (`ERROR_HOTKEY_ALREADY_REGISTERED`, read
    with `GetLastError` right after the call, as `lastError` does);
    other failures wrap the `syscall.Errno`. New modifier `ModWin`
    (value 8, `Modifiers.String` prefixes "Win+"); `ModifiersDown` does
    not report it, so action shortcuts with it never match. No
    declarative counterpart: registration can fail for reasons outside
    the program, and the program should handle the error.
  - `FormBase.BringToForeground`: restores a minimized form and calls
    `SetForegroundWindow` (a hidden form stays hidden); an error when
    Windows refuses. Allowed right after the program's hotkey or a
    handover from another instance.
  - `SingleInstance` (`singleinstance.go`): `NewSingleInstance(appID)`
    on the UI goroutine (locks it to its OS thread); the first instance
    owns the mutex `Local\walk.SingleInstance.<appID>` (backslashes
    become underscores, a hash for long IDs) through
    `WaitForSingleObject(h, 0)`, so `WAIT_ABANDONED` after a crash makes
    the next start the first one and no `GetLastError` is needed. Later
    instances look for the message-only window of class
    `\o/ Walk_SingleInstance_Class \o/` with that name (`FindWindowEx`
    with `HWND_MESSAGE`), polling every 50 ms and trying the mutex again
    in between, up to `SingleInstanceTimeout` (30 seconds), which covers
    a first instance that is still starting or just exiting; then
    `AllowSetForegroundWindow` for its process and `SendMessageTimeout`
    with `WM_COPYDATA` (JSON with `os.Args[1:]` and the working
    directory). `IsFirst`, `AppID`, `Listen(form)` (creates the window,
    titled only after it is in the map), `OtherInstanceStarted`
    (`SingleInstanceEvent`, handler `func(args []string, workingDir
    string)`), `Close`. The window procedure activates the form with
    `BringToForeground` during `WM_COPYDATA` and posts itself a message
    to publish the event, so the second instance does not wait for the
    handlers. A failed send (`SMTO_ABORTIFHUNG`, a refused message, a
    first instance that is ending) is retried until the deadline. The
    message window is destroyed when the form given to `Listen` is
    disposed, and it rejects data of 0 bytes or above 1 MiB. An elevated
    first instance: the mutex cannot be opened (`ERROR_ACCESS_DENIED`,
    taken as "another instance exists") and UIPI blocks `WM_COPYDATA`,
    so the second instance gets an error at once saying so. `Close`
    must run on the thread of `NewSingleInstance` (error otherwise).
    `GlobalHotkey.Dispose` from another goroutine only queues the
    unregistration; `IsDisposed` is an atomic flag.
  - New in `internal/win` (generated): `RegisterHotKey`,
    `UnregisterHotKey`, `CreateMutex` (`uintptr` for the security
    attributes), `ReleaseMutex`, `WaitForSingleObject`, `CloseHandle`,
    `FindWindowEx`, `SendMessageTimeout`, `AllowSetForegroundWindow`,
    `GetWindowThreadProcessId`, `IsIconic`,
    `SetWindowText`, `COPYDATASTRUCT`, `WM_HOTKEY`, `WM_COPYDATA`,
    `MOD_*`, `ERROR_HOTKEY_ALREADY_REGISTERED`, `ERROR_ACCESS_DENIED`,
    `WAIT_*`, `SMTO_ABORTIFHUNG` and `SW_RESTORE`.
- Step 14, part 2: draft pull request lpintes/walk#29, not yet tested
  on Windows (task 20 of `TESTING_ON_WINDOWS.md`, test program
  `tests/_wingui/system` with a `manual` mode). Power, session and
  settings events and system settings.
  - `walk.System()` returns the `SystemService` of the calling thread,
    or an error on a thread without a walk window group (a hidden
    top-level window nobody pumps would make broadcasts and the
    shutdown wait). It owns a hidden top-level
    window (class `\o/ Walk_System_Class \o/`, `WS_POPUP`,
    `WS_EX_TOOLWINDOW`), created on the first call per thread, because
    broadcasts reach top-level windows only, not message-only ones; the
    events are published on that thread; `systemWndProc` publishes
    panics to `App().Panicking` like `defaultWndProc`. `Dispose`
    destroys it and unregisters everything; a later `System` creates a
    new one. A stored service whose window is gone or belongs to
    another thread (reused thread ID) is replaced.
  - Power: `Suspending` (`PBT_APMSUSPEND`), `Resumed`
    (`PBT_APMRESUMEAUTOMATIC`), `ResumedByUser` (`PBT_APMRESUMESUSPEND`),
    `PowerStatusChanged`, and with `RegisterPowerSettingNotification`
    (unregistered on destroy) `PowerSourceChanged`
    (`GUID_ACDC_POWER_SOURCE`), `EnergySaverChanged`
    (`GUID_ENERGY_SAVER_STATUS` on Windows 11, `GUID_POWER_SAVING_STATUS`
    before) and `DisplayStateChanged` (`GUID_CONSOLE_DISPLAY_STATE` and
    `GUID_SESSION_DISPLAY_STATUS`, which also works in Remote Desktop).
    Windows sends each setting right after the registration; that first
    value is cached without an event, later ones publish only on a
    change. `PowerSource`, `EnergySaver` and `DisplayState` return the
    cached values (the first two fall back to `GetSystemPowerStatus`);
    the package function `Battery` returns the battery status.
  - `KeepAwake(reason, display)` returns a release function. It uses
    `PowerCreateRequest` and `PowerSetRequest` (system, optionally
    display required), not `SetThreadExecutionState`: every call has its
    own request, the state does not belong to an OS thread, and the
    reason shows in `powercfg /requests`. A release function never
    called keeps the request until the process exits. `REASON_CONTEXT` is
    hand-written in `internal/win/power.go` (anonymous union).
  - Shutdown: `SessionEnding` (`WM_QUERYENDSESSION`, handler
    `func(canceled *bool, reason EndSessionReason)`) and `SessionEnded`
    (`WM_ENDSESSION`, `func(ending bool, reason)`);
    `FormBase.SetShutdownBlockReason` (empty removes it) and
    `ShutdownBlockReason`. Every top-level window gets
    `WM_QUERYENDSESSION`; the handlers run once per round (until
    `WM_ENDSESSION`, at most ten seconds). When they cancel, a visible
    form with a block reason refuses (`FormBase.WndProc`) and the hidden
    window agrees, so Windows lists the form with its reason; without
    such a form the hidden window refuses. Session: `SessionChanged` (`SessionChange`
    and session ID), `SessionLocked` and `SessionUnlocked`
    (`WTSRegisterSessionNotification` for this session, unregistered on
    destroy), the package function `IsRemoteSession`.
  - Settings: `SettingsChanged` with `SettingsChange` (kind
    `WM_SETTINGCHANGE`, `WM_THEMECHANGED` or `WM_SYSCOLORCHANGE`, the
    SPI action and the area string such as `ImmersiveColorSet`), and
    after it `HighContrastChanged`, `DarkModeChanged` and
    `ScreenReaderChanged`, published only when the value really changed.
    Package functions for any goroutine: `HighContrast` (on and
    scheme), `ScreenReaderRunning`,
    `AppsUseDarkTheme` (registry `AppsUseLightTheme`, the documented
    way), `TextScaleFactor` (registry `TextScaleFactor`, no event),
    `ClientAreaAnimation`.
  - New event types in `systemevent.go`: `SessionEndingEvent`,
    `SessionEndedEvent`, `SessionChangedEvent`, `SettingsChangedEvent`.
  - New in `internal/win` (generated): `wtsapi32.dll`, the power,
    session, shutdown and settings functions, constants and GUIDs,
    `SYSTEM_POWER_STATUS`, `POWERBROADCAST_SETTING`, `IsWindow` and
    `GetWindowThreadProcessId`.
- Step 14, part 3: pull request lpintes/walk#27, merged. Task 21 of
  `TESTING_ON_WINDOWS.md` (test program `tests/_wingui/clipboard` with a
  `manual` mode) passed on Windows 11 (2026-10-27) after the fix below:
  `run.ps1 -Only clipboard,richedit` on 386 and amd64 with every check
  PASS, and the manual test with NVDA on amd64 (pasting into other
  programs; the sensitive password can be pasted and is not in the
  Windows+V history). INFO: Windows adds `CF_BITMAP` to an image and
  `CF_LOCALE`, `CF_TEXT` and `CF_OEMTEXT` to text. More formats in
  `ClipboardService`; `Text`, `SetText`, `ContainsText`, `Clear` and
  `ContentsChanged` are unchanged.
  - Fixed during the test: msftedit ends the output of `EM_STREAMOUT`
    with `SF_RTF` with a NUL, which `RichEdit.RTF` and `SelectedRTF`
    returned as part of the string, so `SetRTF` of the clipboard
    rejected it. `streamOut` trims trailing NULs; the `richedit` test
    checks that.
  - `ClipboardFormat` with `Name`, the constants `ClipboardFormatText`,
    `DIB`, `DIBV5` and `Files` (CF_HDROP), the registered formats as
    package variables (`ClipboardFormatHTML`, `RTF`, `PNG`,
    `PreferredDropEffect` and the three history and cloud exclusion
    formats), `RegisterClipboardFormat`, `DropEffect` and
    `ErrClipboardFormatNotAvailable`.
  - `ClipboardData` collects several formats (`SetText`, `SetHTML`,
    `SetRTF`, `SetFiles`, `SetImage`, `SetFormat` for raw bytes,
    `SetSensitive`); `ClipboardService.SetData` converts everything
    first (a failed conversion leaves the clipboard unchanged), then
    empties the clipboard and sets all formats in one `OpenClipboard`
    session, in the order given. Each format is copied into its own
    `GlobalAlloc(GMEM_MOVEABLE)` memory, freed if `SetClipboardData`
    fails (the clipboard is then emptied); Windows never gets a Go
    pointer.
  - Getters: `Data` (raw bytes), `Formats`, `ContainsFormat`,
    `ContainsHTML`, `ContainsRTF`, `ContainsFiles`, `ContainsImage`,
    `IsSensitive`, `HTML` (`ClipboardHTML`: fragment, document, source
    URL), `RTF`, `Files` (`DragQueryFileW`), `PreferredDropEffect`,
    `Image`, `ImageOfFormat`, `Bitmap(dpi)`. Setters: `SetHTML` and
    `SetRTF` with an optional plain text alternative, `SetFiles` with
    copy or move (cut), `SetImage`, `SetBitmap`, `SetSensitiveText`.
    The new methods retry `OpenClipboard` for about 100 ms, because
    clipboard monitors open the clipboard after every change.
  - The byte formats are in the new package `internal/clipfmt`, which
    calls no Windows function and has unit tests that run in the
    container (`go test ./internal/clipfmt`): CF_HTML with 10-digit
    UTF-8 byte offsets, version 0.9 and a terminating NUL (decoding
    also accepts other offsets, -1 and LF line ends, and falls back to
    the fragment comments), DROPFILES with wide paths, and DIBs without
    GDI (so the `BITMAPINFO` problem under "Known issues" does not
    matter): `SetImage` writes PNG, CF_DIBV5 (32 bits, bit fields with
    an alpha mask, sRGB, straight alpha) and CF_DIB (24 bits, blended
    onto white); decoding reads 1, 4, 8, 16, 24 and 32 bits per pixel,
    bit fields and color tables, bottom-up and top-down, and treats a
    32-bit bitmap whose alpha values are all zero as opaque. It also
    skips the 12 mask bytes some programs write after a V4 or V5 header
    (only when the data is exactly that much longer). Sizes are limited
    to 2^26 pixels and checked without multiplying, so they cannot
    overflow on 386 (the unit tests run with `GOARCH=386` too). RTF must
    be 7-bit ASCII; `SetRTF` rejects other bytes.
  - Fixed on the way: `OpenClipboard` and `CloseClipboard` work per
    thread, so both the old `withOpenClipboard` and the new helper lock
    the goroutine to its thread for the session; before, a goroutine
    could move between them and leave the clipboard open.
  - Windows Sandbox shares the clipboard with the host (`rdpclip`), so
    the test overwrites the host clipboard too and retries the old
    methods while another program has the clipboard open.
  - Sensitive data: `ExcludeClipboardContentFromMonitorProcessing`,
    and 0 for `CanIncludeInClipboardHistory` and
    `CanUploadToCloudClipboard`.
  - New in `internal/win` (generated): the `CF_*` constants,
    `DROPEFFECT_*`, `LCS_sRGB`, `LCS_GM_IMAGES`, `GlobalSize`,
    `RegisterClipboardFormatW`, `GetClipboardFormatNameW` and
    `EnumClipboardFormats`.
  - Open questions: whether `Text` and `SetText` should retry
    `OpenClipboard` too (a behavior change), and whether `SetText`
    should empty the clipboard first (today it adds the text to what
    another program put there without becoming the owner).
- Steps 9 and 11: not started; step 8 can continue (generic
  event types are step 9).

## Candidate next steps (analysis for steps 4 to 11)

Each item is its own step and pull request, in the order given in the
modernization plan. Step 3 (generator) goes first, because RichEdit, WebView2
and real dialogs all need new Win32/COM declarations.

### RichEdit widget

- Today there is no RichEdit widget. `TextEdit` and `LineEdit` use the plain
  `EDIT` class. The unused lxn/win declarations for RichEdit (`richedit.go`,
  `richole.go`, `tom.go`) were removed in step 3 part 3; generate what the
  widget needs with `tools/winmdgen` (constants, structs, `IRichEditOle`
  and TOM interfaces with the `interface` directive).
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
- Option A was done in step 4 (see "Status"), on the client object
  instead of `OBJID_WINDOW`, without `WS_EX_DLGMODALFRAME`.
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

### Live regions and announcements (step 10)

- Goal: let a program have a screen reader read changed text without
  moving focus (status lines, progress, validation errors, search result
  counts). Today nothing is announced unless focus moves; validation
  errors are only tooltips (see "Known issues").
- Part A, live regions on existing widgets (small, try first): Windows 8
  and later accept UI Automation properties through Dynamic Annotation.
  Set `LiveSetting` (Off, Polite or Assertive) with
  `IAccPropServices.SetHwndProp` and the UIA property GUID
  `LiveSetting_Property_GUID`, and after each text change call
  `NotifyWinEvent(EVENT_OBJECT_LIVEREGIONCHANGED, hwnd, OBJID_CLIENT,
  CHILDID_SELF)`. This is the documented way for plain Win32 controls.
  walk already has the pieces in `accessibility.go` (`accSetPropertyInt`,
  `NotifyWinEvent`). Proposed API: `Accessibility.SetLiveSetting` plus
  an automatic live region changed event when the text of a widget with
  a live setting changes (at least `Label`, `TextLabel`, `LineEdit` read
  only, `StatusBarItem`), and a `LiveSetting` property in `declarative`.
  The 386 bug of `SetHwndProp` had to be fixed first, because
  `LiveSetting` is an integer `VARIANT` (done in step 10).
- Part B, one-off announcements: `UiaRaiseNotificationEvent`
  (Windows 10 1709 and later, `uiautomationcore.dll`) speaks any text
  without a visible control, with a priority (`NotificationProcessing`).
  It needs an `IRawElementProviderSimple` for a window; try
  `UiaHostProviderFromHwnd` first, otherwise implement a minimal provider
  with `internal/com`. Proposed API: `walk.Announce(window, text,
  priority)`. Use it, for example, to announce validation errors.
- Declarations (GUIDs, `EVENT_OBJECT_LIVEREGIONCHANGED`, the UIA
  functions and enums) come from the generator.
- Only testable with screen readers: NVDA, JAWS and Narrator handle
  live region and notification events differently (for example how
  Polite and Assertive interrupt speech), so every part needs live
  testing on Windows.

### MDI windows (step 11)

- Goal: multiple-document interface, a frame window whose client area
  is the system `MDICLIENT` window holding child document windows.
  walk has no MDI support today.
- Plan: new `MdiMainWindow` (or an MDI option of `MainWindow`) that
  creates `MDICLIENT` and calls `DefFrameProcW`, and `MdiChildWindow`, a
  container created with `WM_MDICREATE` whose window procedure calls
  `DefMDIChildProcW`. The message loop (`mainloop_default.go` and
  `mainloop_cgo.go`) must call `TranslateMDISysAccel` (Ctrl+F4, Ctrl+F6)
  before `IsDialogMessage`. A Window menu with the list of children comes
  from `WM_MDISETMENU`; cascade, tile and arrange from `WM_MDICASCADE`,
  `WM_MDITILE` and `WM_MDIICONARRANGE`. Declarative counterparts too.
- Risks: walk's `FormBase` assumes top-level windows (activation, focus
  restore, keyboard handling in `handleKeyDown`, layout), and MDI
  children are child windows that behave like top-level ones; menus
  merge when a child is maximized. Larger than step 10; expect a few
  rounds of testing.
- Accessibility: native MDI is understood by screen readers (MSAA
  exposes the children as windows, NVDA reads the active child's title,
  Ctrl+F6 switches documents). Check focus on child switches, the title
  announcement and the maximized state with NVDA, JAWS and Narrator.
- Microsoft considers MDI legacy; tabbed documents (`TabWidget`) are the
  modern alternative. Worth doing for applications that want the classic
  model, but after the accessibility steps.

## Known issues

- `win.GetLastError()` always returns 0: the Go runtime clears the last
  error of the thread before every call into Windows, including the call
  of `GetLastError` itself (checked on Windows 11, 386 and amd64, with
  `RegisterHotKey`, which sets 1409). So `lastError` in `error.go` (about
  100 call sites) never carries a code, contrary to the notes of step 8
  part 3, and `ShellOpen` reports error 0. The code is only available as
  the error returned by `syscall.SyscallN`; the generator option
  `lasterror` (step 14 part 1) returns it from the wrapper, and
  `errnoError` turns it into a walk error. Used so far by `RegisterHotKey`,
  `CreateMutex`, `WaitForSingleObject` and `SendMessageTimeout`; moving the
  remaining `lastError` callers to it is a separate pull request.
- `go vet` reports 3 "possible misuse of unsafe.Pointer" warnings, all in
  `internal/win` (`GlobalLock`, `SysAllocString`, `MAKEINTRESOURCE`).
  They were reviewed in step 3: they convert memory allocated by Windows
  or integer resource IDs, not Go memory, and stay as they are. The 63
  warnings in walk went away in step 8 part 1 (`ptrFromUintptr`).
- `TableView.updateLVSizes` sizes the frozen list view from the column
  widths converted to 96 DPI and back, so at 150 % scaling it can be one
  pixel wider than the frozen columns (column 220 pixels, list view 221).
  Found while testing the `HDN_*` fix; inherited and cosmetic.
- Inherited problems found while testing step 3 on Windows 11 (all are
  the same on a build from before step 3):
  - An owner drawn `ListBox` created directly in a `MainWindow` sends
    `WM_DRAWITEM` to the form window, which does not forward it, so the
    styler is never called; inside a `Composite` it works (as in
    `examples/listbox_ownerdrawing`).
  - `WebView`: the keyboard (Tab, F6) cannot move focus into the web
    content; NVDA can still read the page. `WebView2` (step 7) is meant
    to fix this.
  - Seen with NVDA: when a window opens, only its title is announced, not
    the focused control; most fields in the `databinding` example dialog
    have no accessible name (labels are not associated); Enter on a
    focused `PushButton` in a `MainWindow` does nothing (Space works);
    validation errors are shown only as tooltips and not announced.
- Bugs inherited from lxn/win, found by comparing with the metadata in
  step 3 and kept hand-written so that behavior does not change. Fixing
  them changes behavior and needs GUI testing:
  - `SetViewportOrgEx` returns `COLORREF` and `PostMessage` returns
    `uintptr` instead of a BOOL; `DragAcceptFiles` returns a result although
    the function has none.
- Hand-written structs whose layout differs from the metadata, found in
  step 3 part 2. None of them breaks walk today:
  - `NMTVKEYDOWN` (and `NMTCKEYDOWN`, removed in part 3 as unused) are
    declared with `#pragma pack(1)`,
    so `Flags` is at offset 14 (386) or 26 (64-bit), not 16 or 28 as in
    Go. Walk reads only `WVKey`, which is at the right offset.
  - `BITMAPINFO.BmiColors` is a pointer in Go but an inline array of one
    `RGBQUAD` in C, so the struct is 4 bytes larger on 64-bit. Walk uses
    it with `GetDIBits` for 32-bit bitmaps without a color table; for a
    bitmap with a color table, `GetDIBits` would write past the struct.
  - `HDITEM`, `LVITEM`, `LVCOLUMN` and `NMLVDISPINFO` lack the fields of
    newer Windows versions. This is safe as long as walk does not set
    masks for the missing fields.
  - The RichEdit structs of lxn/win (removed in part 3; for example `ENLINK`,
    `EDITSTREAM`, `GETTEXTEX`, `MSGFILTER`, `SELCHANGE`) are wrong on
    64-bit: `richedit.h` uses `#pragma pack(4)` there, so pointer-sized
    fields can sit at offsets that are not a multiple of 8, which Go
    cannot express with plain fields. Step 5 (RichEdit) declares
    `EDITSTREAM` and `ENLINK` as arrays of 4-byte words with accessors
    and does not use the others. `TABLEROWPARMS` and `TABLECELLPARMS` are wrong on every
    architecture.
- Constants whose value differs from the metadata only in representation
  (for example `E_NOTIMPL` as a positive untyped constant, `HWND_TOPMOST`,
  `TVI_ROOT`, `CB_ERR`) or that the metadata does not have
  (`LPSTR_TEXTCALLBACK`, `SB_SETTIPTEXT`, `TBM_GETPOS`) stay hand-written.
  `STATE_SYSTEM_VALID` is 0x7fffffff as in the current SDK headers; the
  metadata has the older value 0x3fffffff.
