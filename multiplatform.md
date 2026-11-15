# Linux support for walk: analysis and proposal

Status: proposal, written 2026-11-15. Nothing here is implemented.
Statements about third-party projects come from a short web search and
are marked "verify" where they were not checked against the source.

## Summary

- Recommended toolkit on Linux: GTK 4 (4.14 or later), reached without
  cgo through purego. It is the only option that keeps what walk is
  today: native controls, pure Go, and accessibility that the platform
  screen reader (Orca) supports through AT-SPI.
- Recommended architecture: one public API in package `walk`, with a
  separate implementation per operating system in `_windows.go` and
  `_linux.go` files (the model of SWT and of the Go standard library),
  not a backend interface layer under the existing widgets.
- The public API has to change in a few places, because it exposes
  Win32 concepts (`HWND`, `WndProc`, window styles, `HDC`). That is a
  breaking change and belongs together with step 9 (generic API) into
  one "walk v2".
- GTKX is not usable: it is a React and TypeScript framework on Node.js,
  not a Go library. Its idea (a declarative tree over GTK 4) is what
  `walk/declarative` already is.
- Before any refactoring, build a throwaway prototype and test it with
  Orca. Three questions decide the whole plan: the bindings, the layout
  approach and the accessibility of lists and tables.

## What the code looks like today

Numbers from the repository on 2026-11-15:

- Package `walk`: 134 Go files, about 44 000 lines. Every one of them
  has `//go:build windows`, including the files that call no Windows
  function.
- 34 files do not import `internal/win` at all. They are the
  platform-independent core: events (`event.go`, `intevent.go`,
  `stringevent.go`, ...), `property.go`, `expression.go`,
  `condition.go`, `databinding.go`, `validators.go`, `reflectmodels.go`,
  `maptablemodel.go`, `action.go`, `actionlist.go`, `gridlayout.go`,
  `spacer.go`, `color.go`, `inifilesettings.go` and a few list types.
- The other 100 files contain about 4700 references to `win.`. The
  heaviest: `tableview.go` (379), `window.go` (185), `keyboard.go`
  (142), `richedit.go` (140), `accessibility.go` (128), `bitmap.go`
  (101).
- Package `declarative`: 51 files, about 6400 lines. Only 8 files import
  `internal/win`, and only for 8 window style constants (`WS_BORDER`,
  `WS_HSCROLL`, `WS_VSCROLL`, `SS_NOPREFIX`, `ES_NOHIDESEL`,
  `LBS_EXTENDEDSEL`, `LBS_OWNERDRAWVARIABLE`, `LVS_NOSORTHEADER`). The
  rest only calls the public API of `walk`, so it ports almost for free
  once `walk` has the same API on Linux.
- 3 of the 27 examples import `internal/win` (`externalwidgets`,
  `listbox_ownerdrawing`, `logview`).

Where Win32 shows through the public API:

- `Window.Handle() win.HWND`, `Window.WndProc(hwnd, msg, wParam,
  lParam)`, `Window.SendMessage`. 35 types override `WndProc`; it is
  the main extension point inside walk and for custom widgets.
- `InitWindow`, `InitWidget` and `InitWrapperWindow` take a window
  class name and Win32 style bits.
- `LayoutItem.Handle() win.HWND`. The layout engine itself is mostly
  arithmetic; `layout.go` touches Windows in about a dozen calls
  (`DeferWindowPos`, `GetDpiForWindow`, focus and visibility queries).
- `Canvas.HDC()`, `NewIconFromHICON`, `ImageList.Handle()`,
  `SetWindowFont(hwnd, font)`, the `DropFilesEventPublisher`.
- `Key` values are Win32 virtual key codes.
- Two unit systems: `Pixel` (native) and 1/96 inch, with `DPI()` on
  every window and a `...Pixels` variant of most geometry methods.
- `Accessibility` is modeled on MSAA Dynamic Annotation (`AccRole`,
  `AccState` and the property setters).
- `Font`, `Bitmap`, `Icon`, `Cursor`, `Brush`, `Pen` and `Canvas` wrap
  GDI and GDI+ handles.

What is Windows-only by nature: package `uia`, the deprecated
`WebView`, `registry.go` and the registry settings, the taskbar
`ProgressIndicator`, `ShellLink`, the `MdiMainWindow` family.

Conclusion: walk is not a toolkit with a Windows backend. It is a thin
object layer over Win32 controls, and its base class (`WindowBase`) is
a window procedure. A port is a second implementation of the same API,
not a new backend plugged under the existing code.

## Toolkit choice

