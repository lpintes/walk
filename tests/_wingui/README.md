# GUI tests on Windows

Test programs and scripts that run walk on a real Windows desktop. They were
written for the Windows tasks in `TESTING_ON_WINDOWS.md` (the smoke test of
step 3, the `DragFinish`, `HDN_*` and `ODS_*` bugs, the COM objects of
`WebView` in step 6, the `WebView2` widget of step 7, the live regions
of step 10, the changes ported from lxn/walk pull requests, the UI
Automation client of step 13 and the dialogs of step 4) and can be rerun as
regression tests after changes to `internal/win`, `internal/com` or the
affected widgets. `com` is not a directory here: `build.ps1` builds the
unit tests of `internal/com` into `tests\com\com.exe`.

The directory name starts with an underscore, so `go build ./...`,
`go vet ./...` and the other checks skip it. `hdn` and `ods` do not even
compile without their trace patch.

## Requirements

- Windows 10 or 11 with an interactive desktop session. The tests open
  windows; they cannot run in a container or as a service.
- Go 1.23 or newer, git, and PowerShell 7 (`pwsh`). Windows PowerShell 5.1
  may work but was not tried with these scripts.
- The repository as working directory, with no uncommitted changes in the
  files touched by `hdn/trace.patch` (`tableview.go`) and
  `ods/trace.patch` (`listbox.go`, `models.go`).

## Running

From the repository root, for `amd64` or `386`:

```
.\tests\_wingui\scripts\build.ps1 -Arch amd64
.\tests\_wingui\scripts\run.ps1 -Arch amd64
.\tests\_wingui\scripts\examples.ps1 -Arch amd64
```

- `build.ps1` builds all examples and tests into
  `%TEMP%\walk-wingui\<arch>\examples` and `...\tests` (`-Out` changes the
  base directory). The examples and the tests are each built with one
  `go build`, which compiles and links them in parallel. Every exe gets the examples' manifest (common controls
  version 6, DPI awareness) next to it; the examples also have it
  embedded. `hdn` and `ods` are built with their `trace.patch` applied
  to the working tree; the patch is reverted right after the build, also
  when the build fails.
- `run.ps1` runs the self-checking tests and prints their PASS, FAIL,
  KNOWN and SKIP lines and a summary; the full output stays in a file
  next to each exe, whose path is printed (`-Full` prints it all).
  `-Only part3,hdn` runs a subset, `-Drag` adds `olednd`. Exit code 1 if
  a test failed.
- `examples.ps1` starts every example, writes its window tree with the MSAA
  object of every window to `%TEMP%\walk-wingui\examples-<arch>.txt`,
  posts a few Tab presses to the focused window, records where focus went,
  and closes the example with `WM_CLOSE`. It prints PASS, FAIL or KNOWN per
  example. `-Only` and `-Tabs` are available.

Windows pop up and close while the scripts run. No global keyboard input is
sent and no window is forced to the foreground (the `uia` test only asks
`SetForegroundWindow` once for its own window): global input lands in
whatever window has the foreground and once closed an application of the
user. Keys are posted only to a window of the tested process, and windows
are closed with `WM_CLOSE`. Because of that, Tab steps in `examples.ps1`
only work for examples that happen to get the foreground; otherwise the log
says `focus=<none>`. `olednd` moves the real mouse pointer and restores it.

KNOWN marks a check that fails because of an inherited bug listed under
"Known issues" in `CLAUDE.md`; it does not fail the test. If such a bug is
fixed, the check prints PASS, and the KNOWN handling should be removed.

## Running in Windows Sandbox

The windows of the tests take the focus of the desktop, which makes the
machine unusable while they run. `sandbox\sandbox.ps1` runs them in Windows
Sandbox instead (Windows 10 or 11 Pro or Enterprise; enable the optional
feature `Containers-DisposableClientVM` and reboot once):

```
.\tests\_wingui\sandbox\sandbox.ps1
.\tests\_wingui\sandbox\sandbox.ps1 -Arch amd64 -Only dialogs
.\tests\_wingui\sandbox\sandbox.ps1 -Examples
```

- It builds with `build.ps1` on the host into
  `%LOCALAPPDATA%\walk-sandbox\out` (`-NoBuild` reuses the last build),
  starts the sandbox with PowerShell 7 of the host, the `tests\_wingui`
  directory (read-only) and the build output mapped, and waits.
  `sandbox\inside.ps1`, the logon command of the sandbox, runs `run.ps1`
  (and `examples.ps1` with `-Examples`; `-NoRun` runs only that) for each
  architecture, writes their output to
  `%LOCALAPPDATA%\walk-sandbox\results` and shuts the sandbox down. The host
  script then prints every line that is not PASS (`-Full` prints all) and
  exits with 1 if a line is FAIL.
