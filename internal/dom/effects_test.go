package dom

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// #60: OnMount runs after the component's DOM exists — its ref is set and the
// element's mutations were already produced — not during setup.
func TestOnMountRunsAfterTheDOMExists(t *testing.T) {
	ref := h.Ref()
	idAtMount := -1
	ranDuringRender := false
	rendering := true
	comp := core.Component("C", func() core.Node {
		hooks.OnMount(func() func() {
			idAtMount = ref.ID
			ranDuringRender = rendering
			return nil
		})
		return h.Input(h.RefTo(ref))
	})
	r := New()
	muts, _ := r.Render(comp)
	rendering = false
	if idAtMount != -1 {
		t.Fatal("OnMount must wait for the DOM, not run during Render")
	}
	created := false
	for _, m := range muts {
		created = created || (m.Type == core.MutCreateElement && m.NodeID == ref.ID)
	}
	if !created {
		t.Fatal("the element's mutations should be in this frame's batch")
	}
	r.Scheduler.RunEffects()
	if ranDuringRender || idAtMount == 0 || idAtMount != ref.ID {
		t.Fatalf("OnMount must see the rendered element: id=%d ref=%d", idAtMount, ref.ID)
	}
}

// Focus from OnMount reaches the node (it used to be a silent no-op: ref.ID 0).
func TestRefCommandFromOnMount(t *testing.T) {
	ref := h.Ref()
	r := New()
	core.SetActiveScheduler(r.Scheduler)
	defer core.SetActiveScheduler(nil)
	r.Render(core.Component("C", func() core.Node {
		hooks.OnMount(func() func() { ref.Focus(); return nil })
		return h.Input(h.RefTo(ref))
	}))
	r.Scheduler.Flush()
	r.Scheduler.RunEffects()
	got := r.Scheduler.Flush()
	if len(got) != 1 || got[0].Type != core.MutInvoke || got[0].NodeID != ref.ID || got[0].Key != "focus" {
		t.Fatalf("want Invoke(focus) on the input, got %+v", got)
	}
}

// A component mounted and unmounted before its effects ran never runs them.
func TestEffectDroppedWhenUnmountedBeforeFirstRun(t *testing.T) {
	show := core.NewSignal(false)
	runs, cleanups := 0, 0
	r := New()
	r.Render(h.Div(h.Show(show, func() core.Node {
		return core.Component("C", func() core.Node {
			hooks.OnMount(func() func() { runs++; return func() { cleanups++ } })
			return h.Span()
		})
	})))
	show.Set(true)
	r.Scheduler.Flush() // mounts C; its effect is queued
	show.Set(false)
	r.Scheduler.Flush() // unmounts C before the queued effect ran
	r.Scheduler.RunEffects()
	if runs != 0 || cleanups != 0 {
		t.Fatalf("an unmounted component's effect must not run: runs=%d cleanups=%d", runs, cleanups)
	}
}

// A dep change before the first run doesn't run the effect early; the first
// run sees the latest value, and later changes re-run it.
func TestUseEffectFirstRunSeesLatestAndThenTracks(t *testing.T) {
	v := core.NewSignal(1)
	var seen []int
	r := New()
	r.Render(core.Component("C", func() core.Node {
		hooks.UseEffect([]core.SignalAccessor{v}, func() func() {
			seen = append(seen, v.Get())
			return nil
		})
		return h.Span()
	}))
	v.Set(2)
	if len(seen) != 0 {
		t.Fatalf("effect ran before mount: %v", seen)
	}
	r.Scheduler.RunEffects()
	v.Set(3)
	if len(seen) != 2 || seen[0] != 2 || seen[1] != 3 {
		t.Fatalf("want [2 3], got %v", seen)
	}
}

// A panicking effect is contained; the others still run.
func TestEffectPanicIsContained(t *testing.T) {
	ran := false
	r := New()
	r.Render(h.Div(
		core.Component("Bad", func() core.Node {
			hooks.OnMount(func() func() { panic("effect boom") })
			return h.Span()
		}),
		core.Component("Good", func() core.Node {
			hooks.OnMount(func() func() { ran = true; return nil })
			return h.Span()
		}),
	))
	r.Scheduler.RunEffects()
	if !ran {
		t.Fatal("a panicking effect must not stop the next one")
	}
}
