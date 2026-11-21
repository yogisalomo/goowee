// Package jsontext reads JSON text as encoding/json reads it into an any,
// without encoding/json (#126), and finds a top-level member of an object
// without decoding the rest or allocating (#135). It has no dependencies,
// so core can use it for event payloads.
package jsontext

import (
	"errors"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

var errSyntax = errors.New("jsonlite: invalid JSON")

// Decode parses one JSON value, as json.Unmarshal into an any does.
func Decode(s string) (any, error) {
	d := decoder{s: s}
	d.space()
	v, err := d.value()
	if err != nil {
		return nil, err
	}
	d.space()
	if d.i != len(d.s) {
		return nil, errSyntax
	}
	return v, nil
}

// DecodeObject parses a JSON object, as json.Unmarshal into a
// map[string]any does; null gives a nil map.
func DecodeObject(s string) (map[string]any, error) {
	v, err := Decode(s)
	if err != nil {
		return nil, err
	}
	switch m := v.(type) {
	case map[string]any:
		return m, nil
	case nil:
		return nil, nil
	}
	return nil, errors.New("jsonlite: not an object")
}

type decoder struct {
	s string
	i int
}

func (d *decoder) space() {
	for d.i < len(d.s) {
		switch d.s[d.i] {
		case ' ', '\t', '\n', '\r':
			d.i++
		default:
			return
		}
	}
}

func (d *decoder) value() (any, error) {
	if d.i >= len(d.s) {
		return nil, errSyntax
	}
	switch c := d.s[d.i]; {
	case c == '{':
		return d.object()
	case c == '[':
		return d.array()
	case c == '"':
		return d.string()
	case c == 't':
		return true, d.literal("true")
	case c == 'f':
		return false, d.literal("false")
	case c == 'n':
		return nil, d.literal("null")
	case c == '-' || (c >= '0' && c <= '9'):
		return d.number()
	}
	return nil, errSyntax
}

func (d *decoder) literal(word string) error {
	if len(d.s)-d.i < len(word) || d.s[d.i:d.i+len(word)] != word {
		return errSyntax
	}
	d.i += len(word)
	return nil
}

func (d *decoder) object() (any, error) {
	d.i++ // {
	m := map[string]any{}
	d.space()
	if d.i < len(d.s) && d.s[d.i] == '}' {
		d.i++
		return m, nil
	}
	for {
		d.space()
		if d.i >= len(d.s) || d.s[d.i] != '"' {
			return nil, errSyntax
		}
		k, err := d.string()
		if err != nil {
			return nil, err
		}
		d.space()
		if d.i >= len(d.s) || d.s[d.i] != ':' {
			return nil, errSyntax
		}
		d.i++
		d.space()
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		m[k] = v
		d.space()
		if d.i >= len(d.s) {
			return nil, errSyntax
		}
		switch d.s[d.i] {
		case ',':
			d.i++
		case '}':
			d.i++
			return m, nil
		default:
			return nil, errSyntax
		}
	}
}

func (d *decoder) array() (any, error) {
	d.i++ // [
	a := []any{}
	d.space()
	if d.i < len(d.s) && d.s[d.i] == ']' {
		d.i++
		return a, nil
	}
	for {
		d.space()
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		a = append(a, v)
		d.space()
		if d.i >= len(d.s) {
			return nil, errSyntax
		}
		switch d.s[d.i] {
		case ',':
			d.i++
		case ']':
			d.i++
			return a, nil
		default:
			return nil, errSyntax
		}
	}
}

func (d *decoder) number() (any, error) {
	start := d.i
	if d.s[d.i] == '-' {
		d.i++
	}
	if d.i >= len(d.s) {
		return nil, errSyntax
	}
	if d.s[d.i] == '0' {
		d.i++
	} else if d.s[d.i] >= '1' && d.s[d.i] <= '9' {
		for d.i < len(d.s) && d.s[d.i] >= '0' && d.s[d.i] <= '9' {
			d.i++
		}
	} else {
		return nil, errSyntax
	}
	if d.i < len(d.s) && d.s[d.i] == '.' {
		d.i++
		if !d.digits() {
			return nil, errSyntax
		}
	}
	if d.i < len(d.s) && (d.s[d.i] == 'e' || d.s[d.i] == 'E') {
		d.i++
		if d.i < len(d.s) && (d.s[d.i] == '+' || d.s[d.i] == '-') {
			d.i++
		}
		if !d.digits() {
			return nil, errSyntax
		}
	}
	f, err := strconv.ParseFloat(d.s[start:d.i], 64)
	if err != nil {
		return nil, err // out of range, as encoding/json reports it
	}
	return f, nil
}

func (d *decoder) digits() bool {
	start := d.i
	for d.i < len(d.s) && d.s[d.i] >= '0' && d.s[d.i] <= '9' {
		d.i++
	}
	return d.i > start
}

// string reads a JSON string as encoding/json does: invalid UTF-8 and lone
// surrogates become U+FFFD.
func (d *decoder) string() (string, error) {
	d.i++ // "
	start := d.i
	// The common case: no escapes and valid UTF-8, a substring.
	for d.i < len(d.s) {
		c := d.s[d.i]
		if c == '"' {
			s := d.s[start:d.i]
			d.i++
			return s, nil
		}
		if c == '\\' || c < 0x20 || c >= utf8.RuneSelf {
			break
		}
		d.i++
	}
	b := []byte(d.s[start:d.i])
	for d.i < len(d.s) {
		c := d.s[d.i]
		switch {
		case c == '"':
			d.i++
			return string(b), nil
		case c < 0x20:
			return "", errSyntax
		case c == '\\':
			d.i++
			if d.i >= len(d.s) {
				return "", errSyntax
			}
			e := d.s[d.i]
			d.i++
			switch e {
			case '"', '\\', '/':
				b = append(b, e)
			case 'b':
				b = append(b, '\b')
			case 'f':
				b = append(b, '\f')
			case 'n':
				b = append(b, '\n')
			case 'r':
				b = append(b, '\r')
			case 't':
				b = append(b, '\t')
			case 'u':
				r, ok := d.hex4()
				if !ok {
					return "", errSyntax
				}
				if utf16.IsSurrogate(r) {
					r2 := rune(-1)
					if d.i+1 < len(d.s) && d.s[d.i] == '\\' && d.s[d.i+1] == 'u' {
						save := d.i
						d.i += 2
						if x, ok := d.hex4(); ok {
							r2 = x
						} else {
							return "", errSyntax
						}
						if dec := utf16.DecodeRune(r, r2); dec != utf8.RuneError {
							b = utf8.AppendRune(b, dec)
							continue
						}
						d.i = save // not a pair: the second escape stands alone
					}
					r = utf8.RuneError
				}
				b = utf8.AppendRune(b, r)
			default:
				return "", errSyntax
			}
		case c < utf8.RuneSelf:
			b = append(b, c)
			d.i++
		default:
			r, size := utf8.DecodeRuneInString(d.s[d.i:])
			if r == utf8.RuneError && size == 1 {
				b = utf8.AppendRune(b, utf8.RuneError)
			} else {
				b = append(b, d.s[d.i:d.i+size]...)
			}
			d.i += size
		}
	}
	return "", errSyntax
}

func (d *decoder) hex4() (rune, bool) {
	if len(d.s)-d.i < 4 {
		return 0, false
	}
	var r rune
	for _, c := range []byte(d.s[d.i : d.i+4]) {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, false
		}
	}
	d.i += 4
	return r, true
}

