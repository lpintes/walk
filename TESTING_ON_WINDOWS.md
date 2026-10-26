# Testing on Windows

Tasks for an agent working in a clone of this repository on a Windows
machine. Development happens in a Linux container that can build walk for
Windows but not run it; these tasks need a real Windows session.

Read `CLAUDE.md` first. In particular:

- Talk to the user in Slovak; everything committed is in English.
- The user is blind and uses a screen reader. Report results as plain text
  and lists: no tables, screenshots or diagrams in the report. Do not ask
  the user to judge colors or layout; verify such things yourself, as far
  as possible programmatically (logging, Win32 queries, UI Automation),
  and say clearly what could not be verified.
- Keep changes made only for testing (debug logging, test programs) out of
  the commits unless a task says otherwise, or put test programs in a
  clearly named directory and ask the user before committing them.

## Setup

- Ready-made tests: `tests/_wingui` has test programs for the tasks
  below and PowerShell scripts that build all examples and tests, run the
  self-checking tests, and start every example with an MSAA dump and Tab
  steps. Start with its `README.md`; the notes below explain what the
  scripts do for you.
- Install Go 1.23 or newer.
- Build everything: `go build ./...` (on Windows, `GOOS` is already
  `windows`). The examples carry 386 `rsrc.syso` resource files with the
  manifest (common controls version 6, DPI awareness); build and run them
  with `GOARCH=386` if a 64-bit build of an example fails to link or looks
  unthemed, and say which architecture was used.
- A 64-bit build of an example fails to link with "unknown relocation
  type 7" because of the 386 `rsrc.syso`. Build it with
  `go build -overlay overlay.json`, where `overlay.json` is
  `{"Replace":{"C:/full/path/examples/NAME/rsrc.syso":""}}` (an empty
  replacement removes the file; use forward slashes), and copy the
  example's `NAME.exe.manifest` next to the exe under the exe's name, so
  that Windows still applies the manifest.
- Run an example: `go run ./examples/tableview`, or build it with
  `go build -o tableview.exe ./examples/tableview` and start the exe.
  Examples load images from `../img`, so start them with the working
  directory in a sibling directory of a copy of `examples/img`.
- Do not send global keyboard input (`SendInput`, `keybd_event`, Alt
  tricks to take the foreground, Alt+F4) on the user's machine: if the
  example does not have the foreground, the keys go to whatever window
  does, and this once closed the user's own application. Close windows
  with `WM_CLOSE` posted to their handle, and post key messages only to a
  window of the tested process. Keyboard tests go through the screen
  reader (see below), after checking where focus is.
- Useful for programmatic checks:
  - PowerShell with UI Automation
    (`Add-Type -AssemblyName UIAutomationClient`) to find windows and
    controls, read names, states and bounds, and invoke them. On the test
    machine the managed UI Automation client (PowerShell 7 and 5.1)
    reported every Win32 control, also in `charmap.exe`, as `Pane` and not
    keyboard focusable, so it is useless for roles there. MSAA works:
    `AccessibleObjectFromWindow(hwnd, OBJID_CLIENT, IID_IAccessible)` from
    C# loaded with `Add-Type` gives role, name, value, state and help, and
    `GetGUIThreadInfo` gives the focused window of the tested thread; see
    `tests/_wingui/scripts/WinTest.cs`, which declares `IAccessible` itself
    (`-ReferencedAssemblies Accessibility` fails in PowerShell 7.6).
  - NVDA through the `screenreader` MCP server, if it is configured. Use
    `connect_reader` with reader `nvda`, mode `silent` (the user hears
    nothing but `announce` texts, so announce longer runs) and persona
    `validator`, and `disconnect_reader` at the end. Before every key,
    check with `get_focus_info` (and `nvda+t`, which reads the window
    title) that focus is in the tested example; keys land wherever system
    focus is. Build the examples for this with `-ldflags=-H=windowsgui`:
    a console build opens a terminal window, which may take the focus.
    `run_sequence` with `press_gesture`, `type_text`, `delay` and
    `read: ["focus"]` steps returns what NVDA said for each key. The
    keys are the ordinary user's: `tab`, `shift+tab`, arrows, `space`,
    `enter`, `escape`, `alt+f`, `control+o`, `shift+f10`, `windows+b`
    for the notification area, and `nvda+b`, which reads the whole
    foreground window.
  - Temporary `log.Printf` calls in walk to record the notification codes
    and structures a window procedure receives.
  - Small Go test programs that use walk and `internal/win` (they must live
    inside this module because `internal/win` is internal; put them in
    `tests/_wingui` and follow its `README.md`). Since step 3 part 3,
    `internal/win` declares only what walk uses, so a test program may
    have to declare other Win32 functions and constants itself, with
    `windows.NewLazySystemDLL` and plain constants.

## Task 1: smoke test of step 3

