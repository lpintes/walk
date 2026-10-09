// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// The -suggest mode finds hand-written declarations of the Go package that
// walk uses and that can be generated without any change of their Go type,
// value or behavior, and prints specification lines for them.

import (
	"encoding/binary"
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
	info  *types.Info
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
		w.recordVars(f)
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			if fd.Name.Name == "init" {
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
	w.info = &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}
	pkg, err := conf.Check("win", fset, files, w.info)
	if err != nil {
		return nil, err
	}
	w.pkg = pkg
	return w, nil
}

// recordVars records the LazyProc and LazyDLL variables a file assigns,
// in init functions or in var declarations.
func (w *winPkg) recordVars(f *ast.File) {
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
			lib := sel.X.(*ast.Ident).Name
			if _, dup := w.procs[lhs]; dup {
				w.procs[lhs] = [2]string{"", ""}
			} else {
				w.procs[lhs] = [2]string{lib, s}
			}
		case "NewLazySystemDLL":
			w.libs[lhs] = strings.ToLower(s)
		}
	}
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
		// The generated argument comes first; the others are
		// equivalent forms found in hand-written code.
		var want []string
		switch {
		case f.byValue[i] > 0:
			conv := fmt.Sprintf("uintptr(*(*uint%d)(unsafe.Pointer(", 8*f.byValue[i])
			want = []string{conv + "&" + p + ")))", conv + "uintptr(unsafe.Pointer(&" + p + ")))))"}
		case f.shapes[i] == shapeBool:
			want = []string{"uintptr(BoolToBOOL(" + p + "))"}
		case f.params[i].typ.name == "uintptr":
			want = []string{p, "uintptr(" + p + ")"}
		case f.params[i].typ.name == "unsafe.Pointer":
			want = []string{"uintptr(" + p + ")"}
		case f.shapes[i] == shapePointer:
			want = []string{"uintptr(unsafe.Pointer(" + p + "))"}
		default:
			want = []string{"uintptr(" + p + ")"}
		}
		ok := false
		for _, w := range want {
			ok = ok || as == w
		}
		if !ok {
			return "", fmt.Sprintf("arg %d: %s want %s", i, as, want[0])
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
	for _, gs := range s.guids {
		inSpec[gs.name] = true
	}
	for _, is := range s.interfaces {
		inSpec[is.name] = true
		inSpec[is.name+"Vtbl"] = true
	}
	gp.addStructs(s.structs)
	gp.addInterfaces(s.interfaces)

	// Structs passed by value must be generated.
	g := &generator{m: m, pkg: gp, s: &spec{structs: s.structs}}
	structLines, err := suggestStructs(g, dir, w, used, inSpec)
	if err != nil {
		return err
	}
	guidLines := suggestGUIDs(m, w, used, inSpec)
	ifaceLines := suggestInterfaces(g, w, used, inSpec)
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
			// Types referring to structs the package does not define
			// need an override before the function can be checked.
			if md, err := m.lookupMethod(name, false); err == nil {
				tm := &typeMapper{m: m, arch: archAMD64, pkg: gp}
				if msig, err := tm.signature(md); err == nil && len(msig.params) == len(hp) {
					for i, p := range msig.params {
						if p.typ.undefined {
							fs.params[p.name] = hp[i]
						}
					}
					if msig.result.undefined {
						fs.result = hr
					}
				}
			}
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
	fmt.Println()
	for _, l := range guidLines {
		fmt.Println(l)
	}
	fmt.Println()
	for _, l := range ifaceLines {
		fmt.Println(l)
	}
	return nil
}

// varValues returns the initial values of the package-level variables of
// the package.
func (w *winPkg) varValues() map[string]ast.Expr {
	values := make(map[string]ast.Expr)
	for _, f := range w.files {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, id := range vs.Names {
					if i < len(vs.Values) {
						values[id.Name] = vs.Values[i]
					}
				}
			}
		}
	}
	return values
}

// guidBytes returns the value of a GUID composite literal with constant
// elements in memory order.
func (w *winPkg) guidBytes(e ast.Expr) (*[16]byte, error) {
	cl, ok := e.(*ast.CompositeLit)
	if !ok || len(cl.Elts) != 4 {
		return nil, fmt.Errorf("not a GUID literal")
	}
	num := func(e ast.Expr) (uint64, error) {
		if _, ok := e.(*ast.KeyValueExpr); ok {
			return 0, fmt.Errorf("keyed element")
		}
		tv := w.info.Types[e]
		if tv.Value == nil {
			return 0, fmt.Errorf("element %s is not constant", exprString(e))
		}
		v, ok := gc.Uint64Val(tv.Value)
		if !ok {
			return 0, fmt.Errorf("element %s is not an unsigned integer", exprString(e))
		}
		return v, nil
	}
	var g [16]byte
	d1, err := num(cl.Elts[0])
	if err != nil {
		return nil, err
	}
	d2, err := num(cl.Elts[1])
	if err != nil {
		return nil, err
	}
	d3, err := num(cl.Elts[2])
	if err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint32(g[0:], uint32(d1))
	binary.LittleEndian.PutUint16(g[4:], uint16(d2))
	binary.LittleEndian.PutUint16(g[6:], uint16(d3))
	d4, ok := cl.Elts[3].(*ast.CompositeLit)
	if !ok || len(d4.Elts) != 8 {
		return nil, fmt.Errorf("Data4 is not an array literal of 8 elements")
	}
	for i, el := range d4.Elts {
		v, err := num(el)
		if err != nil {
			return nil, err
		}
		g[8+i] = byte(v)
	}
	return &g, nil
}

