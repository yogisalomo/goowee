// Package jsonlite reads and writes the JSON the browser runtime and Go
// exchange (event payloads, read replies, log entries), without
// encoding/json, which costs about 220 KB of WASM (#126). It handles plain
// values: objects, arrays, strings, numbers, bools and null, read as
// encoding/json reads them into an any (objects as map[string]any, numbers as
// float64). Values of other Go types are written through core.UseJSON's
// marshaler when there is one (host builds always have encoding/json).
package jsonlite

import (
	"math"
	"reflect"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/jsontext"
)

// Decode parses one JSON value, as json.Unmarshal into an any does.
func Decode(s string) (any, error) { return jsontext.Decode(s) }

// DecodeObject parses a JSON object, as json.Unmarshal into a
// map[string]any does; null gives a nil map.
func DecodeObject(s string) (map[string]any, error) { return jsontext.DecodeObject(s) }

// Append appends v as JSON. It writes nil, bools, strings, Go's integer and
// float types, []any, map[string]any, []string, map[string]string and
// []map[string]any itself, and anything else through core.UseJSON's
// marshaler; without one, it reports false. NaN and infinities report false,
// as encoding/json fails on them.
func Append(b []byte, v any) ([]byte, bool) {
	switch x := v.(type) {
	case nil:
		return append(b, "null"...), true
	case bool:
		return strconv.AppendBool(b, x), true
	case string:
		return AppendString(b, x), true
	case int:
		return strconv.AppendInt(b, int64(x), 10), true
	case int8:
		return strconv.AppendInt(b, int64(x), 10), true
	case int16:
		return strconv.AppendInt(b, int64(x), 10), true
	case int32:
		return strconv.AppendInt(b, int64(x), 10), true
	case int64:
		return strconv.AppendInt(b, x, 10), true
	case uint:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint8:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint16:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint32:
		return strconv.AppendUint(b, uint64(x), 10), true
	case uint64:
		return strconv.AppendUint(b, x, 10), true
	case float64:
		return appendFloat(b, x, 64)
	case float32:
		return appendFloat(b, float64(x), 32)
	case []any:
		if x == nil {
			return append(b, "null"...), true
		}
		b = append(b, '[')
		for i, e := range x {
			if i > 0 {
				b = append(b, ',')
			}
			var ok bool
			if b, ok = Append(b, e); !ok {
				return b, false
			}
		}
		return append(b, ']'), true
	case []string:
		if x == nil {
			return append(b, "null"...), true
		}
		b = append(b, '[')
		for i, e := range x {
			if i > 0 {
				b = append(b, ',')
			}
			b = AppendString(b, e)
		}
		return append(b, ']'), true
	case []map[string]any:
		if x == nil {
			return append(b, "null"...), true
		}
		b = append(b, '[')
		for i, e := range x {
			if i > 0 {
				b = append(b, ',')
			}
			var ok bool
			if b, ok = Append(b, e); !ok {
				return b, false
			}
		}
		return append(b, ']'), true
	case map[string]any:
		if x == nil {
			return append(b, "null"...), true
		}
		return appendMap(b, x, func(b []byte, v any) ([]byte, bool) { return Append(b, v) })
	case map[string]string:
		if x == nil {
			return append(b, "null"...), true
		}
		keys := sortedKeys(x)
		b = append(b, '{')
		for i, k := range keys {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(AppendString(b, k), ':')
			b = AppendString(b, x[k])
		}
		return append(b, '}'), true
	}
	if m, ok := v.(marshaler); ok {
		return appendMarshaler(b, v, m)
	}
	if b2, ok, handled := appendReflect(b, reflect.ValueOf(v)); handled {
		return b2, ok
	}
	return appendWithHook(b, v)
}

// marshaler is json.Marshaler, without importing encoding/json.
type marshaler interface {
	MarshalJSON() ([]byte, error)
}

// textMarshaler is encoding.TextMarshaler: encoding/json writes such a value
// as the string it gives.
type textMarshaler interface {
	MarshalText() ([]byte, error)
}

