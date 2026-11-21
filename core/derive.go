package core

// Deps is a readable spelling of the []SignalAccessor slice passed to Computed,
// UseEffect, Watch and the reactive helpers. Derive and Derive2 build their own
// declarations, so Deps is mainly useful for the many-dependency case where
// Computed remains the right primitive.
//
//	hooks.UseEffect(core.Deps(a, b), func() func() { … })
func Deps(deps ...SignalAccessor) []SignalAccessor { return deps }

// Derive returns a read-only signal of fn(dep.Get()). It is a thin wrapper over
// Computed: the dependency is still explicit, but it is named once, next to the
// read, so the two cannot drift apart.
//
//	name := core.Derive(user, func(u User) string { return u.Name })
func Derive[T, R any](dep *Signal[T], fn func(T) R) *Signal[R] {
	return Computed([]SignalAccessor{dep}, func() R { return fn(dep.Get()) })
}

// Derive2 is Derive for two dependencies. For three or more, use Computed with
// core.Deps.
//
//	full := core.Derive2(first, last, func(f, l string) string { return f + " " + l })
func Derive2[T1, T2, R any](a *Signal[T1], b *Signal[T2], fn func(T1, T2) R) *Signal[R] {
	return Computed([]SignalAccessor{a, b}, func() R { return fn(a.Get(), b.Get()) })
}
