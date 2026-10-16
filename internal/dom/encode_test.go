package dom

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"testing"

	"github.com/yogisalomo/goowee/core"
)

type celsius float64

// encodeSamples are values that exercise every branch of the encoder and the
// corners of encoding/json's output: HTML and control characters, invalid
// UTF-8, line separators, float formats (exponent switch, -0, float32), integer
// extremes, and the json.Marshal fallback (named types, slices, maps, raw
// messages).
var encodeSamples = []any{
	nil, "", "plain", `quote " and \ backslash`, "<script>&amp;</script>",
	"tab\tnew\nline\rcr\bbs\fff\x00nul\x1funit", "é ü 漢字 🎉", "\u2028\u2029",
	"bad \xff utf8 \xc3", "\x7f del",
	true, false,
	0, 1, -1, math.MaxInt64, math.MinInt64, int64(42), int32(-7), uint(7), uint64(math.MaxUint64),
	0.0, math.Copysign(0, -1), 1.5, -12.375, 0.1, 1e-6, 9.99e-7, 1e-7, 1e20, 1e21, 123456789.125,
	math.SmallestNonzeroFloat64, math.MaxFloat64, float32(0.1), float32(1e-7), float32(3.4e38),
	celsius(21.5), []string{"a", "<b>"}, map[string]int{"z": 1, "a": 2}, json.RawMessage(`{"raw":true}`),
}

func randomMutation(rng *rand.Rand) core.Mutation {
	str := func() string {
		if rng.Intn(3) == 0 {
			return ""
		}
		if s, ok := encodeSamples[rng.Intn(10)].(string); ok {
			return s
		}
		return "k"
	}
	return core.Mutation{
		Type:    core.MutationType(rng.Intn(11)),
		NodeID:  rng.Intn(1 << 20),
		Key:     str(),
		Value:   encodeSamples[rng.Intn(len(encodeSamples))],
		ChildID: rng.Intn(3) * rng.Intn(1<<16),
		RefID:   rng.Intn(3) * rng.Intn(1<<16),
		NS:      []string{"", core.SVGNamespace}[rng.Intn(2)],
	}
}

// The hand-written encoder must produce exactly what json.Marshal does — the
// runtime parses it as JSON, and json.Marshal was the encoder before.
func TestEncodeBatchMatchesEncodingJSON(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		batch := make([]core.Mutation, rng.Intn(8))
		for j := range batch {
			batch[j] = randomMutation(rng)
		}
		want, err := json.Marshal(batch)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		got, dropped := EncodeBatch(batch)
		if dropped != nil || !bytes.Equal(got, want) {
			t.Fatalf("batch %d differs from json.Marshal\n got: %s\nwant: %s\ndropped: %v", i, got, want, dropped)
		}
	}
}

func TestEncodeBatchEmpty(t *testing.T) {
	for _, batch := range [][]core.Mutation{nil, {}} {
		if got, _ := EncodeBatch(batch); string(got) != "[]" {
			t.Fatalf("EncodeBatch(%v) = %s, want []", batch, got)
		}
	}
}

// A dropped mutation leaves no stray comma, wherever it sits in the batch.
func TestEncodeBatchDropsWithoutStrayCommas(t *testing.T) {
	ok := core.Mutation{Type: core.MutSetAttribute, NodeID: 1, Key: "x", Value: "1"}
	bad := core.Mutation{Type: core.MutSetProperty, NodeID: 2, Key: "y", Value: math.NaN()}
	for _, batch := range [][]core.Mutation{{bad, ok}, {ok, bad}, {bad, bad}, {ok, bad, ok}, {bad, ok, bad}} {
		data, dropped := EncodeBatch(batch)
		var kept []core.Mutation
		for _, m := range batch {
			if m.NodeID == 1 {
				kept = append(kept, m)
			}
		}
		want, _ := json.Marshal(kept)
		if kept == nil {
			want = []byte("[]")
		}
		if !bytes.Equal(data, want) || len(dropped) != len(batch)-len(kept) {
			t.Fatalf("batch %v: got %s (dropped %d), want %s", batch, data, len(dropped), want)
		}
	}
}

func FuzzEncodeString(f *testing.F) {
	for _, s := range encodeSamples {
		if s, ok := s.(string); ok {
			f.Add(s)
		}
	}
	f.Fuzz(func(t *testing.T, s string) {
		want, _ := json.Marshal(s)
		if got := appendString(nil, s); !bytes.Equal(got, want) {
			t.Fatalf("appendString(%q) = %s, want %s", s, got, want)
		}
	})
}

func FuzzEncodeFloat(f *testing.F) {
	for _, v := range encodeSamples {
		if v, ok := v.(float64); ok {
			f.Add(v)
		}
	}
	f.Fuzz(func(t *testing.T, v float64) {
		want, err := json.Marshal(v)
		got, ok := appendFloat(nil, v, 64)
		if (err == nil) != ok || (ok && !bytes.Equal(got, want)) {
			t.Fatalf("appendFloat(%v) = %s (ok=%v), want %s (err=%v)", v, got, ok, want, err)
		}
		want32, err32 := json.Marshal(float32(v))
		got32, ok32 := appendFloat(nil, float64(float32(v)), 32)
		if (err32 == nil) != ok32 || (ok32 && !bytes.Equal(got32, want32)) {
			t.Fatalf("appendFloat(float32 %v) = %s, want %s", float32(v), got32, want32)
		}
	})
}
