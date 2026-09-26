package runtime

import (
	"fmt"
	"reflect"
)

// SafeEqual reports whether a and b are equal under ==, without ever
// panicking. Values that can't be compared with == (slices, maps, funcs, or
// structs/interfaces holding one) are reported as not equal, so callers treat
// them as changed. Go can recover the runtime panic an uncomparable == raises;
// TinyGo cannot, so it must never be triggered (see ADR-011).
func SafeEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if !Comparable(a) || !Comparable(b) {
		return false
	}
	return a == b
}

// Comparable reports whether v can be used with == (and as a map key) without
// panicking. It inspects the dynamic value, so a struct whose interface field
// holds a slice is correctly reported as uncomparable.
func Comparable(v any) bool {
	if v == nil {
		return true
	}
	return reflect.ValueOf(v).Comparable()
}

// TextValue formats a value for a text node exactly as the SSR renderer does
// (fmt's %v), so client and server text agree — Go's float formatting differs
// from JavaScript's (1e+08 vs 100000000) — and a NaN never reaches the JSON
// batch as a float.
func TextValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