func appendMarshaler(b []byte, v any, m marshaler) ([]byte, bool) {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return append(b, "null"...), true
	}
	data, err := m.MarshalJSON()
	if err != nil {
		return b, false
	}
	// encoding/json compacts and checks the output: parse it as a check and
	// write it back compacted.
	parsed, err := Decode(string(data))
	if err != nil {
		return b, false
	}
	if _, isObj := parsed.(map[string]any); isObj {
		return appendWithHook(b, v) // keep its key order: only encoding/json does
	}
	return Append(b, parsed)
}

func appendWithHook(b []byte, v any) ([]byte, bool) {
	if marshal := core.JSONMarshaler(); marshal != nil {
		data, err := marshal(v)
		if err != nil {
			return b, false
		}
		return append(b, data...), true
	}
	return b, false
}

// appendReflect writes slices, arrays, string-keyed maps, pointers and named
// basic types as encoding/json does. handled is false for what it leaves to
// the hook: structs, []byte (base64), maps with other keys, interfaces with
// text marshalers, channels and funcs.
func appendReflect(b []byte, rv reflect.Value) (out []byte, ok, handled bool) {
	if rv.IsValid() && rv.CanInterface() {
		if _, isText := rv.Interface().(textMarshaler); isText {
			return b, false, false
		}
	}
	switch rv.Kind() {
	case reflect.Invalid:
		return append(b, "null"...), true, true
	case reflect.String:
		return AppendString(b, rv.String()), true, true
	case reflect.Bool:
		return strconv.AppendBool(b, rv.Bool()), true, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.AppendInt(b, rv.Int(), 10), true, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.AppendUint(b, rv.Uint(), 10), true, true
	case reflect.Float32:
		out, ok = appendFloat(b, rv.Float(), 32)
		return out, ok, true
	case reflect.Float64:
		out, ok = appendFloat(b, rv.Float(), 64)
		return out, ok, true
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return append(b, "null"...), true, true
		}
		out, ok = Append(b, rv.Elem().Interface())
		return out, ok, true
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return b, false, false // base64, as encoding/json writes []byte
		}
		if rv.IsNil() {
			return append(b, "null"...), true, true
		}
		fallthrough
	case reflect.Array:
		b = append(b, '[')
		for i := 0; i < rv.Len(); i++ {
			if i > 0 {
				b = append(b, ',')
			}
			if b, ok = Append(b, rv.Index(i).Interface()); !ok {
				return b, false, true
			}
		}
		return append(b, ']'), true, true
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return b, false, false
		}
		if rv.IsNil() {
			return append(b, "null"...), true, true
		}
		keys := make([]string, 0, rv.Len())
		byKey := make(map[string]reflect.Value, rv.Len())
		for it := rv.MapRange(); it.Next(); {
			k := it.Key().String()
			keys = append(keys, k)
			byKey[k] = it.Value()
		}
		sort.Strings(keys)
		b = append(b, '{')
		for i, k := range keys {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(AppendString(b, k), ':')
			if b, ok = Append(b, byKey[k].Interface()); !ok {
				return b, false, true
			}
		}
		return append(b, '}'), true, true
	}
	return b, false, false
}

func appendMap(b []byte, m map[string]any, elem func([]byte, any) ([]byte, bool)) ([]byte, bool) {
	keys := sortedKeys(m)
	b = append(b, '{')
	for i, k := range keys {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(AppendString(b, k), ':')
		var ok bool
		if b, ok = elem(b, m[k]); !ok {
			return b, false
		}
	}
	return append(b, '}'), true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// appendFloat writes f as encoding/json does (ES6 number formatting).
func appendFloat(b []byte, f float64, bits int) ([]byte, bool) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return b, false
	}
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 {
		if bits == 64 && (abs < 1e-6 || abs >= 1e21) || bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			format = 'e'
		}
	}
	b = strconv.AppendFloat(b, f, format, -1, bits)
	if format == 'e' {
		// clean up e-09 to e-9
		n := len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	return b, true
}

const hexDigits = "0123456789abcdef"

// AppendString writes s as a JSON string, escaped as encoding/json escapes
// it: <, > and & as \u00XX, U+2028 and U+2029, and invalid UTF-8 as U+FFFD.
func AppendString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '\\', '"':
				b = append(b, '\\', c)
			case '\b':
				b = append(b, '\\', 'b')
			case '\f':
				b = append(b, '\\', 'f')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', 'f', 'f', 'f', 'd')
			i += size
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', '2', '0', '2', hexDigits[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}
