# goowee — lessons learned (working notes)

Personal notes to mine for future posts. Pulled *out* of the main blog post so
that stays focused on the framework's core idea. I dropped the "working with an
LLM" observations from the original list — they didn't feel durable or
interesting enough. These are the engineering lessons from building the
framework itself. Each is a candidate for its own short write-up.

## Architecture / performance

1. **Minimize WASM↔JS boundary crossings above almost everything else.**
   `syscall/js` calls are the dominant cost. Batching every DOM change into one
   JSON payload and crossing once per animation frame — instead of once per DOM
   op — is the single biggest performance lever in the whole design.

2. **Address DOM nodes by integer id; never let Go hold a DOM handle.**
   The Go renderer emits a `[]Mutation` addressed by id and holds no `js.Value`.
   Consequence: the entire renderer is unit-testable on the host with a plain
   `go test` — no headless browser, no jsdom. Most tests never leave Go.

3. **Coalescing is where correctness-preserving perf lives.** N writes to the
   same (node, key) in a frame collapse to the last value; N dirty-marks on a
   scope collapse to one re-render. The batch is the natural place to dedupe.

4. **Run-once components eliminate the stale-closure problem** that dependency
   arrays exist to manage. Go closures capture signals directly and are never
   rebuilt. Cost: everything dynamic *must* live in a signal, because the body
   never runs again. Worth stating loudly to newcomers — it's the one rule.

## SSR / hydration

5. **Get SSR/client id parity "by construction," not by shipping a map.**
   Both renderers share the same walker, which allocates ids in a deterministic
   pre-order DFS. Server and client independently arrive at the same id for the
   same node. Guard it with a golden test (`TestSSRDOMIDParity`) — the whole
   hydration scheme rests on that invariant.

6. **Hydration is adoption, not re-render.** The client walks the same tree but
   emits *claim* mutations that adopt existing DOM nodes and wire up handlers,
   instead of creating + diffing. Much cheaper and simpler than a reconciliation
   pass against server markup.

7. **Text nodes can't carry attributes → comment markers `<!--g{id}-->`.**
   And the follow-on constraint that bit later: raw-text elements (`textarea`,
   `title`, `script`) can't hold comment markers at all, so their text can't be
   id-marked the same way. Head content has to stay static for this reason.

8. **The "server renders it AND client renders it fresh" duplication bug class.**
   (This was the PR #44 fix — a clean concrete example.) Portals dodge
   duplication because SSR never renders them, so rendering them fresh on the
   client is correct. Metadata (`<head>`) was mistakenly modeled on portals — but
   the server *does* render head tags, so rendering fresh on hydration produced
   two `<title>`, duplicate `<meta>`, etc. Fix: during hydration, walk the head
   children (to keep id parity) but **discard** the mutations. Lesson: "render
   fresh during hydration" is only safe for content the server never emitted.

9. **Void elements must self-close, and the set is easy to under-specify.**
   Exposed by the metadata feature: `core.VoidElements` was missing
   `meta`/`link`/`base`, producing stray `</meta>`/`</link>`. Small, but it's a
   real HTML-correctness gotcha that only shows up once you render `<head>`.

## Go-in-WASM specifics

10. **Goroutines exist in WASM but rendering is single-threaded.** Timers and
    fetches run cooperatively on one thread and must not touch signals directly.
    Funnel off-loop writes through a scheduler `Post`/`Schedule` and drain them
    at the top of the next flush. The one place Go's concurrency model pokes
    through the abstraction.

11. **Interface equality is a trap for pointer-typed signals.** `Signal[*Foo]`
    compares the pointer, not the pointee — mutate through the pointer, call
    `Set`, and the change is silently swallowed as "equal." Provide a
    `WithEquals` escape hatch and document it. A Go-generics-flavored footgun.

12. **The binary size is the honest cost.** A Go WASM build ships the runtime +
    GC (~4.3 MB here). Enforce a size budget in CI so it doesn't creep, and be
    upfront about it — it's the real tradeoff for "it's all Go."

## DOM reconciliation subtleties

13. **Setting `input.value` resets the caret.** For a focused field, save and
    restore `selectionStart/End` around the write, or typing jumps to the end.

14. **Keyed list moves: use a longest-increasing-subsequence pass.** Compute the
    LIS of retained nodes and only move the nodes outside it — minimizes DOM
    `insertBefore` churn on reorders.
