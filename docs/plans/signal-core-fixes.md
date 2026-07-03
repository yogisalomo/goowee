# Signal Core Fixes: Token Subscriptions, Safe Notification, Equality Skip

*Authored 2026-07-03. Status: proposed.*
*Implements review findings §2.2 and §2.3 from `docs/26-07-02-fable-review.md`.*
*This is step 1 of the checklist in `docs/plans/ergonomics-improvements.md` §6 — everything there that stores an unsubscribe function (BindingRegistry §3.6.2, `Computed` disposers §3.3) is only correct once this plan has landed. It is independently shippable as its own PR.*

This is an **implementation spec**: code blocks are normative unless marked "sketch". Everything happens in `core/signal.go` plus tests; no other file changes except deleting one import.

---

## 1. The Bugs Being Fixed

### 1.1 Wrong-subscriber removal (review §2.2)

`Signal.Subscribe` currently identifies the subscription to remove via `reflect.ValueOf(fn).Pointer()` (`core/signal.go:25-36`). For closures, that returns the **code pointer**, which is identical for every closure created from the same source-code literal — the captured variables don't participate. Concrete failure:

```go
// dom/renderer.go:159 — every scope subscribes with the same literal:
v.Unsubs[i] = dep.Subscribe(func() { r.reRenderScope(v) })
```

Two scopes subscribing to the **same signal** register two different closures with the **same code pointer**. Unsubscribing scope B scans for the pointer and removes the *first* match — which may be scope A's subscription. Scope A silently stops updating; scope B keeps re-rendering after "unmount". Order-dependent, no error, extremely hard to debug.

### 1.2 Unsafe notification (review §2.3)

`Signal.Set` (`core/signal.go:18-23`) iterates `s.subs` directly while subscribers run:

- A subscriber that **unsubscribes** (itself or another) mutates the slice in place mid-iteration → a later subscriber is silently skipped.
- A subscriber that was **unsubscribed by an earlier subscriber** in the same pass is still invoked → calls into torn-down component/effect state.
- A subscriber that calls **`Set` on the same signal** recurses unboundedly — no cycle guard.
- `Set` with an **unchanged value** notifies everyone anyway → e.g. re-setting the same route path re-renders the whole route scope for nothing.

---

## 2. New `core/signal.go` (full replacement)

```go
package core

import "log"

// subscriber pairs a stable token with the callback. Tokens — not
// function identity — are how unsubscription works; two closures from
// the same source literal are always distinct subscriptions.
type subscriber struct {
	id int
	fn func()
}

type Signal[T any] struct {
	value     T
	subs      []subscriber
	nextSubID int
	eq        func(a, b T) bool // optional custom equality; nil = default
	notifying bool              // true while a notification pass runs
	dirty     bool              // a nested Set changed the value mid-pass
}

func NewSignal[T any](v T) *Signal[T] {
	return &Signal[T]{value: v}
}

// WithEquals sets a custom equality function used by Set to decide
// whether a change is real. Intended for construction-time chaining:
//
//	s := core.NewSignal(user).WithEquals(func(a, b User) bool { return a.ID == b.ID })
func (s *Signal[T]) WithEquals(eq func(a, b T) bool) *Signal[T] {
	s.eq = eq
	return s
}

// Get returns the typed value (user code).
func (s *Signal[T]) Get() T { return s.value }

// Value satisfies SignalAccessor (binding layer).
func (s *Signal[T]) Value() any { return s.value }

// Update sets the value derived from the current one.
func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.value)) }

// Subscribe registers fn to run after the value changes. The returned
// function cancels exactly this subscription; calling it more than
// once is a no-op.
func (s *Signal[T]) Subscribe(fn func()) func() {
	s.nextSubID++
	id := s.nextSubID
	s.subs = append(s.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range s.subs {
			if s.subs[i].id == id {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				return
			}
		}
	}
}

// maxNotifyPasses bounds re-notification when subscribers keep setting
// the signal to new values (an update cycle). Generous on purpose:
// legitimate cascades are short; only true cycles hit the cap.
const maxNotifyPasses = 1000

// Set stores v and notifies subscribers, unless v equals the current
// value (see equal). Notification semantics:
//
//   - Subscribers run in subscription order.
//   - The subscriber list is snapshotted per pass: subscribers added
//     during a pass do not run in that pass; subscribers removed during
//     a pass are skipped (liveness is re-checked before each call).
//   - If a subscriber calls Set with a different value, the current
//     pass finishes, then all subscribers run again observing the
//     final value (bounded by maxNotifyPasses; a cycle logs and stops).
//   - A nested Set on the *same* signal never recurses.
func (s *Signal[T]) Set(v T) {
	if s.equal(s.value, v) {
		return
	}
	s.value = v
	if s.notifying {
		// Nested Set from inside a subscriber: mark for a re-pass and
		// return; the outer loop below picks it up.
		s.dirty = true
		return
	}
	s.notifying = true
	defer func() { s.notifying = false }()

	for pass := 0; ; pass++ {
		if pass >= maxNotifyPasses {
			log.Printf("goowee: signal update cycle detected after %d passes; stopping notification", maxNotifyPasses)
			return
		}
		s.dirty = false

		// Snapshot tokens, not closures: liveness is re-checked at call
		// time so a subscriber unsubscribed mid-pass is never invoked.
		ids := make([]int, len(s.subs))
		for i, sub := range s.subs {
			ids[i] = sub.id
		}
		for _, id := range ids {
			if fn := s.lookup(id); fn != nil {
				fn()
			}
		}
		if !s.dirty {
			return
		}
	}
}

func (s *Signal[T]) lookup(id int) func() {
	for i := range s.subs {
		if s.subs[i].id == id {
			return s.subs[i].fn
		}
	}
	return nil
}

// equal reports whether a and b are the same value. With no custom eq,
// it compares via interface equality, treating a runtime "type is not
// comparable" panic as "not equal" — so signals of slice/map/func types
// simply always notify. No reflection is used.
func (s *Signal[T]) equal(a, b T) (eq bool) {
	if s.eq != nil {
		return s.eq(a, b)
	}
	defer func() {
		if recover() != nil {
			eq = false
		}
	}()
	return any(a) == any(b)
}

var _ SignalAccessor = (*Signal[int])(nil)
var _ SignalAccessor = (*Signal[string])(nil)
```

