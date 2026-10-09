// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/microsoft/go-winmd"
)

type param struct {
	name string
	typ  goType
}

type signature struct {
	params []param
	result goType
}

// signature returns the Go signature of a method for one architecture.
func (tm *typeMapper) signature(md *method) (*signature, error) {
	sig, err := tm.m.md.MethodDefSignature(md.def.Signature)
	if err != nil {
		return nil, err
	}
	if sig.VarArgs {
		return nil, fmt.Errorf("variadic functions are not supported")
	}
	s := &signature{params: make([]param, len(sig.Param))}
	for i := md.def.ParamList.Start; i < md.def.ParamList.End; i++ {
		p, err := tm.m.md.Tables.Param.Record(i)
		if err != nil {
			return nil, err
		}
		if p.Sequence == 0 || int(p.Sequence) > len(sig.Param) {
			continue
		}
		s.params[p.Sequence-1].name = p.Name.String()
	}
	for i, p := range sig.Param {
		if p.Kind != winmd.SigParamKind_ByValue {
			return nil, fmt.Errorf("parameter %d is passed by reference", i)
		}
		t, err := tm.sigType(p.Type)
		if err != nil {
			return nil, fmt.Errorf("parameter %s: %w", s.params[i].name, err)
		}
		s.params[i].typ = t
		if s.params[i].name == "" {
			s.params[i].name = fmt.Sprintf("p%d", i)
		}
	}
	switch sig.RetType.Kind {
	case winmd.SigRetTypeKind_Void:
		s.result = goType{kind: kindVoid}
	case winmd.SigRetTypeKind_ByValue:
		t, err := tm.sigType(sig.RetType.Type)
		if err != nil {
			return nil, fmt.Errorf("result: %w", err)
		}
		s.result = t
	default:
		return nil, fmt.Errorf("unsupported return kind %v", sig.RetType.Kind)
	}
	return s, nil
}
