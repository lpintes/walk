// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command removedecl removes the hand-written declarations of a package
// that a winmdgen specification generates: constants, structs, functions, the
// LazyProc variables only those functions use (with their assignments in
// init functions) and the LazyDLL variables of the dll directives. It then
// removes empty groups, empty init functions and unused imports and
// formats the changed files. Generated files (zwinmd_*.go) are not touched.
//
// Usage, from the repository root:
//
//	go run ./tools/winmigrate/removedecl internal/win/winmd.txt internal/win
//
// Constant blocks using iota and specs declaring several names are left
// alone and reported. Check the result for orphaned comments.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type lineRange struct{ from, to int } // 1-based inclusive

func main() {
	specFile, dir := os.Args[1], os.Args[2]
	consts := map[string]bool{}
	funcs := map[string]bool{}
	libs := map[string]bool{}
	structs := map[string]bool{}
	f, _ := os.Open(specFile)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 2 {
			continue
		}
		switch fs[0] {
		case "const":
			consts[fs[1]] = true
		case "func":
			funcs[fs[1]] = true
		case "dll":
			libs[fs[2]] = true
		case "struct":
			structs[fs[1]] = true
		}
	}

	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	for _, name := range files {
		if strings.HasPrefix(filepath.Base(name), "zwinmd_") {
			continue
		}
		af, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			panic(err)
		}
		parsed[name] = af
	}

	// Pass 1: find the proc vars used by removed functions, and count all
	// uses of every identifier outside init functions and removed functions.
	removedProcs := map[string]bool{}
	uses := map[string]int{}
	for _, af := range parsed {
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fd.Recv == nil && funcs[fd.Name.Name] {
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					if se, ok := n.(*ast.SelectorExpr); ok && se.Sel.Name == "Addr" {
						if id, ok := se.X.(*ast.Ident); ok {
							removedProcs[id.Name] = true
						}
					}
					return true
				})
				continue
			}
			if fd.Recv == nil && fd.Name.Name == "init" {
				continue
			}
			ast.Inspect(fd, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					uses[id.Name]++
				}
				return true
			})
		}
	}
	for p := range removedProcs {
		if uses[p] > 0 {
			fmt.Fprintln(os.Stderr, "keep proc", p, "still used")
			delete(removedProcs, p)
		}
	}
	vars := map[string]bool{}
	for p := range removedProcs {
		vars[p] = true
	}
	for l := range libs {
		vars[l] = true
	}

	count := map[string]int{}
	for name, af := range parsed {
		src, _ := os.ReadFile(name)
		var ranges []lineRange
		line := func(p token.Pos) int { return fset.Position(p).Line }
		del := func(from, to token.Pos) {
			ranges = append(ranges, lineRange{line(from), line(to)})
		}
		// Line comments trailing a node end on the same line, so deleting
		// whole lines removes them too.
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					continue
				}
				if funcs[d.Name.Name] {
					start := d.Pos()
					if d.Doc != nil {
						start = d.Doc.Pos()
					}
					del(start, d.End())
					count["func"]++
					continue
				}
				if d.Name.Name == "init" {
					for _, st := range d.Body.List {
						as, ok := st.(*ast.AssignStmt)
						if !ok || len(as.Lhs) != 1 {
							continue
						}
						if id, ok := as.Lhs[0].(*ast.Ident); ok && vars[id.Name] {
							del(as.Pos(), as.End())
						}
					}
				}
			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, s := range d.Specs {
						ts := s.(*ast.TypeSpec)
						if !structs[ts.Name.Name] {
							continue
						}
						start, end := ts.Pos(), ts.End()
						switch {
						case !d.Lparen.IsValid():
							start, end = d.Pos(), d.End()
							if d.Doc != nil {
								start = d.Doc.Pos()
							}
						case ts.Doc != nil:
							start = ts.Doc.Pos()
						}
						del(start, end)
						count["struct"]++
					}
					continue
				}
				if d.Tok != token.CONST && d.Tok != token.VAR {
					continue
				}
				set := consts
				if d.Tok == token.VAR {
					set = vars
				}
				hasIota := false
				ast.Inspect(d, func(n ast.Node) bool {
					if id, ok := n.(*ast.Ident); ok && id.Name == "iota" {
						hasIota = true
					}
					return true
				})
				removed := make([]bool, len(d.Specs))
				for i, s := range d.Specs {
					vs := s.(*ast.ValueSpec)
					all := true
					any := false
					for _, n := range vs.Names {
						all = all && set[n.Name]
						any = any || set[n.Name]
					}
					if !any {
						continue
					}
					if d.Tok == token.CONST && (hasIota || len(vs.Values) == 0) {
						fmt.Fprintln(os.Stderr, "skip iota/implicit", vs.Names[0].Name)
						continue
					}
					if !all {
						fmt.Fprintln(os.Stderr, "skip partial", vs.Names[0].Name)
						continue
					}
					removed[i] = true
				}
				for i, s := range d.Specs {
					if !removed[i] {
						continue
					}
					vs := s.(*ast.ValueSpec)
					start := vs.Pos()
					if vs.Doc != nil {
						// Keep a doc comment that also introduces the next,
						// remaining spec.
						keep := i+1 < len(d.Specs) && !removed[i+1] &&
							line(d.Specs[i+1].Pos()) == line(vs.End())+1
						if !keep {
							start = vs.Doc.Pos()
						} else {
							// The comment must now precede the next spec;
							// deleting the spec lines between does that.
						}
					}
					del(start, vs.End())
					if d.Tok == token.CONST {
						count["const"]++
					} else {
						count["var"]++
					}
				}
			}
		}
		if len(ranges) == 0 {
			continue
		}
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
		res := cleanup(out.Bytes())
		formatted, err := format.Source(res)
		if err != nil {
			fmt.Fprintln(os.Stderr, name, err)
			os.WriteFile(name, res, 0o644)
			continue
		}
		os.WriteFile(name, formatted, 0o644)
	}
	fmt.Fprintln(os.Stderr, count)
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
				if (d.Tok == token.CONST || d.Tok == token.VAR || d.Tok == token.TYPE) && len(d.Specs) == 0 {
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
		// Comments inside empty groups.
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
		sort.Slice(ranges, func(i, j int) bool { return ranges[i].from < ranges[j].from })
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
		src = out.Bytes()
	}
}
