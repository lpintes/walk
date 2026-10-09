// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command prune removes the hand-written declarations of a package such as
// internal/win that no other package of the module uses, directly or
// through other declarations of the package, on any of the architectures
// 386, amd64 and arm64.
//
// Usage, from the repository root:
//
//	go run ./tools/winmigrate/prune internal/win
//
// The roots are the objects of the package that the other packages of the
// module (with their tests) refer to, the declarations of generated files
// (zwinmd_*.go), which are never changed, and the String, Error, Format and
// GoString methods of the types that stay, which interfaces may call. A
// declaration stays if a root refers to it, directly or indirectly.
// Assignments in init functions belong to the variable they assign;
// other statements of init functions are roots. A const declaration using
// iota or implicit values is kept or removed as a whole, as is a value
// spec declaring several names.
//
// Removed are functions, methods, type specs, value specs and init
// assignments; then empty groups, empty init functions and unused imports,
// and files left without declarations. Check the result for orphaned
// comments and compare the output of apidump before and after: lines may
// only disappear.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var archs = []string{"386", "amd64", "arm64"}

const generatedPrefix = "zwinmd_"

// keepMethods are always kept for the types that stay: interfaces such as
// fmt.Stringer and error may call them without any reference in the code.
var keepMethods = map[string]bool{"String": true, "Error": true, "Format": true, "GoString": true}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: prune DIR")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "prune:", err)
		os.Exit(1)
	}
}

type listedPackage struct {
	ImportPath  string
	Dir         string
	Export      string
	GoFiles     []string
	TestGoFiles []string
	Imports     []string
	TestImports []string
	Module      *struct{ Main bool }
}

// goList runs go list -json for windows/goarch.
func goList(goarch string, args ...string) ([]*listedPackage, error) {
	cmd := exec.Command("go", append([]string{"list", "-e", "-json"}, args...)...)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+goarch)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var pkgs []*listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, &p)
	}
	return pkgs, nil
}

// key names a package-level object: "Name" or "Type.Method".
func key(obj types.Object) string {
	if fn, ok := obj.(*types.Func); ok {
		if recv := fn.Type().(*types.Signature).Recv(); recv != nil {
			t := recv.Type()
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			if n, ok := t.(*types.Named); ok {
				return n.Obj().Name() + "." + fn.Name()
			}
			return ""
		}
	}
	if obj.Parent() != nil && obj.Parent() == obj.Pkg().Scope() {
		return obj.Name()
	}
	return ""
}

func run(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	live := make(map[string]bool)
	for _, goarch := range archs {
		if err := markLive(absDir, goarch, live); err != nil {
			return fmt.Errorf("%s: %w", goarch, err)
		}
	}
	return remove(absDir, live)
}

// decl is a removable unit of the package: the keys it defines and the
// keys it refers to.
type decl struct {
	defines []string
	refs    map[string]bool
	root    bool
}

