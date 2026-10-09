// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// The -suggest mode finds hand-written declarations of the Go package that
// walk uses and that can be generated without any change of their Go type,
// value or behavior, and prints specification lines for them.

import (
	"fmt"
	"go/ast"
	"go/build"
	gc "go/constant"
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

type winPkg struct {
	pkg   *types.Package
	files map[string]*ast.File
	funcs map[string]*ast.FuncDecl
	// proc var -> (lib var, entry); "" entry if ambiguous
	procs map[string][2]string
	libs  map[string]string // lib var -> dll name
}

func loadWin(dir, goarch string) (*winPkg, error) {
	ctx := build.Default
	ctx.GOOS = "windows"
	ctx.GOARCH = goarch
	w := &winPkg{files: map[string]*ast.File{}, funcs: map[string]*ast.FuncDecl{}, procs: map[string][2]string{}, libs: map[string]string{}}
	var files []*ast.File
	names, _ := filepath.Glob(dir + "/*.go")
	for _, n := range names {
		ok, _ := ctx.MatchFile(dir, filepath.Base(n))
		if !ok {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
		w.files[n] = f
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			if fd.Name.Name == "init" {
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					as, ok := n.(*ast.AssignStmt)
					if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
						return true
					}
					lhs, ok := as.Lhs[0].(*ast.Ident)
					if !ok {
						return true
					}
					call, ok := as.Rhs[0].(*ast.CallExpr)
					if !ok || len(call.Args) != 1 {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					lit, ok := call.Args[0].(*ast.BasicLit)
					if !ok {
						return true
					}
					s, _ := strconv.Unquote(lit.Value)
					switch sel.Sel.Name {
					case "NewProc":
						lib := sel.X.(*ast.Ident).Name
						if _, dup := w.procs[lhs.Name]; dup {
							w.procs[lhs.Name] = [2]string{"", ""}
						} else {
							w.procs[lhs.Name] = [2]string{lib, s}
						}
					case "NewLazySystemDLL":
						w.libs[lhs.Name] = strings.ToLower(s)
					}
					return true
				})
				continue
			}
			w.funcs[fd.Name.Name] = fd
		}
	}
	imp, err := exportImporter(goarch)
	if err != nil {
		return nil, err
	}
	conf := types.Config{Importer: imp, Sizes: types.SizesFor("gc", goarch)}
	pkg, err := conf.Check("win", fset, files, nil)
	if err != nil {
		return nil, err
	}
	w.pkg = pkg
	return w, nil
}

// exportImporter imports the dependencies of the package from export data
// built by the go command for Windows.
func exportImporter(goarch string) (types.Importer, error) {
	exports := make(map[string]string)
	cmd := exec.Command("go", "list", "-export", "-deps", "-f", "{{.ImportPath}}={{.Export}}",
		"golang.org/x/sys/windows", "syscall", "unsafe")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+goarch)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	for _, l := range strings.Split(string(out), "\n") {
		if i := strings.IndexByte(l, '='); i > 0 {
			exports[l[:i]] = l[i+1:]
		}
	}
	lookup := func(path string) (io.ReadCloser, error) { return os.Open(exports[path]) }
	return importer.ForCompiler(fset, "gc", lookup), nil
}

// usedSymbols returns the identifiers selected from package win (win.X) in
// the Go files under root, outside the package itself.
func usedSymbols(root, winDir string) ([]string, error) {
	winAbs, err := filepath.Abs(winDir)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if abs, _ := filepath.Abs(path); abs == winAbs || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if se, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := se.X.(*ast.Ident); ok && id.Name == "win" {
					seen[se.Sel.Name] = true
				}
			}
			return true
		})
		return nil
	})
	var names []string
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, err
}

func exprString(e ast.Expr) string {
	var b strings.Builder
	printer.Fprint(&b, fset, e)
	return b.String()
}