Requirements, in order: accessibility with Orca; native controls (walk's
whole point); no cgo if possible, so that cross-compilation and the
checks in the cloud container stay simple; an acceptable license next
to BSD; a toolkit that is maintained.

### GTK 4 (recommended)

- Accessibility: GTK 4 talks AT-SPI directly (no ATK). Orca supports it.
  The application side is the `GtkAccessible` interface: role,
  properties, states and relations, which maps well onto walk's
  `Accessibility` type. `gtk_accessible_announce` (GTK 4.14) is the
  counterpart of `walk.Announce`. A `GtkLabel` with a mnemonic widget
  sets the labelled-by relation itself, which matches the automatic
  `Label` association walk got on 2026-11-11.
- Known weak spots, to check in the prototype: `GtkColumnView` and
  `GtkListView` with Orca (rows, cells, column headers, selection),
  text widgets other than the built-in ones (`GtkAccessibleText` exists
  only since 4.14), and that an accessible role can only be chosen when
  a widget is created.
- Minimum version 4.14 (Ubuntu 24.04 has it; Debian 13 has 4.18).
- License LGPL, dynamically linked: no conflict with BSD.
- GTK 4 also runs on Windows, but walk keeps Win32 there. Replacing the
  Windows side with GTK would throw away the accessibility work of
  steps 4, 10 and 13.

### GTK 3

- The most mature accessibility with Orca (ATK), but the toolkit is in
  maintenance only, the Go bindings (`gotk3`) are cgo and barely
  maintained, and new work would start on a dead end. Only a fallback
  if the prototype shows that GTK 4 lists and tables are unusable with
  Orca.

### Qt 6

- Good accessibility on Linux (AT-SPI bridge) and on Windows.
- Against: C++ API, so every Go binding (`miqt` is the live one,
  verify) needs cgo and a C++ toolchain and builds slowly; a large
  runtime to ship; controls are drawn by Qt, not by the desktop; the
  LGPL obligations are harder to meet with static builds. It does not
  fit a small pure Go library.

### wxWidgets

- Would be the closest in spirit (native controls on both systems; GTK 3
  on Linux), but it is C++ and has no maintained Go binding.

### Toolkits that draw everything themselves (Fyne, Gio, and similar)

- No usable screen reader support today. Adding it through AccessKit is
  a project of its own. Rejected.

### GTKX

- A framework for writing GTK 4 applications in React and TypeScript on
  Node.js (packages `@gtkx/react`, `@gtkx/ffi`, a Rust native module).
  It cannot be called from Go. What it does for React, a declarative
  description that creates and updates GTK widgets, is the role of
  `walk/declarative`.

## Bindings: how Go reaches GTK

Three options:

1. `gotk4` (diamondburned): generated, complete, cgo. Very long first
   builds, needs a C toolchain and the GTK development headers, cannot
   be cross-compiled from Windows. Whether it is still maintained in
   2026 was not confirmed (verify).
2. `puregotk` (started by jwijenbergh; the fork `bnema/puregotk`
   published v0.5.2 in May 2026, verify): generated from GObject
   Introspection, built on `github.com/ebitengine/purego`, no cgo. GTK
   4, libadwaita and WebKitGTK are covered.
3. An own generator, `tools/girgen`, that reads the GIR files and
   generates only the symbols walk uses into `internal/gtk`, on top of
   purego. This is the same design as `tools/winmdgen` with `winmd.txt`
   and `internal/win`.

Recommendation: prototype with option 2 to get answers fast. For the
real port prefer option 3 if the prototype shows that the needed
surface is small enough (a few hundred functions): no large dependency
whose Go version requirement can move (the project is pinned to Go
1.23), the same review and regeneration workflow as on Windows, and
subclassing support exactly as walk needs it. Stay with option 2 if it
turns out to be solid and its Go requirement fits.

Points to check with purego:

- Go version required by the current purego release.
- Callbacks from C into Go (signals, idle functions) and their limit on
  the number of live callbacks; walk should register a small fixed set
  of trampolines and dispatch by an ID, as it does with
  `syscall.NewCallback` on Windows.
- Defining a GObject subclass at run time (needed for a custom layout
  container and for list models).
- Functions with floating-point arguments (purego supports them on
  amd64 and arm64; 32-bit Linux can be left out).

## Architecture

### Rejected: a backend interface under the existing widgets

