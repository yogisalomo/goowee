package core

import "testing"

type deriveUser struct {
	Name  string
	Admin bool
}

func TestDerive(t *testing.T) {
	user := NewSignal(deriveUser{Name: "Ada", Admin: false})
	name := Derive(user, func(u deriveUser) string { return u.Name })

	if got := name.Get(); got != "Ada" {
		t.Fatalf("initial value = %q, want Ada", got)
	}
	user.Set(deriveUser{Name: "Grace", Admin: true})
	if got := name.Get(); got != "Grace" {
		t.Fatalf("unobserved derive was stale: %q", got)
	}

	var seen []string
	unsub := name.Subscribe(func() { seen = append(seen, name.Get()) })
	user.Set(deriveUser{Name: "Alan", Admin: false})
	if len(seen) != 1 || seen[0] != "Alan" {
		t.Fatalf("observer saw %v, want [Alan]", seen)
	}
	unsub()
}

func TestDerive2(t *testing.T) {
	first := NewSignal("Ada")
	last := NewSignal("Lovelace")
	full := Derive2(first, last, func(f, l string) string { return f + " " + l })

	if got := full.Get(); got != "Ada Lovelace" {
		t.Fatalf("initial value = %q", got)
	}
	last.Set("Byron")
	if got := full.Get(); got != "Ada Byron" {
		t.Fatalf("after last changed: %q", got)
	}
	first.Set("Anne")
	if got := full.Get(); got != "Anne Byron" {
		t.Fatalf("after first changed: %q", got)
	}
}

func TestDeps(t *testing.T) {
	a, b := NewSignal(1), NewSignal("x")
	deps := Deps(a, b)
	if len(deps) != 2 || deps[0] != a || deps[1] != b {
		t.Fatalf("Deps returned %v", deps)
	}
	c := Computed(deps, func() string { return b.Get() })
	if got := c.Get(); got != "x" {
		t.Fatalf("Computed with Deps = %q", got)
	}
}
