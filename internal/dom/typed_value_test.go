package dom

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

// #136: a binding's typed mutation encodes exactly as the boxed one it
// replaces (the value fmt's %v gave an attribute or textContent, the raw
// value a property), in both transports.
func TestTypedBindingEncodesLikeBoxed(t *testing.T) {
	floats := []float64{0, math.Copysign(0, -1), 0.1, 1.5, -2.25, 1e6, 1e21, 1e-7, 123456789.125, math.MaxFloat64, math.SmallestNonzeroFloat64, 1.0 / 3}
	ints := []int{0, -1, 7, 1 << 53, math.MaxInt64, math.MinInt64}
	strs := []string{"", "a", "héllo", "😀", "a\"b\\c\n", " ", "javascript:alert(1)", "\xff"}
	var sigs []core.SignalAccessor
	for _, f := range floats {
		sigs = append(sigs, core.NewSignal(f))
	}
	for _, i := range ints {
		sigs = append(sigs, core.NewSignal(i))
	}
	for _, s := range strs {
		sigs = append(sigs, core.NewSignal(s))
	}
	sigs = append(sigs, core.NewSignal(true), core.NewSignal(false))

	binds := []core.Bind{
		{Target: core.BindToAttr, Name: "data-x"},
		{Target: core.BindToAttr, Name: "href"},
		{Target: core.BindToProp, Name: "textContent"},
		{Target: core.BindToProp, Name: "value"},
		{Target: core.BindToProp, Name: "src"},
	}
	var enc BatchEncoder
	for _, sig := range sigs {
		for _, b := range binds {
			b.Signal = sig
			typed := runtime.MutationForBind(3, b)
			if typed.Kind == core.ValAny {
				t.Fatalf("%T: no typed value", sig)
			}
			boxed := core.Mutation{Type: typed.Type, NodeID: 3, Key: b.Name, Value: typed.Val()}
			// The boxed path formats with fmt: check Val agrees with it.
			if b.Target == core.BindToAttr || b.Name == "textContent" {
				want := runtime.SafeURL(b.Name, fmt.Sprintf("%v", sig.Value()))
				if got := typed.Val(); got != want {
					t.Errorf("%s=%#v: Val %#v, fmt gives %#v", b.Name, sig.Value(), got, want)
				}
			}
			gotJSON, _ := EncodeBatch([]core.Mutation{typed})
			wantJSON, _ := EncodeBatch([]core.Mutation{boxed})
			if !bytes.Equal(gotJSON, wantJSON) {
				t.Errorf("%s=%#v: JSON %s, boxed %s", b.Name, sig.Value(), gotJSON, wantJSON)
			}
			got, _ := enc.Encode([]core.Mutation{typed})
			got = append([]byte(nil), got...)
			want, _ := enc.Encode([]core.Mutation{boxed})
			if !bytes.Equal(got, want) {
				t.Errorf("%s=%#v: binary % x, boxed % x", b.Name, sig.Value(), got, want)
			}
		}
	}
}