A `backend.LineEdit`, `backend.TableView`, ... interface per widget,
implemented once for Win32 and once for GTK, with the present types as
the portable front. It doubles the API surface, forces the 44 000 lines
of working and tested Windows code through a rewrite, and the
interfaces would have to carry the union of both toolkits' quirks. The
risk of accessibility regressions on Windows is high, and there is no
second consumer of such an interface.

### Recommended: one API, two implementations

- The public types and methods are the same on both systems. Each
  widget has a `_windows.go` and a `_linux.go` file; code that needs no
  toolkit lives in a file without a suffix.
- The existing Windows code is mostly moved, not rewritten: `git mv
  lineedit.go lineedit_windows.go`, then the portable parts (properties,
  events, validation glue) are lifted out when the Linux file is
  written and the duplication becomes visible.
- API parity is checked by a tool, not by discipline:
  `tools/winmigrate/apidump` already dumps a package's exported API for
  a target. Extend it to take `GOOS` and compare the dump for Windows
  and Linux against a list of allowed differences. Run it with the
  other checks before every commit.
- Features that exist on one system only keep their type on both and
  return a documented error (`ErrNotSupported`) on the other, or exist
  only on one system when there is no sensible meaning (`uia`,
  `Handle() HWND`). The allowed-differences list records which.

### What has to be redesigned

Each item is a public API change unless noted.

1. Native handle and message procedure. `Handle() win.HWND`, `WndProc`
   and `SendMessage` leave the portable `Window` interface. On Windows
   they stay as methods of `WindowBase` (old programs that only call
   `Handle()` keep working); on Linux `WindowBase` has
   `NativeHandle() uintptr` (the `GtkWidget` pointer). Custom widgets
   get portable hooks instead of `WndProc`: paint, mouse, key, focus
   and size events already exist as publishers; what is missing are the
   few cases where a widget must consume an event.
2. Widget creation. `InitWindow(window, parent, className, style,
   exStyle)` becomes platform-specific. `declarative` stops using style
   bits: the 8 constants become options of the widgets (`Border`,
   `HScroll`, `VScroll`, `NoPrefix`, `AlwaysShowSelection`,
   `MultiSelection`, `NoSortHeader`), which is cleaner on Windows too.
3. Layout. Keep walk's own layout engine on both systems, so that a
   form looks and resizes the same and `declarative` layouts mean the
   same thing. It needs three services from the platform: measure a
   widget (minimum size, ideal size, height for width), place a widget,
   and the scale factor. On Windows that is today's code. On GTK the
   measuring is `gtk_widget_measure`, which has the same minimum,
   natural and height-for-width model, and placing needs a container
   that lets walk decide: either a small custom container (a GObject
   subclass whose allocate function calls walk) or `GtkFixed` with
   explicit size requests. `LayoutItem.Handle()` becomes an opaque
   reference. The alternative, mapping `HBox`, `VBox` and `Grid` onto
   `GtkBox` and `GtkGrid`, gives up identical behavior and the
   `StretchFactor` and `MinSize`/`MaxSize` semantics; not recommended.
4. Units. On GTK, coordinates are logical pixels and the toolkit scales
   them. On Linux `DPI()` returns 96 and `Pixel` values equal the
   1/96 inch values, so the `...Pixels` methods remain correct and need
   no second implementation. No API change.
5. Event loop and threads. `Form.Run`, `Dialog.Run` (blocking, returns
   the result) and `Synchronize` keep their meaning. On GTK:
   `GApplication` or a plain `GMainLoop`, `g_idle_add` for
   `Synchronize`, and a nested main loop for modal dialogs (GTK 4
   removed `gtk_dialog_run`, a nested `g_main_loop_run` still works).
   The UI goroutine must stay locked to its OS thread, as today.
6. Keyboard. `Key` keeps its values (they are just numbers in the
   API); the Linux side translates GDK key values to `Key` and back.
   `Shortcut` and `Modifiers` are unchanged; `ModWin` maps to Super.
7. Drawing and resources. `Canvas`, `Font`, `Bitmap`, `Icon`, `Cursor`,
   `Brush` and `Pen` keep their Go API where it speaks in Go terms
   (`image.Image`, rectangles, colors, font family and point size).
   On Linux: cairo for `Canvas`, Pango for fonts and text measuring,
   `GdkTexture` for images. Handle-based functions (`Canvas.HDC`,
   `NewIconFromHICON`, `ImageList.Handle`, `SetWindowFont`) are
   Windows-only. Resource lookup by ID (`NewIconFromResourceId`) has no
   Linux meaning; resources by name can map to `GResource` or to files
   next to the executable.