// markLive adds the keys of the objects that stay on one architecture.
func markLive(dir, goarch string, live map[string]bool) error {
	pkgs, err := goList(goarch, "./...")
	if err != nil {
		return err
	}
	var winPath string
	for _, p := range pkgs {
		if p.Dir == dir {
			winPath = p.ImportPath
		}
	}
	if winPath == "" {
		return fmt.Errorf("package in %s not found", dir)
	}

	// Export data for all dependencies.
	exports := make(map[string]string)
	var paths []string
	for _, p := range pkgs {
		paths = append(paths, p.ImportPath)
	}
	deps, err := goList(goarch, append([]string{"-export", "-deps", "-test"}, paths...)...)
	if err != nil {
		return err
	}
	for _, p := range deps {
		if p.Export != "" {
			exports[p.ImportPath] = p.Export
		}
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		if exports[path] == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(exports[path])
	})
	sizes := types.SizesFor("gc", goarch)

	// Roots from the other packages of the module.
	roots := make(map[string]bool)
	for _, p := range pkgs {
		if p.Dir == dir || !contains(append(p.Imports, p.TestImports...), winPath) {
			continue
		}
		// Internal test files only; external tests import the package
		// under test from export data, which go list -export -test
		// provides only for the test variant. walk has none.
		files := append(append([]string{}, p.GoFiles...), p.TestGoFiles...)
		var afs []*ast.File
		for _, f := range files {
			af, err := parser.ParseFile(fset, filepath.Join(p.Dir, f), nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			afs = append(afs, af)
		}
		info := &types.Info{Uses: make(map[*ast.Ident]types.Object)}
		conf := types.Config{Importer: imp, Sizes: sizes}
		if _, err := conf.Check(p.ImportPath, fset, afs, info); err != nil {
			return fmt.Errorf("%s: %w", p.ImportPath, err)
		}
		for _, obj := range info.Uses {
			if obj.Pkg() != nil && obj.Pkg().Path() == winPath {
				if k := key(obj); k != "" {
					roots[k] = true
				}
			}
		}
	}

	// The package itself, from source.
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH = "windows", goarch
	names, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	var afs []*ast.File
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") {
			continue
		}
		if ok, _ := ctx.MatchFile(dir, filepath.Base(n)); !ok {
			continue
		}
		af, err := parser.ParseFile(fset, n, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		afs = append(afs, af)
	}
	info := &types.Info{Uses: make(map[*ast.Ident]types.Object), Defs: make(map[*ast.Ident]types.Object)}
	conf := types.Config{Importer: imp, Sizes: sizes}
	pkg, err := conf.Check(winPath, fset, afs, info)
	if err != nil {
		return err
	}
	refsOf := func(n ast.Node) map[string]bool {
		refs := make(map[string]bool)
		ast.Inspect(n, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if obj := info.Uses[id]; obj != nil && obj.Pkg() == pkg {
					if k := key(obj); k != "" {
						refs[k] = true
					}
				}
			}
			return true
		})
		return refs
	}

	var decls []*decl
	byKey := make(map[string][]*decl)
	add := func(d *decl) {
		decls = append(decls, d)
		for _, k := range d.defines {
			byKey[k] = append(byKey[k], d)
		}
	}
	for _, af := range afs {
		generated := strings.HasPrefix(filepath.Base(fset.Position(af.Pos()).Filename), generatedPrefix)
		for _, d := range af.Decls {
			for _, u := range units(d, info) {
				u.root = u.root || generated
				u.refs = refsOf(u.node)
				add(&u.decl)
			}
		}
	}
	for k := range roots {
		for _, d := range byKey[k] {
			d.root = true
		}
	}

	// Propagate.
	var queue []*decl
	seen := make(map[*decl]bool)
	mark := func(d *decl) {
		if !seen[d] {
			seen[d] = true
			queue = append(queue, d)
		}
	}
	for _, d := range decls {
		if d.root {
			mark(d)
		}
	}
	for {
		for len(queue) > 0 {
			d := queue[0]
			queue = queue[1:]
			for _, k := range d.defines {
				live[k] = true
			}
			for k := range d.refs {
				for _, d2 := range byKey[k] {
					mark(d2)
				}
			}
		}
		// Methods interfaces may call, for the types that stay.
		added := false
		for _, d := range decls {
			if seen[d] {
				continue
			}
			for _, k := range d.defines {
				if i := strings.IndexByte(k, '.'); i > 0 && keepMethods[k[i+1:]] && live[k[:i]] {
					mark(d)
					added = true
				}
			}
		}
		if !added {
			return nil
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// unit is a decl together with its syntax.
type unit struct {
	decl
	node ast.Node
}

// units splits a top-level declaration into removable units.
func units(d ast.Decl, info *types.Info) []unit {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil && d.Name.Name == "init" {
			var us []unit
			for _, st := range d.Body.List {
				if v := initVar(st); v != "" {
					us = append(us, unit{decl{defines: []string{v}}, st})
				} else {
					us = append(us, unit{decl{root: true}, st})
				}
			}
			return us
		}
		if d.Name.Name == "_" {
			return []unit{{decl{root: true}, d}}
		}
		obj := info.Defs[d.Name]
		if obj == nil {
			return nil
		}
		k := key(obj)
		if k == "" {
			return []unit{{decl{root: true}, d}}
		}
		return []unit{{decl{defines: []string{k}}, d}}
	case *ast.GenDecl:
		if d.Tok == token.IMPORT {
			return nil
		}
		if d.Tok == token.CONST && wholeConst(d) {
			var names []string
			for _, s := range d.Specs {
				for _, n := range s.(*ast.ValueSpec).Names {
					names = append(names, n.Name)
				}
			}
			return []unit{{decl{defines: names}, d}}
		}
		var us []unit
		for _, s := range d.Specs {
			switch s := s.(type) {
			case *ast.TypeSpec:
				us = append(us, unit{decl{defines: []string{s.Name.Name}}, s})
			case *ast.ValueSpec:
				var names []string
				root := false
				for _, n := range s.Names {
					if n.Name == "_" {
						root = true
					}
					names = append(names, n.Name)
				}
				us = append(us, unit{decl{defines: names, root: root}, s})
			}
		}
		return us
	}
	return nil
}

// wholeConst reports whether a const declaration must be kept or removed
// as a whole: it uses iota or implicit values.
func wholeConst(d *ast.GenDecl) bool {
	whole := false
	ast.Inspect(d, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "iota" {
			whole = true
		}
		if vs, ok := n.(*ast.ValueSpec); ok && len(vs.Values) == 0 {
			whole = true
		}
		return true
	})
	return whole
}

