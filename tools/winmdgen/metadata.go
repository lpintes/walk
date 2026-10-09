// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/microsoft/go-winmd"
	"github.com/microsoft/go-winmd/coded"
	"github.com/microsoft/go-winmd/flags"
)

// arch is a bit mask of target architectures, using the values of the
// Windows.Win32.Foundation.Metadata.Architecture enum.
type arch uint32

const (
	arch386   arch = 1
	archAMD64 arch = 2
	archARM64 arch = 4
	archAll        = arch386 | archAMD64 | archARM64
)

var archNames = []struct {
	arch arch
	name string
}{
	{arch386, "386"},
	{archAMD64, "amd64"},
	{archARM64, "arm64"},
}

func (a arch) String() string {
	if a == archAll {
		return "all"
	}
	var names []string
	for _, an := range archNames {
		if a&an.arch != 0 {
			names = append(names, an.name)
		}
	}
	return strings.Join(names, ",")
}

// ptrSize returns the pointer size of a single architecture.
func (a arch) ptrSize() int {
	if a == arch386 {
		return 4
	}
	return 8
}

// attribute is a decoded custom attribute.
type attribute struct {
	name  string // type name without namespace, e.g. "GuidAttribute"
	value []byte // raw blob, see §II.23.3
}

// typeDef is a TypeDef together with the information walk needs.
type typeDef struct {
	index winmd.Index
	def   *winmd.TypeDef
	arch  arch
	attrs []attribute
}

func (t *typeDef) name() string      { return t.def.Name.String() }
func (t *typeDef) namespace() string { return t.def.Namespace.String() }

func (t *typeDef) hasAttr(name string) bool {
	for _, a := range t.attrs {
		if a.name == name {
			return true
		}
	}
	return false
}

// method is a function exported by a DLL, a MethodDef of an Apis class.
type method struct {
	index     winmd.Index
	def       *winmd.MethodDef
	namespace string
	arch      arch
	dll       string // lower case, e.g. "user32.dll"
	entry     string // DLL entry point name
	lastError bool   // SetLastError=true
}

// constant is a field of an Apis class or an enum member with a constant
// value.
type constant struct {
	name      string
	namespace string
	enum      string // name of the enum type, empty for Apis constants
	arch      arch

	// Exactly one of the following groups is set.
	kind flags.ElementType // type of an integer, float or string value
	ival int64             // signed integer value (kind I1..I8)
	uval uint64            // unsigned integer value (kind U1..U8, CHAR, BOOLEAN)
	fval float64           // kind R4, R8
	sval string            // kind STRING
	guid *[16]byte         // GUID constant (GuidAttribute)
}

// metadata indexes a Windows.Win32.winmd file.
type metadata struct {
	md *winmd.Metadata

	typeDefs  map[string][]*typeDef // by name
	methods   map[string][]*method  // by name
	constants map[string][]*constant

	attrs      map[winmd.CodedIndex][]attribute // by HasCustomAttribute parent
	fieldConst map[winmd.Index]*winmd.Constant
	implMaps   map[winmd.Index]*winmd.ImplMap
	nested     map[winmd.Index]bool   // nested TypeDefs
	packing    map[winmd.Index]uint16 // #pragma pack of TypeDefs, 0 for the default
}

