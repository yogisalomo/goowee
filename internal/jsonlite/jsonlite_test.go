package jsonlite

import (
	"encoding/json"

	"github.com/yogisalomo/goowee/core"
	"math"
	"reflect"
	"testing"
)

var decodeCases = []string{
	`null`, `true`, `false`, `0`, `-0`, `1.5e3`, `-12.25E-2`, `1e400`, `01`, `1.`, `.5`, `-`, `+1`,
	`""`, `"a"`, `"héllo"`, `"\"\\\/\b\f\n\r\t"`, `"é\u0000"`, `"😀"`, `"\ud83d"`, `"\ude00x"`,
	`"\ud83dA"`, `"\ud83d😀"`, "\"\xff\xfe\"", "\"a\x01\"", `"\x"`, `"\u12"`, `"unterminated`,
	`[]`, `[1,2,[3,{}]]`, `[1,]`, `[,1]`, `{}`, `{"a":1,"b":[true,null],"c":{"d":"e"}}`, `{"a":1,"a":2}`,
	`{"a" 1}`, `{"a":1,}`, `{1:2}`, ` { "x" : [ 1 , 2 ] } `, `[1] x`, ``, `nul`, `truex`,
	`{"type":"click","clientX":12.5,"key":"Enter","files":[{"name":"a.txt","size":3}]}`,
}

// Decode accepts what json.Unmarshal into an any accepts, and gives the same
// value.
func TestDecodeMatchesEncodingJSON(t *testing.T) {
	for _, s := range decodeCases {
		checkDecode(t, s)
	}
}

func checkDecode(t *testing.T, s string) {
	t.Helper()
	var want any
	wantErr := json.Unmarshal([]byte(s), &want)
	got, err := Decode(s)
	if (err != nil) != (wantErr != nil) {
		t.Fatalf("Decode(%q): err %v, encoding/json err %v", s, err, wantErr)
	}
	if err == nil && !reflect.DeepEqual(got, want) {
		t.Fatalf("Decode(%q) = %#v, encoding/json gives %#v", s, got, want)
	}
}

func FuzzDecode(f *testing.F) {
	for _, s := range decodeCases {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { checkDecode(t, s) })
}

// Append writes what json.Marshal writes for the types it handles itself.
func TestAppendMatchesEncodingJSON(t *testing.T) {
	values := []any{
		nil, true, false, "", "a<b>&c\"\\/\n\t\x01", "héllo 😀", "  ", "\xff\xfe",
		0, -7, math.MaxInt64, int8(-1), int16(2), int32(-3), int64(math.MinInt64),
		uint(1), uint8(255), uint16(3), uint32(4), uint64(math.MaxUint64),
		0.0, math.Copysign(0, -1), 0.1, 1.5, 1e6, 1e20, 1e21, 1e-6, 1e-7, 123456789.125, math.MaxFloat64, math.SmallestNonzeroFloat64,
		float32(0.1), float32(1e21), float32(1e-7), float32(3.4e38),
		[]any{1, "x", []any{}, map[string]any{}}, []any{}, []string{"a", "b"}, []string{},
		map[string]any{"b": 1, "a": []any{true, nil}, "c": map[string]any{"z": "y"}}, map[string]string{"k": "v", "a": "b"},
		[]map[string]any{{"x": 1}, {}},
		[]int{1, 2}, []int(nil), []float64{0.5}, [2]bool{true, false}, map[string]int{"b": 2, "a": 1},
		map[string]any(nil), []any(nil), []string(nil), map[string]string(nil), []map[string]any(nil), label("named"), level(-4), ptr(label("p")), (*label)(nil), []label{"x"},
		map[string][]int{"k": {1}}, []any{[]int{3}, map[string]float32{"f": 0.5}}, num(7), ptr(num(8)),
	}
	for _, v := range values {
		want, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := Append(nil, v)
		if !ok || string(got) != string(want) {
			t.Errorf("Append(%#v) = %s (%v), json.Marshal gives %s", v, got, ok, want)
		}
	}
	for _, v := range []any{math.NaN(), math.Inf(1), []any{math.Inf(-1)}} {
		if _, ok := Append(nil, v); ok {
			t.Errorf("Append(%v) succeeded; encoding/json fails on it", v)
		}
	}
}

type label string
type level int16

// num marshals itself.
type num int

func (n num) MarshalJSON() ([]byte, error) { return []byte(` "n" `), nil }

// Without a hook, what encoding/json alone can write is refused, not guessed.
func TestAppendLeavesStructsToTheHook(t *testing.T) {
	prev := core.JSONMarshaler() // host builds install json.Marshal
	core.UseJSON(nil)
	defer core.UseJSON(prev)
	type pt struct{ X int }
	for _, v := range []any{pt{1}, []byte("ab"), map[int]string{1: "a"}} {
		if _, ok := Append(nil, v); ok {
			t.Errorf("Append(%#v) succeeded without a marshaler", v)
		}
	}
}

func ptr[T any](v T) *T { return &v }
