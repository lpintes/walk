// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/microsoft/go-winmd"
	"github.com/microsoft/go-winmd/coded"
	"github.com/microsoft/go-winmd/flags"
)

// isStruct reports whether a TypeDef is a struct or union, as opposed to
// an enum, a native typedef such as HWND, an interface, a delegate or an
// Apis class.
func (m *metadata) isStruct(td *typeDef) bool {
	if td.def.Extends.Tag != coded.TypeDefOrRef_TypeRef || td.hasAttr("NativeTypedefAttribute") {
		return false
	}
	tr, err := m.md.Tables.TypeRef.Record(td.def.Extends.Index)
	if err != nil {
		return false
	}
	return tr.Namespace.String() == "System" && tr.Name.String() == "ValueType"
}

func (m *metadata) isUnion(td *typeDef) bool {
	return td.def.Flags&flags.TypeAttributes_LayoutMask == flags.TypeAttributes_ExplicitLayout
}

// structField is a field of a metadata struct.
type structField struct {
	name string
	typ  winmd.SigType
}

// structFields returns the instance fields of a struct.
func (m *metadata) structFields(td *typeDef) ([]structField, error) {
	var fields []structField
	for i := td.def.FieldList.Start; i < td.def.FieldList.End; i++ {
		fd, err := m.md.Tables.Field.Record(i)
		if err != nil {
			return nil, err
		}
		if fd.Flags&(flags.FieldAttributes_Static|flags.FieldAttributes_Literal) != 0 {
			continue
		}
		sig, err := m.md.FieldSignature(fd.Signature)
		if err != nil {
			return nil, err
		}
		fields = append(fields, structField{fd.Name.String(), sig.Type})
	}
	return fields, nil
}

// lookupStruct finds the metadata struct for an architecture, trying the
// candidate names in order.
func (m *metadata) lookupStruct(candidates []string, a arch) (*typeDef, error) {
	for _, n := range candidates {
		var found []*typeDef
		for _, td := range m.typeDefs[n] {
			if m.isStruct(td) {
				found = append(found, td)
			}
		}
		if len(found) == 0 {
			continue
		}
		var match *typeDef
		for _, td := range found {
			if td.namespace() != found[0].namespace() {
				return nil, fmt.Errorf("struct %s is defined in %s and %s", n, found[0].namespace(), td.namespace())
			}
			if td.arch&a != 0 {
				if match != nil {
					return nil, fmt.Errorf("struct %s has several definitions for %v", n, a)
				}
				match = td
			}
		}
		if match == nil {
			return nil, fmt.Errorf("struct %s is not defined for %v", n, a)
		}
		return match, nil
	}
	return nil, fmt.Errorf("struct not found in the metadata")
}

// layoutRules selects how sizes and alignments are computed.
type layoutRules int

const (
	cRules  layoutRules = iota // the C compiler, honoring #pragma pack
	goRules                    // the Go compiler
)

// typeLayout is the size and alignment of a type.
type typeLayout struct {
	size, align int
}

// resolve finds the TypeDef a VALUETYPE or CLASS signature type refers to.
// It returns nil and no error for System.Guid.
func (m *metadata) resolve(ci winmd.CodedIndex, a arch) (*typeDef, error) {
	switch ci.Tag {
	case coded.TypeDefOrRefOrSpec_TypeDef:
		def, err := m.md.Tables.TypeDef.Record(ci.Index)
		if err != nil {
			return nil, err
		}
		td := &typeDef{
			index: ci.Index,
			def:   def,
			attrs: m.attrs[winmd.CodedIndex{Index: ci.Index, Tag: coded.HasCustomAttribute_TypeDef}],
		}
		td.arch = archFromAttrs(td.attrs)
		return td, nil
	case coded.TypeDefOrRefOrSpec_TypeRef:
		tr, err := m.md.Tables.TypeRef.Record(ci.Index)
		if err != nil {
			return nil, err
		}
		name, ns := tr.Name.String(), tr.Namespace.String()
		if ns == "System" && name == "Guid" {
			return nil, nil
		}
		if tr.ResolutionScope.Tag == coded.ResolutionScope_TypeRef {
			return nil, fmt.Errorf("nested type %s is not supported", name)
		}
		td := m.lookupTypeDef(name, ns, a)
		if td == nil {
			return nil, fmt.Errorf("type %s.%s is not defined for %v", ns, name, a)
		}
		return td, nil
	}
	return nil, fmt.Errorf("unexpected coded index %#v", ci)
}