- The `.out.txt` files that the output names under `C:\walk-wingui` are in
  `%LOCALAPPDATA%\walk-sandbox\out` on the host.
- The sandbox has no network and no shared clipboard. Its window appears on
  the host desktop once, when it starts, and captures the keyboard while it
  has the focus; the tests themselves run inside it. `-Minimize` minimizes
  the window as soon as it appears. The shutdown inside the sandbox does not
  close the window, so the host script closes it after the results are
  written; `-Keep` leaves it running.
- Only automated tests belong there: the manual modes need a screen reader
  on the real desktop, and `olednd` (`-Drag`) is not run.

## Tests

- `part3`: the functions generated in step 3 part 3. `GetObject` (bitmap
  and icon size), `AlphaBlend` (exact pixels of an opaque, a half
  transparent and a transparent pixel drawn on white), `ITaskbarList3`
  (progress and overlay icon of the taskbar button), `IAccPropServices`
  (`Accessibility.SetName` and `SetRole`; `run.ps1` reads the annotated
  name back through MSAA) and, with the argument `browse`,
  `FileDialog.ShowBrowseFolder` (the folder picker of the system file
  dialog, `IFileDialog`, since the shell dialogs step; it is accepted
  with a message to it, the returned path must be the initial one). `run.ps1`
  also checks the annotated role (`ROLE_SYSTEM_LINK`); `SetRole` failed
  on 386 before step 10.
- `liveregion`: `liveregion/app` is a walk window with live regions (a
  polite `Label`, an assertive read-only `LineEdit`, the `StatusBar`), a
  plain `Label` and buttons that change them or call `walk.Announce`.
  `liveregion/client` is a UI Automation client: it reads the
  `LiveSetting` property of these windows, subscribes to the live region
  changed and notification events of the main window, signals the app
  (named event `Local\walk-liveregion-run`) to change the texts and
  announce, and checks the events. The app checks with a WinEvent hook
  that `EVENT_OBJECT_LIVEREGIONCHANGED` is raised. `app.exe manual`
  only shows the window, for a test with a screen reader.
- `upstream`: the changes ported from open lxn/walk pull requests (see
  `opened-prs.md`). Inserts root items into a `TreeView` at the top and
  the end (no panic, model order), toggles a `LineEdit` whose
  `PasswordMode` is bound to a "Show password" `CheckBox` (mode,
  `ES_PASSWORD` style, change events), sends `NIN_SELECT`,
  `NIN_KEYSELECT` and `WM_LBUTTONDBLCLK` to the `NotifyIcon` callback
  (`Activated`, `MouseDoubleClick`), sends `WM_CONTEXTMENU` as the shell
  does for Shift+F10 and checks that the menu opens next to the icon
  (`Shell_NotifyIconGetRect`), and opens the file dialog with a filter
  containing U+017C and reads the items of its file type combo box. A
  thread timer closes the menu and the dialog; the notify icon shows for
  a moment and walk brings its window to the foreground for the menu.
  `upstream.exe manual` keeps the window open for a test by hand; its
  events appear in a polite live region.
- `shell`: the shell functions of step 12 part 2, without a window.
  `KnownFolderPath` for every known folder (the ones with an environment
  variable, such as `APPDATA`, must match it; Documents must match
  `PersonalPath`), `MoveToRecycleBin` with a file and a folder from the
  temporary folder (they must be gone afterwards; they stay in the
  Recycle Bin under names starting with `walk-shell-test`) and with a
  missing file (error), a `ShellLink` with every field saved, loaded
  back and compared, and `ShellOpen` and `ShowInFolder` with a missing
  file (error). `shell.exe manual` shows a window with buttons for
  `ShellOpen` of a text file and a web page, `ShowInFolder` and
  `RequestAttention` after 3 seconds; results go to a polite live
  region and standard output.
