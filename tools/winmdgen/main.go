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
	flag.Parse()

	if err := run(*specPath, *winmdPath, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "winmdgen: %v\n", err)
		os.Exit(1)
	}
}

func run(specPath, winmdPath, dir string) error {
	s, err := parseSpec(specPath)
	if err != nil {
		return err
	}
	if dir == "" {
		dir = filepath.Dir(specPath)
	}

	var data []byte
	if winmdPath != "" {
		data, err = os.ReadFile(winmdPath)
	} else {
		data, err = fetchWinmd(s.metaVersion, s.metaSHA256)
	}
	if err != nil {
		return err
	}
	m, err := loadMetadata(data)
	if err != nil {
		return err
	}
	pkg, err := parseGoPackage(dir)
	if err != nil {
		return err
	}

	g := &generator{m: m, s: s, pkg: pkg}
	outputs := []struct {
		name string
		src  []byte
	}{
		{outputPrefix + "constants.go", g.constants()},
		{outputPrefix + "functions.go", g.functions()},
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
