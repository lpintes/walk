// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/microsoft/go-winmd"
	"github.com/microsoft/go-winmd/coded"
	"github.com/microsoft/go-winmd/flags"
)

// typeKind classifies a metadata type for code generation.
type typeKind int

const (
	kindScalar    typeKind = iota // integer, handle or other value that fits a register
	kindBool                      // BOOL translated to bool
	kindFloat                     // float32 or float64
	kindPointer                   // any pointer, passed through unsafe.Pointer
	kindStruct                    // struct passed by value
	kindInterface                 // COM interface pointer
	kindVoid
)

// goType is a metadata type translated to Go.
type goType struct {
	name string   // Go type expression
	kind typeKind // how a value is passed to a syscall
	size int      // size in bytes for kindScalar and kindFloat on the target architecture
}

// typeMapper translates metadata signature types to Go types of package
// win.
type typeMapper struct {
	m    *metadata
	arch arch // single target architecture

	// pkg holds the types defined in the Go package. A metadata type with
	// the same name is used as is.
	pkg *goPackage

	// rawBool keeps BOOL instead of translating it to bool.
	rawBool bool
}

// defaultRenames maps metadata type names that walk's win package spells
// differently.
var defaultRenames = map[string]string{
	"PWSTR":   "*uint16",
	"PCWSTR":  "*uint16",
	"PSTR":    "*byte",
	"PCSTR":   "*byte",
	"WPARAM":  "uintptr",
	"LPARAM":  "uintptr",
	"LRESULT": "uintptr",
	"Guid":    "syscall.GUID",
}

func (tm *typeMapper) sigType(t winmd.SigType) (goType, error) {
	ptr := tm.arch.ptrSize()
	switch t.Kind {
	case flags.ElementType_VOID:
		return goType{kind: kindVoid}, nil
	case flags.ElementType_BOOLEAN:
		return goType{"bool", kindScalar, 1}, nil
	case flags.ElementType_I1:
		return goType{"int8", kindScalar, 1}, nil
	case flags.ElementType_U1:
		return goType{"byte", kindScalar, 1}, nil
	case flags.ElementType_I2:
		return goType{"int16", kindScalar, 2}, nil
	case flags.ElementType_U2, flags.ElementType_CHAR:
		return goType{"uint16", kindScalar, 2}, nil
	case flags.ElementType_I4:
		return goType{"int32", kindScalar, 4}, nil
	case flags.ElementType_U4:
		return goType{"uint32", kindScalar, 4}, nil
	case flags.ElementType_I8:
		return goType{"int64", kindScalar, 8}, nil
	case flags.ElementType_U8:
		return goType{"uint64", kindScalar, 8}, nil
	case flags.ElementType_R4:
		return goType{"float32", kindFloat, 4}, nil
	case flags.ElementType_R8:
		return goType{"float64", kindFloat, 8}, nil
	case flags.ElementType_I, flags.ElementType_U:
		return goType{"uintptr", kindScalar, ptr}, nil
	case flags.ElementType_PTR:
		elem, ok := t.Value.(winmd.SigType)
		if !ok {
			return goType{}, fmt.Errorf("unexpected pointer value %#v", t.Value)
		}
		if elem.Kind == flags.ElementType_VOID {
			return goType{"unsafe.Pointer", kindPointer, ptr}, nil
		}
		et, err := tm.sigType(elem)
		if err != nil {
			return goType{}, err
		}
		return goType{"*" + et.name, kindPointer, ptr}, nil
	case flags.ElementType_ARRAY:
		a, ok := t.Value.(winmd.SigArray)
		if !ok || a.Rank != 1 || len(a.Sizes) != 1 {
			return goType{}, fmt.Errorf("unsupported array %#v", t.Value)
		}
		et, err := tm.sigType(a.Type)
		if err != nil {
			return goType{}, err
		}
		return goType{"[" + strconv.Itoa(int(a.Sizes[0])) + "]" + et.name, kindStruct, 0}, nil
	case flags.ElementType_VALUETYPE, flags.ElementType_CLASS:
		ci, ok := t.Value.(winmd.CodedIndex)
		if !ok {
			return goType{}, fmt.Errorf("unexpected type value %#v", t.Value)
		}
		return tm.named(ci)
	}
	return goType{}, fmt.Errorf("unsupported element type %v", t.Kind)
}