// layoutOf returns the layout of a metadata type on one architecture.
func (m *metadata) layoutOf(t winmd.SigType, a arch, rules layoutRules) (typeLayout, error) {
	scalar := func(size int) typeLayout {
		l := typeLayout{size, size}
		if rules == goRules && a == arch386 && size == 8 {
			// Go aligns 64-bit values to 4 bytes on 386, C to 8 bytes.
			l.align = 4
		}
		return l
	}
	switch t.Kind {
	case flags.ElementType_BOOLEAN, flags.ElementType_I1, flags.ElementType_U1:
		return scalar(1), nil
	case flags.ElementType_I2, flags.ElementType_U2, flags.ElementType_CHAR:
		return scalar(2), nil
	case flags.ElementType_I4, flags.ElementType_U4, flags.ElementType_R4:
		return scalar(4), nil
	case flags.ElementType_I8, flags.ElementType_U8, flags.ElementType_R8:
		return scalar(8), nil
	case flags.ElementType_I, flags.ElementType_U, flags.ElementType_PTR, flags.ElementType_FNPTR:
		return scalar(a.ptrSize()), nil
	case flags.ElementType_ARRAY:
		arr, ok := t.Value.(winmd.SigArray)
		if !ok || arr.Rank != 1 || len(arr.Sizes) != 1 {
			return typeLayout{}, fmt.Errorf("unsupported array %#v", t.Value)
		}
		el, err := m.layoutOf(arr.Type, a, rules)
		if err != nil {
			return typeLayout{}, err
		}
		return typeLayout{el.size * int(arr.Sizes[0]), el.align}, nil
	case flags.ElementType_VALUETYPE, flags.ElementType_CLASS:
		ci, ok := t.Value.(winmd.CodedIndex)
		if !ok {
			return typeLayout{}, fmt.Errorf("unexpected type value %#v", t.Value)
		}
		td, err := m.resolve(ci, a)
		if err != nil {
			return typeLayout{}, err
		}
		switch {
		case td == nil: // System.Guid
			return typeLayout{16, 4}, nil
		case m.isInterface(td), m.isDelegate(td):
			return scalar(a.ptrSize()), nil
		case m.isEnum(td), td.hasAttr("NativeTypedefAttribute"):
			u, err := m.underlying(td)
			if err != nil {
				return typeLayout{}, err
			}
			return m.layoutOf(u, a, rules)
		case m.isStruct(td):
			sl, err := m.structLayout(td, a, rules)
			return sl.typeLayout, err
		}
		return typeLayout{}, fmt.Errorf("type %s has no layout", td.name())
	}
	return typeLayout{}, fmt.Errorf("unsupported element type %v", t.Kind)
}

// structLayout is the layout of a struct with the offsets of its fields.
type structLayout struct {
	typeLayout
	offsets []int
}

// structLayout computes the layout of a metadata struct or union.
func (m *metadata) structLayout(td *typeDef, a arch, rules layoutRules) (structLayout, error) {
	fields, err := m.structFields(td)
	if err != nil {
		return structLayout{}, err
	}
	union := m.isUnion(td)
	if union && rules == goRules {
		return structLayout{}, fmt.Errorf("union %s cannot be expressed in Go", td.name())
	}
	pack := 0
	if rules == cRules {
		pack = int(m.packing[td.index])
	}
	sl := structLayout{typeLayout: typeLayout{0, 1}}
	for _, f := range fields {
		fl, err := m.layoutOf(f.typ, a, rules)
		if err != nil {
			return structLayout{}, fmt.Errorf("%s.%s: %w", td.name(), f.name, err)
		}
		if pack > 0 && fl.align > pack {
			fl.align = pack
		}
		off := 0
		if !union {
			off = alignUp(sl.size, fl.align)
		}
		sl.offsets = append(sl.offsets, off)
		sl.size = max(sl.size, off+fl.size)
		sl.align = max(sl.align, fl.align)
	}
	if len(fields) > 0 && rules == goRules {
		// Go pads a struct whose last field has size zero.
		if last, _ := m.layoutOf(fields[len(fields)-1].typ, a, rules); last.size == 0 {
			sl.size++
		}
	}
	sl.size = alignUp(sl.size, sl.align)
	return sl, nil
}

func alignUp(n, align int) int {
	return (n + align - 1) / align * align
}

// genStruct is a struct ready to be written.
type genStruct struct {
	spec      *structSpec
	namespace string
	fields    []genField
	layouts   map[arch]structLayout
}

type genField struct {
	meta string // metadata field name
	name string // Go field name
	typ  string // Go type
}

// fieldName returns the default Go name of a metadata field: the name
// with its first letter in upper case.
func fieldName(meta string) string {
	r, n := utf8.DecodeRuneInString(meta)
	return string(unicode.ToUpper(r)) + meta[n:]
}

// goDecl returns the Go struct type.
func (s *genStruct) goDecl() string {
	var b strings.Builder
	b.WriteString("struct {\n")
	for _, f := range s.fields {
		fmt.Fprintf(&b, "\t%s %s\n", f.name, f.typ)
	}
	b.WriteString("}")
	return b.String()
}

