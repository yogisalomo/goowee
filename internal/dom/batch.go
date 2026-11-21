package dom

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/yogisalomo/goowee/core"
)

// BatchVersion is the version of the binary batch format below. The runtime
// announces the version it reads (goowee.batchVersion in runtime/goowee.js),
// and the bridge sends JSON instead when the two disagree.
const BatchVersion = 1

// The binary batch format, read by decodeBatch in runtime/goowee.js. A batch
// is a header, an op section and a string section:
//
//	header   uint32, little-endian: the byte length of the op section
//	ops      per mutation: its type (1 byte), flags (1 byte) and nodeId
//	         (zigzag varint), then, each only when its flag is set: key
//	         (string), value (by kind), childId and refId (zigzag varints)
//	         and ns (string)
//	strings  the UTF-8 of every string in the batch, in op order
//
// A string in the op section is just its length in UTF-16 code units, as a
// uvarint. The runtime decodes the whole string section with one TextDecoder
// call and slices each string out of the result in turn, so a frame costs one
// decode however many strings it carries. Invalid UTF-8 becomes U+FFFD per
// byte, as encoding/json does, so the lengths always match what the
// TextDecoder produces.
//
// The runtime sees exactly the values JSON.parse gave it for the JSON
// encoding: absent fields are undefined (the JSON left them out), numbers are
// the same doubles, and a value JSON encodes some other way (a named type, a
// slice, a json.Marshaler) is carried as JSON text and parsed there.
const (
	batchKey        = 1 << 0
	batchChild      = 1 << 1
	batchRef        = 1 << 2
	batchNS         = 1 << 3
	batchValueShift = 4 // bits 4–6: the value's kind
)

// Value kinds.
const (
	valueNone   = iota // Value is nil: absent, as omitempty left it out of the JSON
	valueString        // a string
	valueInt           // a zigzag varint, for integers a double holds exactly
	valueFloat         // a float64, 8 bytes little-endian
	valueFalse
	valueTrue
	valueJSON // a string of JSON text, for any other value json.Marshal encodes
)

// maxVarintInt bounds the integers sent as varints. The runtime decodes
// varints with doubles, exact below 2^53, and zigzag doubles the magnitude, so
// a larger integer goes as a float64 instead: exact up to 2^53, and beyond
// that the double JSON.parse would have rounded it to.
const maxVarintInt = 1 << 52

// Buffers past shrinkAbove bytes are released after shrinkAfter batches in a
// row that used under an eighth of them, so one huge frame (a big initial
// mount) doesn't pin its buffer for the life of the page.
const (
	shrinkAbove = 1 << 20
	shrinkAfter = 64
)

// BatchEncoder writes mutation batches in the binary format. It reuses its
// buffers from one batch to the next, so a steady stream of frames encodes
// without allocating. The zero value is ready to use; it is not safe for
// concurrent use.
type BatchEncoder struct {
	buf, strs []byte
	small     int // consecutive batches that used under an eighth of buf
}

// Encode serializes muts. The returned data is valid until the next call. A
// mutation whose value can't be encoded (a NaN or ±Inf float, or a value
// json.Marshal rejects) is left out and returned in dropped, as EncodeBatch
// does.
func (e *BatchEncoder) Encode(muts []core.Mutation) (data []byte, dropped []core.Mutation) {
	if need := 16*len(muts) + 64; cap(e.buf) < need {
		e.buf = make([]byte, 0, need)
	}
	ops := append(e.buf[:0], 0, 0, 0, 0)
	strs := e.strs[:0]
	for _, m := range muts {
		markOps, markStrs := len(ops), len(strs)
		var ok bool
		if ops, strs, ok = appendBatchMutation(ops, strs, m); !ok {
			ops, strs = ops[:markOps], strs[:markStrs]
			dropped = append(dropped, m)
		}
	}
	binary.LittleEndian.PutUint32(ops, uint32(len(ops)-4))
	data = append(ops, strs...)
	e.buf, e.strs = data, strs
	if cap(data) > shrinkAbove && len(data) < cap(data)/8 {
		if e.small++; e.small >= shrinkAfter {
			e.buf, e.strs, e.small = nil, nil, 0
		}
	} else {
		e.small = 0
	}
	return data, dropped
}

// appendBatchMutation appends m to the op and string sections. It reports
// false when m can't be encoded; the caller truncates what was appended.
func appendBatchMutation(ops, strs []byte, m core.Mutation) ([]byte, []byte, bool) {
	if m.Type < 0 || m.Type > math.MaxUint8 {
		return ops, strs, false
	}
	ops = append(ops, byte(m.Type), 0)
	at := len(ops) - 1
	var flags byte
	ops = binary.AppendVarint(ops, int64(m.NodeID))
	if m.Key != "" {
		flags |= batchKey
		ops, strs = appendBatchString(ops, strs, m.Key)
	}
	if m.Kind != core.ValAny {
		var kind byte
		var ok bool
		if ops, strs, kind, ok = appendBatchTyped(ops, strs, &m); !ok {
			return ops, strs, false
		}
		flags |= kind << batchValueShift
	} else if m.Value != nil {
		var kind byte
		var ok bool
		if ops, strs, kind, ok = appendBatchValue(ops, strs, m.Value); !ok {
			return ops, strs, false
		}
		flags |= kind << batchValueShift
	}
	if m.ChildID != 0 {
		flags |= batchChild
		ops = binary.AppendVarint(ops, int64(m.ChildID))
	}
	if m.RefID != 0 {
		flags |= batchRef
		ops = binary.AppendVarint(ops, int64(m.RefID))
	}
	if m.NS != "" {
		flags |= batchNS
		ops, strs = appendBatchString(ops, strs, m.NS)
	}
	ops[at] = flags
	return ops, strs, true
}

