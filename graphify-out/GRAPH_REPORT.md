# Graph Report - goowee  (2026-08-05)

## Corpus Check
- 108 files · ~94,155 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1192 nodes · 3370 edges · 66 communities (46 shown, 20 thin omitted)
- Extraction: 65% EXTRACTED · 35% INFERRED · 0% AMBIGUOUS · INFERRED: 1183 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `1cc7e52c`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- PushComponent
- Node
- NewSignal
- ElementNode
- core_test.go
- Router
- El
- devtools.go
- Item
- router_test.go
- mount
- core package — signals, computed, scheduler, node types
- h.go
- New
- SignalAccessor
- PortalNode
- DOMRenderer
- serveStatic
- events.go
- Log
- boot.mjs
- EventData
- ssrVisitor
- Road to 1.0 — production readiness roadmap
- svg.go
- goowee.js
- Goowee Architecture & Plan Review (Fable)
- devtools.mjs
- Bind
- smoke.mjs
- ADR-012: Router map-based routes with reactive params
- metadata_test.go
- Scheduler
- Typed h DSL / Item interface (typed ElementNode fields)
- ADR-013: Keyed reconciliation correct-but-naive (no LIS yet)
- [0.1.0] — 2026-07-23
- TestFormElements
- TestMiscElements
- ScopeNode
- ADR-008: Hydration by node-id parity + claim-based reuse
- ADR-006: Batched rendering via mutation queue + dirty-scope scheduler
- Native wasm exception handling (Wasm 3.0)
- Signal[T]
- ADR-011: Signal equality with WithEquals opt-in
- Review §2: subscription lifecycle leaks & unsound notify
- RegisterSignal
- Minimal JS bridge (event forwarding + mutation apply)
- Mutation system (batched DOM operations)
- Frame-based batching scheduler
- Router
- boot.sh script
- run.sh
- ADR-002: Manual dependency declaration
- ADR-014: Dev workflow — small PRs, squash-merge, -race + E2E
- Permanent experimental status failure mode
- Goowee DX issues (from building yogi.sh)
- Global state / UseContext mechanism (missing)
- SPA deployment docs & Docker/nginx reference request
- TestAllocBudgetCoalesce
- github.com/yogisalomo/goowee
- .applyHydrating
- CLAUDE.md
- Ul

## God Nodes (most connected - your core abstractions)
1. `El()` - 147 edges
2. `ElementNode` - 119 edges
3. `Node` - 112 edges
4. `Item` - 107 edges
5. `NewSignal()` - 88 edges
6. `New()` - 62 edges
7. `Text()` - 48 edges
8. `Component()` - 39 edges
9. `formPage()` - 35 edges
10. `DOMRenderer` - 34 edges

## Surprising Connections (you probably didn't know these)
- `WASM compression — the biggest boot-latency lever` --semantically_similar_to--> `CI workflow — vet, race tests, WASM size budget, e2e`  [INFERRED] [semantically similar]
  docs/guides/serving.md → .github/workflows/ci.yml
- `Run()` --calls--> `Enabled()`  [INFERRED]
  bridge/wasm.go → devtools/devtools.go
- `initLogSink()` --calls--> `SetLogSink()`  [INFERRED]
  bridge/wasm.go → core/observability.go
- `initBridge()` --calls--> `SetActiveScheduler()`  [INFERRED]
  bridge/wasm.go → core/schedule.go
- `initBridge()` --calls--> `EventCapture()`  [INFERRED]
  bridge/wasm.go → internal/dom/node_registry.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Reactivity pipeline: signals to binding to scheduler to JS bridge** — docs_guides_concepts_signal, docs_guides_concepts_schedule, docs_api_reference_core, docs_api_reference_bridge, docs_guides_concepts_component [INFERRED 0.85]
- **Public API packages under the v0 stability contract** — docs_api_reference_core, docs_api_reference_h, docs_api_reference_hooks, docs_api_reference_router, docs_api_reference_ssr, docs_api_reference_bridge [EXTRACTED 1.00]
- **SSR and hydration flow** — docs_api_reference_ssr, docs_guides_concepts_ssr_hydration, docs_api_reference_bridge, docs_guides_concepts_dynamic [INFERRED 0.85]
- **Hydration by SSR/DOM node-id parity** — docs_canonical_adr_adr_008, docs_canonical_adr_adr_009, docs_canonical_adr_adr_016, docs_reports_goowee_lessons_learned_id_parity, docs_reports_26_07_02_fable_review_hydration_parity [INFERRED 0.85]
- **TinyGo wasm recover() upstream effort** — docs_plans_tinygo_recover_implementation, docs_plans_tinygo_2914_comment, docs_plans_tinygo_recover_implementation_wasm_eh, docs_plans_tinygo_recover_implementation_asyncify_eh [INFERRED 0.85]
- **Go WASM UI predecessors stalled in permanent experimental status** — docs_learnings_competitor_research_vecty, docs_learnings_competitor_research_vugu, docs_learnings_competitor_research_go_app, docs_learnings_competitor_research_experimental_status [INFERRED 0.85]