8. Accessibility. Keep the `Accessibility` type and its setters; the
   Linux implementation calls `gtk_accessible_update_property`,
   `update_state` and `update_relation`. `AccRole` values need a table
   to `GtkAccessibleRole`; because GTK fixes the role at construction,
   a declarative `Accessibility.Role` must be known before the widget
   is created, or the setter returns an error after that. `SetLabelFor`
   can set a real labelled-by relation on Linux, better than on Windows.
   `LiveSetting` has no direct GTK counterpart; implement it with
   `gtk_accessible_announce` on text change, which is what the Windows
   side already does for edit controls. `Announce` maps directly.
9. Settings. `RegistrySettings` is Windows-only; `IniFileSettings` is
   portable already. `AppDataPath` and the known folders map to the
   XDG directories.

### Widget mapping

Straightforward (a native GTK widget with the same role):

- `MainWindow`, `Dialog`: `GtkApplicationWindow`, `GtkWindow` (modal,
  transient for the owner; role dialog).
- `Label`, `LineEdit`, `TextEdit`, `PushButton`, `CheckBox`,
  `RadioButton`, `GroupBox`, `Composite`, `ScrollView`, `Splitter`,
  `TabWidget`, `Slider`, `ProgressBar`, `ImageView`, `LinkLabel`,
  `Separator`, `ToolButton`, `SplitButton`: `GtkLabel`, `GtkEntry`
  (`GtkPasswordEntry` or the visibility flag for `PasswordMode`),
  `GtkTextView`, `GtkButton`, `GtkCheckButton` (also for radio groups),
  `GtkFrame`, a container, `GtkScrolledWindow`, `GtkPaned`,
  `GtkNotebook`, `GtkScale`, `GtkProgressBar`, `GtkPicture`,
  `GtkLinkButton` or label markup, `GtkSeparator`, `GtkMenuButton`.
- `NumberEdit`: `GtkSpinButton`. `DateEdit`: an entry with a
  `GtkCalendar` popover (GTK has no date field; check with Orca).
- `ComboBox`: `GtkDropDown`, with an entry for the editable form.
- Menus and `Action`: `GMenu` models with `GtkPopoverMenuBar` and
  `GtkPopoverMenu`; walk's `Action` already is the model.
- `MsgBox`, `TaskDialog`: `GtkAlertDialog`; the richer task dialog
  features (radio buttons, expander, verification check box) as a walk
  dialog built from widgets.
- `FileDialog`: `GtkFileDialog` (asynchronous; run a nested loop to keep
  the blocking API).
- `CustomWidget`: `GtkDrawingArea`.
- `ToolBar`, `StatusBar`: plain boxes with buttons and labels (GTK 4
  has no toolbar or status bar widgets).

Harder:

- `ListBox`, `TableView`, `TreeView`: `GtkListView`, `GtkColumnView`,
  and `GtkTreeListModel` with `GtkTreeExpander` (`GtkTreeView` is
  deprecated since 4.10). They are driven by `GListModel` and item
  factories, a different model from walk's `TableModel` and
  `TreeModel`; an adapter object is needed. This is also the main
  accessibility risk. `TableView` is the largest single piece of work
  on both systems.
- `RichEdit`: `GtkTextView` with text tags. Formatting, selection, find
  and links map; RTF import and export do not exist in GTK and would
  need an own converter, or `RTF`/`SetRTF` return `ErrNotSupported`.
- `WebView2`: WebKitGTK 6.0 behind a neutral name (for example
  `WebView` again, once the Internet Explorer one is removed, or
  `Browser`), with `WebView2` kept as the Windows name.
- `Clipboard`: `GdkClipboard` is asynchronous and uses MIME types;
  `internal/clipfmt` (HTML, DIB) is Windows-specific, text, HTML, files
  and images map.
- `NotifyIcon`: GTK 4 has no tray icon. Implement the
  StatusNotifierItem D-Bus protocol; GNOME shows it only with an
  extension.
- `GlobalHotkey`: the XDG GlobalShortcuts portal on Wayland, a key grab
  on X11. `SingleInstance`: `GApplication` uniqueness over D-Bus.
- `System`: logind and UPower over D-Bus for sleep, lock and power; the
  XDG Settings portal for dark mode and high contrast;
  `ScreenReaderRunning` from the `org.a11y.Status` D-Bus property.
- Shell functions: `MoveToRecycleBin` is `g_file_trash`, `ShellOpen`
  and `ShowInFolder` are `GtkUriLauncher` and `GtkFileLauncher`.

No counterpart:

- MDI. GTK has no child windows inside a window. Either not available
  on Linux, or the same API shown as tabs.
