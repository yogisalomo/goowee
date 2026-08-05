# Graph Report - .  (2026-08-04)

## Corpus Check
- 109 files · ~91,982 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1171 nodes · 3325 edges · 63 communities (44 shown, 19 thin omitted)
- Extraction: 65% EXTRACTED · 35% INFERRED · 0% AMBIGUOUS · INFERRED: 1163 edges (avg confidence: 0.8)
- Token cost: 250,122 input · 0 output

## Community Hubs (Navigation)
- Component Runtime & Lifecycle
- Example App Pages
- Signals & Core Tests
- HTML Element DSL
- Scheduler & Async Resources
- Router & Reactive Control Flow
- Form Binding Tests
- WASM Bridge & Devtools
- Attributes & Properties DSL
- Router Guards & Navigation
- Headless Test Harness
- Package Overview & Guides
- DSL Helpers & VirtualList
- SSR Metadata & Head
- Two-Way Data Binding
- Node Types & Visitors
- DOM Renderer & Diff
- SSR Server & Gzip
- Event Handlers DSL
- Observability & Logging
- Boot-Latency Harness
- Typed Event Payloads
- SSR Rendering Visitor
- Roadmap & Lifecycle ADRs
- SVG Elements
- JS Runtime (goowee.js)
- Architecture Review & Run-Once Model
- Devtools E2E Test
- Binding Registry
- Smoke E2E Test
- Boot & Bundle Size
- Metadata Hydration Tests
- Component Node & Visitor
- DSL Design & Template Prior Art
- Reconciliation & Control-Flow ADRs
- Keyed-Diff Helpers
- Raw HTML Node
- Error Boundary
- Text Node
- Hydration Parity ADRs
- Batched Rendering Rationale
- TinyGo recover() Effort
- Signal Inspection
- Signal Equality & Computed
- Subscription Lifecycle Fix
- Integer Attribute Tests
- Thin JS Bridge Principle
- Mutation Addressing
- Batching & Signals
- Router History Binding
- Boot Script
- E2E Run Script
- Manual-Deps ADR
- Dev-Workflow ADR
- Experimental-Status Risk
- DX Issues Report
- Missing UseContext
- SPA Deployment Request
- Stray linkedin.txt
- Module Path

## God Nodes (most connected - your core abstractions)
1. `El()` - 147 edges
2. `ElementNode` - 119 edges
3. `Node` - 112 edges
4. `Item` - 107 edges
5. `NewSignal()` - 82 edges
6. `New()` - 59 edges
7. `Text()` - 48 edges
8. `Component()` - 36 edges
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

## Communities (63 total, 19 thin omitted)

### Community 0 - "Component Runtime & Lifecycle"
Cohesion: 0.06
Nodes (59): ComponentFrame, newFakeDOM(), domIDKinds(), fakeDOM, T, sortedKinds(), ssrIDKinds(), TestGreetParamRouteReactive() (+51 more)

### Community 1 - "Example App Pages"
Cohesion: 0.11
Nodes (73): dashboardRow, entry, todo, tutorialStep, Component(), aboutPage(), App(), appHeader() (+65 more)

### Community 2 - "Signals & Core Tests"
Cohesion: 0.07
Nodes (72): NewSignal(), T, TestEqualValueSkipsNotify(), TestInterfaceTypedNonComparableValueNoPanic(), TestNestedSetObservesFinalValue(), TestNonComparableAlwaysNotifiesWithoutPanic(), TestNotificationOrderIsSubscriptionOrder(), TestSetCycleDetectionStops() (+64 more)

### Community 3 - "HTML Element DSL"
Cohesion: 0.08
Nodes (66): ElementNode, Area(), Article(), Aside(), Audio(), B(), Blockquote(), Br() (+58 more)

### Community 4 - "Scheduler & Async Resources"
Cohesion: 0.06
Nodes (45): T, TestAllocBudgetCoalesce(), TestAllocBudgetSignalNotifyConstant(), BenchmarkCoalesce(), BenchmarkSignalSetFanout(), T, TestElementNodeApply(), TestEventDataAccessors() (+37 more)

