// Copyright 2026 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// spec is a parsed specification file. See the package documentation for
// the format.
type spec struct {
	metaVersion string // NuGet package version
	metaSHA256  string // SHA-256 of the .nupkg file

	dlls      []*dllSpec
	constants []*constSpec
	functions []*funcSpec
	structs   []*structSpec
}

type dllSpec struct {
	name   string // lower case file name, e.g. "user32.dll"
	goName string // Go variable, e.g. "libuser32"
}

type constSpec struct {
	line   int
	name   string
	goType string // empty for an untyped constant
}

type funcSpec struct {
	line    int
	name    string            // Go function name
	entry   string            // metadata method name, empty means name+"W" or name
	result  string            // Go result type overriding the metadata type
	params  map[string]string // metadata parameter name -> Go type
	rawBool bool              // keep BOOL instead of translating it to bool
}

type structSpec struct {
	line  int
	name  string            // Go type name
	entry string            // metadata struct name, empty means name+"W" or name
	names map[string]string // metadata field name -> Go field name
	types map[string]string // metadata field name -> Go type
}

// metaNames returns the names to look up in the metadata, in order.
func (s *structSpec) metaNames() []string {
	if s.entry != "" {
		return []string{s.entry}
	}
	return []string{s.name + "W", s.name}
}

func parseSpec(path string) (*spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := new(spec)
	seen := make(map[string]int)
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		errorf := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: %s", path, line, fmt.Sprintf(format, args...))
		}
		if len(fields) >= 2 && fields[0] != "metadata" && fields[0] != "dll" {
			if prev, ok := seen[fields[1]]; ok {
				return nil, errorf("%s already declared on line %d", fields[1], prev)
			}
			seen[fields[1]] = line
		}
		switch fields[0] {
		case "metadata":
			if len(fields) != 3 || !strings.HasPrefix(fields[2], "sha256:") {
				return nil, errorf("want: metadata VERSION sha256:HASH")
			}
			s.metaVersion = fields[1]
			s.metaSHA256 = strings.TrimPrefix(fields[2], "sha256:")
		case "dll":
			if len(fields) != 3 {
				return nil, errorf("want: dll FILE GONAME")
			}
			s.dlls = append(s.dlls, &dllSpec{strings.ToLower(fields[1]), fields[2]})
		case "const":
			if len(fields) != 2 && len(fields) != 3 {
				return nil, errorf("want: const NAME [TYPE]")
			}
			c := &constSpec{line: line, name: fields[1]}
			if len(fields) == 3 {
				c.goType = fields[2]
			}
			s.constants = append(s.constants, c)
		case "func":
			if len(fields) < 2 {
				return nil, errorf("want: func NAME [OPTION...]")
			}
			fs := &funcSpec{line: line, name: fields[1], params: make(map[string]string)}
			for _, opt := range fields[2:] {
				switch {
				case opt == "rawbool":
					fs.rawBool = true
				case strings.HasPrefix(opt, "entry="):
					fs.entry = strings.TrimPrefix(opt, "entry=")
				case strings.HasPrefix(opt, "result="):
					fs.result = strings.TrimPrefix(opt, "result=")
				case strings.Contains(opt, ":"):
					i := strings.IndexByte(opt, ':')
					fs.params[opt[:i]] = opt[i+1:]
				default:
					return nil, errorf("unknown option %q", opt)
				}
			}
			s.functions = append(s.functions, fs)
		case "struct":
			if len(fields) < 2 {
				return nil, errorf("want: struct NAME [OPTION...]")
			}
			ss := &structSpec{line: line, name: fields[1], names: make(map[string]string), types: make(map[string]string)}
			for _, opt := range fields[2:] {
				switch {
				case strings.HasPrefix(opt, "entry="):
					ss.entry = strings.TrimPrefix(opt, "entry=")
				case strings.Contains(opt, "="):
					i := strings.IndexByte(opt, '=')
					ss.names[opt[:i]] = opt[i+1:]
				case strings.Contains(opt, ":"):
					i := strings.IndexByte(opt, ':')
					ss.types[opt[:i]] = opt[i+1:]
				default:
					return nil, errorf("unknown option %q", opt)
				}
			}
			s.structs = append(s.structs, ss)
		default:
			return nil, errorf("unknown directive %q", fields[0])
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if s.metaVersion == "" {
		return nil, fmt.Errorf("%s: missing metadata directive", path)
	}
	return s, nil
}