func (g *generator) structDef(ss *structSpec) (*genStruct, error) {
	var s *genStruct
	for _, an := range archNames {
		td, err := g.m.lookupStruct(ss.metaNames(), an.arch)
		if err != nil {
			return nil, err
		}
		if g.m.isUnion(td) {
			return nil, fmt.Errorf("%s is a union", td.name())
		}
		fields, err := g.m.structFields(td)
		if err != nil {
			return nil, err
		}
		as := &genStruct{spec: ss, namespace: td.namespace(), layouts: make(map[arch]structLayout)}
		tm := &typeMapper{m: g.m, arch: an.arch, pkg: g.pkg, rawBool: true}
		for _, f := range fields {
			gf := genField{meta: f.name, name: fieldName(f.name)}
			if n, ok := ss.names[f.name]; ok {
				gf.name = n
			}
			if t, ok := ss.types[f.name]; ok {
				gf.typ = t
			} else {
				t, err := tm.sigType(f.typ)
				if err != nil {
					return nil, fmt.Errorf("%s: field %s: %w", an.name, f.name, err)
				}
				gf.typ = t.name
			}
			as.fields = append(as.fields, gf)
		}
		cl, err := g.m.structLayout(td, an.arch, cRules)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", an.name, err)
		}
		gl, err := g.m.structLayout(td, an.arch, goRules)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", an.name, err)
		}
		if err := sameLayout(as.fields, cl, gl); err != nil {
			return nil, fmt.Errorf("%s: %w", an.name, err)
		}
		as.layouts[an.arch] = cl
		if s == nil {
			s = as
			continue
		}
		if as.goDecl() != s.goDecl() {
			return nil, fmt.Errorf("Go struct differs between architectures")
		}
		s.layouts[an.arch] = cl
	}
	for meta := range ss.names {
		if !s.hasField(meta) {
			return nil, fmt.Errorf("no field %s", meta)
		}
	}
	for meta := range ss.types {
		if !s.hasField(meta) {
			return nil, fmt.Errorf("no field %s", meta)
		}
	}
	return s, nil
}

func (s *genStruct) hasField(meta string) bool {
	for _, f := range s.fields {
		if f.meta == meta {
			return true
		}
	}
	return false
}

// sameLayout checks that Go lays out a struct like C.
func sameLayout(fields []genField, c, g structLayout) error {
	for i, f := range fields {
		if c.offsets[i] != g.offsets[i] {
			return fmt.Errorf("field %s is at offset %d in C but %d in Go", f.meta, c.offsets[i], g.offsets[i])
		}
	}
	if c.size != g.size || c.align != g.align {
		return fmt.Errorf("size and alignment are %d and %d in C but %d and %d in Go", c.size, c.align, g.size, g.align)
	}
	return nil
}

// structs generates the struct declarations and, for each architecture,
// compile-time checks of their layout against the metadata.
func (g *generator) structs() (decls []byte, layouts map[arch][]byte) {
	var structs []*genStruct
	for _, ss := range g.s.structs {
		s, err := g.structDef(ss)
		if err != nil {
			g.errorf(ss.line, "%s: %v", ss.name, err)
			continue
		}
		structs = append(structs, s)
	}
	sort.Slice(structs, func(i, j int) bool { return structs[i].spec.name < structs[j].spec.name })

	groups := make(map[string][]*genStruct)
	var namespaces []string
	for _, s := range structs {
		if groups[s.namespace] == nil {
			namespaces = append(namespaces, s.namespace)
		}
		groups[s.namespace] = append(groups[s.namespace], s)
	}
	sort.Strings(namespaces)
	var body bytes.Buffer
	for _, ns := range namespaces {
		fmt.Fprintf(&body, "// %s\n\n", ns)
		for _, s := range groups[ns] {
			fmt.Fprintf(&body, "type %s %s\n\n", s.spec.name, s.goDecl())
		}
	}
	var b bytes.Buffer
	var imports []string
	if bytes.Contains(body.Bytes(), []byte("syscall.")) {
		imports = append(imports, "syscall")
	}
	if bytes.Contains(body.Bytes(), []byte("unsafe.")) {
		imports = append(imports, "unsafe")
	}
	header(&b, g.s, imports...)
	b.Write(body.Bytes())

	layouts = make(map[arch][]byte)
	for _, an := range archNames {
		var lb bytes.Buffer
		header(&lb, g.s, "unsafe")
		lb.WriteString("// The functions below do not compile if the layout of a generated\n" +
			"// struct differs from the metadata: an index is out of range or a\n" +
			"// constant overflows uintptr.\n\n")
		lb.WriteString("var layoutCheck [1]struct{}\n\n")
		for _, s := range structs {
			l := s.layouts[an.arch]
			fmt.Fprintf(&lb, "func _() {\n")
			fmt.Fprintf(&lb, "\t_ = layoutCheck[unsafe.Sizeof(%s{})-%d]\n", s.spec.name, l.size)
			fmt.Fprintf(&lb, "\t_ = layoutCheck[unsafe.Alignof(%s{})-%d]\n", s.spec.name, l.align)
			for i, f := range s.fields {
				fmt.Fprintf(&lb, "\t_ = layoutCheck[unsafe.Offsetof(%s{}.%s)-%d]\n", s.spec.name, f.name, l.offsets[i])
			}
			lb.WriteString("}\n\n")
		}
		layouts[an.arch] = lb.Bytes()
	}
	return b.Bytes(), layouts
}
