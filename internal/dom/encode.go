package dom

import (
	"encoding/json"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/yogisalomo/goowee/core"
)

// EncodeBatch serializes a mutation batch for the JS runtime, as the JSON
// array json.Marshal would produce, byte for byte — but written directly
// instead of through reflection, which made encoding a large frame cost more
// in WASM than parsing it in JS (roadmap 3.4: ~6 ms vs ~1 ms for 5,000
// attribute updates). A mutation whose value can't be encoded — typically a
// float NaN/±Inf, which JSON can't carry — is left out and returned in
// dropped, so a single bad value costs one mutation instead of the whole
// frame's DOM updates. The caller logs the dropped ones.
func EncodeBatch(muts []core.Mutation) (data []byte, dropped []core.Mutation) {
	buf := make([]byte, 0, 56*len(muts)+2) // ~52 bytes for an attribute update
	buf = append(buf, '[')
	first := true
	for _, m := range muts {
		mark := len(buf)
		if !first {
			buf = append(buf, ',')
		}
		var ok bool
		if buf, ok = appendMutation(buf, m); !ok {
			buf = buf[:mark]
			dropped = append(dropped, m)
			continue
		}
		first = false
	}
	return append(buf, ']'), dropped
}

// appendMutation appends m as a JSON object, with core.Mutation's field names
// and omitempty rules. It reports false when m.Value can't be encoded.
func appendMutation(b []byte, m core.Mutation) ([]byte, bool) {
	b = append(b, `{"type":`...)
	b = strconv.AppendInt(b, int64(m.Type), 10)
	b = append(b, `,"nodeId":`...)
	b = strconv.AppendInt(b, int64(m.NodeID), 10)
	if m.Key != "" {
		b = append(b, `,"key":`...)
		b = appendString(b, m.Key)
	}
	if m.Value != nil {
		b = append(b, `,"value":`...)
		var ok bool
		if b, ok = appendValue(b, m.Value); !ok {
			return b, false
		}
	}
	if m.ChildID != 0 {
		b = append(b, `,"childId":`...)
		b = strconv.AppendInt(b, int64(m.ChildID), 10)
	}
	if m.RefID != 0 {
		b = append(b, `,"refId":`...)
		b = strconv.AppendInt(b, int64(m.RefID), 10)
	}
	if m.NS != "" {
		b = append(b, `,"ns":`...)
		b = appendString(b, m.NS)
	}
	return append(b, '}'), true
}

// appendValue encodes the value types mutations carry directly; anything else
// (a named type, a slice or map prop, a json.Marshaler) goes through
// json.Marshal, which is what encoding the whole batch did.
func appendValue(b []byte, v any) ([]byte, bool) {
	switch v := v.(type) {
	case string:
		return appendString(b, v), true
	case bool:
		return strconv.AppendBool(b, v), true
	case int:
		return strconv.AppendInt(b, int64(v), 10), true
	case int64:
		return strconv.AppendInt(b, v, 10), true
	case int32:
		return strconv.AppendInt(b, int64(v), 10), true
	case uint:
		return strconv.AppendUint(b, uint64(v), 10), true
	case uint64:
		return strconv.AppendUint(b, v, 10), true
	case float64:
		return appendFloat(b, v, 64)
	case float32:
		return appendFloat(b, float64(v), 32)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return b, false
	}
	return append(b, data...), true
}

// appendFloat formats f as encoding/json does (ES6 number formatting).
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
		// Clean up e-09 to e-9.
		if n := len(b); n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	return b, true
}

const hexDigits = "0123456789abcdef"

// appendString quotes s as encoding/json does: HTML-significant characters,
// control characters, U+2028 and U+2029 are escaped, and invalid UTF-8 becomes
// U+FFFD.
func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
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
			b = append(b, `\ufffd`...)
			i += size
			start = i
			continue
		}
		if r == '\u2028' || r == '\u2029' {
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

// LogDropped reports mutations EncodeBatch could not serialize.
func LogDropped(dropped []core.Mutation) {
	for _, m := range dropped {
		core.Log(core.LogWarn, "dropped a DOM update whose value can't be encoded (NaN/Inf or non-JSON value)", map[string]any{
			"type":   m.Type.String(),
			"nodeId": m.NodeID,
			"key":    m.Key,
			"value":  m.Value,
		})
	}
}
