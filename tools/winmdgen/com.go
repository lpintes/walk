// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/microsoft/go-winmd"
)

// GUIDs and COM interfaces.

// guidPrefixes maps the prefixes of GUID variable names to the kind of
// metadata type whose GUID they hold and the default Go type.
var guidPrefixes = []struct {
	prefix    string
	iface     bool // interface, otherwise coclass
	defaultGo string
}{
	{"IID_", true, "IID"},
	{"DIID_", true, "IID"},
	{"CLSID_", false, "CLSID"},
}

// typeGUID returns the GUID of a TypeDef from its GuidAttribute.
func typeGUID(td *typeDef) *[16]byte {
	for _, a := range td.attrs {
		if a.name == "GuidAttribute" && len(a.value) >= 18 {
			var g [16]byte
			copy(g[:], a.value[2:18])
			return &g
		}
	}
	return nil
}

// lookupGUID returns the GUID a variable name stands for and the namespace
// it is defined in: a GUID constant of that name, or the GUID of the
// interface (IID_, DIID_) or coclass (CLSID_) named by the rest of the
// name.
func (m *metadata) lookupGUID(name string) (*[16]byte, string, error) {
	if cs := m.constants[name]; len(cs) > 0 {
		c := cs[0]
		for _, c2 := range cs[1:] {
			if c2.guid == nil || c.guid == nil || *c2.guid != *c.guid {
				return nil, "", fmt.Errorf("constant %s has different values in %s and %s", name, c.namespace, c2.namespace)
			}
		}
		if c.guid == nil {
			return nil, "", fmt.Errorf("constant %s is not a GUID", name)
		}
		return c.guid, c.namespace, nil
	}
	for _, p := range guidPrefixes {
		if !strings.HasPrefix(name, p.prefix) {
			continue
		}
		var found *typeDef
		var guid *[16]byte
		for _, td := range m.typeDefs[strings.TrimPrefix(name, p.prefix)] {
			if m.isInterface(td) != p.iface || (!p.iface && !m.isStruct(td)) {
				continue
			}
			g := typeGUID(td)
			if g == nil {
				continue
			}
			if found != nil && *g != *guid {
				return nil, "", fmt.Errorf("%s has different GUIDs in %s and %s", name, found.namespace(), td.namespace())
			}
			found, guid = td, g
		}
		if found != nil {
			return guid, found.namespace(), nil
		}
	}
	return nil, "", fmt.Errorf("GUID not found in the metadata")
}

// goGUID returns a GUID as a Go composite literal of the given type, whose
// underlying type must be syscall.GUID.
func goGUID(typ string, g *[16]byte) string {
	// go vet wants keyed fields in literals of types from other packages.
	format := "%s{0x%08x, 0x%04x, 0x%04x, [8]byte{"
	if strings.Contains(typ, ".") {
		format = "%s{Data1: 0x%08x, Data2: 0x%04x, Data3: 0x%04x, Data4: [8]byte{"
	}
	var b strings.Builder
	fmt.Fprintf(&b, format, typ,
		binary.LittleEndian.Uint32(g[0:]), binary.LittleEndian.Uint16(g[4:]), binary.LittleEndian.Uint16(g[6:]))
	for i, c := range g[8:] {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "0x%02x", c)
	}
	b.WriteString("}}")
	return b.String()
}

// guidType returns the Go type of a GUID variable.
func (gs *guidSpec) guidType() string {
	if gs.goType != "" {
		return gs.goType
	}
	for _, p := range guidPrefixes {
		if strings.HasPrefix(gs.name, p.prefix) {
			return p.defaultGo
		}
	}
	return "syscall.GUID"
}

