package core

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
)

type myString string
type myInt int8
type myStringer struct{ s string }

func (m myStringer) String() string { return "S:" + m.s }

type both struct{}

func (both) String() string { return "string" }
func (both) Error() string  { return "error" }

type ptrStringer struct{}

func (*ptrStringer) String() string { return "ptr" }

// FormatValue writes what fmt's %v writes for every kind it handles itself.
func TestFormatValueMatchesFmt(t *testing.T) {
	var nilPtr *ptrStringer
	values := []any{
		nil, "", "héllo", true, false,
		0, -7, math.MaxInt64, math.MinInt64, int8(-3), int16(300), int32(-70000), int64(1) << 62,
		uint(0), uint8(255), uint16(65535), uint32(1 << 31), uint64(math.MaxUint64), uintptr(42),
		0.0, math.Copysign(0, -1), 0.1, 1e6, 1e21, 1e-7, 123456789.125, math.Inf(1), math.Inf(-1), math.NaN(),
		float32(0.1), float32(1e6), float32(3.4e38),
		myString("named"), myInt(-5),
		myStringer{"x"}, &myStringer{"p"}, both{}, errors.New("boom"), nilPtr, &ptrStringer{},
		time.Duration(1500) * time.Millisecond,
	}
	for _, v := range values {
		if got, want := FormatValue(v), fmt.Sprintf("%v", v); got != want {
			t.Errorf("FormatValue(%#v) = %q, fmt gives %q", v, got, want)
		}
	}
}

type nilPanics struct{ s string }

func (n *nilPanics) String() string { return n.s }

func TestFormatValueNilReceiverPanic(t *testing.T) {
	var p *nilPanics
	if got, want := FormatValue(p), fmt.Sprintf("%v", p); got != want {
		t.Errorf("got %q, fmt gives %q", got, want)
	}
}

// The inspector formats every signal, bound or not, so a value it can't
// format without fmt must not log the "call core.UseFmt" warning.
func TestFormatValueFieldsDoesNotWarn(t *testing.T) {
	type area struct{ W, H float64 }
	formatMu.Lock()
	savedFn, savedWarned := formatFn, warned
	formatFn, warned = nil, nil
	formatMu.Unlock()
	var logged []string
	SetLogSink(func(e LogEntry) { logged = append(logged, e.Message) })
	defer func() {
		SetLogSink(nil)
		formatMu.Lock()
		formatFn, warned = savedFn, savedWarned
		formatMu.Unlock()
	}()

	if got := FormatValueFields(area{1, 2}); got != "core.area" {
		t.Errorf("FormatValueFields without fmt = %q, want the type name core.area", got)
	}
	if len(logged) != 0 {
		t.Errorf("FormatValueFields logged %q; the inspector must not warn", logged)
	}
	FormatValue(area{1, 2})
	if len(logged) != 1 {
		t.Errorf("FormatValue logged %d entries, want the one UseFmt warning", len(logged))
	}
}
