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
- Step 2: done. `github.com/lxn/win` (version
  v0.0.0-20210218163916-a377121e959e) lives in `internal/win`; see its
  `README.md`.
- Step 3: not started.

## Known issues

- `go vet` reports 72 "possible misuse of unsafe.Pointer" warnings: 69 in
  walk, mostly `lParam` to struct pointer conversions in window procedures,
  and 3 in `internal/win` (`kernel32.go`, `oleaut32.go`, `win.go`). Review
  them in step 3.
- The examples have no `rsrc.syso` for arm64.
