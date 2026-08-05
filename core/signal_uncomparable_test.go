package core

import "testing"

// A signal holding an uncomparable value (slice, map) must Set without panicking
// and must treat every Set as a change. Go's recover caught the uncomparable-==
// panic; TinyGo can't, so equal() decides comparability by reflection. This test
// passes under both `go test` and `tinygo test` and guards the route-navigation
// crash that motivated the fix (pages that hold []T / map[K]V state).
func TestSignalUncomparableSliceSets(t *testing.T) {
	s := NewSignal([]int{1, 2})
	n := 0
	s.Subscribe(func() { n++ })

	s.Set([]int{3, 4}) // changed slice — must not panic
	if n != 1 {
		t.Fatalf("slice Set should notify once, got %d", n)
	}
	s.Set([]int{3, 4}) // uncomparable → always treated as changed
	if n != 2 {
		t.Fatalf("uncomparable slice Set should always notify, got %d", n)
	}
	if got := s.Get(); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("unexpected value after Set: %v", got)
	}
}

func TestSignalUncomparableMapSets(t *testing.T) {
	s := NewSignal(map[string]int{"a": 1})
	n := 0
	s.Subscribe(func() { n++ })
	s.Set(map[string]int{"b": 2}) // must not panic
	if n != 1 {
		t.Fatalf("map Set should notify, got %d", n)
	}
}

// Comparable signals keep the equality-skip optimization: setting the same value
// must not notify.
func TestSignalComparableEqualitySkip(t *testing.T) {
	s := NewSignal(42)
	n := 0
	s.Subscribe(func() { n++ })
	s.Set(42) // equal → no notify
	if n != 0 {
		t.Fatalf("equal int Set should not notify, got %d", n)
	}
	s.Set(43) // changed → notify
	if n != 1 {
		t.Fatalf("changed int Set should notify, got %d", n)
	}
}