## Communities (66 total, 20 thin omitted)

### Community 0 - "PushComponent"
Cohesion: 0.07
Nodes (52): ComponentFrame, SetActiveScheduler(), TestHydrateDynamicReappliesValues(), T, TestOnMountNilCleanup(), TestOnMountRunsOnceAndCleansUp(), TestOnMountSkippedOnServer(), TestUseEffect() (+44 more)

### Community 1 - "Node"
Cohesion: 0.12
Nodes (71): dashboardRow, entry, todo, tutorialStep, Run(), Node, Component(), TextNode (+63 more)

### Community 2 - "NewSignal"
Cohesion: 0.07
Nodes (79): NewSignal(), T, TestEqualValueSkipsNotify(), TestInterfaceTypedNonComparableValueNoPanic(), TestNestedSetObservesFinalValue(), TestNonComparableAlwaysNotifiesWithoutPanic(), TestNotificationOrderIsSubscriptionOrder(), TestSetCycleDetectionStops() (+71 more)

### Community 3 - "ElementNode"
Cohesion: 0.07
Nodes (55): Attr, ElementNode, Prop, Area(), Article(), Aside(), Audio(), B() (+47 more)

### Community 4 - "core_test.go"
Cohesion: 0.17
Nodes (24): T, TestElementNodeApply(), TestEventDataAccessors(), TestFlatTree(), TestFlatTreeFragmentInElement(), TestFlatTreeLeavesComponentsIntact(), TestFragmentNodeApply(), TestItemApplyNilSafety() (+16 more)

### Community 5 - "Router"
Cohesion: 0.05
Nodes (37): Computed(), T, SetKey(), Signal, Signal[T], T, subscriber, For() (+29 more)

### Community 6 - "El"
Cohesion: 0.08
Nodes (69): OnFocus(), El(), T, TestAriaAtomic(), TestAriaBusy(), TestAriaControls(), TestAriaCurrent(), TestAriaDescribedBy() (+61 more)

### Community 7 - "devtools.go"
Cohesion: 0.08
Nodes (35): initBridge(), initInspector(), initLogSink(), initObservability(), Run(), sendMutations(), startScheduler(), Handler (+27 more)

### Community 8 - "Item"
Cohesion: 0.08
Nodes (56): Item, Accept(), Action(), Alt(), Aria(), AriaAtomic(), AriaBusy(), AriaChecked() (+48 more)

### Community 9 - "router_test.go"
Cohesion: 0.10
Nodes (41): Schedule(), Resource[T], stripBase(), basePath(), CurrentPath(), Router, Guard(), Lazy() (+33 more)

### Community 10 - "mount"
Cohesion: 0.12
Nodes (31): fnode, harness, T, TestErrorPageBoundaryAndStructuredLog(), TestLandingDevtoolsSnapshot(), TestLandingHeroIncrementHeadless(), fakeDOM, Router (+23 more)

### Community 11 - "core package — signals, computed, scheduler, node types"
Cohesion: 0.08
Nodes (40): CI workflow — vet, race tests, WASM size budget, e2e, Pages deploy workflow — builds & deploys landing/tutorial, AGENT.md — dev commands & architecture, AGENTS.md — agent guide for building goowee apps, bridge package — bridge.Run WASM entry point, core package — signals, computed, scheduler, node types, h package — element/attr/event DSL & control flow, hooks package — UseState/UseEffect/OnMount/UseResource (+32 more)

### Community 12 - "h.go"
Cohesion: 0.07
Nodes (19): BindTarget, RawNode, Ref, attrItem, bindItem, dynamicItem, groupItem, Dynamic() (+11 more)

### Community 13 - "New"
Cohesion: 0.13
Nodes (34): FragmentNode, Content(), Name(), Fragment(), JSONLD(), Link(), LLM(), Meta() (+26 more)

### Community 14 - "SignalAccessor"
Cohesion: 0.23
Nodes (17): SignalAccessor, BindAttr(), BindChecked(), BindProp(), BindSelect(), BindValue(), BindValueLazy(), CheckedS() (+9 more)

### Community 16 - "DOMRenderer"
Cohesion: 0.20
Nodes (13): Mutation, flattenChildren(), FlatTree(), DOMRenderer, scopeState, computeNeedsMove(), hasAnyKey(), keyOf() (+5 more)

### Community 17 - "serveStatic"
Cohesion: 0.21
Nodes (19): acceptsGzip(), compressible(), contentType(), gzipStatic(), main(), serveStatic(), T, gunzip() (+11 more)