// Field returns the JSON text of member key of the object obj, as a
// substring of obj (no allocation), and whether there is one. Only the
// members before it are scanned, and only far enough to skip them. With a
// repeated key the last one wins, as in Decode.
func Field(obj, key string) (string, bool) {
	d := decoder{s: obj}
	d.space()
	if d.i >= len(d.s) || d.s[d.i] != '{' {
		return "", false
	}
	d.i++
	var found string
	ok := false
	for {
		d.space()
		if d.i >= len(d.s) || d.s[d.i] != '"' {
			return found, ok
		}
		kStart := d.i
		if !d.skipString() {
			return found, ok
		}
		k := d.s[kStart+1 : d.i-1]
		d.space()
		if d.i >= len(d.s) || d.s[d.i] != ':' {
			return found, ok
		}
		d.i++
		d.space()
		vStart := d.i
		if !d.skip() {
			return found, ok
		}
		if k == key || (needsDecoding(k) && unquoteKey(d.s[kStart:kStart+len(k)+2]) == key) {
			found, ok = d.s[vStart:d.i], true
		}
		d.space()
		if d.i >= len(d.s) || d.s[d.i] != ',' {
			return found, ok
		}
		d.i++
	}
}

func unquoteKey(quoted string) string {
	d := decoder{s: quoted}
	s, _ := d.string()
	return s
}

// needsDecoding reports a key whose decoded form may differ from its text:
// an escape, or bytes that may be invalid UTF-8.
func needsDecoding(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' || s[i] >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

// skip moves past one JSON value without decoding it.
func (d *decoder) skip() bool {
	if d.i >= len(d.s) {
		return false
	}
	switch c := d.s[d.i]; c {
	case '"':
		return d.skipString()
	case '{', '[':
		depth := 0
		for d.i < len(d.s) {
			switch d.s[d.i] {
			case '"':
				if !d.skipString() {
					return false
				}
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					d.i++
					return true
				}
			}
			d.i++
		}
		return false
	default:
		start := d.i
		for d.i < len(d.s) {
			switch d.s[d.i] {
			case ',', '}', ']', ' ', '\t', '\n', '\r':
				return d.i > start
			}
			d.i++
		}
		return d.i > start
	}
}

func (d *decoder) skipString() bool {
	d.i++ // "
	for d.i < len(d.s) {
		switch d.s[d.i] {
		case '\\':
			d.i += 2
			continue
		case '"':
			d.i++
			return true
		}
		d.i++
	}
	return false
}

// String reads a JSON string value's text (from Field): the substring
// between the quotes when it has no escapes (no allocation), else the
// decoded string. ok is false for anything but a string.
func String(v string) (s string, ok bool) {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return "", false
	}
	inner := v[1 : len(v)-1]
	plain := true
	for i := 0; i < len(inner); i++ {
		if c := inner[i]; c == '\\' || c < 0x20 {
			plain = false
			break
		}
	}
	if plain && utf8.ValidString(inner) {
		return inner, true
	}
	d := decoder{s: v}
	s, err := d.string()
	return s, err == nil
}

// Number reads a JSON number value's text (from Field).
func Number(v string) (float64, bool) {
	if v == "" || !(v[0] == '-' || v[0] >= '0' && v[0] <= '9') {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}