// named translates a reference to a TypeDef or TypeRef.
func (tm *typeMapper) named(ci winmd.CodedIndex) (goType, error) {
	var td *typeDef
	var name string
	switch ci.Tag {
	case coded.TypeDefOrRefOrSpec_TypeDef:
		def, err := tm.m.md.Tables.TypeDef.Record(ci.Index)
		if err != nil {
			return goType{}, err
		}
		name = def.Name.String()
		td = tm.m.lookupTypeDef(name, def.Namespace.String(), tm.arch)
	case coded.TypeDefOrRefOrSpec_TypeRef:
		tr, err := tm.m.md.Tables.TypeRef.Record(ci.Index)
		if err != nil {
			return goType{}, err
		}
		name = tr.Name.String()
		if tr.ResolutionScope.Tag == coded.ResolutionScope_TypeRef {
			return goType{}, fmt.Errorf("nested type %s is not supported", name)
		}
		td = tm.m.lookupTypeDef(name, tr.Namespace.String(), tm.arch)
	default:
		return goType{}, fmt.Errorf("unexpected coded index %#v", ci)
	}

	if name == "BOOL" && !tm.rawBool {
		return goType{"bool", kindBool, 4}, nil
	}
	if r, ok := defaultRenames[name]; ok {
		return tm.fixedType(r, td)
	}
	if td == nil {
		return goType{}, fmt.Errorf("type %s is not defined in the metadata", name)
	}

	switch {
	case tm.m.isInterface(td):
		return goType{"*" + name, kindInterface, tm.arch.ptrSize()}, nil
	case tm.m.isDelegate(td):
		// Callbacks are passed as uintptr values created by
		// syscall.NewCallback.
		return goType{"uintptr", kindScalar, tm.arch.ptrSize()}, nil
	case tm.m.isEnum(td), td.hasAttr("NativeTypedefAttribute"):
		u, err := tm.m.underlying(td)
		if err != nil {
			return goType{}, err
		}
		ut, err := tm.sigType(u)
		if err != nil {
			return goType{}, err
		}
		if tm.pkg.hasType(name) {
			ut.name = name
		}
		return ut, nil
	}
	// walk spells the Unicode variants of structs without the W suffix.
	if strings.HasSuffix(name, "W") && !tm.pkg.hasType(name) && tm.pkg.hasType(name[:len(name)-1]) {
		name = name[:len(name)-1]
	}
	if !tm.pkg.hasType(name) {
		return goType{}, fmt.Errorf("struct %s is not defined in the Go package", name)
	}
	return goType{name, kindStruct, 0}, nil
}

// fixedType returns a renamed type. The kind is derived from the
// metadata definition if there is one.
func (tm *typeMapper) fixedType(name string, td *typeDef) (goType, error) {
	if name[0] == '*' || name == "unsafe.Pointer" {
		return goType{name, kindPointer, tm.arch.ptrSize()}, nil
	}
	if name == "uintptr" {
		return goType{name, kindScalar, tm.arch.ptrSize()}, nil
	}
	if td != nil && (tm.m.isEnum(td) || td.hasAttr("NativeTypedefAttribute")) {
		u, err := tm.m.underlying(td)
		if err != nil {
			return goType{}, err
		}
		ut, err := tm.sigType(u)
		if err != nil {
			return goType{}, err
		}
		ut.name = name
		return ut, nil
	}
	return goType{name, kindStruct, 0}, nil
}

// lookupTypeDef finds a top level TypeDef by name and namespace that
// supports the given architecture.
func (m *metadata) lookupTypeDef(name, namespace string, a arch) *typeDef {
	for _, td := range m.typeDefs[name] {
		if td.namespace() == namespace && td.arch&a != 0 {
			return td
		}
	}
	return nil
}

func (m *metadata) isInterface(td *typeDef) bool {
	return td.def.Flags&flags.TypeAttributes_ClassSemanticsMask == flags.TypeAttributes_Interface
}

func (m *metadata) isDelegate(td *typeDef) bool {
	if td.def.Extends.Tag != coded.TypeDefOrRef_TypeRef {
		return false
	}
	tr, err := m.md.Tables.TypeRef.Record(td.def.Extends.Index)
	if err != nil {
		return false
	}
	return tr.Namespace.String() == "System" && tr.Name.String() == "MulticastDelegate"
}

// underlying returns the type of the single field of a native typedef or
// the value__ field of an enum.
func (m *metadata) underlying(td *typeDef) (winmd.SigType, error) {
	for i := td.def.FieldList.Start; i < td.def.FieldList.End; i++ {
		fd, err := m.md.Tables.Field.Record(i)
		if err != nil {
			return winmd.SigType{}, err
		}
		if m.isEnum(td) && fd.Name.String() != "value__" {
			continue
		}
		sig, err := m.md.FieldSignature(fd.Signature)
		if err != nil {
			return winmd.SigType{}, err
		}
		return sig.Type, nil
	}
	return winmd.SigType{}, fmt.Errorf("type %s has no underlying type", td.name())
}