### Community 5 - "Router & Reactive Control Flow"
Cohesion: 0.05
Nodes (33): Computed(), T, SetKey(), Signal, Signal[T], T, subscriber, For() (+25 more)

### Community 6 - "Form Binding Tests"
Cohesion: 0.07
Nodes (56): ID(), Input(), Option(), OnFocus(), SelectOnFocus(), Key(), T, TestAriaAtomic() (+48 more)

### Community 7 - "WASM Bridge & Devtools"
Cohesion: 0.07
Nodes (40): initBridge(), initInspector(), initLogSink(), initObservability(), Run(), sendMutations(), startScheduler(), Handler (+32 more)

### Community 8 - "Attributes & Properties DSL"
Cohesion: 0.08
Nodes (52): Item, Accept(), Action(), Alt(), Aria(), AriaAtomic(), AriaBusy(), AriaChecked() (+44 more)

### Community 9 - "Router Guards & Navigation"
Cohesion: 0.11
Nodes (39): stripBase(), basePath(), CurrentPath(), Router, Guard(), Lazy(), New(), T (+31 more)

### Community 10 - "Headless Test Harness"
Cohesion: 0.15
Nodes (21): fnode, harness, T, TestErrorPageBoundaryAndStructuredLog(), TestLandingDevtoolsSnapshot(), TestLandingHeroIncrementHeadless(), fakeDOM, Router (+13 more)

### Community 11 - "Package Overview & Guides"
Cohesion: 0.08
Nodes (40): CI workflow — vet, race tests, WASM size budget, e2e, Pages deploy workflow — builds & deploys landing/tutorial, AGENT.md — dev commands & architecture, AGENTS.md — agent guide for building goowee apps, bridge package — bridge.Run WASM entry point, core package — signals, computed, scheduler, node types, h package — element/attr/event DSL & control flow, hooks package — UseState/UseEffect/OnMount/UseResource (+32 more)

### Community 12 - "DSL Helpers & VirtualList"
Cohesion: 0.07
Nodes (26): Ref, attrItem, bindItem, dynamicItem, groupItem, Dynamic(), Fragment(), T (+18 more)

### Community 13 - "SSR Metadata & Head"
Cohesion: 0.17
Nodes (32): Content(), Name(), JSONLD(), Link(), LLM(), Meta(), Metadata(), Page() (+24 more)

### Community 14 - "Two-Way Data Binding"
Cohesion: 0.12
Nodes (23): BindAttr(), BindChecked(), BindProp(), BindSelect(), BindValue(), BindValueLazy(), CheckedS(), ClassS() (+15 more)

### Community 15 - "Node Types & Visitors"
Cohesion: 0.11
Nodes (8): Run(), Attr, FragmentNode, MetadataNode, Node, PortalNode, Prop, Portal()

### Community 16 - "DOM Renderer & Diff"
Cohesion: 0.27
Nodes (8): Mutation, flattenChildren(), FlatTree(), DOMRenderer, nodeID(), rootIDFromTree(), RestoreFrameStack(), SaveFrameStack()

### Community 17 - "SSR Server & Gzip"
Cohesion: 0.21
Nodes (19): acceptsGzip(), compressible(), contentType(), gzipStatic(), main(), serveStatic(), T, gunzip() (+11 more)

### Community 18 - "Event Handlers DSL"
Cohesion: 0.18
Nodes (19): On(), OnBlur(), OnChange(), OnChangeE(), OnCopy(), OnCut(), OnDblClick(), OnFocusIn() (+11 more)

### Community 19 - "Observability & Logging"
Cohesion: 0.22
Nodes (16): LogEntry, LogKind, copyFields(), formatFields(), Log(), Recover(), SetLogSink(), T (+8 more)

### Community 20 - "Boot-Latency Harness"
Cohesion: 0.14
Nodes (15): chrome, DP, ITERS, kb(), logs, main(), median(), ms() (+7 more)

### Community 21 - "Typed Event Payloads"
Cohesion: 0.16
Nodes (6): EventData, OnDblClickE(), OnInputE(), OnKeyDownE(), OnKeyUpE(), OnScrollE()

### Community 22 - "SSR Rendering Visitor"
Cohesion: 0.17
Nodes (4): Builder, ScopeNode, escapeHTML(), ssrVisitor