// simpleWrapper checks the body of a hand-written function against the code
// the generator would write. It returns the proc var name.
func simpleWrapper(fd *ast.FuncDecl, f *genFunc) (string, string) {
	var names []string
	for _, fld := range fd.Type.Params.List {
		for _, n := range fld.Names {
			names = append(names, n.Name)
		}
	}
	if len(names) != len(f.params) {
		return "", "param count"
	}
	stmts := fd.Body.List
	var call *ast.CallExpr
	var retVar string
	switch s := stmts[0].(type) {
	case *ast.AssignStmt:
		if len(s.Rhs) != 1 || len(s.Lhs) != 3 {
			return "", "assign shape"
		}
		call, _ = s.Rhs[0].(*ast.CallExpr)
		if id, ok := s.Lhs[0].(*ast.Ident); ok {
			retVar = id.Name
		}
		for _, l := range s.Lhs[1:] {
			if id, ok := l.(*ast.Ident); !ok || id.Name != "_" {
				return "", "uses err"
			}
		}
	case *ast.ExprStmt:
		call, _ = s.X.(*ast.CallExpr)
	default:
		return "", "first stmt"
	}
	if call == nil {
		return "", "no call"
	}
	fun := exprString(call.Fun)
	if !strings.HasPrefix(fun, "syscall.Syscall") {
		return "", "not syscall: " + fun
	}
	addr := exprString(call.Args[0])
	if !strings.HasSuffix(addr, ".Addr()") {
		return "", "addr"
	}
	proc := strings.TrimSuffix(addr, ".Addr()")
	args := call.Args[1:]
	if fun != "syscall.SyscallN" {
		n, err := strconv.Atoi(exprString(args[0]))
		if err != nil || n != len(names) {
			return "", "nargs " + exprString(args[0])
		}
		args = args[1:]
	}
	for i, a := range args {
		as := exprString(a)
		if i >= len(names) {
			if as != "0" {
				return "", "extra arg " + as
			}
			continue
		}
		p := names[i]
		var want string
		switch {
		case f.shapes[i] == shapeBool:
			want = "uintptr(BoolToBOOL(" + p + "))"
		case f.params[i].typ.name == "uintptr":
			want = p
		case f.params[i].typ.name == "unsafe.Pointer":
			want = "uintptr(" + p + ")"
		case f.shapes[i] == shapePointer:
			want = "uintptr(unsafe.Pointer(" + p + "))"
		default:
			want = "uintptr(" + p + ")"
		}
		if as != want {
			return "", fmt.Sprintf("arg %d: %s want %s", i, as, want)
		}
	}
	if len(args) < len(names) {
		return "", "too few args"
	}
	rest := stmts[1:]
	if f.result == "" {
		if len(rest) != 0 || retVar != "" && retVar != "_" {
			return "", "void with result"
		}
		return proc, ""
	}
	if len(rest) != 1 {
		return "", "stmts after call"
	}
	rs, ok := rest[0].(*ast.ReturnStmt)
	if !ok || len(rs.Results) != 1 {
		return "", "return"
	}
	got := exprString(rs.Results[0])
	var want string
	switch {
	case f.rshape == shapeBool:
		want = retVar + " != 0"
	case f.result == "uintptr":
		want = retVar
	default:
		want = f.result + "(" + retVar + ")"
	}
	if got != want {
		return "", "return " + got + " want " + want
	}
	return proc, ""
}

