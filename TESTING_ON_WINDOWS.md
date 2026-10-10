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
    C# in Windows PowerShell 5.1 (`Add-Type -ReferencedAssemblies
    Accessibility`, cast to `Accessibility.IAccessible`) gives role, name,
    value, state and help, and `GetGUIThreadInfo` gives the focused window
    of the tested thread.
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
    inside this module, for example under a temporary directory of
    `examples/`, because `internal/win` is internal). Since step 3 part 3,
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
  - A `MainWindow` without a layout panics in `SetVisible` (nil layout in
    `ContainerBase.CreateLayoutItem`), so give test windows a layout.

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

## Reporting

For each task, tell the user in Slovak, in plain text: what was run, on
which architecture, what passed, what failed with the exact error or log
lines, and what could not be checked. If a fix is made, follow the workflow
in `CLAUDE.md` (one logical step per pull request, the checks before
committing) and record the result in `CLAUDE.md`.
