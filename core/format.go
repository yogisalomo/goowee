package core

import (
	"reflect"
	"strconv"
	"sync"
)

// FormatValue formats v as fmt's %v does, without fmt, for what bound text
// and attributes carry: strings, bools, integers and floats (named types
// too), a fmt.Stringer or error, and nil (#126). Anything else (a slice, a
// map, a struct) goes through fmt when it is linked: host builds always have
// it, and in the browser h.Textf or UseFmt installs it. Without it, such a
// value is written as its type and a warning is logged once per type.
func FormatValue(v any) string { return formatValue(v, "%v") }

// FormatValueFields is FormatValue with a struct's field names, as %+v
// writes it (for the inspector), when fmt is linked.
func FormatValueFields(v any) string { return formatValue(v, "%+v") }

func formatValue(v any, verb string) string {
	switch x := v.(type) {
	case nil:
		return "<nil>"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case error: // fmt prefers Error to String
		return callFormat(v, x.Error)
	case stringer:
		return callFormat(v, x.String)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return rv.String()
	case reflect.Bool:
		return strconv.FormatBool(rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.Float32:
		return strconv.FormatFloat(rv.Float(), 'g', -1, 32)
	case reflect.Float64:
		return strconv.FormatFloat(rv.Float(), 'g', -1, 64)
	}
	if sprintf := formatFallback(); sprintf != nil {
		return sprintf(verb, v)
	}
	name := rv.Type().String()
	warnFormatOnce(name)
	return name
}

// stringer is fmt.Stringer, without importing fmt.
type stringer interface{ String() string }

var (
	formatMu sync.Mutex
	formatFn func(format string, a ...any) string
	warned   map[string]bool
)

// UseFmt makes FormatValue format composite values (slices, maps, structs)
// with fmt.Sprintf, as %v does. Host builds always do; in the browser, call
// it from main (core.UseFmt(fmt.Sprintf)) if bound text or attributes carry
// such values, or format them into a string yourself. h.Textf calls it.
// Passing fmt.Sprintf links fmt into the app, about 110 KB brotli of WASM,
// which is why it isn't the default.
func UseFmt(sprintf func(format string, a ...any) string) {
	formatMu.Lock()
	formatFn = sprintf
	formatMu.Unlock()
}

func formatFallback() func(format string, a ...any) string {
	formatMu.Lock()
	defer formatMu.Unlock()
	return formatFn
}

func warnFormatOnce(typ string) {
	formatMu.Lock()
	if warned == nil {
		warned = map[string]bool{}
	}
	seen := warned[typ]
	warned[typ] = true
	formatMu.Unlock()
	if !seen {
		Log(LogWarn, "a bound value of this type needs fmt to be formatted; call core.UseFmt(fmt.Sprintf) from main, or bind a string", map[string]any{"type": typ})
	}
}

// callFormat calls a String or Error method as fmt does: if it panics on a
// nil pointer, the text is <nil>; any other panic propagates.
func callFormat(v any, method func() string) (s string) {
	defer func() {
		if rec := recover(); rec != nil {
			if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
				s = "<nil>"
				return
			}
			panic(rec)
		}
	}()
	return method()
}

var (
	jsonMu      sync.Mutex
	jsonMarshal func(any) ([]byte, error)
)

// UseJSON gives goowee a JSON marshaler (json.Marshal) for values it can't
// write itself: a struct or a typed slice as a property value or a ref.Call
// argument, a log field. Strings, numbers, bools, []any and map[string]any
// need none. Host builds always have encoding/json; in the browser, without
// it, such a value is dropped with a warning. Passing json.Marshal links
// encoding/json into the app, about 220 KB brotli of WASM (#126).
func UseJSON(marshal func(any) ([]byte, error)) {
	jsonMu.Lock()
	jsonMarshal = marshal
	jsonMu.Unlock()
}

// JSONMarshaler is the marshaler UseJSON set, or nil.
func JSONMarshaler() func(any) ([]byte, error) {
	jsonMu.Lock()
	defer jsonMu.Unlock()
	return jsonMarshal
}