### Community 18 - "events.go"
Cohesion: 0.17
Nodes (22): On(), OnBlur(), OnChange(), OnCopy(), OnCut(), OnDblClick(), OnDblClickE(), OnFocusIn() (+14 more)

### Community 19 - "Log"
Cohesion: 0.22
Nodes (16): LogEntry, LogKind, copyFields(), formatFields(), Log(), Recover(), SetLogSink(), T (+8 more)

### Community 20 - "boot.mjs"
Cohesion: 0.14
Nodes (15): chrome, DP, ITERS, kb(), logs, main(), median(), ms() (+7 more)

### Community 22 - "ssrVisitor"
Cohesion: 0.08
Nodes (12): Builder, ComponentNode, ErrorBoundaryNode, MetadataNode, HydrationMeta, Renderer, escapeAttr(), escapeHTML() (+4 more)

### Community 23 - "Road to 1.0 — production readiness roadmap"
Cohesion: 0.19
Nodes (13): ADR-007: Per-render RenderContext; effects never run on server, ADR-010: Signals are single-threaded (no locking), ADR-015: Off-loop state updates go through core.Schedule, ADR-016: Hydration trusts parity; mismatches opt-in via h.Dynamic, ADR-017: Refs are imperative command handles; portals rebuild, ADR-018: ErrorBoundary for render panics, containment for updates, Internal package boundary (internal/dom, internal/runtime), API stability policy (v0 breaking changes allowed) (+5 more)

### Community 24 - "svg.go"
Cohesion: 0.21
Nodes (11): logo(), Circle(), Ellipse(), G(), Line(), Path(), Polygon(), Polyline() (+3 more)

### Community 25 - "goowee.js"
Cohesion: 0.24
Nodes (11): buildPayload(), dispatchToGo(), getRoot(), hydrateOnce(), listening, mark(), markOnce(), nodeMap (+3 more)

### Community 26 - "Goowee Architecture & Plan Review (Fable)"
Cohesion: 0.24
Nodes (11): Codex Implementation Plan v1 (archived original design), ADR-001: Run-once Solid-style components, ComponentFrame (ownership/disposal scope), Lifecycle primitives (OnMount/Watch/UseEffect/UseScope), Run-once component model (Solid-style), Competitor research: Go WASM UI frameworks, Ergonomics improvements plan (typed DSL, control flow), Signal core fixes plan (token subs, safe notify) (+3 more)

### Community 27 - "devtools.mjs"
Cohesion: 0.24
Nodes (9): chrome, consoleLogs, consoleText(), DP, main(), pending, send(), sleep() (+1 more)

### Community 28 - "Bind"
Cohesion: 0.31
Nodes (5): Bind, MutationForBind(), NewBindingRegistry(), BindingRegistry, boundBinding

### Community 29 - "smoke.mjs"
Cohesion: 0.24
Nodes (8): chrome, DP, logs, main(), pending, send(), sleep(), userDir

### Community 30 - "ADR-012: Router map-based routes with reactive params"
Cohesion: 0.22
Nodes (9): ADR-012: Router map-based routes with reactive params, Boot latency / TTI measurement harness, gzip/brotli wasm compression download win, goowee.boot() unified loader instrumentation, WASM binary size — no tree-shaking/splitting, Router query parameter support (missing), Binary size is the honest cost of Go-in-WASM, index.html SPA fallback requirement (try_files) (+1 more)

### Community 31 - "metadata_test.go"
Cohesion: 0.47
Nodes (8): createID(), T, headAppends(), hydrateID(), metadataTree(), TestMetadataFreshRenderAppendsHead(), TestMetadataHydrationIDParity(), TestMetadataHydrationNoDuplicateHead()

### Community 32 - "Scheduler"
Cohesion: 0.14
Nodes (7): BenchmarkCoalesce(), BenchmarkSignalSetFanout(), dirtyScope, MutationType, Scheduler, coalesce(), Mutex

### Community 33 - "Typed h DSL / Item interface (typed ElementNode fields)"
Cohesion: 0.29
Nodes (8): ADR-003: Typed Go DSL (h package) is the authoring API, ADR-004: -S suffix separates static vs reactive helpers, go-app (PWA typed builder API), templ (tooling-first template success), Vugu (Vue .vugu template port), .gwx template compiler (gated Phase 3), Typed h DSL / Item interface (typed ElementNode fields), h.Raw raw HTML rendering (missing)

### Community 34 - "ADR-013: Keyed reconciliation correct-but-naive (no LIS yet)"
Cohesion: 0.25
Nodes (8): ADR-009: Text nodes get comment hydration markers, ADR-013: Keyed reconciliation correct-but-naive (no LIS yet), ScopeNode & reconciliation, Control-flow primitives Show/ShowElse/Switch/For, Keyed reconciliation (RefID-based diff), For loses component state across re-renders, ShowResource / Suspense-like async pattern, Goowee lessons learned (engineering notes)

