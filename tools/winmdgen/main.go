// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command winmdgen generates Win32 declarations for package internal/win
// from the Windows.Win32.winmd metadata of the NuGet package
// Microsoft.Windows.SDK.Win32Metadata.
//
// It is run by go generate in internal/win:
//
//	go generate ./internal/win
//
// The metadata is downloaded once into the user cache directory. The -winmd
// flag uses a local Windows.Win32.winmd file instead.
//
// # Specification
//
// The specification file (winmd.txt by default) lists the symbols to
// generate, one directive per line. Text after # is a comment.
//
//	metadata VERSION sha256:HASH
//
// selects the version of the NuGet package and the SHA-256 hash of the
// .nupkg file.
//
//	dll FILE GONAME
//
// declares the Go variable holding the lazily loaded DLL, for example
// "dll user32.dll libuser32".
//
//	const NAME [TYPE]
//
// generates a constant with the value from the metadata. Without TYPE the
// constant is untyped.
//
//	func NAME [OPTION...]
//
// generates a function calling the DLL entry point NAME+"W", or NAME if
// there is no such entry point. The Go signature follows the metadata:
// handles and enums use the Go type of the same name if the package
// defines one, otherwise their underlying integer type; structs use the Go
// struct of the same name, also with a trailing W removed; BOOL becomes
// bool. Options:
//
//	entry=NAME    use the metadata function NAME
//	result=TYPE   use TYPE as the result type
//	PARAM:TYPE    use TYPE for the parameter named PARAM in the metadata
//	rawbool       keep BOOL instead of translating it to bool
//
//	struct NAME [OPTION...]
//
// generates a struct type from the metadata struct NAME+"W", or NAME if
// there is no such struct. Each field gets the metadata name with its
// first letter in upper case and the Go type chosen as for function
// parameters, except that BOOL stays BOOL. Options:
//
//	entry=NAME    use the metadata struct NAME
//	FIELD=NAME    use NAME as the Go name of the metadata field FIELD
//	FIELD:TYPE    use TYPE as the Go type of the metadata field FIELD
//
// Unions, structs with nested types (anonymous unions), structs the Go
// compiler would lay out differently from the C compiler (#pragma pack,
// 64-bit fields on 386) and structs whose Go fields differ between
// architectures are rejected and stay hand-written. For every
// architecture, a zwinmd_layout_GOARCH.go file checks at compile time
// that the size, alignment and field offsets of each generated struct
// match the metadata, which also covers the TYPE overrides.
//
// # Migration
//
// With -suggest, winmdgen does not generate anything. It prints
// specification lines for the hand-written symbols of the package that the
// Go files under the repository root use (as win.NAME) and that can be
// generated without changing their Go type, value or behavior, together
// with the required dll directives. Structs are considered if walk uses
// them directly or as the type of a field of another such struct; their
// layout is checked on every architecture. Reasons for skipping the other symbols
// go to standard error. tools/winmigrate has the companion tools that
// remove the replaced hand-written declarations and check that the API
// did not change.
//
// The generated wrappers ignore the error returned by syscall.SyscallN,
// like the hand-written ones in the package. Functions taking or returning
// floating-point values or structs by value, functions with parameters
// larger than a pointer and functions that exist only on some
// architectures are rejected and stay hand-written.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	specPath := flag.String("spec", "winmd.txt", "specification `file`")
	winmdPath := flag.String("winmd", "", "use a local Windows.Win32.winmd `file`")
	dir := flag.String("dir", "", "output `directory` (default: directory of the specification)")
	suggestRoot := flag.String("suggest", "", "print specification lines for symbols used under the `root` directory instead of generating")
	flag.Parse()

	var err error
	if *suggestRoot != "" {
		err = runSuggest(*specPath, *winmdPath, *dir, *suggestRoot)
	} else {
		err = run(*specPath, *winmdPath, *dir)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "winmdgen: %v\n", err)
		os.Exit(1)
	}
}

// load parses the specification and loads the metadata.
func load(specPath, winmdPath string) (*spec, *metadata, error) {
	s, err := parseSpec(specPath)
	if err != nil {
		return nil, nil, err
	}
	var data []byte
	if winmdPath != "" {
		data, err = os.ReadFile(winmdPath)
	} else {
		data, err = fetchWinmd(s.metaVersion, s.metaSHA256)
	}
	if err != nil {
		return nil, nil, err
	}
	m, err := loadMetadata(data)
	return s, m, err
}

func runSuggest(specPath, winmdPath, dir, root string) error {
	s, m, err := load(specPath, winmdPath)
	if err != nil {
		return err
	}
	if dir == "" {
		dir = filepath.Dir(specPath)
	}
	return suggest(m, s, dir, root)
}

func run(specPath, winmdPath, dir string) error {
	s, m, err := load(specPath, winmdPath)
	if err != nil {
		return err
	}
	if dir == "" {
		dir = filepath.Dir(specPath)
	}
	pkg, err := parseGoPackage(dir)
	if err != nil {
		return err
	}

	pkg.addStructs(s.structs)

	g := &generator{m: m, s: s, pkg: pkg}
	structs, layouts := g.structs()
	outputs := []struct {
		name string
		src  []byte
	}{
		{outputPrefix + "constants.go", g.constants()},
		{outputPrefix + "functions.go", g.functions()},
		{outputPrefix + "structs.go", structs},
	}
	for _, an := range archNames {
		outputs = append(outputs, struct {
			name string
			src  []byte
		}{outputPrefix + "layout_" + an.name + ".go", layouts[an.arch]})
	}
	if len(g.errs) > 0 {
		return fmt.Errorf("%s:\n\t%s", specPath, strings.Join(g.errs, "\n\t"))
	}
	for _, o := range outputs {
		src, err := formatSource(o.name, o.src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, o.name), src, 0o644); err != nil {
			return err
		}
	}
	return nil
}