// initVar returns the variable a statement of an init function assigns,
// or "" if it is not a simple assignment of a package-level variable.
func initVar(st ast.Stmt) string {
	as, ok := st.(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 1 || as.Tok != token.ASSIGN {
		return ""
	}
	id, ok := as.Lhs[0].(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

type lineRange struct{ from, to int } // 1-based inclusive

// remove deletes the declarations of the hand-written files whose keys are
// all dead.
func remove(dir string, live map[string]bool) error {
	names, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	sort.Strings(names)
	total := 0
	for _, name := range names {
		if strings.HasPrefix(filepath.Base(name), generatedPrefix) || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			return err
		}
		line := func(p token.Pos) int { return fset.Position(p).Line }
		var ranges []lineRange
		del := func(doc *ast.CommentGroup, from, to token.Pos) {
			if doc != nil {
				from = doc.Pos()
			}
			ranges = append(ranges, lineRange{line(from), line(to)})
			total++
		}
		dead := func(keys []string) bool {
			for _, k := range keys {
				if live[k] {
					return false
				}
			}
			return len(keys) > 0
		}
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" {
					for _, st := range d.Body.List {
						if v := initVar(st); v != "" && !live[v] {
							del(nil, st.Pos(), st.End())
						}
					}
					continue
				}
				k := d.Name.Name
				if d.Recv != nil {
					t := d.Recv.List[0].Type
					if s, ok := t.(*ast.StarExpr); ok {
						t = s.X
					}
					id, ok := t.(*ast.Ident)
					if !ok {
						continue
					}
					k = id.Name + "." + k
				}
				if k != "_" && !live[k] {
					del(d.Doc, d.Pos(), d.End())
				}
			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				var all []string
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						all = append(all, s.Name.Name)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							all = append(all, n.Name)
						}
					}
				}
				if (d.Tok == token.CONST && wholeConst(d)) || !d.Lparen.IsValid() {
					if dead(all) && !contains(all, "_") {
						del(d.Doc, d.Pos(), d.End())
					}
					continue
				}
				for _, s := range d.Specs {
					var keys []string
					var doc *ast.CommentGroup
					switch s := s.(type) {
					case *ast.TypeSpec:
						keys, doc = []string{s.Name.Name}, s.Doc
					case *ast.ValueSpec:
						for _, n := range s.Names {
							keys = append(keys, n.Name)
						}
						doc = s.Doc
					}
					if dead(keys) && !contains(keys, "_") {
						del(doc, s.Pos(), s.End())
					}
				}
			}
		}
		if len(ranges) == 0 {
			continue
		}
		out := cleanup(dropLines(src, ranges))
		if empty(out) {
			if err := os.Remove(name); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "removed", filepath.Base(name))
			continue
		}
		formatted, err := format.Source(out)
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		if err := os.WriteFile(name, formatted, 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintln(os.Stderr, "removed", total, "declarations")
	return nil
}

func dropLines(src []byte, ranges []lineRange) []byte {
	lines := bytes.SplitAfter(src, []byte("\n"))
	drop := make([]bool, len(lines)+2)
	for _, r := range ranges {
		for l := r.from; l <= r.to; l++ {
			drop[l] = true
		}
	}
	var out bytes.Buffer
	for i, l := range lines {
		if !drop[i+1] {
			out.Write(l)
		}
	}
	return out.Bytes()
}

// empty reports whether a file has no declarations left.
func empty(src []byte) bool {
	af, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		return false
	}
	for _, d := range af.Decls {
		if gd, ok := d.(*ast.GenDecl); !ok || gd.Tok != token.IMPORT {
			return false
		}
	}
	return true
}

// cleanup removes empty const, var and type groups, empty init functions and
// unused imports.
func cleanup(src []byte) []byte {
	for {
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, "", src, parser.ParseComments)
		if err != nil {
			return src
		}
		var ranges []lineRange
		line := func(p token.Pos) int { return fset.Position(p).Line }
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				if d.Tok != token.IMPORT && len(d.Specs) == 0 {
					start := d.Pos()
					if d.Doc != nil {
						start = d.Doc.Pos()
					}
					ranges = append(ranges, lineRange{line(start), line(d.End())})
				}
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" && len(d.Body.List) == 0 {
					ranges = append(ranges, lineRange{line(d.Pos()), line(d.End())})
				}
			}
		}
		used := map[string]bool{}
		ast.Inspect(af, func(n ast.Node) bool {
			if se, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := se.X.(*ast.Ident); ok {
					used[id.Name] = true
				}
			}
			return true
		})
		for _, imp := range af.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			name := path[strings.LastIndex(path, "/")+1:]
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if !used[name] && name != "_" {
				ranges = append(ranges, lineRange{line(imp.Pos()), line(imp.End())})
			}
		}
		if len(ranges) == 0 {
			return src
		}
		src = dropLines(src, ranges)
	}
}