// appendBatchTyped encodes a typed value (#136) as appendBatchValue encodes
// the same value boxed, without allocating: a number or bool as text is
// formatted straight into the string section.
func appendBatchTyped(ops, strs []byte, m *core.Mutation) ([]byte, []byte, byte, bool) {
	switch m.Kind {
	case core.ValString:
		ops, strs = appendBatchString(ops, strs, m.Str)
		return ops, strs, valueString, true
	case core.ValFloatText, core.ValIntText, core.ValBoolText:
		// ASCII, so its length in bytes is its length in UTF-16 units.
		n := len(strs)
		strs = m.AppendText(strs)
		return binary.AppendUvarint(ops, uint64(len(strs)-n)), strs, valueString, true
	case core.ValFloat:
		f := m.Float()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return ops, strs, 0, false
		}
		return appendBatchFloat(ops, f), strs, valueFloat, true
	case core.ValInt:
		ops, kind := appendBatchInt(ops, int64(m.Int()))
		return ops, strs, kind, true
	case core.ValBool:
		if m.Bool() {
			return ops, strs, valueTrue, true
		}
		return ops, strs, valueFalse, true
	}
	return ops, strs, 0, false
}

// appendBatchValue encodes v and returns its kind. Numbers become the double
// JSON.parse would have produced from appendValue's output.
func appendBatchValue(ops, strs []byte, v any) ([]byte, []byte, byte, bool) {
	switch v := v.(type) {
	case string:
		ops, strs = appendBatchString(ops, strs, v)
		return ops, strs, valueString, true
	case bool:
		if v {
			return ops, strs, valueTrue, true
		}
		return ops, strs, valueFalse, true
	case int:
		ops, kind := appendBatchInt(ops, int64(v))
		return ops, strs, kind, true
	case int64:
		ops, kind := appendBatchInt(ops, v)
		return ops, strs, kind, true
	case int32:
		ops, kind := appendBatchInt(ops, int64(v))
		return ops, strs, kind, true
	case uint:
		ops, kind := appendBatchUint(ops, uint64(v))
		return ops, strs, kind, true
	case uint64:
		ops, kind := appendBatchUint(ops, v)
		return ops, strs, kind, true
	case float64:
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return ops, strs, 0, false
		}
		return appendBatchFloat(ops, v), strs, valueFloat, true
	case float32:
		f := float64(v)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return ops, strs, 0, false
		}
		// JSON writes a float32 with the shortest digits that identify it as
		// a float32, and JSON.parse reads those digits as a double, which is
		// not float64(v): float32(0.1) arrives as 0.1.
		var digits [32]byte
		f, _ = strconv.ParseFloat(string(strconv.AppendFloat(digits[:0], f, 'g', -1, 32)), 64)
		return appendBatchFloat(ops, f), strs, valueFloat, true
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ops, strs, 0, false
	}
	ops, strs = appendBatchString(ops, strs, string(data))
	return ops, strs, valueJSON, true
}

func appendBatchInt(ops []byte, v int64) ([]byte, byte) {
	if v <= -maxVarintInt || v >= maxVarintInt {
		return appendBatchFloat(ops, float64(v)), valueFloat
	}
	return binary.AppendVarint(ops, v), valueInt
}

func appendBatchUint(ops []byte, v uint64) ([]byte, byte) {
	if v >= maxVarintInt {
		return appendBatchFloat(ops, float64(v)), valueFloat
	}
	return binary.AppendVarint(ops, int64(v)), valueInt
}

func appendBatchFloat(ops []byte, f float64) []byte {
	return binary.LittleEndian.AppendUint64(ops, math.Float64bits(f))
}

// appendBatchString appends s to the string section and its UTF-16 length to
// the op section.
func appendBatchString(ops, strs []byte, s string) ([]byte, []byte) {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return appendBatchStringUTF8(ops, strs, s)
		}
	}
	return binary.AppendUvarint(ops, uint64(len(s))), append(strs, s...)
}

// replacementChar is U+FFFD, which encoding/json writes for each byte of
// invalid UTF-8.
const replacementChar = string(utf8.RuneError)

func appendBatchStringUTF8(ops, strs []byte, s string) ([]byte, []byte) {
	units, run := 0, 0 // run: start of the valid bytes not yet copied
	for i := 0; i < len(s); {
		if s[i] < utf8.RuneSelf {
			i++
			units++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			strs = append(strs, s[run:i]...)
			strs = append(strs, replacementChar...)
			i++
			run = i
			units++
			continue
		}
		if size == 4 { // outside the BMP: a surrogate pair in UTF-16
			units += 2
		} else {
			units++
		}
		i += size
	}
	strs = append(strs, s[run:]...)
	return binary.AppendUvarint(ops, uint64(units)), strs
}