// guids generates the GUID variables.
func (g *generator) guids() []byte {
	type entry struct {
		name, decl string
	}
	groups := make(map[string][]entry)
	for _, gs := range g.s.guids {
		guid, ns, err := g.m.lookupGUID(gs.name)
		if err != nil {
			g.errorf(gs.line, "%s: %v", gs.name, err)
			continue
		}
		groups[ns] = append(groups[ns], entry{gs.name, gs.name + " = " + goGUID(gs.guidType(), guid)})
	}

	var body bytes.Buffer
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		entries := groups[k]
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		fmt.Fprintf(&body, "// %s\nvar (\n", k)
		for _, e := range entries {
			fmt.Fprintf(&body, "\t%s\n", e.decl)
		}
		body.WriteString(")\n\n")
	}
	var b bytes.Buffer
	var imports []string
	if bytes.Contains(body.Bytes(), []byte("syscall.")) {
		imports = append(imports, "syscall")
	}
	header(&b, g.s, imports...)
	b.Write(body.Bytes())
	return b.Bytes()
}

// lookupInterface finds the metadata interface of a name. Interfaces must
// be the same on all architectures.
func (m *metadata) lookupInterface(name string) (*typeDef, error) {
	var found *typeDef
	for _, td := range m.typeDefs[name] {
		if !m.isInterface(td) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("interface has several definitions in %s and %s", found.namespace(), td.namespace())
		}
		found = td
	}
	if found == nil {
		return nil, fmt.Errorf("interface not found in the metadata")
	}
	if found.arch != archAll {
		return nil, fmt.Errorf("interface is specific to %v", found.arch)
	}
	return found, nil
}

// base returns the interface an interface inherits from, or nil for
// IUnknown.
func (m *metadata) base(td *typeDef) (*typeDef, error) {
	var ci *winmd.CodedIndex
	t := m.md.Tables
	for i := uint32(0); i < t.InterfaceImpl.Len; i++ {
		ii, err := t.InterfaceImpl.Record(winmd.Index(i))
		if err != nil {
			return nil, err
		}
		if ii.Class != td.index {
			continue
		}
		if ci != nil {
			return nil, fmt.Errorf("interface %s has several base interfaces", td.name())
		}
		ci = &ii.Interface
	}
	if ci == nil {
		return nil, nil
	}
	b, err := m.resolve(*ci, archAll)
	if err != nil {
		return nil, err
	}
	if b == nil || !m.isInterface(b) {
		return nil, fmt.Errorf("base of interface %s is not an interface", td.name())
	}
	return b, nil
}

// vtable returns the method names of an interface in vtable order,
// including the methods of its base interfaces.
func (m *metadata) vtable(td *typeDef) ([]string, error) {
	defs, err := m.vtableMethods(td)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name.String()
	}
	return names, nil
}

// vtableMethods returns the methods of an interface in vtable order,
// including the methods of its base interfaces.
func (m *metadata) vtableMethods(td *typeDef) ([]*winmd.MethodDef, error) {
	var chain []*typeDef
	for t := td; t != nil; {
		chain = append(chain, t)
		if len(chain) > 32 {
			return nil, fmt.Errorf("interface %s: inheritance cycle", td.name())
		}
		b, err := m.base(t)
		if err != nil {
			return nil, err
		}
		t = b
	}
	var defs []*winmd.MethodDef
	for i := len(chain) - 1; i >= 0; i-- {
		def := chain[i].def
		for j := def.MethodList.Start; j < def.MethodList.End; j++ {
			md, err := m.md.Tables.MethodDef.Record(j)
			if err != nil {
				return nil, err
			}
			defs = append(defs, md)
		}
	}
	return defs, nil
}

// genInterface is a COM interface ready to be written.
type genInterface struct {
	spec      *interfaceSpec
	namespace string
	fields    []genField // vtable fields
	methods   []*genFunc // Go methods calling through the vtable
}

// vtableDecl returns the Go struct type of the vtable.
func (gi *genInterface) vtableDecl() string {
	var b strings.Builder
	b.WriteString("struct {\n")
	for _, f := range gi.fields {
		fmt.Fprintf(&b, "\t%s uintptr\n", f.name)
	}
	b.WriteString("}")
	return b.String()
}

