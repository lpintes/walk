// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command apidump prints the API of a Windows package such as internal/win
// for one GOARCH: every package-level object with its type (and value for
// constants), and a normalized form of every function that is a plain
// syscall wrapper, with the DLL and entry point it calls and its argument
// and result conversions. Parameter names do not appear in the output.
//
// Usage, from the repository root:
//
//	go run ./tools/winmigrate/apidump internal/win amd64 > after.txt
//
// Compare the output before and after a change for 386, amd64 and arm64.
package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var fset = token.NewFileSet()

func exprString(e ast.Expr) string {
	var b strings.Builder
	printer.Fprint(&b, fset, e)
	return b.String()
}

func main() {
	dir, goarch := os.Args[1], os.Args[2]
	ctx := build.Default
	ctx.GOOS = "windows"
	ctx.GOARCH = goarch
	build.Default = ctx

	var files []*ast.File
	names, _ := filepath.Glob(dir + "/*.go")
	for _, n := range names {
		ok, _ := ctx.MatchFile(dir, filepath.Base(n))
		if !ok {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			panic(err)
		}
		files = append(files, f)
	}
	exports := map[string]string{}
	cmd := exec.Command("go", "list", "-export", "-deps", "-f", "{{.ImportPath}}={{.Export}}", "golang.org/x/sys/windows", "syscall", "unsafe")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+goarch)
	listing, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	for _, l := range strings.Split(string(listing), "\n") {
		if i := strings.IndexByte(l, '='); i > 0 {
			exports[l[:i]] = l[i+1:]
		}
	}
	lookup := func(path string) (io.ReadCloser, error) { return os.Open(exports[path]) }
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
	pkg, err := conf.Check("win", fset, files, nil)
	if err != nil {
		panic(err)
	}
	q := types.RelativeTo(pkg)

	var out []string
	for _, n := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(n)
		switch obj := obj.(type) {
		case *types.Const:
			out = append(out, fmt.Sprintf("const %s %s = %s", n, types.TypeString(obj.Type(), q), obj.Val().ExactString()))
		case *types.Var:
			if obj.Exported() {
				out = append(out, fmt.Sprintf("var %s %s", n, types.TypeString(obj.Type(), q)))
			}
		case *types.Func:
			sig := obj.Type().(*types.Signature)
			var ps, rs []string
			for i := 0; i < sig.Params().Len(); i++ {
				ps = append(ps, types.TypeString(sig.Params().At(i).Type(), q))
			}
			for i := 0; i < sig.Results().Len(); i++ {
				rs = append(rs, types.TypeString(sig.Results().At(i).Type(), q))
			}
			out = append(out, fmt.Sprintf("func %s(%s) (%s)", n, strings.Join(ps, ", "), strings.Join(rs, ", ")))
		case *types.TypeName:
			out = append(out, fmt.Sprintf("type %s %s", n, types.TypeString(obj.Type().Underlying(), q)))
		}
	}

	// Procs and DLLs, assigned in init functions or var declarations.
	procs := map[string][2]string{}
	libs := map[string]string{}
	record := func(lhs string, rhs ast.Expr) {
		call, ok := rhs.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return
		}
		s, _ := strconv.Unquote(lit.Value)
		switch sel.Sel.Name {
		case "NewProc":
			v := [2]string{sel.X.(*ast.Ident).Name, s}
			if old, dup := procs[lhs]; dup && old != v {
				v = [2]string{"?", "?"}
			}
			procs[lhs] = v
		case "NewLazySystemDLL":
			libs[lhs] = strings.ToLower(s)
		}
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
					if id, ok := n.Lhs[0].(*ast.Ident); ok {
						record(id.Name, n.Rhs[0])
					}
				}
			case *ast.ValueSpec:
				for i, id := range n.Names {
					if i < len(n.Values) {
						record(id.Name, n.Values[i])
					}
				}
			}
			return true
		})
	}

	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name.Name == "init" {
				continue
			}
			if s := normalize(fd, procs, libs); s != "" {
				out = append(out, "body "+fd.Name.Name+" "+s)
			}
		}
	}
	sort.Strings(out)
	for _, l := range out {
		fmt.Println(l)
	}
}

// normalize returns a canonical form of a plain syscall wrapper, or "".
func normalize(fd *ast.FuncDecl, procs map[string][2]string, libs map[string]string) string {
	rename := map[string]string{}
	n := 0
	for _, fld := range fd.Type.Params.List {
		for _, id := range fld.Names {
			rename[id.Name] = fmt.Sprintf("$%d", n)
			n++
		}
	}
	stmts := fd.Body.List
	if len(stmts) == 0 || len(stmts) > 2 {
		return ""
	}
	var call *ast.CallExpr
	switch s := stmts[0].(type) {
	case *ast.AssignStmt:
		if len(s.Rhs) != 1 || len(s.Lhs) != 3 {
			return ""
		}
		call, _ = s.Rhs[0].(*ast.CallExpr)
		if id, ok := s.Lhs[0].(*ast.Ident); ok {
			rename[id.Name] = "$r"
		}
		for _, l := range s.Lhs[1:] {
			if id, ok := l.(*ast.Ident); !ok || id.Name != "_" {
				return ""
			}
		}
	case *ast.ExprStmt:
		call, _ = s.X.(*ast.CallExpr)
	}
	if call == nil {
		return ""
	}
	fun := exprString(call.Fun)
	if !strings.HasPrefix(fun, "syscall.Syscall") || len(call.Args) == 0 {
		return ""
	}
	addr := exprString(call.Args[0])
	if !strings.HasSuffix(addr, ".Addr()") {
		return ""
	}
	pe := procs[strings.TrimSuffix(addr, ".Addr()")]
	target := libs[pe[0]] + "!" + pe[1]
	args := call.Args[1:]
	if fun != "syscall.SyscallN" {
		if len(args) == 0 {
			return ""
		}
		cnt, err := strconv.Atoi(exprString(args[0]))
		if err != nil {
			return ""
		}
		args = args[1:]
		for _, a := range args[cnt:] {
			if exprString(a) != "0" {
				return ""
			}
		}
		args = args[:cnt]
	}
	sub := func(e ast.Expr) string {
		ast.Inspect(e, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if r, ok := rename[id.Name]; ok {
					id.Name = r
				}
			}
			return true
		})
		return exprString(e)
	}
	var as []string
	for _, a := range args {
		as = append(as, sub(a))
	}
	s := "call " + target + "(" + strings.Join(as, ", ") + ")"
	if len(stmts) == 2 {
		rs, ok := stmts[1].(*ast.ReturnStmt)
		if !ok || len(rs.Results) != 1 {
			return ""
		}
		s += " return " + sub(rs.Results[0])
	}
	return s
}