// suggest prints specification lines for the symbols used under root that
// are hand-written in dir and can be generated. Reasons for skipping a
// symbol go to standard error.
func suggest(m *metadata, s *spec, dir, root string) error {
	w, err := loadWin(dir, "amd64")
	if err != nil {
		return err
	}
	gp, err := parseGoPackage(dir)
	if err != nil {
		return err
	}
	q := types.RelativeTo(w.pkg)
	used, err := usedSymbols(root, dir)
	if err != nil {
		return err
	}
	inSpec := make(map[string]bool)
	for _, c := range s.constants {
		inSpec[c.name] = true
	}
	for _, f := range s.functions {
		inSpec[f.name] = true
	}
	for _, st := range s.structs {
		inSpec[st.name] = true
	}
	gp.addStructs(s.structs)

	g := &generator{m: m, pkg: gp, s: &spec{}}
	structLines, err := suggestStructs(g, dir, w, used, inSpec)
	if err != nil {
		return err
	}
	var constLines, funcLines []string
	dllsUsed := map[string]string{}
	for _, name := range used {
		if inSpec[name] {
			continue
		}
		obj := w.pkg.Scope().Lookup(name)
		switch obj := obj.(type) {
		case *types.Const:
			c, err := m.lookupConstant(name)
			if err != nil {
				fmt.Fprintln(os.Stderr, "SKIP const", name, err)
				continue
			}
			v, err := c.goValue()
			if err != nil {
				fmt.Fprintln(os.Stderr, "SKIP const", name, err)
				continue
			}
			typ := ""
			if b, ok := obj.Type().(*types.Basic); !ok || b.Info()&types.IsUntyped == 0 {
				typ = types.TypeString(obj.Type(), q)
			}
			// Evaluate the generated declaration in the package scope.
			tv, err := types.Eval(fset, w.pkg, token.NoPos, func() string {
				if typ != "" {
					return typ + "(" + v + ")"
				}
				return v
			}())
			if err != nil {
				fmt.Fprintln(os.Stderr, "SKIP const", name, "eval", err)
				continue
			}
			if !types.Identical(tv.Type, obj.Type()) || !gc.Compare(tv.Value, token.EQL, obj.Val()) {
				fmt.Fprintln(os.Stderr, "SKIP const", name, "differs:", obj.Type(), obj.Val(), "vs", tv.Type, tv.Value)
				continue
			}
			line := "const " + name
			if typ != "" {
				line += " " + typ
			}
			constLines = append(constLines, line)
		case *types.Func:
			fd := w.funcs[name]
			hs := obj.Type().(*types.Signature)
			var hp []string
			for i := 0; i < hs.Params().Len(); i++ {
				hp = append(hp, types.TypeString(hs.Params().At(i).Type(), q))
			}
			hr := ""
			if hs.Results().Len() > 1 {
				fmt.Fprintln(os.Stderr, "SKIP func", name, "multiple results")
				continue
			} else if hs.Results().Len() == 1 {
				hr = types.TypeString(hs.Results().At(0).Type(), q)
			}
			hsig := "(" + strings.Join(hp, ", ") + ") " + hr

			fs := &funcSpec{name: name, params: map[string]string{}}
			var gf *genFunc
			var ferr error
			for attempt := 0; attempt < 3; attempt++ {
				gf, ferr = g.function(fs)
				if ferr != nil {
					break
				}
				if gf.goSignature() == hsig {
					break
				}
				if len(gf.params) != len(hp) {
					ferr = fmt.Errorf("param count %d vs %d", len(gf.params), len(hp))
					break
				}
				// Derive overrides.
				tm := &typeMapper{m: m, arch: archAMD64, pkg: gp, rawBool: fs.rawBool}
				msig, _ := tm.signature(gf.method)
				for i, p := range gf.params {
					if p.typ.name != hp[i] {
						if hp[i] == "BOOL" && msig.params[i].typ.kind == kindBool {
							fs.rawBool = true
						} else {
							fs.params[msig.params[i].name] = hp[i]
						}
					}
				}
				if gf.result != hr {
					if hr == "BOOL" && gf.rshape == shapeBool {
						fs.rawBool = true
					} else {
						fs.result = hr
					}
				}
			}
			if ferr != nil {
				fmt.Fprintln(os.Stderr, "SKIP func", name, ferr)
				continue
			}
			if gf.goSignature() != hsig {
				fmt.Fprintln(os.Stderr, "SKIP func", name, "signature", gf.goSignature(), "vs", hsig)
				continue
			}
			if fd == nil {
				fmt.Fprintln(os.Stderr, "SKIP func", name, "no decl")
				continue
			}
			proc, why := simpleWrapper(fd, gf)
			if why != "" {
				fmt.Fprintln(os.Stderr, "SKIP func", name, "body:", why)
				continue
			}
			pe, ok := w.procs[proc]
			if !ok || pe[1] == "" {
				fmt.Fprintln(os.Stderr, "SKIP func", name, "proc", proc, pe)
				continue
			}
			if pe[1] != gf.method.entry || w.libs[pe[0]] != gf.method.dll {
				fmt.Fprintln(os.Stderr, "SKIP func", name, "entry", pe, w.libs[pe[0]], "vs", gf.method.dll, gf.method.entry)
				continue
			}
			dllsUsed[gf.method.dll] = pe[0]
			line := "func " + name
			if fs.rawBool {
				line += " rawbool"
			}
			if fs.result != "" {
				line += " result=" + fs.result
			}
			var opts []string
			for k, v := range fs.params {
				opts = append(opts, k+":"+v)
			}
			sort.Strings(opts)
			for _, o := range opts {
				line += " " + o
			}
			funcLines = append(funcLines, line)
		}
	}
	var dlls []string
	for d, v := range dllsUsed {
		dlls = append(dlls, "dll "+d+" "+v)
	}
	sort.Strings(dlls)
	for _, l := range dlls {
		fmt.Println(l)
	}
	fmt.Println()
	for _, l := range constLines {
		fmt.Println(l)
	}
	fmt.Println()
	for _, l := range funcLines {
		fmt.Println(l)
	}
	fmt.Println()
	for _, l := range structLines {
		fmt.Println(l)
	}
	return nil
}