// suggestGUIDs returns specification lines for the hand-written GUID
// variables used under root whose value is the same in the metadata.
func suggestGUIDs(m *metadata, w *winPkg, used []string, inSpec map[string]bool) []string {
	q := types.RelativeTo(w.pkg)
	values := w.varValues()
	var lines []string
	for _, name := range used {
		v, ok := w.pkg.Scope().Lookup(name).(*types.Var)
		if !ok || inSpec[name] {
			continue
		}
		if types.TypeString(v.Type().Underlying(), nil) != types.TypeString(guidStruct(w.pkg), nil) {
			continue
		}
		hand, err := w.guidBytes(values[name])
		if err != nil {
			fmt.Fprintln(os.Stderr, "SKIP guid", name, err)
			continue
		}
		meta, _, err := m.lookupGUID(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "SKIP guid", name, err)
			continue
		}
		if *meta != *hand {
			fmt.Fprintf(os.Stderr, "SKIP guid %s differs: %x vs %x\n", name, hand, meta)
			continue
		}
		line := "guid " + name
		gs := &guidSpec{name: name}
		if typ := types.TypeString(v.Type(), q); typ != gs.guidType() {
			line += " " + typ
		}
		lines = append(lines, line)
	}
	return lines
}

// guidStruct returns the underlying type of syscall.GUID as imported by
// the package.
func guidStruct(pkg *types.Package) types.Type {
	for _, imp := range pkg.Imports() {
		if imp.Path() == "syscall" {
			if tn, ok := imp.Scope().Lookup("GUID").(*types.TypeName); ok {
				return tn.Type().Underlying()
			}
		}
	}
	return nil
}

// handInterface returns the vtable field names of a hand-written COM
// interface: a struct NAME with the single field LpVtbl *NAMEVtbl, whose
// fields are all uintptr.
func handInterface(pkg *types.Package, name string) ([]string, error) {
	tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("not a type")
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	if !ok || st.NumFields() != 1 || st.Field(0).Name() != "LpVtbl" {
		return nil, fmt.Errorf("not a struct with the single field LpVtbl")
	}
	ptr, ok := st.Field(0).Type().(*types.Pointer)
	if !ok {
		return nil, fmt.Errorf("LpVtbl is not a pointer")
	}
	vt, ok := ptr.Elem().(*types.Named)
	if !ok || vt.Obj().Name() != name+"Vtbl" || vt.Obj().Pkg() != pkg {
		return nil, fmt.Errorf("LpVtbl is not a *%sVtbl", name)
	}
	vst, ok := vt.Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("%sVtbl is not a struct", name)
	}
	var fields []string
	for i := 0; i < vst.NumFields(); i++ {
		f := vst.Field(i)
		if f.Embedded() || vst.Tag(i) != "" || !types.Identical(f.Type(), types.Typ[types.Uintptr]) {
			return nil, fmt.Errorf("%sVtbl.%s is not a plain uintptr field", name, f.Name())
		}
		fields = append(fields, f.Name())
	}
	return fields, nil
}

// suggestInterfaces returns specification lines for the hand-written COM
// interfaces whose vtable has the methods of the metadata interface in the
// same order. An interface is considered if walk uses it or its vtable, or
// if the type of a parameter or result of a method of another such
// interface refers to it.
func suggestInterfaces(g *generator, w *winPkg, used []string, inSpec map[string]bool) []string {
	candidates := make(map[string]bool)
	var queue []string
	add := func(name string) {
		name = strings.TrimSuffix(name, "Vtbl")
		if candidates[name] {
			return
		}
		if _, err := handInterface(w.pkg, name); err != nil {
			return
		}
		candidates[name] = true
		queue = append(queue, name)
	}
	for _, name := range used {
		add(name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		tn := w.pkg.Scope().Lookup(name).(*types.TypeName)
		ms := types.NewMethodSet(types.NewPointer(tn.Type()))
		for i := 0; i < ms.Len(); i++ {
			sig := ms.At(i).Type().(*types.Signature)
			for _, tup := range []*types.Tuple{sig.Params(), sig.Results()} {
				for j := 0; j < tup.Len(); j++ {
					t := tup.At(j).Type()
					for {
						p, ok := t.(*types.Pointer)
						if !ok {
							break
						}
						t = p.Elem()
					}
					if n, ok := t.(*types.Named); ok && n.Obj().Pkg() == w.pkg {
						add(n.Obj().Name())
					}
				}
			}
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
		hand, _ := handInterface(w.pkg, name)
		is := &interfaceSpec{name: name, names: make(map[string]string)}
		gi, err := g.interfaceDef(is)
		if err != nil {
			fmt.Fprintln(os.Stderr, "SKIP interface", name, err)
			continue
		}
		if len(hand) != len(gi.fields) {
			fmt.Fprintf(os.Stderr, "SKIP interface %s: vtable has %d methods, metadata has %d\n", name, len(hand), len(gi.fields))
			continue
		}
		line := "interface " + name
		var opts []string
		for i, f := range gi.fields {
			if hand[i] != f.name {
				opts = append(opts, f.meta+"="+hand[i])
			}
		}
		sort.Strings(opts)
		for _, o := range opts {
			line += " " + o
		}
		lines = append(lines, line)
	}
	return lines
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
	// Field types referring to structs the package does not define need
	// an override before the struct can be checked.
	if td, err := g.m.lookupStruct(ss.metaNames(), archAMD64); err == nil {
		if fields, err := g.m.structFields(td); err == nil && len(fields) == len(hand[archAMD64]) {
			tm := &typeMapper{m: g.m, arch: archAMD64, pkg: g.pkg, rawBool: true}
			for i, f := range fields {
				if t, err := tm.sigType(f.typ); err == nil && t.undefined {
					ss.types[f.name] = hand[archAMD64][i].typ
				}
			}
		}
	}
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
