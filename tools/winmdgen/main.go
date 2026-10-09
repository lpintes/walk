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
//	fallback=NAME call the metadata function NAME (or NAME+"W") on the
//	              architectures where the function does not exist, for
//	              example GetWindowLong for GetWindowLongPtr on 386
//
// A pointer to a struct the package does not define, such as ITEMIDLIST,
// needs a type override; so does a struct field of such a type.
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
//	guid NAME [TYPE]
//
// generates a variable holding a GUID: the GUID constant NAME of the
// metadata, or the GUID of the interface (for names starting with IID_ or
// DIID_) or coclass (CLSID_) named by the rest of NAME. Without TYPE the
// variable has the type IID, or CLSID for names starting with CLSID_.
// TYPE must have the underlying type syscall.GUID.
//
//	interface NAME [OPTION...]
//
// generates the vtable struct NAMEVtbl of the metadata interface NAME, with
// one uintptr field per method in vtable order, including the methods of
// the base interfaces, and the struct NAME with the single field
// LpVtbl *NAMEVtbl. Field names are the method names with the first letter
// in upper case, so get_Name becomes Get_Name. Methods calling through the
// vtable stay hand-written. Options:
//
//	entry=NAME    use the metadata interface NAME
//	METHOD=NAME   use NAME as the Go name of the vtable field of METHOD
//
// # Migration
//
// With -suggest, winmdgen does not generate anything. It prints
// specification lines for the hand-written symbols of the package that the
// Go files under the repository root use (as win.NAME) and that can be
// generated without changing their Go type, value or behavior, together
// with the required dll directives. Structs are considered if walk uses
// them directly or as the type of a field of another such struct; their
// layout is checked on every architecture. GUID variables must have the
// value of the metadata. COM interfaces are considered if walk uses them
// or their vtable, or if a method of another such interface refers to
// them; their vtable must list the methods of the metadata in the same
// order. Reasons for skipping the other symbols
// go to standard error. tools/winmigrate has the companion tools that
// remove the replaced hand-written declarations and check that the API
// did not change.
//
// The generated wrappers ignore the error returned by syscall.SyscallN,
// like the hand-written ones in the package. A struct parameter passed by
// value must be a generated struct of 1, 2 or 4 bytes without
// floating-point fields; the calling conventions of all architectures pass
// it like an integer of that size. Functions taking or returning other
// structs or floating-point values by value, returning a pointer, with
// parameters larger than a pointer, or existing only on some architectures
// without a fallback are rejected and stay hand-written. The procedure
// variables of functions with a fallback go to the files
// zwinmd_functions_GOARCH.go.
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
	pkg.addInterfaces(s.interfaces)

	g := &generator{m: m, s: s, pkg: pkg}
	structs, layouts := g.structs()
	functions, archFunctions := g.functions()
	type output struct {
		name string
		src  []byte // nil to remove the file
	}
	outputs := []output{
		{outputPrefix + "constants.go", g.constants()},
		{outputPrefix + "functions.go", functions},
		{outputPrefix + "guids.go", g.guids()},
		{outputPrefix + "interfaces.go", g.interfaces()},
		{outputPrefix + "structs.go", structs},
	}
	for _, an := range archNames {
		outputs = append(outputs,
			output{outputPrefix + "layout_" + an.name + ".go", layouts[an.arch]},
			// Only needed for functions with a fallback.
			output{outputPrefix + "functions_" + an.name + ".go", archFunctions[an.arch]})
	}
	if len(g.errs) > 0 {
		return fmt.Errorf("%s:\n\t%s", specPath, strings.Join(g.errs, "\n\t"))
	}
	for _, o := range outputs {
		path := filepath.Join(dir, o.name)
		if o.src == nil {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		src, err := formatSource(o.name, o.src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, src, 0o644); err != nil {
			return err
		}
	}
	return nil
}