// suggestStructs returns specification lines for the hand-written structs
// used under root, directly or as the type of a field of another used
// struct, whose generated form has the same Go fields on every
// architecture. Field names and types the generator would choose
// differently become overrides.
func suggestStructs(g *generator, dir string, amd64 *winPkg, used []string, inSpec map[string]bool) ([]string, error) {
	pkgs := map[arch]*types.Package{archAMD64: amd64.pkg}
	for _, an := range archNames {
		if pkgs[an.arch] == nil {
			w, err := loadWin(dir, an.name)
			if err != nil {
				return nil, err
			}
			pkgs[an.arch] = w.pkg
		}
	}

	// Collect the used structs and the structs their fields refer to.
	candidates := make(map[string]bool)
	var visit func(t types.Type)
	visit = func(t types.Type) {
		switch t := t.(type) {
		case *types.Named:
			if t.Obj().Pkg() != amd64.pkg || candidates[t.Obj().Name()] {
				return
			}
			st, ok := t.Underlying().(*types.Struct)
			if !ok {
				return
			}
			candidates[t.Obj().Name()] = true
			for i := 0; i < st.NumFields(); i++ {
				visit(st.Field(i).Type())
			}
		case *types.Array:
			visit(t.Elem())
		}
	}
	for _, name := range used {
		if tn, ok := amd64.pkg.Scope().Lookup(name).(*types.TypeName); ok {
			visit(tn.Type())
		}
	}
	var names []string
	for n := range candidates {
		if !inSpec[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		line, err := suggestStruct(g, name, pkgs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "SKIP struct", name, err)
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

type handField struct {
	name, typ string
}

// handStruct returns the fields of a hand-written struct.
func handStruct(pkg *types.Package, name string) ([]handField, *types.Struct, error) {
	tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil, nil, fmt.Errorf("not a type")
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, nil, fmt.Errorf("not a struct")
	}
	var fields []handField
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Embedded() {
			return nil, nil, fmt.Errorf("embedded field %s", f.Name())
		}
		if st.Tag(i) != "" {
			return nil, nil, fmt.Errorf("field %s has a tag", f.Name())
		}
		fields = append(fields, handField{f.Name(), types.TypeString(f.Type(), types.RelativeTo(pkg))})
	}
	return fields, st, nil
}

func suggestStruct(g *generator, name string, pkgs map[arch]*types.Package) (string, error) {
	hand := make(map[arch][]handField)
	structs := make(map[arch]*types.Struct)
	for _, an := range archNames {
		fields, st, err := handStruct(pkgs[an.arch], name)
		if err != nil {
			return "", err
		}
		hand[an.arch], structs[an.arch] = fields, st
	}

	ss := &structSpec{name: name, names: make(map[string]string), types: make(map[string]string)}
	gs, err := g.structDef(ss)
	if err != nil {
		return "", err
	}
	for _, an := range archNames {
		h := hand[an.arch]
		if len(h) != len(gs.fields) {
			return "", fmt.Errorf("%s: %d fields, metadata has %d", an.name, len(h), len(gs.fields))
		}
		for i, f := range gs.fields {
			if h[i].name != f.name {
				if n, ok := ss.names[f.meta]; ok && n != h[i].name {
					return "", fmt.Errorf("field %s differs between architectures", f.meta)
				}
				ss.names[f.meta] = h[i].name
			}
			if h[i].typ != f.typ {
				if t, ok := ss.types[f.meta]; ok && t != h[i].typ {
					return "", fmt.Errorf("field %s differs between architectures", f.meta)
				}
				ss.types[f.meta] = h[i].typ
			}
		}
	}
	if gs, err = g.structDef(ss); err != nil {
		return "", err
	}

	// Check the layout of the hand-written struct, which the overrides
	// may have changed, against the metadata.
	for _, an := range archNames {
		sizes := types.SizesFor("gc", an.name)
		st := structs[an.arch]
		vars := make([]*types.Var, st.NumFields())
		for i := range vars {
			vars[i] = st.Field(i)
		}
		offsets := sizes.Offsetsof(vars)
		l := gs.layouts[an.arch]
		for i, f := range gs.fields {
			if int(offsets[i]) != l.offsets[i] {
				return "", fmt.Errorf("%s: field %s is at offset %d, metadata has %d", an.name, f.meta, offsets[i], l.offsets[i])
			}
		}
		if size, align := int(sizes.Sizeof(st)), int(sizes.Alignof(st)); size != l.size || align != l.align {
			return "", fmt.Errorf("%s: size and alignment are %d and %d, metadata has %d and %d", an.name, size, align, l.size, l.align)
		}
	}

	line := "struct " + name
	var opts []string
	for k, v := range ss.names {
		opts = append(opts, k+"="+v)
	}
	for k, v := range ss.types {
		opts = append(opts, k+":"+v)
	}
	sort.Strings(opts)
	for _, o := range opts {
		line += " " + o
	}
	return line, nil
}
