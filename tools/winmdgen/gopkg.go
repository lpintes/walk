// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
)

// shape is the kind of the underlying type of a Go type, as far as code
// generation needs to know it.
type shape int

const (
	shapeScalar  shape = iota // integer or uintptr
	shapePointer              // pointer or unsafe.Pointer
	shapeBool
	shapeFloat
	shapeOther // struct, array, interface, function, ...
)

// goPackage describes the hand-written types of the target package.
type goPackage struct {
	types map[string]ast.Expr // type name -> type expression
}

// parseGoPackage parses the Go files in dir, skipping generated files.
func parseGoPackage(dir string) (*goPackage, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	p := &goPackage{types: make(map[string]ast.Expr)}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasPrefix(filepath.Base(file), outputPrefix) {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, s := range gd.Specs {
				ts := s.(*ast.TypeSpec)
				p.types[ts.Name.Name] = ts.Type
			}
		}
	}
	return p, nil
}

// addStructs adds the structs a specification generates, so that the
// generated code can refer to them.
func (p *goPackage) addStructs(structs []*structSpec) {
	for _, s := range structs {
		p.types[s.name] = &ast.StructType{}
	}
}

func (p *goPackage) hasType(name string) bool {
	_, ok := p.types[name]
	return ok
}

// shapeOf returns the shape of a Go type expression as written in
// generated code.
func (p *goPackage) shapeOf(expr string) shape {
	return p.shape(expr, 0)
}

// basic returns the predeclared type underlying a Go type expression, or
// "" if it is not a predeclared type.
func (p *goPackage) basic(expr string) string {
	for depth := 0; depth < 10; depth++ {
		if types.Universe.Lookup(expr) != nil {
			return expr
		}
		id, ok := p.types[expr].(*ast.Ident)
		if !ok {
			return ""
		}
		expr = id.Name
	}
	return ""
}

func (p *goPackage) shape(expr string, depth int) shape {
	switch {
	case strings.HasPrefix(expr, "*"), expr == "unsafe.Pointer":
		return shapePointer
	case expr == "bool":
		return shapeBool
	case expr == "float32", expr == "float64":
		return shapeFloat
	case expr == "uintptr", expr == "byte", expr == "uint", expr == "int",
		strings.HasPrefix(expr, "int"), strings.HasPrefix(expr, "uint"):
		return shapeScalar
	}
	t, ok := p.types[expr]
	if !ok || depth > 10 {
		return shapeOther
	}
	switch t := t.(type) {
	case *ast.StarExpr:
		return shapePointer
	case *ast.Ident:
		return p.shape(t.Name, depth+1)
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok && x.Name == "unsafe" && t.Sel.Name == "Pointer" {
			return shapePointer
		}
	}
	return shapeOther
}