Delete the `reflect` import — after this change nothing in `core` uses reflection (relevant to binary size and any future TinyGo evaluation; review §9).

### Design notes (why these choices)

- **Ordered slice, not a map.** Notification order must be deterministic (subscription order) for predictable behavior and stable tests. Removal and lookup are O(n) scans, so a full pass is O(n²) worst case — fine at UI scale, where a signal has a handful of subscribers (bindings + scopes). If profiling ever shows otherwise, swap the scan for an id→index map; do not do this preemptively.
- **Snapshot ids, not closures.** Snapshotting closures would fix skipping but still invoke subscribers that were unsubscribed mid-pass — calling into disposed effect/component state. Re-checking liveness by token at call time gives the correct semantics: *an unsubscribed subscriber is never called afterward, even within the same pass*.
- **Re-pass instead of recursion for nested `Set`.** Subscribers always observe the final value, the same signal never recurses, and the pass counter turns infinite update cycles into a logged stop instead of a stack overflow. Subscribers *do* run again on a re-pass — that is intended (they must see the newest value) and must be documented on `Set` (done, in the doc comment above).
- **Equality skip via safe interface comparison.** The alternatives were rejected: requiring `comparable` breaks `Signal[[]Todo]` and every slice-typed `UseState` in the examples; reflection-based comparability checks keep the `reflect` dependency this plan removes. The panic-recover costs one deferred call per `Set` (open-coded defers make this cheap) and only actually recovers for non-comparable `T`, which then behaves exactly as today (always notify). Custom types opt into smarter equality with `WithEquals`.
- **Interface-typed `T` caveat.** For `Signal[any]` (or other interface types), `any(a) == any(b)` compares *dynamic* values and would itself panic if a dynamic value is non-comparable — the recover handles that too: it degrades to "not equal → notify". Never let the panic escape `equal`.

---

## 3. Behavior Changes (intended, audit call sites)

| Change | Effect | In-repo impact |
|---|---|---|
| `Set` with an equal value no longer notifies | Redundant re-renders/effects stop | `router.Navigate` to the current path no longer re-renders the route scope (desirable). `Path.Set` is still followed by `navFn` (pushState) as before — unchanged by this plan. No other call site in the repo relies on same-value notification; verify with `grep -rn "\.Set(" --include="*.go"` during implementation and note anything suspicious in the PR. |
| Unsubscribed-mid-pass subscribers are skipped | No callbacks into torn-down state | Strictly a fix; `RunFrameCleanup` during a scope re-render can no longer cause a just-cleaned effect to fire once more. |
| Nested `Set` defers to a re-pass | Subscribers see final values; cycles terminate with a log instead of a stack overflow | Strictly a fix. |
| Unsubscribe is token-exact and idempotent | No cross-subscriber removal | Strictly a fix for the §2.2 bug. |