func (g *generator) interfaceDef(is *interfaceSpec) (*genInterface, error) {
	td, err := g.m.lookupInterface(is.metaName())
	if err != nil {
		return nil, err
	}
	defs, err := g.m.vtableMethods(td)
	if err != nil {
		return nil, err
	}
	gi := &genInterface{spec: is, namespace: td.namespace()}
	seen := make(map[string]bool)
	used := make(map[string]bool)
	for _, def := range defs {
		meta := def.Name.String()
		if seen[meta] {
			return nil, fmt.Errorf("method %s appears twice in the vtable", meta)
		}
		seen[meta] = true
		f := genField{meta: meta, name: fieldName(meta), typ: "uintptr"}
		if n, ok := is.names[meta]; ok {
			f.name = n
		}
		if used[f.name] {
			return nil, fmt.Errorf("vtable field %s appears twice", f.name)
		}
		used[f.name] = true
		gi.fields = append(gi.fields, f)
		if is.methods && !is.skip[meta] {
			gf, err := g.interfaceMethod(is, def, f.name)
			if err != nil {
				return nil, fmt.Errorf("method %s: %w", meta, err)
			}
			gi.methods = append(gi.methods, gf)
		}
	}
	for _, names := range []map[string]bool{keys(is.names), is.skip, keys(is.params)} {
		for meta := range names {
			if !seen[meta] {
				return nil, fmt.Errorf("no method %s", meta)
			}
		}
	}
	return gi, nil
}

func keys[V any](m map[string]V) map[string]bool {
	k := make(map[string]bool, len(m))
	for name := range m {
		k[name] = true
	}
	return k
}

// interfaceMethod translates a method of an interface to a Go method of
// the interface pointer struct. The receiver is passed as the first
// argument; the parameters follow the rules for functions.
func (g *generator) interfaceMethod(is *interfaceSpec, def *winmd.MethodDef, goName string) (*genFunc, error) {
	fs := &funcSpec{line: is.line, name: goName, params: is.params[def.Name.String()]}
	if fs.params == nil {
		fs.params = make(map[string]string)
	}
	md := &method{def: def}
	var f *genFunc
	for _, an := range archNames {
		tm := &typeMapper{m: g.m, arch: an.arch, pkg: g.pkg}
		sig, err := tm.signature(md)
		if err != nil {
			return nil, err
		}
		af, err := g.project(fs, md, sig, an.arch)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", an.name, err)
		}
		if f == nil {
			f = af
			continue
		}
		if af.goSignature() != f.goSignature() || fmt.Sprint(af.byValue) != fmt.Sprint(f.byValue) {
			return nil, fmt.Errorf("signature differs between architectures")
		}
	}
	for name := range fs.params {
		found := false
		for _, p := range f.params {
			found = found || p.name == name
		}
		if !found {
			return nil, fmt.Errorf("no parameter %s", name)
		}
	}
	for i := range f.params {
		f.params[i].name = goParamName(f.params[i].name)
		if f.params[i].name == "obj" {
			f.params[i].name = "obj_"
		}
	}
	return f, nil
}

// interfaces generates the vtable structs of COM interfaces and the
// structs standing for interface pointers.
func (g *generator) interfaces() []byte {
	var ifaces []*genInterface
	for _, is := range g.s.interfaces {
		gi, err := g.interfaceDef(is)
		if err != nil {
			g.errorf(is.line, "%s: %v", is.name, err)
			continue
		}
		ifaces = append(ifaces, gi)
	}
	sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].spec.name < ifaces[j].spec.name })

	groups := make(map[string][]*genInterface)
	var namespaces []string
	for _, gi := range ifaces {
		if groups[gi.namespace] == nil {
			namespaces = append(namespaces, gi.namespace)
		}
		groups[gi.namespace] = append(groups[gi.namespace], gi)
	}
	sort.Strings(namespaces)
	var body bytes.Buffer
	for _, ns := range namespaces {
		fmt.Fprintf(&body, "// %s\n\n", ns)
		for _, gi := range groups[ns] {
			name := gi.spec.name
			fmt.Fprintf(&body, "type %sVtbl %s\n\n", name, gi.vtableDecl())
			fmt.Fprintf(&body, "type %s struct {\n\tLpVtbl *%sVtbl\n}\n\n", name, name)
			for _, f := range gi.methods {
				g.writeMethod(&body, name, f)
			}
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
	return b.Bytes()
}