Part 1 of step 3 (pull request lpintes/walk#4) replaced 1035 hand-written
constants and 189 hand-written syscall wrappers in `internal/win` with
code generated from the Win32 metadata (`internal/win/zwinmd_*.go`).
Part 2 replaced 62 hand-written structs (for example `MSG`, `RECT`,
`WNDCLASSEX`, `NMHDR`, `NMLISTVIEW`, `TVITEM`, `LOGFONT`, `MENUITEMINFO`,
`SCROLLINFO`) with generated ones; their layout is checked against the
metadata at compile time. Part 3 (pull request lpintes/walk#6) generated
the GUID variables and COM interface vtables and a few more functions
(`AlphaBlend`, `GetObject`, `GetWindowLongPtr`, `SetWindowLongPtr`,
`SHBrowseForFolder`, `SHGetPathFromIDList`), and removed every
declaration walk does not use. The Go API was checked to be identical
(apart from the removed declarations), but nothing was run on Windows.

Run every example in `examples/` (except where it needs something missing,
such as a network for `webview`; say so) and check:

- It starts without a panic. A panic mentioning `Failed to find ...
  procedure` would mean a wrong DLL entry point name.
- Windows, menus, toolbars, dialogs, tables, tree views, tab pages, the
  notify icon, the clipboard example, drawing and images work as the
  example intends.
- Keyboard: Tab moves focus, Enter and Escape work in dialogs, menu
  accelerators work.
- Accessibility: with UI Automation, controls have names and the focused
  element follows keyboard focus. If possible, check with Narrator or NVDA
  that focus changes are announced.
- Part 3 in particular:
  - `GetWindowLongPtr` and `SetWindowLongPtr` call `GetWindowLongW` and
    `SetWindowLongW` on 386 and the `Ptr` functions on amd64 and arm64.
    walk uses them for every widget (subclassing and window styles), so
    any example that starts and reacts to input exercises them; do run a
    386 build.
  - `AlphaBlend` (bitmaps with transparency, for example
    `examples/imageviewer` and toolbar icons) and `GetObject` (bitmap and
    icon sizes).
  - `SHBrowseForFolder` and `SHGetPathFromIDList`: a folder dialog from
    `FileDialog.ShowBrowseFolder`; check that the chosen path is returned.
  - COM GUIDs and vtables: `examples/webview` (OLE hosting of the
    `WebBrowser` control, its events), `examples/progressindicator`
    (`ITaskbarList3`, progress on the taskbar button), and the
    accessibility annotations of `Accessibility` in walk
    (`IAccPropServices`; for example check with UI Automation that a name
    or role set with `Accessibility().SetName` or `SetRole` is reported).

Do this on both 386 and amd64 builds if possible (`GOARCH=386` and
`GOARCH=amd64`), and arm64 if the machine is arm64. Report per example what
was checked and what failed.

Automated parts: `tests/_wingui/scripts/examples.ps1` (start, MSAA tree,
Tab, `WM_CLOSE` for every example) and the test `tests/_wingui/part3`
(`GetObject`, `AlphaBlend`, `ITaskbarList3`, `IAccPropServices`,
`SHBrowseForFolder`). Keyboard use of menus and dialogs and what a screen
reader announces still need NVDA.

## Bugs inherited from lxn/win

These were found in step 3 by comparing `internal/win` with the metadata.
They are recorded in `CLAUDE.md` under "Known issues". The values below were
checked against the metadata (`Microsoft.Windows.SDK.Win32Metadata`
71.0.26-preview).

### Task 2: DragFinish never frees the drop handle

- `internal/win/shell32.go`: `DragFinish` calls the procedure variable of
  `DragAcceptFiles`, not `DragFinish`. So the `HDROP` that Windows passes
  with `WM_DROPFILES` is never freed (a memory leak per drop), and instead
  `DragAcceptFiles(hDrop, <garbage>)` is called with the drop handle as a
  window handle.
- Used by `DropFilesEventPublisher.Publish` in `dropfilesevent.go`; example
  `examples/dropfiles`.
- The fix: declare a `dragFinish` procedure for `DragFinish` and call it,
  or generate `DragFinish` with `tools/winmdgen` (add `func DragFinish` to
  `internal/win/winmd.txt`, run `go generate ./internal/win`, and remove
  the hand-written function and its proc variable).
- How to test without dragging by hand:
  1. In a test program with a walk `MainWindow` that handles `DropFiles`,
     build an `HDROP` yourself: `GlobalAlloc(GMEM_MOVEABLE|GMEM_ZEROINIT,
     ...)` with a `DROPFILES` header (`pFiles` = size of the header,
     `fWide` = 1) followed by a double-NUL-terminated UTF-16 list of file
     paths, and post `WM_DROPFILES` with it as `wParam` to the window.
  2. Check that the handler receives the paths.
  3. After the handler ran, `GlobalFlags(hDrop)` (kernel32; not declared in
     `internal/win`, load it with `windows.NewLazySystemDLL` in the test
     program, and declare `GMEM_ZEROINIT` and the `DROPFILES` struct there
     too) returns
     `GMEM_INVALID_HANDLE` (0x8000) if the handle was freed. Before the fix
     the handle is still valid; after the fix it must be freed.
  4. Also do a real drag of a file from Explorer onto `examples/dropfiles`
     (UI Automation or `SendInput` can drive Explorer, or ask the user to
     drop a file: dragging a file onto a window is something a screen
     reader user can do) and check the file names arrive.
- This fix changes behavior (the handle is now freed, the bogus
  `DragAcceptFiles` call is gone), so it needs this test.
- Done: `DragFinish` is generated now. Results on Windows 11:
  - Steps 1 to 3 on 386 and amd64: before the fix the paths (including
    non-ASCII ones) arrived and `GlobalFlags` returned 0 (handle still
    valid); after the fix it returned 0x8000.
  - Step 4 without global keyboard or mouse input, on amd64: a second
    process got the shell's own `IDataObject` for two files
    (`SHParseDisplayName`, `SHBindToParent`,
    `IShellFolder::GetUIObjectOf`), moved the pointer over the walk window
    with `SetCursorPos` (the window made topmost with `SWP_NOACTIVATE`)
    and called `DoDragDrop` with a Go `IDropSource` whose
    `QueryContinueDrag` returns `DRAGDROP_S_DROP` after a few calls. OLE
    turned the drop into `WM_DROPFILES` and the handler got both paths;
    `DoDragDrop` returned `DRAGDROP_S_DROP` with `DROPEFFECT_COPY`. Without
    real mouse input the `DoDragDrop` loop waits for mouse messages and
    hangs, so the source posts `WM_MOUSEMOVE` to its own thread with
    `PostThreadMessage` every 50 ms. Restore the pointer position at the
    end, also from a watchdog.
  - Test programs: `tests/_wingui/dragfinish` (steps 1 to 3) and
    `tests/_wingui/olednd` (step 4, `run.ps1 -Drag`).

### Task 3: HDN_* values are off by one

- `internal/win/header.go`: `HDN_FIRST = ^uint32(300)`, which is
  0xFFFFFED3 (-301). The correct value is 0xFFFFFED4 (-300). Every `HDN_*`
  constant derived from it is one too small.
- walk uses only `HDN_ITEMCHANGING`, in `TableView.lvWndProc`
  (`tableview.go`), to call `tv.updateLVSizes()`. With the wrong value it
  equals 0xFFFFFEBF, which is really `HDN_ITEMCHANGEDW` (sent after a
  header item changed). The real `HDN_ITEMCHANGINGW` is 0xFFFFFEC0.
- So today `updateLVSizes` runs after a column changed, which is what it
  needs. A fix that keeps this behavior: correct `HDN_FIRST`, and in
  `tableview.go` use `win.HDN_ITEMCHANGED` instead of
  `win.HDN_ITEMCHANGING`. The `HDN_*` constants should then come from the
  generator where the metadata has them (note the A/W variants: the
  Unicode values end in W in the metadata, for example `HDN_ITEMCHANGEDW`).
- How to test (before and after the fix):
  1. Add temporary logging of the `WM_NOTIFY` codes from the header in
     `TableView.lvWndProc` and of each `updateLVSizes` call.
  2. No example has frozen columns, so write a test program with a
     `TableView` that has a frozen column (`Frozen: true` in the declarative
     `TableViewColumn`, or `TableViewColumn.SetFrozen(true)`) and
     some normal ones: `updateLVSizes` keeps the frozen and the normal
     list view aligned. Also run `examples/tableview`.
  3. Change a column width programmatically with `LVM_SETCOLUMNWIDTH`, and
     by dragging the header divider with `SendInput`, and also with the
     keyboard if possible.
  4. Check the log: `updateLVSizes` must run once per change, after the
     header sent `HDN_ITEMCHANGEDW` (0xFFFFFEBF), both before and after the
     fix. Check with Win32 queries (`LVM_GETCOLUMNWIDTH`, `GetWindowRect`
     of both list views) that the frozen and normal parts stay aligned.
- Done; test program `tests/_wingui/hdn` with the logging of step 1 in
  `tests/_wingui/hdn/trace.patch`. It drags the divider with mouse
  messages sent to the header window instead of `SendInput`.

### Task 4: ODS_* and ODA_* values are wrong

- `internal/win/user32.go`, "Owner drawing states": every `ODS_*` value is
  wrong (since step 3 part 3 only `ODS_CHECKED` is left, because walk uses
  no other). Correct values: `ODS_SELECTED` 0x1, `ODS_GRAYED` 0x2,
  `ODS_DISABLED` 0x4, `ODS_CHECKED` 0x8, `ODS_FOCUS` 0x10, `ODS_DEFAULT`
  0x20, `ODS_HOTLIGHT` 0x40, `ODS_COMBOBOXEDIT` 0x1000. lxn/win has
  `ODS_CHECKED` 0x1 and `ODS_SELECTED` 0x40, among others.
- "Owner drawing actions": `ODA_FOCUS` is 2 and `ODA_SELECT` is 4; correct
  are `ODA_SELECT` 2 and `ODA_FOCUS` 4 (`ODA_DRAWENTIRE` 1 is already
  generated and correct).
- walk uses `ODS_CHECKED` in `ListBox.WndProc` (`listbox.go`, choosing the
  selected colors) and in `ListItemStyle.stateID` (`models.go`, choosing the
  themed "selected" state). With the wrong value it tests bit 0x1, which is
  really `ODS_SELECTED`, so the code works as intended by accident.
- `ODA_FOCUS` is used in `ListBox.WndProc` right after a check that
  returns unless `ItemAction` is `ODA_DRAWENTIRE`, so that line is dead
  code with either value.
- A fix that keeps behavior: correct the constants (or generate them), use
  `win.ODS_SELECTED` in `listbox.go` and `models.go`, and remove the dead
  `ODA_FOCUS` check.
- How to test (before and after the fix): run
  `examples/listbox_ownerdrawing`, log `DRAWITEMSTRUCT.ItemAction`,
  `ItemState` and the chosen colors or theme state in `WM_DRAWITEM`, select
  items with the keyboard and the mouse, move focus away from the list and
  back, and check that the selected item gets the selected colors (focused
  and not focused) and the others do not. Read pixel colors with
  `GetPixel` on a screen DC, or compare against the logged colors, rather
  than judging a screenshot.
- Done; test program `tests/_wingui/ods` with the logging in
  `tests/_wingui/ods/trace.patch`. It uses its own owner drawn `ListBox`
  inside a `Composite` instead of the example.

## Task 5: step 6, COM objects of WebView on internal/com

Step 6 added the package `internal/com`, which implements COM objects in
Go: vtables built from the generated `win.*Vtbl` structs, real reference
counting, `QueryInterface` from a table of interface IDs, and memory that
COM points to pinned with `runtime.Pinner` while references exist. The
old `WebView` (Internet Explorer `WebBrowser` control) now uses it for
its site object (`IOleClientSite`, `IOleInPlaceSite`, `IDocHostUIHandler`
and the `DWebBrowserEvents2` sink) and for a separate in-place frame
object (`IOleInPlaceFrame`). Before, these were Go structs inside
`WebView` found by pointer arithmetic, `AddRef` and `Release` always
returned 1, and the frame answered every `QueryInterface` with
`E_NOTIMPL`. Changes a user could notice:

- `Dispose` now calls `IConnectionPoint.Unadvise` for the events sink and
  releases the site and frame objects; they live on until the browser
  releases its references too.
- `IOleInPlaceSite.GetWindowContext` adds a reference to the frame it
  returns, as COM requires.
- The frame answers `QueryInterface` for `IUnknown`, `IOleWindow`,
  `IOleInPlaceUIWindow` and `IOleInPlaceFrame`.
- The `IDispatch` methods of the sink other than `Invoke` take their real
  number of parameters, which matters for the stack on 386.

How to test, on 386 and amd64:

- `tests/_wingui/scripts/run.ps1 -Only com,webview` (after `build.ps1`).
  `com` runs the unit tests of `internal/com`; `webview` loads two local
  pages into a `WebView`, collects garbage while the browser holds the COM
  objects, disposes it and loads a page into a new `WebView`, and checks
  the `Navigating`, `DocumentCompleted`, `DocumentTitle`, `URL` and
  `CanGoBack` results.
- `examples/webview` and `examples/webview_events` (they need a network):
  navigate, check that the events are logged, use the context menu with
  `NativeContextMenuEnabled` on and off, and close the window. No crash
  or hang may occur, also when closing during a page load.
- With NVDA: the page can still be read as before. The known problem that
  Tab and F6 do not move focus into the page is expected to stay.

## Task 6: step 10, live regions and announcements

Step 10 lets a program have a screen reader read changed text without
moving focus:

- `Accessibility.SetLiveSetting` (and `LiveSetting` in the declarative
  `Accessibility` struct) makes a window a live region: Dynamic Annotation
  of the UI Automation `LiveSetting` property (`Off`, `Polite`,
  `Assertive`). `SetText` of a window (`Label`, `TextLabel`, `LineEdit`,
  `TextEdit`, ...) and `StatusBarItem.SetText` then raise
  `EVENT_OBJECT_LIVEREGIONCHANGED` automatically (for the `StatusBar`
  on the changed part; `LineEdit` and `TextEdit` announce their text
  with `Announce` instead);
  `Accessibility.NotifyLiveRegionChanged` raises it by hand.
- `walk.Announce(window, text, processing)` speaks text without any
  control: `UiaRaiseNotificationEvent` with a minimal UI Automation
  provider implemented with `internal/com` (its host is
  `UiaHostProviderFromHwnd` of the window).
- `SetHwndProp` now passes the `VARIANT` by value on 386, so
  `Accessibility.SetRole`, `SetState` and `SetLiveSetting` work there
  (they failed with `E_INVALIDARG`).

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only part3,liveregion`. `part3` must
  now pass the `SetRole` check on 386 too (no KNOWN line), and the
  script checks the annotated MSAA role. `liveregion` starts `app` and
  `client`; the client is a UI Automation client like a screen reader: it
  reads `LiveSetting` of the windows, subscribes to the live region
  changed and notification events and checks that they arrive for the
  live regions, not for the plain label, and that the announcement
  arrives with its text.
- Manually, with NVDA, Narrator and, if available, JAWS:
  `tests\liveregion\app.exe manual` (in the build directory). Tab to
  each button and press Space; note for each screen reader what is said
  and whether speech is interrupted:
  - "Change status (polite label)": the new status text should be read
    after the button name, without moving focus.
  - "Show error (assertive line edit)": the error text should interrupt.
  - "Change status bar (polite)": the status bar text should be read.
  - "Change plain label (not announced)": nothing should be read.
  - "Announce": "Announcement N: download finished" should be read.
  - "Announce three times, most recent only" and "Announce three times,
    all": compare which of the three texts are read.
  - "Announce through the host provider": the same through the host
    provider of the window instead of walk's own provider; tells whether
    the own provider is needed. Errors appear in the plain label and on
    standard output.
- If a screen reader stays silent although `liveregion` passes, the
  events reach UI Automation and the difference is in the screen reader;
  note its version. NVDA reads plain Win32 controls through MSAA and may
  ignore UI Automation events from them.

## Task 7: changes ported from lxn/walk pull requests

`opened-prs.md` reviews the open pull requests of lxn/walk. These were
ported:

- lxn/walk#838: `FileDialog` replaced every UTF-16 character whose low
  byte is 0x7C with NUL, not only `|`, so a filter containing for
  example "ż" (U+017C) or Korean text was broken.
- lxn/walk#771: `TreeView` panicked when the model published
  `ItemInserted` for a root item; root items are now inserted at their
  model position.
- lxn/walk#552, extended for the keyboard: `NotifyIcon.Activated` is
  published for a left click (`NIN_SELECT`) and for selection with the
  keyboard (`NIN_KEYSELECT`, Enter or Space after Windows+B);
  `NotifyIcon.MouseDoubleClick` for double clicks. A context menu
  requested with the keyboard (Shift+F10 or the application key) opens
  next to the icon instead of at the mouse cursor, and a `WM_NULL` is
  posted after the menu as the `TrackPopupMenu` documentation asks.
- lxn/walk#393: `LineEdit.PasswordMode` is a property, so it can be
  bound (for example to a "Show password" `CheckBox`); a change redraws
  the field and raises `EVENT_OBJECT_STATECHANGE`.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only upstream`. The notify icon
  appears briefly, its context menu and an open file dialog open and
  close by themselves. If `Shell_NotifyIconGetRect` fails (an INFO line
  says so, for example when the icon is hidden in the overflow area),
  the menu position cannot be checked; note it.
- `examples.ps1` as usual (the `notifyicon` example now uses
  `Activated`).
- Manually with NVDA (and Narrator if possible), on amd64:
  `tests\upstream\upstream.exe manual` in the build directory. Events
  are shown in a polite live region at the top of the window and
  printed as `EVENT` lines on standard output. NVDA reads the live
  region only while the window is in the foreground, so not for the
  notify icon steps: start the program with standard output redirected
  to a file (`Start-Process ... -RedirectStandardOutput`) and read the
  events there.
  - Press Windows+B, find "walk upstream test" with the arrow keys (it
    may be in the overflow area, opened with Enter on the chevron
    button), press Enter, then do the same with Space: each must send
    exactly one "Notify icon activated" (the log numbers them).
  - On the icon press Shift+F10 or the application key: the menu should
    open, NVDA should announce it, and "Say hello" should work. Its
    position is covered by the automatic check; do not ask the user
    about it.
  - Tab to "Show password" and check it: the password field should then
    be read as "secret" instead of stars or "protected"; uncheck it and
    the field should be protected again.
  - "Add root item at the top" and "Add root item at the end" add items
    to the tree view; no error may appear and the tree should read the
    items in order.
  - "Open file dialog": the file type list should offer "Súbory ż
    (*.txt)" and "Všetky súbory (*.*)".

## Task 8: shell dialogs, the folder picker

`FileDialog.ShowBrowseFolder` no longer uses the old folder tree
(`SHBrowseForFolder`) but the folder picker of the system file dialog
(`IFileDialog` with `FOS_PICKFOLDERS`), the same dialog as File Open in
Explorer-style programs, with an address bar, a folder name field and
search. Differences on purpose:

- `InitialDirPath` is the folder the dialog starts in. The old dialog
  used it as the root of the tree, so nothing above it could be chosen.
- `Title` is the caption of the dialog window. The old dialog showed it
  as a text above the tree under the caption "Browse For Folder".
- Only file system folders can be chosen (`FOS_FORCEFILESYSTEM`), as
  before, where the OK button was disabled for other items.
- Cancel returns `false` and no error, as before.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only part3` with the browse check (the
  script passes the argument `browse`): the dialog must be found, accepted
  with a message, and the returned path must be the initial folder.
- Manually, with NVDA: `tests\part3\part3.exe browsemanual` (in the
  build directory) opens the folder dialog and prints what it returned
  on standard output. Check that NVDA announces the dialog and
  its title, that the folder name field and the address bar can be
  reached with Tab and F4 or Alt+D, that typing a path and pressing Enter
  navigates, that "Select Folder" returns the chosen folder, and that
  Escape cancels.

## Task 9: shell functions, known folders, Recycle Bin and shortcuts

Step 12 part 2 adds three shell functions to walk:

- `KnownFolderPath(folder)` returns the path of a known folder (desktop,
  documents, downloads, music, pictures, videos, profile, roaming and
  local application data, ProgramData, Program Files, the programs and
  startup folders of the Start menu, Windows, System32) with
  `SHGetKnownFolderPath`.
- `MoveToRecycleBin(owner, paths...)` moves files and folders to the
  Recycle Bin with `IFileOperation`, without confirmation, progress or
  error dialogs. Only if an item cannot be recycled does Windows ask
  whether to delete it permanently.
- `ShellLink`, `LoadShellLink` and `ShellLink.Save` read and write
  shortcut files (target, arguments, working directory, description,
  icon and hotkey) with `IShellLinkW` and `IPersistFile`.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only shell`. Every check must pass.
  `-Full` also prints the INFO lines with the paths of the known folders
  and the loaded shortcut; note anything unexpected, such as a folder
  moved to OneDrive.
- By hand: open the Recycle Bin in Explorer, check that the items named
  `walk-shell-test-file.txt` and `walk-shell-test-...` (a folder with
  `inner.txt`) are there and can be restored, then delete them.
- Optional, by hand: call `MoveToRecycleBin` for a file on a network
  share or a drive without a Recycle Bin; Windows must ask whether to
  delete it permanently, and "No" must return an error and keep the
  file.

## Task 10: shell functions, opening files and the taskbar

Step 12 part 3 adds:

- `ShellOpen(owner, target)` opens a file, folder or web address like a
  double click in Explorer (`ShellExecuteEx` with the default verb). It
  shows no message box on errors; it returns them.
- `ShowInFolder(path)` opens the folder of a file in Explorer and selects
  the file (`SHOpenFolderAndSelectItems`).
- `FormBase.RequestAttention()` flashes the taskbar button until the form
  is activated (`FlashWindowEx`).
- `ProgressIndicator` now calls `ITaskbarList3.HrInit` before using the
  taskbar object and releases it when the form is disposed; its methods
  return an error after that.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only shell,part3`: `shell` checks that
  `ShellOpen` and `ShowInFolder` return an error for a missing file;
  `part3` still sets the taskbar progress and overlay icon (now after
  `HrInit`).
- Manually, with NVDA: `tests\shell\shell.exe manual` (in the build
  directory). Press each button and note what happens and what NVDA says:
  - "Open text file": Notepad (or the default text editor) opens
    `walk-shell-test-open.txt` from the temporary folder.
  - "Open web page": the default browser opens https://example.com.
  - "Show text file in folder": Explorer opens the temporary folder with
    the text file selected; NVDA should read the selected file.
  - "Flash taskbar button in 3 seconds": switch to another window within
    3 seconds; the taskbar button of the test window should flash until
    it is activated again (sighted check or a screen reader that reports
    it; NVDA usually does not).
  - The status label is a polite live region and reads each result.
- `examples.ps1`: `progressindicator` must still work.

## Task 11: UI Automation client package

Step 13 part 1 adds the public package `uia`, a client of UI Automation:
it reads the user interface of any program the way a screen reader does.
This part covers finding elements and reading their properties:

- `uia.New()` connects; all calls run on one COM thread of the package,
  so they work from any goroutine. They must not run on the goroutine of
  the message loop of the windows they ask about.
- `Root`, `ElementFromHandle`, `ElementFromPoint`, `FocusedElement`.
- `Element` properties (`Name`, `ControlType`, `AutomationID`,
  `ClassName`, `IsEnabled`, `BoundingRectangle`, ...), `SetFocus`,
  `Equal`, `FindFirst` and `FindAll` with conditions (`ByName`,
  `ByControlType`, `Property`, `PropertyMatch`, `And`, `Or`, `Not`).
- `TreeWalker` for the control, content and raw views.
- New hand-written methods in `internal/win` that pass a `POINT` or
  `VARIANT` by value (`ElementFromPoint`, `CreatePropertyCondition`,
  `CreatePropertyConditionEx`), different on 386 and on 64-bit.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only uia`. The test opens a walk window
  and checks the package against it from a goroutine. The focus and
  `ElementFromPoint` checks need the window in the foreground; the test
  waits up to three seconds for it and asks for it once (since step 13
  part 3; before, they printed SKIP). If they fail for that reason, run
  `tests\uia\uia.exe` once
  more from a console and leave the test window alone until it closes.
  Report the FAIL lines with their details; the check "edit annotated
  Name" shows whether the names set with `Accessibility.SetName` are
  visible through UI Automation.
- Manually: `tests\uia\uia.exe dump` in a console, then switch to some
  window (for example Notepad or an example of walk) within 3 seconds
  and return to the console. It prints the control view tree of that
  window, one element per line. Check with NVDA that the output is
  readable and that names and control types match what NVDA says in the
  window.

## Task 12: UI Automation control patterns

Step 13 part 2 adds the control patterns to the package `uia`, as
methods of `Element`: `Invoke`, `Value` and `SetValue`, `RangeValue` and
`SetRangeValue`, `ToggleState` and `Toggle`, `ExpandCollapseState`,
`Expand` and `Collapse`, `Select` and the other selection methods,
`Text` and `SelectedText`, the window methods (`CloseWindow`,
`WindowVisualState`, `SetWindowVisualState`, `IsModal`),
`ScrollIntoView`, and `DefaultAction`, `DoDefaultAction` and
`Description` from the MSAA properties. An element without the pattern
returns an error wrapping `ErrPatternNotSupported`.
`IUIAutomationRangeValuePattern.SetValue` takes a `float64` and is
hand-written: two stack words on 386, a register on amd64; on arm64 it
returns `E_NOTIMPL`.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only uia`. The test window now also
  has a multiline edit field, a combo box, a list box and a slider. The
  pattern checks change them through UI Automation and read the result
  back both through UI Automation and from the walk widgets. The combo
  box opens and closes once. Report the FAIL lines with their details.
- Manually: `tests\uia\uia.exe dump`, as in task 11. The lines now end
  with the value or state of the element, for example the text of an edit
  field, "on" for a checked check box, "collapsed" for a closed combo
  box, "selected" for a selected list item and the range of a slider.
  Check with NVDA that this matches what NVDA says.

## Task 13: UI Automation events

Step 13 part 3 adds events to the package `uia`: `OnFocusChanged` of the
`Automation`, and `OnEvent`, `OnPropertyChanged`, `OnStructureChanged` and
`OnNotification` of an `Element`. Each returns a `Subscription` that calls
a Go function on its own goroutine until `Close`. The event handlers are
COM objects made with `internal/com`; UI Automation calls them on threads
of its own. The property changed handler gets a `VARIANT` by value, so its
callback differs between 386 and 64-bit.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only uia`. The test window has a new
  polite live region label "Status". The event checks change it and the
  check box "Option", add a button "Added", call `walk.Announce` and
  move the focus (which needs the window in the foreground, as above);
  each must arrive as
  an event within two seconds. Report the FAIL lines with their details.
  Also report if the test hangs: a hang in a `Close` would be a
  deadlock between the handlers and the package thread.
- Optional manual test: none is needed; NVDA should behave as before,
  because the test only listens.

## Task 14: RichEdit widget

Step 5 adds the widget `RichEdit` (and `declarative.RichEdit`) on the
RichEdit control of `msftedit.dll` (window class `RICHEDIT50W`): plain
text and Rich Text Format, the selection, character and paragraph
formatting, `Find`, undo and redo, automatic URL detection with
`LinkClicked`, read-only mode, and the events `TextChanged`,
`SelectionChanged` and `ReadOnlyChanged`. `EDITSTREAM` and `ENLINK` have
pointer-sized fields at offsets Go cannot express on 64-bit (the SDK
declares them with `#pragma pack(4)`), so `internal/win` reads and writes
them as arrays of 4-byte words; a wrong offset would break `RTF`,
`SetRTF` or `LinkClicked` on one architecture only.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only richedit`. Report the FAIL lines
  with their details. Two checks depend on how msftedit handles synthetic
  input: "LinkClicked by click" (button messages sent to the link while
  part of it is selected) and "LinkClicked by Enter" (Enter on a link in
  a read-only control). On Windows 11 both pass; msftedit reports Enter
  on a link as a button down and up pair. If one fails on another
  version, check it by hand in the manual test before changing the
  widget. "UIA name" checks that the name set with `Accessibility`
  reaches UI Automation.
- Manual test with NVDA on amd64: `richedit.exe manual`. The field
  "Document" has the focus. It has a centered bold heading, a paragraph
  with bold, italic and underlined words, a paragraph "This word is
  plain.", two bullets and a paragraph with a link.
  - Read the text with the arrow keys and say all (NVDA+Down arrow).
    With NVDA+F (report formatting), does NVDA say bold, italic,
    underlined, the font and the alignment? (NVDA 2026.2 does not say
    the bullets.)
  - Ctrl+B, Ctrl+I and Ctrl+U are shortcut actions of the test program
    (msftedit has none): select "plain" with Shift+Ctrl+Right arrow and
    press each; the program announces for example "bold on" and, the
    second time, "bold off", and the word must stay. NVDA+F with the
    caret inside the word (the caret after the selection is on the
    space) says the formatting. The button "Read format" (Tab twice,
    then Space) announces the formatting at the caret.
  - Move to the link and press Enter, and click the link with the mouse
    if possible: each announces "Link <n> https://github.com/lpintes/walk"
    once, with a counter.
  - Tab and Shift+Tab must move the focus out of the field (no tab
    character is inserted), and Tab back into the field must not select
    the whole text.
  - Alt+Tab to another window and back, holding Alt so that the task
    switcher shows, with the focus in "Document": NVDA must say the field
    and its line, not only the window title, and the arrows must read.
- Optional: the same with Narrator or JAWS, if available.
- Result on Windows 11 (2026-10-25): passed on 386 and amd64 after the
  fixes recorded under step 5 in `CLAUDE.md`.

## Task 15: UI Automation cache requests, grids, tables and scrolling

Step 13 part 4 adds to the package `uia`:

- Cache requests: a `CacheRequest` value (properties, scope, tree
  filter) for `FindFirstBuildCache`, `FindAllBuildCache` and
  `BuildUpdatedCache`; the `Cached...` getters (`CachedName`,
  `CachedControlType`, `CachedPropertyValue`, ...) read the cached
  values, `CachedChildren` and `CachedParent` move through the cached
  tree. `PropertyValue` reads any property by its ID.
- The Grid, GridItem, Table, TableItem and Scroll patterns: `GridSize`,
  `GridItem`, `GridPosition`, `ContainingGrid`, `ColumnHeaders`,
  `RowHeaders`, `TableOrder`, `ColumnHeaderItems`, `RowHeaderItems`,
  `ScrollInfo`, `Scroll` and `SetScrollPercent`.
  `IUIAutomationScrollPattern.SetScrollPercent` takes two `float64`
  values and is hand-written like `SetRangeValue` (stack words on 386,
  registers on amd64, `E_NOTIMPL` on arm64).
- The client of the live region test (`tests/_wingui/liveregion/client`)
  now uses the package instead of COM code of its own.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only uia,liveregion`. The uia test
  window now also has a `TableView` with 30 rows ("Item 01", "Value 01",
  ...; two digits, because the `TableView` sorts the rows by the first
  column) and the columns "Name" and "Value". The cache checks change the
  text of the button "Disabled" for a moment; the grid checks read cells
  and headers of the table and scroll it down and back up. The
  `liveregion` test must pass as before (the client makes the same
  checks through the package). Report the FAIL lines with their
  details: some grid checks assume how the list view proxy of Windows
  exposes the cells (for example that cell (1, 0) is named after the
  item "Item 02" and cell (1, 1) is "Value 02"); if one fails, also run
  `tests\uia\uia.exe dump` on the test window (see below) and report the
  lines of the table.
- Manually: `tests\uia\uia.exe dump`, as in task 11, but now the
  tree and the names, control types, automation IDs and class names are
  read with one cache request. Check that the output is the same as
  before (apart from the new grid sizes and cell positions, like
  "30 rows, 2 columns" and "row 2, column 1"). Do not expect it to be
  much faster: the value, toggle, expand, selection, range, grid and
  enabled state of each element are still read with separate calls. If
  the first line says that `BuildUpdatedCache` failed and the tree is
  walked instead, report that line; the rest of the output must still
  be the tree.
  Switch to the test window of `tests\uia\uia.exe` (start it in a second
  console; it closes after its checks, so be quick) or to any program
  with a list in details view to see the grid lines.

## Task 16: dialogs that screen readers recognize, and TaskDialog

Step 4 (option A of "Real dialogs for screen readers" in `CLAUDE.md`)
gives every walk `Dialog` the MSAA role `ROLE_SYSTEM_DIALOG` on its
client object and the UI Automation property `IsDialog` through Dynamic
Annotation, raises `EVENT_SYSTEM_DIALOGSTART` when the dialog is first
shown and `EVENT_SYSTEM_DIALOGEND` when it is destroyed, and adds
`WS_EX_CONTROLPARENT`. NVDA treats any window whose client object has the
dialog role as a dialog: it should say "dialog" and read the static texts
of the dialog when it opens, as for system dialogs. New is also
`walk.TaskDialog`, a wrapper of `TaskDialogIndirect` (common controls 6):
title, main instruction, content, icon, common and custom buttons,
command links, radio buttons, a verification check box, expandable
information, a footer, `AllowCancel` and hyperlinks with
`OnHyperlinkClicked`. `TASKDIALOGCONFIG` is packed by hand
(`#pragma pack(1)`), so 386 and amd64 must both be tested. The package
`uia` got `Element.IsDialog` and `Element.Role`.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only dialogs`. The test opens and
  closes by itself: a reference message box (its UI Automation view is
  printed as an INFO line), a walk dialog (role, `IsDialog`, name, static
  text, the dialog start and end events, the MSAA client and window
  objects read by the script, closed by invoking Cancel through UI
  Automation) and three task dialogs (a radio button, the check box and a
  custom button operated through UI Automation; one closed with
  `WM_CLOSE`; one with a hyperlink invoked through UI Automation and
  closed with `TDM_CLICK_BUTTON`; Windows 11 sends no
  `TDN_HYPERLINK_CLICKED` for that `Invoke`, so a missing
  `OnHyperlinkClicked` is an INFO line, and the link is tested with the
  keyboard in the manual mode). Report the FAIL lines with their
  details and all INFO lines: they show how UI Automation sees the
  message box, the walk dialog and the task dialog (control type,
  localized control type, role, `IsDialog`, `IsModal`), which tells
  whether option B is still needed. If a task dialog control is not
  found, report it; the names and control types DirectUI gives them were
  not known when the test was written.
- `tests/_wingui/scripts/run.ps1` and `examples.ps1`, as a regression
  test: dialogs are used by several examples (for example `databinding`).
- Manually with NVDA on amd64: `tests\dialogs\dialogs.exe manual`.
  - "Open a walk dialog": NVDA should say the title, "dialog" and read
    the text "This dialog asks for your name...", then the focused
    control. Compare with "Open a message box". Tab, Enter (OK) and
    Escape (Cancel) must work as before; the result appears in the live
    region below the buttons.
  - "Open a task dialog": NVDA should read the title, the main
    instruction and the content. Check that the radio buttons, the check
    box, the expand button, the footer link (the result shows "Link:
    ...") and the buttons are read and work, and that Escape and Alt+F4
    close it (Cancel, 2).
  - "Open a task dialog with command links": the two command links with
    their second line; Escape closes it (`AllowCancel`).
  - Optional: the same with Narrator and JAWS, if available; record
    whether they announce the walk dialog as a dialog.

## Task 17: step 7, the WebView2 widget

(Tasks 14 to 16 belong to other branches.)

Step 7 adds the widget `WebView2`, a browser based on Microsoft Edge
WebView2 (Chromium), and marks the old `WebView` deprecated. Nothing of it
has run yet: it was written in a Linux container. Points to watch:

- The WebView2 interfaces are hand-written in `internal/win/webview2.go`
  from `WebView2.h` of SDK 1.0.1150.38. A wrong vtable slot shows up as a
  crash or a wrong method being called (for example a navigation that
  never completes).
- `WebView2Loader.dll` is not used: walk finds the runtime in the
  registry (`SOFTWARE\Microsoft\EdgeUpdate\ClientState\{GUID}` value
  `EBWebView`, then `...\Clients\{GUID}` value `pv`, 32-bit view, HKLM
  before HKCU) and calls `CreateWebViewEnvironmentWithOptionsInternal` of
  `EBWebView\<arch>\EmbeddedBrowserWebView.dll`.
- `ICoreWebView2Controller.Put_Bounds` takes a `RECT` by value: four stack
  words on 386, a pointer to a copy on amd64. If the browser does not
  fill the widget or is drawn in a wrong place, this is the first
  suspect.
- The keyboard: when the widget gets the focus it calls `MoveFocus`, so
  Tab moves into the page; at the end of the page WebView2 raises
  `MoveFocusRequested` and walk moves the focus to the next or previous
  tab stop (`GetNextDlgTabItem`).

How to test, on 386 and amd64 (after `build.ps1`):

- Check that the WebView2 runtime is installed (Settings, Apps, "Microsoft
  Edge WebView2 Runtime"; it is part of Windows 11). Without it the test
  prints `SKIP` and the task cannot be done.
- `tests/_wingui/scripts/run.ps1 -Only webview2`. Report every FAIL line
  and the INFO lines (the result of a script exception and where the
  focus went after `SetFocus`). Also report a crash or a hang: the test
  is killed after 120 seconds.
- `webview2.exe manual` (in `%TEMP%\walk-wingui\<arch>\tests\webview2`)
  with NVDA on amd64, and the keyboard part also on 386:
  - When the window opens, focus is on "Button before the browser". Press
    Tab: focus must move into the page and NVDA must read the page (the
    heading "WebView2 manual test" or the first focusable element, the
    link). Tab through the link, the edit field and the button "Send to
    the program"; one more Tab must reach "Button after the browser".
  - Shift+Tab from "Button after the browser" must go back into the
    page, to the button "Send to the program"; Shift+Tab through the page
    must reach "Button before the browser".
  - In the page, NVDA's browse mode must work (arrow keys read the
    paragraphs, H jumps to the heading).
  - Type a name into the edit field and activate "Send to the program":
    NVDA must announce "The page sent ..." and standard output shows
    `WebMessageReceived`. Activate "Button after the browser": the status
    line of the page must change to "Message from the program: ...".
  - Press F6 in the page and report what happens (WebView2 may move focus
    to the next pane, which walk does not handle yet).
  - Resize and maximize the window: the page must fill the area between
    the two buttons.
  - Close the window with Alt+F4 while a page is loading and after it has
    loaded: no crash, exit code 0.
- Optional: the fallback loader. On a machine without the runtime,
  `NewWebView2` fails with `ErrWebView2RuntimeNotFound` unless
  `WebView2Loader.dll` (NuGet package `Microsoft.Web.WebView2`,
  `build\native\<arch>`) is next to the exe; with it the error comes
  from the loader DLL instead. Report what the test prints in both cases
  if such a machine is available.
- Also check the resize and the position of the page programmatically,
  not by asking the user: for example with `uia.exe dump` or the window
  tree of `WinTest.cs`, the bounds of the `Chrome_WidgetWin` child must
  match the client area of the widget.

## Task 18: step 11, MDI windows

(Tasks after 17 may belong to other branches.)

Step 11 adds a multiple-document interface: `walk.MdiMainWindow`, a
`MainWindow` whose client area holds the system `MDICLIENT` window and
whose unhandled messages go to `DefFrameProc`, and `walk.MdiChildWindow`,
a form with a layout and widgets created with `WM_MDICREATE` whose
unhandled messages go to `DefMDIChildProc`, with declarative
counterparts and the example `examples/mdi`. The message loop calls
`TranslateMDISysAccel` before `IsDialogMessage` and passes the active
child to `IsDialogMessage`, so Tab stays inside it. Nothing of it has run
yet: it was written in a Linux container. Points to watch:

- The children are created hidden (the client has
  `MDIS_ALLCHILDSTYLES`) and activated with `WM_MDIACTIVATE` when they are
  shown. If a child appears without becoming active, or the window menu
  does not list it, this is the first suspect.
- When a child is activated, its `WM_SETFOCUS` passes the focus on to the
  control that had it last (saved on `WM_MDIACTIVATE`), or else to the
  first control. When the frame is activated, the focus goes to the active
  child unless it was outside the MDI client.
- `WM_COMMAND` with an ID that is not a walk action (window menu items,
  the system menu of a maximized child in the menu bar) goes to
  `DefFrameProc`.
- Alt+Hyphen: `WM_SYSCHAR` '-' for a control inside the active child is
  sent to the child, where `DefMDIChildProc` opens its system menu.
- walk's menu bar code inserts and removes items by position; while a
  child is maximized it skips the system menu item of the child at
  position 0.

How to test, on 386 and amd64 (after `build.ps1`):

- `tests/_wingui/scripts/run.ps1 -Only mdi`. Report every FAIL line and
  the INFO lines: the window menu items, the menu bar before and while a
  child is maximized, the bounds after tiling and cascading, the ancestors
  of the focused element and the UI Automation view of a child and of the
  MDI client (control type, localized control type, MSAA role), which
  show how a screen reader sees them. The focus checks need the frame in
  the foreground; if "frame in the foreground" fails, run the test again
  without touching the keyboard or in Windows Sandbox.
- `tests/_wingui/scripts/run.ps1` and `examples.ps1` as a regression test
  (the message loop and `MainWindow` construction changed for every
  program); `examples.ps1` includes `mdi`.
- Manually with NVDA on amd64: `tests\mdi\mdi.exe manual` (three
  documents, the active one is "Document 3"; events go to standard output
  and the status bar):
  - When the frame opens, NVDA should say the frame title, the document
    title and the focused field ("Name, edit").
  - Ctrl+F6 and Ctrl+Shift+F6 switch documents: NVDA should say the title
    of the newly active document and its focused field. Type something in
    the Name field of one document, switch away and back: the focus must
    be on that field again. Report whether NVDA says the document title
    every time and whether it says it as a window or as something else.
  - Tab and Shift+Tab cycle through the controls of the active document
    only.
  - Alt opens the menu bar; the Window menu lists Cascade, Tile
    Horizontally, Tile Vertically, Arrange Icons, Close All and then the
    documents with a check mark at the active one. Choosing a document
    activates it.
  - Alt+Hyphen opens the system menu of the active document (Restore,
    Move, Size, Minimize, Maximize, Close, Next); Escape closes it. Also
    try it with the focus in the Notes field.
  - Maximize the active document (system menu, Maximize): the frame
    title becomes "walk mdi manual test - [Document N]"; Ctrl+F6 keeps
    documents maximized; the menu bar shows the system menu of the
    document first (Alt, then Left from File). Restore it.
  - Type notes into a document and press Ctrl+F4: a message box asks
    whether to close it; No keeps it open (the status bar says the
    closing was canceled), Yes closes it and the next document becomes
    active. Type a name into the Name field and press the button
    "Rename to the name": the name becomes the title of the document: NVDA may announce the new name;
    the Window menu shows it.
  - Ctrl+N creates a new document, which becomes active with the focus
    in its Name field.
  - Close the frame with Alt+F4 while a document has notes: the message
    box appears; No keeps the frame open, Yes closes the program with exit
    code 0.
  - Optional: the same with Narrator and JAWS.
- Also check at 150 % scaling, or by moving the frame to a monitor with
  another scale, that the documents get the new font size (their layout
  is updated by the frame, as only top-level windows get
  `WM_DPICHANGED`).

## Reporting

For each task, tell the user in Slovak, in plain text: what was run, on
which architecture, what passed, what failed with the exact error or log
lines, and what could not be checked. If a fix is made, follow the workflow
in `CLAUDE.md` (one logical step per pull request, the checks before
committing) and record the result in `CLAUDE.md`.