No public API signatures change: `NewSignal`, `Get`, `Set`, `Subscribe`, `Value` are untouched; `Update` and `WithEquals` are additive. `hooks.UseState` needs no modification.

---

## 4. Test Plan (`core/signal_test.go`)

All plain `go test`. Each test states the regression it pins.

1. **`TestUnsubscribeRemovesOnlyItsOwn`** *(the §2.2 bug)* — in a loop, create two closures from the *same literal* (each appending to its own captured slice) and subscribe both to one signal. Call the first unsubscribe. `Set` a new value. Assert: second closure ran, first did not. This test **must fail** against the old reflect implementation — verify that before fixing (run it on the unmodified file once).
2. **`TestUnsubscribeIdempotent`** — call one unsubscribe function twice with three subscribers present; assert no panic and both other subscribers still fire.
3. **`TestUnsubscribeDuringNotifySkipsRemoved`** — subscriber A (subscribed first) unsubscribes subscriber B (subscribed second) when it runs. `Set` once. Assert B never ran.
4. **`TestSubscribeDuringNotifyDeferredToNextSet`** — subscriber A subscribes a new subscriber C during notification. Assert C did not run for this `Set` (no nested-set/dirty involved), then a second `Set` runs C.
5. **`TestNestedSetObservesFinalValue`** — subscriber sets the signal from 1→2 exactly once (guarded). Assert: no panic, final `Get()` is 2, and a second recording subscriber observed value 2 in its last invocation.
6. **`TestSetCycleDetectionStops`** — subscriber always sets `value+1` (a true cycle). Assert `Set` returns (does not hang or overflow the stack); value reflects the last write; use a counter to confirm passes stopped at `maxNotifyPasses`. Keep the subscriber body trivial so the test stays fast.
7. **`TestEqualValueSkipsNotify`** — `Set(5)` on a signal already holding 5; assert zero subscriber calls. Then `Set(6)`; assert one call.
8. **`TestNonComparableAlwaysNotifiesWithoutPanic`** — `Signal[[]int]`: `Set` the same slice value twice; assert two notifications and no panic.
9. **`TestInterfaceTypedNonComparableValueNoPanic`** — `Signal[any]` holding `[]int{1}`; `Set([]int{1})` again; assert notify happened and no panic escaped.
10. **`TestWithEqualsCustom`** — struct signal with `WithEquals` comparing one field; `Set` with a value equal under the custom eq but different under `==`; assert no notification.
11. **`TestNotificationOrderIsSubscriptionOrder`** — three subscribers appending markers; assert exact order across two `Set` calls, including after unsubscribing the middle one.
12. **Existing suites** — `go test ./...` green: `core`, `hooks`, `dom`, `ssr`, `router` tests must pass unmodified. If any existing test relied on same-value notification, fix the *test* and flag it in the PR description.

---

## 5. Implementation Order

1. [ ] Add `core/signal_test.go` with tests 1–11; run test 1 against the **current** implementation and confirm it fails (proves the test catches the §2.2 bug).
2. [ ] Replace `core/signal.go` with §2 of this plan; delete the `reflect` import.
3. [ ] `go test ./...` — all green, including the pre-existing suites (test 12).
4. [ ] `grep -rn "reflect" core/` returns nothing.
5. [ ] Audit `.Set(` call sites for same-value-notification reliance (§3 table); note findings in the PR.
6. [ ] Commit. This unblocks step 2 of `docs/plans/ergonomics-improvements.md` §6.

## 6. Explicitly Out of Scope

- **Batching renders / dirty-scope scheduling** (review §4): `Set` still notifies synchronously and scope re-renders still run inside it. This plan only makes that synchronous behavior *correct*; making it *scheduled* is separate work with its own design (it changes when subscribers run, not whether the bookkeeping is sound).
- **`BindingRegistry` unsub storage** (review §2.1): specced in `docs/plans/ergonomics-improvements.md` §3.6.2, which depends on this plan.
- **Thread safety.** Signals remain single-"thread" (the WASM event loop / one goroutine). Cross-goroutine `Set` (see the stopwatch goroutine, review §3.2) stays undefined behavior until the concurrency model lands (review §10.3). Do not add a mutex here — it would paper over a design question with a lock.