### Community 35 - "[0.1.0] — 2026-07-23"
Cohesion: 0.17
Nodes (11): [0.1.0] — 2026-07-23, Build, serving & tooling, Changelog, Components & hooks, Docs, DX, devtools & observability, Known limitations, Reactive core (+3 more)

### Community 36 - "TestFormElements"
Cohesion: 0.20
Nodes (10): Form(), Input(), Label(), Option(), Select(), Textarea(), TestBindSelect(), TestFormElements() (+2 more)

### Community 37 - "TestMiscElements"
Cohesion: 0.22
Nodes (9): Blockquote(), Br(), Code(), Em(), Hr(), Pre(), Strong(), TestBrElement() (+1 more)

### Community 39 - "ADR-008: Hydration by node-id parity + claim-based reuse"
Cohesion: 0.50
Nodes (5): Lenient hydration system (deferred client reconnection), ADR-008: Hydration by node-id parity + claim-based reuse, Review §6: make SSR/DOM ID parity a first-class invariant, Hydration is adoption (claim mutations), not re-render, SSR/client id parity by construction (shared walker)

### Community 40 - "ADR-006: Batched rendering via mutation queue + dirty-scope scheduler"
Cohesion: 0.50
Nodes (4): ADR-006: Batched rendering via mutation queue + dirty-scope scheduler, Vecty (React/VDOM port), Review §4: batch renders, not just mutations, Minimize WASM↔JS boundary crossings

### Community 41 - "Native wasm exception handling (Wasm 3.0)"
Cohesion: 0.50
Nodes (4): Draft comment for TinyGo PR #4380, Asyncify × EH coexistence risk (the #1 gate), SjLj-backed recover (Approach A), Native wasm exception handling (Wasm 3.0)

### Community 43 - "ADR-011: Signal equality with WithEquals opt-in"
Cohesion: 0.67
Nodes (3): ADR-011: Signal equality with WithEquals opt-in, Computed derived signals with manual deps, Equality skip / WithEquals (no reflect)

### Community 44 - "Review §2: subscription lifecycle leaks & unsound notify"
Cohesion: 0.67
Nodes (3): Reentrancy-safe notification with cycle guard, Token-based Signal.Subscribe, Review §2: subscription lifecycle leaks & unsound notify

### Community 45 - "RegisterSignal"
Cohesion: 0.33
Nodes (3): RegisterSignal(), SetSignalInspectID(), wireHooks()

### Community 61 - "TestAllocBudgetCoalesce"
Cohesion: 0.67
Nodes (3): T, TestAllocBudgetCoalesce(), TestAllocBudgetSignalNotifyConstant()

## Knowledge Gaps
- **74 isolated node(s):** `dirtyScope`, `subscriber`, `subscriberCount`, `entry`, `todo` (+69 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **20 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Node` connect `Node` to `PushComponent`, `ElementNode`, `core_test.go`, `Router`, `ScopeNode`, `devtools.go`, `Item`, `router_test.go`, `mount`, `h.go`, `New`, `PortalNode`, `DOMRenderer`, `EventData`, `ssrVisitor`, `svg.go`, `Bind`, `metadata_test.go`?**
  _High betweenness centrality (0.210) - this node is a cross-community bridge._
- **Why does `NewSignal()` connect `NewSignal` to `Scheduler`, `PushComponent`, `core_test.go`, `Router`, `El`, `devtools.go`, `TestFormElements`, `router_test.go`, `mount`, `New`, `TestAllocBudgetCoalesce`?**
  _High betweenness centrality (0.161) - this node is a cross-community bridge._
- **Why does `ElementNode` connect `ElementNode` to `Node`, `Ul`, `NewSignal`, `TestFormElements`, `TestMiscElements`, `ScopeNode`, `devtools.go`, `El`, `h.go`, `New`, `PortalNode`, `ssrVisitor`, `svg.go`, `Bind`?**
  _High betweenness centrality (0.130) - this node is a cross-community bridge._
- **Are the 144 inferred relationships involving `El()` (e.g. with `A()` and `Area()`) actually correct?**
  _`El()` has 144 INFERRED edges - model-reasoned connections that need verification._
- **Are the 86 inferred relationships involving `NewSignal()` (e.g. with `TestAllocBudgetSignalNotifyConstant()` and `BenchmarkSignalSetFanout()`) actually correct?**
  _`NewSignal()` has 86 INFERRED edges - model-reasoned connections that need verification._
- **What connects `dirtyScope`, `subscriber`, `subscriberCount` to the rest of the system?**
  _74 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `PushComponent` be split into smaller, more focused modules?**
  _Cohesion score 0.07246376811594203 - nodes in this community are weakly interconnected._