- `uia`: the UI Automation client package `uia` (step 13 part 1),
  checked from a goroutine against a walk window of its own process:
  properties of the window and its controls, `FindFirst` and `FindAll`
  with every kind of condition, the tree walkers, focus and
  `ElementFromPoint` (FAIL if the window does not get the foreground
  within three seconds),
  concurrent calls, finalizers, `Release`, `Close` and an element of a
  closed window. Since step 13 part 2 also the control patterns, each
  change read back through UI Automation and from the walk widget:
  Invoke and DoDefaultAction (button), Toggle (check box), Value (edit
  field, combo box), Text (multiline edit field), ExpandCollapse (combo
  box), Selection and SelectionItem (list box), RangeValue (slider),
  Window (main window) and `ErrPatternNotSupported`. Since part 3 also
  the events: focus (also needs the foreground),
  property changes, the live region changed event, structure changes,
  notifications from `walk.Announce` and closing subscriptions. `uia.exe dump`
  prints the control view tree of the foreground window after 3
  seconds, with the value or state of each element.
- `richedit`: the `RichEdit` widget (step 5): plain text (also not
  ASCII and above the default limit of 32767 characters), selection,
  `Find`, undo and redo, character and paragraph formatting of the
  selection, `RTF` and `SetRTF` round trips, `ReplaceSelectedRTF`,
  automatic URL detection with `LinkClicked` by a click and by Enter,
  read-only mode, and through the `uia` package the control type, the
  name and the Text pattern. `richedit.exe manual` shows formatted text
  for listening with a screen reader; Ctrl+B, Ctrl+I and Ctrl+U format
  the selection, the button "Read format" announces the formatting at
  the caret, and activating a link announces its URL.
- `dialogs`: the dialogs of step 4. A reference `MsgBox` (its UI
  Automation view is printed as INFO), a walk `Dialog` (through `uia`:
  MSAA role `ROLE_SYSTEM_DIALOG`, `IsDialog`, name and static text, like
  the message box; a WinEvent hook must see `EVENT_SYSTEM_DIALOGSTART`
  and `EVENT_SYSTEM_DIALOGEND`; closed by invoking Cancel through UI
  Automation, `Run` must return `DlgCmdCancel`), and three `TaskDialog`s:
  one with custom and common buttons, radio buttons and a verification
  check box operated through UI Automation (`Show` must return them), one
  with `AllowCancel` closed with `WM_CLOSE`, and one whose hyperlink is
  invoked through UI Automation (`OnHyperlinkClicked`) and that is closed
  with `TDM_CLICK_BUTTON`. While the walk dialog is open the test prints
  `DIALOG_HWND` and `MSAA_READY` and waits up to 15 seconds for the named
  event `Local\walk-dialogs-msaa`; `run.ps1` reads the MSAA client
  object (role dialog) and window object (role window) meanwhile and then
  sets the event. `dialogs.exe manual` shows buttons that open a walk
  dialog, two task dialogs and a message box, for a test with a screen
  reader; results go to a polite live region and standard output.
- `webview2`: the `WebView2` widget (step 7) with a temporary user data
  folder. Checks that calls before `Ready` are queued (`SetURL`) or
  return `ErrWebView2NotReady` (`ExecuteScript`), then a local HTML file:
  `NavigationStarting`, `NavigationCompleted`, the title, the URL and
  their events; a web message posted to the page and echoed back by its
  script; `ExecuteScript` with a number, a string and an exception
  (reported as INFO); `NavigateToString`, `CanGoBack`, `GoBack` and
  `CanGoForward`; where the focus goes after `SetFocus` (INFO only,
  because the test window may not get the foreground); and `Dispose`
  followed by a second `WebView2`. Without a WebView2 runtime it prints
  a SKIP line and exits with 0. `webview2.exe manual` shows a button, a
  `WebView2` with a small form and another button, for checking Tab,
  Shift+Tab and a screen reader; events go to standard output and web
  messages from the page are announced with `walk.Announce`.
