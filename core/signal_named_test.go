package core

import (
	"reflect"
	"testing"
)

func TestNamedGivesTheNameToDevtools(t *testing.T) {
	saved := NameSignalHook
	defer func() { NameSignalHook = saved }()
	var gotSig SignalAccessor
	var gotName string
	NameSignalHook = func(sig SignalAccessor, name string) bool {
		gotSig, gotName = sig, name
		return true
	}
	s := NewSignal(0)
	if s.Named("cart") != s {
		t.Fatal("Named must return the signal, for chaining")
	}
	if gotSig != s || gotName != "cart" {
		t.Fatalf("hook got %v %q", gotSig, gotName)
	}
}

// A browser build holds names given before bridge.Run decides on dev mode (a
// store in a package-level var), until TakeEarlyNames; then it stops holding.
func TestEarlyNamesAreHeldUntilTaken(t *testing.T) {
	savedHook := NameSignalHook
	earlyNamesMu.Lock()
	savedHolding, savedNames := holdingNames, earlyNames
	holdingNames, earlyNames = true, nil
	earlyNamesMu.Unlock()
	defer func() {
		NameSignalHook = savedHook
		earlyNamesMu.Lock()
		holdingNames, earlyNames = savedHolding, savedNames
		earlyNamesMu.Unlock()
	}()

	NameSignalHook = func(SignalAccessor, string) bool { return false } // dev mode not on yet
	a := NewSignal(1).Named("a")
	b := NewSignal("x").Named("b")
	got := TakeEarlyNames()
	want := []NamedSignal{{Signal: a, Name: "a"}, {Signal: b, Name: "b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("early names %+v", got)
	}
	NewSignal(2).Named("late")
	if late := TakeEarlyNames(); len(late) != 0 {
		t.Fatalf("names given after the app started are not held: %+v", late)
	}
}

// Outside the browser (a server, a test) nothing is held: a server naming
// signals per request must not accumulate them.
func TestNoEarlyNamesOutsideTheBrowser(t *testing.T) {
	if holdingNames {
		t.Skip("browser build")
	}
	savedHook := NameSignalHook
	NameSignalHook = nil
	defer func() { NameSignalHook = savedHook }()
	NewSignal(1).Named("per-request")
	if held := TakeEarlyNames(); len(held) != 0 {
		t.Fatalf("held %+v", held)
	}
}

type write struct {
	sig      SignalAccessor
	from, to any
}

func captureWrites(t *testing.T) *[]write {
	var got []write
	SetSignalWriteHook(func(sig SignalAccessor, from, to any) { got = append(got, write{sig, from, to}) })
	t.Cleanup(func() { SetSignalWriteHook(nil) })
	return &got
}

func TestWriteHookSeesEachChange(t *testing.T) {
	got := captureWrites(t)
	s := NewSignal(1)
	s.Set(2)
	s.Set(2) // equal: not a change
	s.Update(func(v int) int { return v + 1 })
	Batch(func() { s.Set(10); s.Set(11) })
	want := []write{{s, 1, 2}, {s, 2, 3}, {s, 3, 10}, {s, 10, 11}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("writes %+v", *got)
	}
}

// A Computed isn't written; only its sources are.
func TestWriteHookSkipsComputed(t *testing.T) {
	src := NewSignal(1)
	double := Computed([]SignalAccessor{src}, func() int { return src.Get() * 2 })
	unsub := double.Subscribe(func() {})
	defer unsub()
	got := captureWrites(t)
	src.Set(5)
	if double.Get() != 10 {
		t.Fatalf("computed = %d", double.Get())
	}
	if len(*got) != 1 || (*got)[0].sig != src {
		t.Fatalf("writes %+v", *got)
	}
}

// The hook is only for dev mode: a Set without it doesn't allocate.
func TestSetWithoutWriteHookDoesNotAllocate(t *testing.T) {
	s := NewSignal(0)
	n := 0
	if a := testing.AllocsPerRun(100, func() { n++; s.Set(n) }); a != 0 {
		t.Fatalf("Set allocated %v times", a)
	}
}