### Community 23 - "Roadmap & Lifecycle ADRs"
Cohesion: 0.19
Nodes (13): ADR-007: Per-render RenderContext; effects never run on server, ADR-010: Signals are single-threaded (no locking), ADR-015: Off-loop state updates go through core.Schedule, ADR-016: Hydration trusts parity; mismatches opt-in via h.Dynamic, ADR-017: Refs are imperative command handles; portals rebuild, ADR-018: ErrorBoundary for render panics, containment for updates, Internal package boundary (internal/dom, internal/runtime), API stability policy (v0 breaking changes allowed) (+5 more)

### Community 24 - "SVG Elements"
Cohesion: 0.21
Nodes (11): logo(), Circle(), Ellipse(), G(), Line(), Path(), Polygon(), Polyline() (+3 more)

### Community 25 - "JS Runtime (goowee.js)"
Cohesion: 0.24
Nodes (11): buildPayload(), dispatchToGo(), getRoot(), hydrateOnce(), listening, mark(), markOnce(), nodeMap (+3 more)

### Community 26 - "Architecture Review & Run-Once Model"
Cohesion: 0.24
Nodes (11): Codex Implementation Plan v1 (archived original design), ADR-001: Run-once Solid-style components, ComponentFrame (ownership/disposal scope), Lifecycle primitives (OnMount/Watch/UseEffect/UseScope), Run-once component model (Solid-style), Competitor research: Go WASM UI frameworks, Ergonomics improvements plan (typed DSL, control flow), Signal core fixes plan (token subs, safe notify) (+3 more)

### Community 27 - "Devtools E2E Test"
Cohesion: 0.24
Nodes (9): chrome, consoleLogs, consoleText(), DP, main(), pending, send(), sleep() (+1 more)

### Community 28 - "Binding Registry"
Cohesion: 0.31
Nodes (6): Bind, BindTarget, MutationForBind(), NewBindingRegistry(), BindingRegistry, boundBinding

### Community 29 - "Smoke E2E Test"
Cohesion: 0.24
Nodes (8): chrome, DP, logs, main(), pending, send(), sleep(), userDir

### Community 30 - "Boot & Bundle Size"
Cohesion: 0.22
Nodes (9): ADR-012: Router map-based routes with reactive params, Boot latency / TTI measurement harness, gzip/brotli wasm compression download win, goowee.boot() unified loader instrumentation, WASM binary size — no tree-shaking/splitting, Router query parameter support (missing), Binary size is the honest cost of Go-in-WASM, index.html SPA fallback requirement (try_files) (+1 more)

### Community 31 - "Metadata Hydration Tests"
Cohesion: 0.47
Nodes (8): createID(), T, headAppends(), hydrateID(), metadataTree(), TestMetadataFreshRenderAppendsHead(), TestMetadataHydrationIDParity(), TestMetadataHydrationNoDuplicateHead()

### Community 33 - "DSL Design & Template Prior Art"
Cohesion: 0.29
Nodes (8): ADR-003: Typed Go DSL (h package) is the authoring API, ADR-004: -S suffix separates static vs reactive helpers, go-app (PWA typed builder API), templ (tooling-first template success), Vugu (Vue .vugu template port), .gwx template compiler (gated Phase 3), Typed h DSL / Item interface (typed ElementNode fields), h.Raw raw HTML rendering (missing)

### Community 34 - "Reconciliation & Control-Flow ADRs"
Cohesion: 0.25
Nodes (8): ADR-009: Text nodes get comment hydration markers, ADR-013: Keyed reconciliation correct-but-naive (no LIS yet), ScopeNode & reconciliation, Control-flow primitives Show/ShowElse/Switch/For, Keyed reconciliation (RefID-based diff), For loses component state across re-renders, ShowResource / Suspense-like async pattern, Goowee lessons learned (engineering notes)

### Community 35 - "Keyed-Diff Helpers"
Cohesion: 0.32
Nodes (7): scopeState, computeNeedsMove(), hasAnyKey(), keyOf(), lis(), rootTypeChanged(), typeCompatible()

