package hooks

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/internal/runtime"
)

// Watch runs fn whenever any dep changes, once per batch: a handler that sets
// two deps runs fn once, after both are written. It does not run on mount (unlike
// OnMount) and takes no cleanup (unlike UseEffect) — it is the plain "when
// these signals change, react" primitive. Subscriptions are registered on the
// current component frame and torn down when it unmounts. No-op on the server.
//
//	hooks.Watch([]core.SignalAccessor{query}, func() {
//	    refetch(query.Get())
//	})
func Watch(deps []core.SignalAccessor, fn func()) {
	if runtime.CurrentEnv() == runtime.EnvServer {
		return
	}
	// One run per batch however many deps it wrote (#138).
	trigger, stop := core.Coalesce(fn)
	unsubs := make([]func(), 0, len(deps))
	for _, dep := range deps {
		unsubs = append(unsubs, dep.Subscribe(trigger))
	}
	core.RegisterDisposer(func() {
		stop()
		for _, u := range unsubs {
			if u != nil {
				u()
			}
		}
	})
}
