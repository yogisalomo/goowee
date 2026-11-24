package main

import (
	"encoding/binary"
	"errors"
	"regexp"
	"sort"
	"strings"
)

// wasmPackages reads a module's name section and sums each Go package's
// code bytes, as scripts/wasm-size.mjs does, largest first.
func wasmPackages(buf []byte) ([]pkgSize, error) {
	if len(buf) < 8 || binary.LittleEndian.Uint32(buf) != 0x6d736100 {
		return nil, errors.New("not a wasm file")
	}
	r := &wreader{b: buf, pos: 8}
	var bodies []int
	names := map[int]string{}
	imported := 0
	for r.pos < len(buf) && r.err == nil {
		id := r.u8()
		size := r.u32()
		end := r.pos + size
		switch id {
		case 0:
			if r.name() == "name" {
				for r.pos < end && r.err == nil {
					sub, subSize := r.u8(), r.u32()
					subEnd := r.pos + subSize
					if sub == 1 {
						for n := r.u32(); n > 0 && r.err == nil; n-- {
							idx := r.u32()
							names[idx] = r.name()
						}
					}
					r.pos = subEnd
				}
			}
		case 2:
			for n := r.u32(); n > 0 && r.err == nil; n-- {
				r.name()
				r.name()
				switch r.u8() {
				case 0:
					r.u32()
					imported++
				case 1:
					r.u8()
					limits(r)
				case 2:
					limits(r)
				case 3:
					r.u8()
					r.u8()
				}
			}
		case 10:
			for n := r.u32(); n > 0 && r.err == nil; n-- {
				s := r.u32()
				bodies = append(bodies, s)
				r.pos += s
			}
		}
		r.pos = end
	}
	if r.err != nil {
		return nil, r.err
	}
	sizes := map[string]int{}
	for i, s := range bodies {
		sizes[pkgOf(names[imported+i])] += s
	}
	out := make([]pkgSize, 0, len(sizes))
	for p, s := range sizes {
		out = append(out, pkgSize{p, s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].bytes > out[j].bytes })
	return out, nil
}

// limits skips a table's or memory's limits: a flag, a minimum and, with
// flag bit 0, a maximum.
func limits(r *wreader) {
	f := r.u8()
	r.u32()
	if f&1 != 0 {
		r.u32()
	}
}

type pkgSize struct {
	pkg   string
	bytes int
}

type wreader struct {
	b   []byte
	pos int
	err error
}

func (r *wreader) u8() byte {
	if r.pos >= len(r.b) {
		r.err = errors.New("truncated wasm")
		return 0
	}
	c := r.b[r.pos]
	r.pos++
	return c
}

func (r *wreader) u32() int {
	var v, shift uint
	for {
		c := r.u8()
		if r.err != nil {
			return 0
		}
		v |= uint(c&0x7f) << shift
		if c < 0x80 {
			return int(v)
		}
		shift += 7
	}
}

func (r *wreader) name() string {
	n := r.u32()
	if r.err != nil || r.pos+n > len(r.b) {
		r.err = errors.New("truncated wasm")
		return ""
	}
	s := string(r.b[r.pos : r.pos+n])
	r.pos += n
	return s
}

var domain = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)

// pkgOf is the Go package a function belongs to (see scripts/wasm-size.mjs):
// TinyGo keeps "/", the Go linker writes "_" for it.
func pkgOf(fn string) string {
	fn = strings.TrimLeft(fn, "(*")
	base := fn
	if i := strings.IndexByte(fn, '['); i >= 0 {
		base = fn[:i]
	}
	if strings.Contains(base, "/") {
		slash := strings.LastIndexByte(base, '/')
		if dot := strings.IndexByte(base[slash+1:], '.'); dot >= 0 {
			return base[:slash+1+dot]
		}
		return base
	}
	if strings.HasPrefix(base, "type_.") {
		return "type:"
	}
	under, firstDot := strings.IndexByte(base, '_'), strings.IndexByte(base, '.')
	if firstDot < 0 {
		return "(asm)"
	}
	dot := firstDot
	if under > 0 && firstDot < under && domain.MatchString(base[:under]) {
		d := strings.IndexByte(base[under:], '.')
		if d < 0 {
			return strings.ReplaceAll(base, "_", "/")
		}
		dot = under + d
	}
	return strings.ReplaceAll(base[:dot], "_", "/")
}