- `mdi`: the MDI windows of step 11, a frame (`MdiMainWindow`) with
  three children (`MdiChildWindow`) with two edit fields and a button
  each. Checks the window classes and parents (walk's child class inside
  `MDIClient` inside the frame, `WS_EX_MDICHILD`), activation
  (`WM_MDIGETACTIVE`, `ActiveMdiChildChanged`, `Activating`,
  `Deactivating`), the layout and the bounds of a child, the window menu
  (lists the children; `WM_COMMAND` with an item's ID activates it),
  Ctrl+F6, Ctrl+Shift+F6 and Ctrl+F4 posted to the test's own windows with
  Ctrl and Shift set by `SetKeyboardState` for the UI thread only (so
  they go through `TranslateMDISysAccel` in the message loop), `Closing`
  that cancels, the focus (first field of a newly activated child, the
  last focused field when a child is activated again, Tab inside the
  active child, `EVENT_OBJECT_FOCUS`, the focused element inside the child
  through UI Automation; FAIL if the frame does not get the foreground
  within three seconds), `SetTitle` (`EVENT_OBJECT_NAMECHANGE`, the window
  menu, the UI Automation name), maximizing (the system menu of the child
  in the menu bar, the frame title, hiding and showing a menu meanwhile,
  Alt+Hyphen opening the system menu), cascade, tile, minimize, arrange
  icons, `CloseAllMdiChildren` and closing the frame. `mdi.exe manual`
  shows a frame with three documents for a test with a screen reader;
  events go to standard output and the status bar.
- `dragfinish`: builds an `HDROP` with two paths, one of them non-ASCII,
  posts `WM_DROPFILES` to a `MainWindow`, checks that `DropFiles` gets the
  paths and that `GlobalFlags` reports the handle freed afterwards. With
  the argument `manual` it only shows a window that prints dropped files.
- `olednd`: a real OLE drag without keyboard or mouse button input.
  `olednd/target` is a walk window that prints the files dropped on it;
  `olednd/source` creates two files in `%TEMP%\walk-olednd`, gets the
  shell's `IDataObject` for them, moves the pointer over the target and
  calls `DoDragDrop` with an `IDropSource` that drops after a few calls.
  `DoDragDrop` waits for mouse messages, so the source posts
  `WM_MOUSEMOVE` to its own thread. `run.ps1 -Drag` checks that the target
  got exactly the files the source sent.
- `hdn`: a `TableView` with a frozen column. Column widths are changed with
  `LVM_SETCOLUMNWIDTH` and by dragging the header divider with mouse
  messages sent to the header window. Each change must call
  `updateLVSizes` exactly once, right after `HDN_ITEMCHANGEDW`, and the
  frozen and the normal list view must stay adjacent. Needs
  `hdn/trace.patch` (hooks `HDNTrace` and `TableViewHandles`).
- `ods`: an owner drawn `ListBox` in a `Composite`. Items are selected with
  mouse and key messages, focus is moved away and back; for every step the
  pixel colors (`GetPixel`) and the theme state chosen for each item must
  match the selection and focus. Needs `ods/trace.patch` (hook
  `ODSTrace`).

With the argument `manual`, `hdn` and `ods` keep the window open after the
checks, for a look with a screen reader; the scripts do not use it.

## Changing the tests

- Build a single test by hand with
  `go build -o part3.exe ./tests/_wingui/part3` and copy
  `examples\actions\actions.exe.manifest` next to it as
  `part3.exe.manifest` before the first start: Windows caches the
  activation context and ignores a manifest added later until the exe
  changes.
- For `hdn` and `ods`: `git apply tests/_wingui/hdn/trace.patch`, build,
  `git apply -R tests/_wingui/hdn/trace.patch`. Never commit the patched
  files. If walk changes so that a patch no longer applies, apply its hunks
  by hand and regenerate it with `git diff` (`git add -N` the new
  `zz_*.go` file first so that the diff includes it, and `git rm --cached`
  it afterwards).
- Tests print `PASS name: details` or `FAIL name: details` per check and
  exit with 1 if a check failed. A test that keeps its window open (like
  `part3`) prints a line the script waits for and is closed with
  `WM_CLOSE`.
- `scripts/WinTest.cs` is the C# helper the scripts load with `Add-Type`:
  MSAA (`AccessibleObjectFromWindow`), the focused window of a thread
  (`GetGUIThreadInfo`), the child window tree, and posting Tab and
  `WM_CLOSE`. It declares `IAccessible` itself, because `Add-Type
  -ReferencedAssemblies Accessibility` fails in PowerShell 7.6. The managed
  UI Automation client reports every Win32 control as `Pane` on the test
  machine, so the scripts use MSAA.
- PowerShell passes `""`, not NULL, for a string P/Invoke parameter given
  `$null`; pass `[NullString]::Value` instead (for example for the class
  name of `FindWindow`).
- The 386 builds of the examples are console programs, so
  `Process.MainWindowHandle` is the console window; find the walk window
  with `FindWindow` by its title.
- `internal/win` declares only what walk uses. A test that needs other
  Win32 functions or constants declares them itself with
  `windows.NewLazySystemDLL` and plain constants.