func loadMetadata(data []byte) (*metadata, error) {
	f, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	md, err := winmd.New(f)
	if err != nil {
		return nil, err
	}
	m := &metadata{
		md:         md,
		typeDefs:   make(map[string][]*typeDef),
		methods:    make(map[string][]*method),
		constants:  make(map[string][]*constant),
		attrs:      make(map[winmd.CodedIndex][]attribute),
		fieldConst: make(map[winmd.Index]*winmd.Constant),
		implMaps:   make(map[winmd.Index]*winmd.ImplMap),
		nested:     make(map[winmd.Index]bool),
		packing:    make(map[winmd.Index]uint16),
	}
	if err := m.index(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *metadata) index() error {
	t := m.md.Tables

	for i := uint32(0); i < t.CustomAttribute.Len; i++ {
		ca, err := t.CustomAttribute.Record(winmd.Index(i))
		if err != nil {
			return err
		}
		name, err := m.attributeName(ca)
		if err != nil {
			return err
		}
		m.attrs[ca.Parent] = append(m.attrs[ca.Parent], attribute{name, ca.Value})
	}
	for i := uint32(0); i < t.Constant.Len; i++ {
		c, err := t.Constant.Record(winmd.Index(i))
		if err != nil {
			return err
		}
		if c.Parent.Tag == coded.HasConstant_Field {
			m.fieldConst[c.Parent.Index] = c
		}
	}
	for i := uint32(0); i < t.ImplMap.Len; i++ {
		im, err := t.ImplMap.Record(winmd.Index(i))
		if err != nil {
			return err
		}
		if im.MemberForwarded.Tag == coded.MemberForwarded_MethodDef {
			m.implMaps[im.MemberForwarded.Index] = im
		}
	}
	for i := uint32(0); i < t.ClassLayout.Len; i++ {
		cl, err := t.ClassLayout.Record(winmd.Index(i))
		if err != nil {
			return err
		}
		m.packing[cl.Parent] = cl.PackingSize
	}
	for i := uint32(0); i < t.NestedClass.Len; i++ {
		nc, err := t.NestedClass.Record(winmd.Index(i))
		if err != nil {
			return err
		}
		m.nested[nc.NestedClass] = true
	}

	for i := uint32(0); i < t.TypeDef.Len; i++ {
		def, err := t.TypeDef.Record(winmd.Index(i))
		if err != nil {
			return err
		}
		td := &typeDef{
			index: winmd.Index(i),
			def:   def,
			attrs: m.attrs[winmd.CodedIndex{Index: winmd.Index(i), Tag: coded.HasCustomAttribute_TypeDef}],
		}
		td.arch = archFromAttrs(td.attrs)
		ns := td.namespace()
		if !strings.HasPrefix(ns, "Windows.Win32.") {
			continue
		}
		if !m.nested[td.index] {
			m.typeDefs[td.name()] = append(m.typeDefs[td.name()], td)
		}
		switch {
		case td.name() == "Apis":
			if err := m.indexApis(td); err != nil {
				return err
			}
		case m.isEnum(td):
			if err := m.indexEnum(td); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *metadata) attributeName(ca *winmd.CustomAttribute) (string, error) {
	if ca.Type.Tag != coded.CustomAttributeType_MemberRef {
		return "", nil
	}
	mr, err := m.md.Tables.MemberRef.Record(ca.Type.Index)
	if err != nil {
		return "", err
	}
	if mr.Class.Tag != coded.MemberRefParent_TypeRef {
		return "", nil
	}
	tr, err := m.md.Tables.TypeRef.Record(mr.Class.Index)
	if err != nil {
		return "", err
	}
	return tr.Name.String(), nil
}

func archFromAttrs(attrs []attribute) arch {
	for _, a := range attrs {
		if a.name == "SupportedArchitectureAttribute" && len(a.value) >= 6 {
			return arch(binary.LittleEndian.Uint32(a.value[2:]))
		}
	}
	return archAll
}

func (m *metadata) isEnum(td *typeDef) bool {
	if td.def.Extends.Tag != coded.TypeDefOrRef_TypeRef {
		return false
	}
	tr, err := m.md.Tables.TypeRef.Record(td.def.Extends.Index)
	if err != nil {
		return false
	}
	return tr.Namespace.String() == "System" && tr.Name.String() == "Enum"
}

func (m *metadata) indexApis(td *typeDef) error {
	t := m.md.Tables
	for i := td.def.MethodList.Start; i < td.def.MethodList.End; i++ {
		def, err := t.MethodDef.Record(i)
		if err != nil {
			return err
		}
		attrs := m.attrs[winmd.CodedIndex{Index: i, Tag: coded.HasCustomAttribute_MethodDef}]
		md := &method{
			index:     i,
			def:       def,
			namespace: td.namespace(),
			arch:      archFromAttrs(attrs),
		}
		if im, ok := m.implMaps[i]; ok {
			mr, err := t.ModuleRef.Record(im.ImportScope)
			if err != nil {
				return err
			}
			md.dll = strings.ToLower(mr.Name.String())
			md.entry = im.ImportName.String()
			md.lastError = im.MappingFlags&flags.PInvokeAttributes_SupportsLastError != 0
		}
		name := def.Name.String()
		m.methods[name] = append(m.methods[name], md)
	}
	for i := td.def.FieldList.Start; i < td.def.FieldList.End; i++ {
		c, err := m.decodeConstant(i, td, "")
		if err != nil {
			return err
		}
		if c != nil {
			m.constants[c.name] = append(m.constants[c.name], c)
		}
	}
	return nil
}

func (m *metadata) indexEnum(td *typeDef) error {
	for i := td.def.FieldList.Start; i < td.def.FieldList.End; i++ {
		c, err := m.decodeConstant(i, td, td.name())
		if err != nil {
			return err
		}
		if c != nil {
			m.constants[c.name] = append(m.constants[c.name], c)
		}
	}
	return nil
}

func (m *metadata) decodeConstant(field winmd.Index, owner *typeDef, enum string) (*constant, error) {
	fd, err := m.md.Tables.Field.Record(field)
	if err != nil {
		return nil, err
	}
	if fd.Name.String() == "value__" {
		return nil, nil
	}
	attrs := m.attrs[winmd.CodedIndex{Index: field, Tag: coded.HasCustomAttribute_Field}]
	c := &constant{
		name:      fd.Name.String(),
		namespace: owner.namespace(),
		enum:      enum,
		arch:      archFromAttrs(attrs) & owner.arch,
	}
	for _, a := range attrs {
		if a.name == "GuidAttribute" && len(a.value) >= 18 {
			var g [16]byte
			copy(g[:], a.value[2:18])
			c.guid = &g
			return c, nil
		}
	}
	k, ok := m.fieldConst[field]
	if !ok {
		// Constants without a value, e.g. PROPERTYKEY constants described
		// by an attribute. walk does not need them yet.
		return nil, nil
	}
	c.kind = k.Type
	v := k.Value
	switch k.Type {
	case flags.ElementType_I1:
		c.ival = int64(int8(v[0]))
	case flags.ElementType_I2:
		c.ival = int64(int16(binary.LittleEndian.Uint16(v)))
	case flags.ElementType_I4:
		c.ival = int64(int32(binary.LittleEndian.Uint32(v)))
	case flags.ElementType_I8:
		c.ival = int64(binary.LittleEndian.Uint64(v))
	case flags.ElementType_U1, flags.ElementType_BOOLEAN:
		c.uval = uint64(v[0])
	case flags.ElementType_U2, flags.ElementType_CHAR:
		c.uval = uint64(binary.LittleEndian.Uint16(v))
	case flags.ElementType_U4:
		c.uval = uint64(binary.LittleEndian.Uint32(v))
	case flags.ElementType_U8:
		c.uval = binary.LittleEndian.Uint64(v)
	case flags.ElementType_R4:
		c.fval = float64(math.Float32frombits(binary.LittleEndian.Uint32(v)))
	case flags.ElementType_R8:
		c.fval = math.Float64frombits(binary.LittleEndian.Uint64(v))
	case flags.ElementType_STRING:
		u := make([]uint16, len(v)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(v[2*i:])
		}
		c.sval = string(utf16.Decode(u))
	case flags.ElementType_CLASS:
		// Null reference.
		return nil, nil
	default:
		return nil, fmt.Errorf("constant %s has unsupported type %v", c.name, k.Type)
	}
	return c, nil
}

// integer returns the value of an integer constant as a signed or unsigned
// value and reports whether the constant is an integer.
func (c *constant) integer() (i int64, u uint64, signed, ok bool) {
	switch c.kind {
	case flags.ElementType_I1, flags.ElementType_I2, flags.ElementType_I4, flags.ElementType_I8:
		return c.ival, 0, true, true
	case flags.ElementType_U1, flags.ElementType_U2, flags.ElementType_U4, flags.ElementType_U8,
		flags.ElementType_CHAR, flags.ElementType_BOOLEAN:
		return 0, c.uval, false, true
	}
	return 0, 0, false, false
}