- Package `uia`. The Linux counterpart would be an AT-SPI client
  package; useful for automated tests, but a separate project.
- Wayland limits that change behavior, not API: a program cannot
  position its top-level windows, read their screen position, or take
  the foreground at will (`SetBounds` X and Y, `BringToForeground`,
  centering a dialog on its owner are requests the compositor may
  ignore).

## Testing

- The cloud container runs Linux, so unlike the Windows side the Linux
  port can be built and run there (headless compositor or Xvfb, with an
  AT-SPI bus). Verify what the container allows.
- Automated accessibility checks: a small AT-SPI client that plays the
  screen reader, the role `tests/_wingui/liveregion/client` and `uia`
  have on Windows. Mirror the layout as `tests/_gtkgui`.
- Manual tests with Orca by the user, per widget group, as with NVDA.
  Expect differences between X11 and Wayland sessions; test on the
  session the user works in first.
- The Windows test suite (`tests/_wingui`, sandbox) must stay green
  after every refactoring step; the file moves must not change behavior.

## Proposed phases

Each phase is one or more pull requests. Phases 0 and 1 change nothing
for users of walk and can be done at any time.

0. Prototype, outside the module or in a throwaway directory: a window
   with a label and entry, a button, a check box, a radio group, a combo
   box, a list, a table with a few hundred rows, a menu bar and a modal
   dialog, positioned by walk's layout arithmetic, written with
   puregotk. Test with Orca. Answers: are the bindings usable, which
   placement technique works, how do lists and tables read, how do
   announcements behave. Go or no-go decision.
1. Free the portable core: remove `//go:build windows` from the 34
   files that need no Windows, fix what they still reach for, and make
   `GOOS=linux go build` and `go test` pass for that subset (a Linux
   stub for the rest is acceptable at first). Add the Linux build to the
   checks.
2. API cleanup for portability (breaking; decide together with step 9
   and release as v2): items 1, 2 and 3 of "What has to be redesigned",
   the handle-based functions moved to Windows-only files, the parity
   check with `apidump`.
3. File split on Windows: rename to `_windows.go`, lift shared code out
   widget by widget. No behavior change; Windows tests after each batch.
4. Linux core: bindings (`internal/gtk`), application and main loop,
   `Synchronize`, `WindowBase`, layout placement, `MainWindow`,
   `Dialog`, the simple widgets, menus and actions, `MsgBox`,
   `FileDialog`, `Accessibility`, `Announce`. First Orca test round.
   `declarative` builds on Linux for these.
5. Model widgets: `ListBox`, `TableView`, `TreeView`, then `TabWidget`,
   `Splitter`, `ScrollView`, tool bar and status bar, `NumberEdit`,
   `DateEdit`, `Canvas` and `CustomWidget`, images and fonts. Second
   Orca round. The examples run on Linux.
6. The rest, each on demand: `RichEdit`, the browser widget, clipboard
   formats, `NotifyIcon`, `System`, hotkeys, single instance, shell
   functions.

Rough size: phases 0 and 1 are small (days of agent work). Phases 2
and 3 touch every file but are mostly mechanical. Phases 4 and 5 are
the real port, comparable to the whole modernization so far; phase 5
carries the accessibility risk. Phase 6 is open-ended.

## Open decisions for the user

1. Is a breaking v2 acceptable, and should it be combined with step 9
   (generic events and models)? Without it, Linux support is possible
   only as a second-class subset.
2. Bindings: depend on puregotk, or write `tools/girgen` and
   `internal/gtk`? Decide after the prototype.
3. Plain GTK 4 or libadwaita as well? Plain GTK is recommended: walk's
   model is classic desktop windows with menu bars, and libadwaita adds
   a second dependency and GNOME-specific design rules.
4. MDI on Linux: unsupported, or tabs behind the same API?
5. Target sessions: X11, Wayland or both from the start? Both work with
   GTK without extra code, but the Wayland limits above affect what the
   tests can check.
6. The name: walk stands for "Windows Application Library Kit". The
   import path can stay; only the documentation needs to explain it.

## Sources

- GTK accessibility: https://docs.gtk.org/gtk4/section-accessibility.html
  and https://blog.gtk.org/2020/10/21/accessibility-in-gtk-4/
- GTK 4.14 accessibility improvements: https://blog.gtk.org/author/ebassi/
- puregotk: https://pkg.go.dev/github.com/bnema/puregotk
- gotk4: https://github.com/diamondburned/gotk4
- GTKX: https://eugeniodepalo.github.io/gtkx/llms.txt
