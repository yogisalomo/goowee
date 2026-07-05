package hooks

import "github.com/yogisalomo/goowee/core"

// Watch runs fn whenever any dep changes. It does not run on mount (unlike
// OnMount) and takes no cleanup (unlike UseEffect) — it is the plain "when
// these signals change, react" primitive. Subscriptions are registered on the
// current component frame and torn down when it unmounts. No-op on the server.
//
//	hooks.Watch([]core.SignalAccessor{query}, func() {
//	    refetch(query.Get())
//	})
func Watch(deps []core.SignalAccessor, fn func()) {
	if core.CurrentEnv() == core.EnvServer {
		return
	}
	unsubs := make([]func(), 0, len(deps))
	for _, dep := range deps {
		unsubs = append(unsubs, dep.Subscribe(fn))
	}
	core.RegisterDisposer(func() {
		for _, u := range unsubs {
			if u != nil {
				u()
			}
		}
	})
}