### Community 39 - "Hydration Parity ADRs"
Cohesion: 0.50
Nodes (5): Lenient hydration system (deferred client reconnection), ADR-008: Hydration by node-id parity + claim-based reuse, Review §6: make SSR/DOM ID parity a first-class invariant, Hydration is adoption (claim mutations), not re-render, SSR/client id parity by construction (shared walker)

### Community 40 - "Batched Rendering Rationale"
Cohesion: 0.50
Nodes (4): ADR-006: Batched rendering via mutation queue + dirty-scope scheduler, Vecty (React/VDOM port), Review §4: batch renders, not just mutations, Minimize WASM↔JS boundary crossings

### Community 41 - "TinyGo recover() Effort"
Cohesion: 0.50
Nodes (4): Draft comment for TinyGo PR #4380, Asyncify × EH coexistence risk (the #1 gate), SjLj-backed recover (Approach A), Native wasm exception handling (Wasm 3.0)

### Community 43 - "Signal Equality & Computed"
Cohesion: 0.67
Nodes (3): ADR-011: Signal equality with WithEquals opt-in, Computed derived signals with manual deps, Equality skip / WithEquals (no reflect)

### Community 44 - "Subscription Lifecycle Fix"
Cohesion: 0.67
Nodes (3): Reentrancy-safe notification with cycle guard, Token-based Signal.Subscribe, Review §2: subscription lifecycle leaks & unsound notify

### Community 45 - "Integer Attribute Tests"
Cohesion: 0.67
Nodes (3): Cols(), Rows(), TestIntAttrs()

## Knowledge Gaps
- **65 isolated node(s):** `dirtyScope`, `subscriber`, `subscriberCount`, `entry`, `todo` (+60 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **19 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Node` connect `Node Types & Visitors` to `Component Runtime & Lifecycle`, `Example App Pages`, `HTML Element DSL`, `Scheduler & Async Resources`, `Router & Reactive Control Flow`, `WASM Bridge & Devtools`, `Attributes & Properties DSL`, `Router Guards & Navigation`, `Headless Test Harness`, `DSL Helpers & VirtualList`, `DOM Renderer & Diff`, `Typed Event Payloads`, `SSR Rendering Visitor`, `SVG Elements`, `Binding Registry`, `Metadata Hydration Tests`, `Component Node & Visitor`, `Keyed-Diff Helpers`, `Raw HTML Node`, `Error Boundary`, `Text Node`?**
  _High betweenness centrality (0.247) - this node is a cross-community bridge._
- **Why does `NewSignal()` connect `Signals & Core Tests` to `Component Runtime & Lifecycle`, `Example App Pages`, `Scheduler & Async Resources`, `Router & Reactive Control Flow`, `Form Binding Tests`, `WASM Bridge & Devtools`, `Router Guards & Navigation`, `Headless Test Harness`, `SSR Metadata & Head`, `Two-Way Data Binding`?**
  _High betweenness centrality (0.145) - this node is a cross-community bridge._
- **Why does `ElementNode` connect `HTML Element DSL` to `Component Node & Visitor`, `Example App Pages`, `Signals & Core Tests`, `Component Runtime & Lifecycle`, `Raw HTML Node`, `Error Boundary`, `Text Node`, `WASM Bridge & Devtools`, `Form Binding Tests`, `DSL Helpers & VirtualList`, `SSR Metadata & Head`, `Node Types & Visitors`, `SSR Rendering Visitor`, `SVG Elements`, `Binding Registry`?**
  _High betweenness centrality (0.138) - this node is a cross-community bridge._
- **Are the 144 inferred relationships involving `El()` (e.g. with `A()` and `Area()`) actually correct?**
  _`El()` has 144 INFERRED edges - model-reasoned connections that need verification._
- **Are the 80 inferred relationships involving `NewSignal()` (e.g. with `TestAllocBudgetSignalNotifyConstant()` and `BenchmarkSignalSetFanout()`) actually correct?**
  _`NewSignal()` has 80 INFERRED edges - model-reasoned connections that need verification._
- **What connects `dirtyScope`, `subscriber`, `subscriberCount` to the rest of the system?**
  _65 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Component Runtime & Lifecycle` be split into smaller, more focused modules?**
  _Cohesion score 0.06139240506329114 - nodes in this community are weakly interconnected._