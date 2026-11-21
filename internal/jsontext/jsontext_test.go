package jsontext

import (
	"reflect"
	"testing"
)

var objects = []string{
	`{}`, `{"a":1}`, `{"a":1,"a":2}`, ` { "x" : [1, {"y": "}"}] , "s":"a\"b", "n": -1.5e3, "t": true, "z": null } `,
	`{"clientX":12.5,"key":"Enter","files":[{"name":"a,b.txt","size":3}],"values":{"q":"x y"}}`,
	`{"key":"escaped key","key":"plain"}`, `{"u":"héllo","e":"é\n"}`, `{"a":[[[]]],"b":{"c":{"d":"]"}}}`,
}

// Field finds what Decode would put under each key, without decoding.
func TestFieldMatchesDecode(t *testing.T) {
	for _, s := range objects {
		checkFields(t, s)
	}
}

func checkFields(t *testing.T, s string) {
	t.Helper()
	m, err := DecodeObject(s)
	if err != nil || m == nil {
		return
	}
	for k, want := range m {
		text, ok := Field(s, k)
		if !ok {
			t.Fatalf("Field(%q, %q): not found", s, k)
		}
		got, err := Decode(text)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Field(%q, %q) = %q (%v), want %#v", s, k, text, got, want)
		}
		if str, isStr := want.(string); isStr {
			if got, ok := String(text); !ok || got != str {
				t.Fatalf("String(%q) = %q, want %q", text, got, str)
			}
		}
		if num, isNum := want.(float64); isNum {
			if got, ok := Number(text); !ok || got != num {
				t.Fatalf("Number(%q) = %v, want %v", text, got, num)
			}
		}
	}
	if _, ok := Field(s, "\x00missing"); ok {
		t.Fatalf("Field found a missing key in %q", s)
	}
}

func FuzzField(f *testing.F) {
	for _, s := range objects {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { checkFields(t, s) })
}

// Reading a plain field allocates nothing.
func TestFieldAllocs(t *testing.T) {
	s := `{"type":"pointermove","clientX":120.5,"clientY":44,"pointerType":"mouse","shiftKey":false}`
	if n := testing.AllocsPerRun(100, func() {
		v, _ := Field(s, "clientY")
		Number(v)
		v, _ = Field(s, "pointerType")
		String(v)
	}); n != 0 {
		t.Fatalf("%v allocs", n)
	}
}
