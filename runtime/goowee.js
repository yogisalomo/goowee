const nodeMap = {};
let rootNode = null;

function getRoot() {
    if (!rootNode) rootNode = document.getElementById("root");
    return rootNode;
}

// Boot instrumentation (roadmap 3.2). All marks are one-shot and namespaced so
// they never collide with app or browser marks, and SPA route changes don't
// overwrite them. See docs/plans/boot-latency-measurement.md.
const goowee = (window.goowee = window.goowee || {});

// Dev-mode inspector (roadmap 4.2). Enable with ?goowee-dev or
// localStorage.setItem("goowee-dev", "1"). Then call goowee.inspect() in the
// console for the component/scope tree and signal graph.
goowee.devEnabled = function devEnabled() {
    if (goowee._dev !== undefined) return goowee._dev;
    try {
        goowee._dev =
            /(?:\?|&)goowee-dev(?:=1)?(?:&|$)/.test(location.search) ||
            localStorage.getItem("goowee-dev") === "1";
    } catch (_) {
        goowee._dev = false;
    }
    return goowee._dev;
};

goowee.log = function log(entryJSON) {
    let entry;
    try {
        entry = typeof entryJSON === "string" ? JSON.parse(entryJSON) : entryJSON;
    } catch (_) {
        entry = { message: String(entryJSON) };
    }
    const kind = entry.kind || "goowee";
    const msg = entry.message || kind;
    const level = kind.startsWith("recover.") ? "error"
        : kind === "signal.cycle" || kind === "warn" ? "warn"
        : "log";
    console[level]("[goowee]", msg, entry);
};

goowee.inspect = function inspect() {
    if (typeof goowee.inspectGo !== "function") {
        console.warn(
            "[goowee] inspector unavailable — enable dev mode with ?goowee-dev or localStorage.setItem('goowee-dev','1') and reload."
        );
        return null;
    }
    const snap = goowee.inspectGo();
    console.log("[goowee] inspector snapshot:", snap);
    return snap;
};

function mark(name) {
    try { performance.mark(name); } catch (_) {}
}
function markOnce(name) {
    if (goowee["_" + name]) return;
    goowee["_" + name] = true;
    mark(name);
}

// boot loads and runs the WASM module, replacing the copy-pasted loader that
// used to live in each HTML shell. `opts.wasm` overrides the module URL (default
// "main.wasm", resolved against <base> like the other assets).
goowee.boot = function boot(opts) {
    opts = opts || {};
    const wasmURL = opts.wasm || "main.wasm";
    const go = new Go();
    mark("goowee:fetch-start");
    return WebAssembly.instantiateStreaming(fetch(wasmURL), go.importObject)
        .then(function (result) {
            mark("goowee:instantiated");
            mark("goowee:run");
            go.run(result.instance);
        });
};

// bootTimings collects the marks and the wasm Resource Timing entry into a plain
// object (read by value over CDP by test/e2e/boot.mjs). Durations are ms.
goowee.bootTimings = function bootTimings() {
    const at = function (name) {
        const e = performance.getEntriesByName(name, "mark");
        return e.length ? e[0].startTime : null;
    };
    const marks = {
        fetchStart: at("goowee:fetch-start"),
        instantiated: at("goowee:instantiated"),
        run: at("goowee:run"),
        firstListen: at("goowee:first-listen"),
        hydrateStart: at("goowee:hydrate-start"),
        hydrateEnd: at("goowee:hydrate-end"),
        interactive: at("goowee:interactive"),
    };
    const res = performance.getEntriesByType("resource")
        .find(function (r) { return r.name.split("?")[0].endsWith("main.wasm"); });
    // First Contentful Paint, from navigation start (timeOrigin) — same basis as
    // tti. With SSR the server HTML paints before WASM boots, so FCP lands well
    // before tti; a client-only render paints only after WASM renders (FCP ~ tti).
    const paint = performance.getEntriesByType("paint")
        .find(function (p) { return p.name === "first-contentful-paint"; });
    const sub = function (a, b) { return (a != null && b != null) ? a - b : null; };
    return {
        interactive: goowee._interactive === true,
        marks: marks,
        phases: {
            downloadCompile: sub(marks.instantiated, marks.fetchStart),
            goBoot: sub(marks.interactive, marks.run),
            hydrate: sub(marks.hydrateEnd, marks.hydrateStart),
            fcp: paint ? paint.startTime : null, // from navigation start (timeOrigin)
            tti: marks.interactive, // from navigation start (timeOrigin)
        },
        wasm: res ? {
            transferSize: res.transferSize,
            encodedBodySize: res.encodedBodySize,
            duration: res.duration,
            startTime: res.startTime,
            responseEnd: res.responseEnd,
        } : null,
    };
};

const preexistingNodes = {};
let hydrated = false;

// Collect server-rendered nodes for reuse, the first time mutations are
// applied. This is deferred rather than run at module load because this
// script sits in <head>, before <div id="root"> (and its SSR content) exists;
// by the first applyMutations the WASM app has booted and the DOM is present.
// For a pure client app (no SSR) there simply are no nodes to claim.
function hydrateOnce() {
    if (hydrated) return;
    hydrated = true;
    mark("goowee:hydrate-start");
    const root = getRoot();
    if (!root) { mark("goowee:hydrate-end"); return; }

    root.querySelectorAll("[data-node-id]").forEach(el => {
        preexistingNodes[parseInt(el.getAttribute("data-node-id"), 10)] = el;
    });

    // Text nodes can't carry attributes; SSR marks each with a preceding
    // <!--g{id}--> comment. Claim the comment's next sibling, then drop the
    // marker so the live DOM matches the client tree.
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_COMMENT);
    const markers = [];
    while (walker.nextNode()) {
        const m = /^g(\d+)$/.exec(walker.currentNode.data);
        if (m) markers.push([parseInt(m[1], 10), walker.currentNode]);
    }
    for (const [id, comment] of markers) {
        const next = comment.nextSibling;
        if (next && next.nodeType === 3 /* TEXT_NODE */) {
            preexistingNodes[id] = next;
        }
        comment.remove();
    }
    mark("goowee:hydrate-end");
}

const listening = {};
// Coalesce high-frequency events: only the latest event per type is kept,
// and they flush on the next requestAnimationFrame.
const pendingEvents = {};
let rAFEventPending = false;
function queueEvent(type, e) {
    pendingEvents[type] = e;
    if (!rAFEventPending) {
        rAFEventPending = true;
        requestAnimationFrame(function () {
            rAFEventPending = false;
            for (var t in pendingEvents) {
                dispatchToGo(t, pendingEvents[t]);
                delete pendingEvents[t];
            }
        });
    }
}

window.goListen = function (type, capture) {
    // First registration means Go has booted through bridge.Init (roadmap 3.2).
    markOnce("goowee:first-listen");
    if (listening[type]) return;
    listening[type] = true;
    var handler = (type === "scroll" || type === "pointermove")
        ? function (e) { queueEvent(type, e); }
        : function (e) { dispatchToGo(type, e); };
    document.addEventListener(type, handler, capture);
};

function buildPayload(type, e) {
    switch (type) {
        case "click": case "dblclick":
        case "pointerdown": case "pointerup": case "pointermove":
            return {clientX: e.clientX, clientY: e.clientY, button: e.button,
                    ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey, metaKey: e.metaKey};
        case "input": case "change": {
            const t = e.target;
            if (t.type === "checkbox" || t.type === "radio")
                return {value: t.value, checked: t.checked};
            return {value: t.value};
        }
        case "submit": {
            const values = {};
            for (const el of e.target.elements) {
                if (!el.name) continue;
                values[el.name] = (el.type === "checkbox" || el.type === "radio")
                    ? (el.checked ? "on" : "off") : el.value;
            }
            return {values};
        }
        case "keydown": case "keyup":
            return {key: e.key, code: e.code,
                    ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey, metaKey: e.metaKey};
        case "scroll":
            return {scrollTop: e.target.scrollTop, scrollLeft: e.target.scrollLeft};
        default:
            return {};
    }
}

function dispatchToGo(type, e) {
    const payload = JSON.stringify(buildPayload(type, e));
    let el = e.target;
    while (el) {
        if (el._nodeID !== undefined) {
            const r = handleEvent(el._nodeID, type, payload);
            if (r && r.handled) {
                if (r.preventDefault) e.preventDefault();
                if (r.stopPropagation) e.stopPropagation();
                if (r.selectOnFocus && type === "focus") el.select();
                return;
            }
        }
        el = el.parentElement;
    }
}

window.applyMutations = function applyMutations(json) {
    hydrateOnce();
    // First flush = first paint of app-produced DOM = interactive (roadmap 3.2).
    if (!goowee._interactive) {
        goowee._interactive = true;
        mark("goowee:interactive");
    }
    const muts = JSON.parse(json);
    for (const mut of muts) {
        let el;
        switch (mut.type) {
            case 0: // CreateElement
                if (preexistingNodes[mut.nodeId]) {
                    el = preexistingNodes[mut.nodeId];
                    delete preexistingNodes[mut.nodeId];
                } else if (mut.value === "#text") {
                    el = document.createTextNode("");
                } else if (mut.ns) {
                    el = document.createElementNS(mut.ns, mut.value); // SVG etc.
                } else {
                    el = document.createElement(mut.value);
                }
                el._nodeID = mut.nodeId;
                nodeMap[mut.nodeId] = el;
                break;
            case 1: // RemoveNode
                el = nodeMap[mut.nodeId];
                if (el && el.parentNode) el.parentNode.removeChild(el);
                delete nodeMap[mut.nodeId];
                break;
            case 2: // SetAttribute
                el = nodeMap[mut.nodeId];
                if (el) el.setAttribute(mut.key, mut.value);
                break;
            case 3: // SetProperty
                el = nodeMap[mut.nodeId];
                if (el) {
                    // Setting .value resets the caret to the end. For a focused
                    // text field, preserve the selection so typing doesn't jump
                    // (and programmatic updates still apply — we don't skip the
                    // write, we restore the caret after it).
                    if (mut.key === "value" && el === document.activeElement &&
                        typeof el.selectionStart === "number") {
                        if (el.value !== mut.value) {
                            const s = el.selectionStart, end = el.selectionEnd;
                            el.value = mut.value;
                            try { el.setSelectionRange(s, end); } catch (_) {}
                        }
                    } else {
                        el[mut.key] = mut.value;
                    }
                }
                break;
            case 4: { // AppendChild
                const parent = mut.nodeId === 0 ? getRoot() : nodeMap[mut.nodeId];
                const child = nodeMap[mut.childId];
                if (parent && child && !child.parentNode) {
                    parent.appendChild(child);
                }
                break;
            }
            case 5: { // InsertBefore
                const parent = mut.nodeId === 0 ? getRoot() : nodeMap[mut.nodeId];
                const child = nodeMap[mut.childId];
                const ref = mut.refId ? nodeMap[mut.refId] : null;
                if (parent && child) {
                    parent.insertBefore(child, ref);
                }
                break;
            }
            case 6: { // RemoveAttribute
                el = nodeMap[mut.nodeId];
                if (el) el.removeAttribute(mut.key);
                break;
            }
            case 8: { // Invoke — call a method on a node (refs: focus/blur/…)
                el = nodeMap[mut.nodeId];
                if (el && typeof el[mut.key] === "function") el[mut.key]();
                break;
            }
            case 9: { // PortalAppend — append child to a container by selector
                const parent = document.querySelector(mut.value);
                const child = nodeMap[mut.childId];
                if (parent && child && child.parentNode !== parent) {
                    parent.appendChild(child);
                }
                break;
            }
            case 7: { // Hydrate — claim a server-rendered node by id
                const pre = preexistingNodes[mut.nodeId];
                const wantText = mut.value === "#text";
                // The claimed node must match what the client expects; a wrong
                // tag/type means the server and client rendered different trees.
                const matches = pre && (wantText
                    ? pre.nodeType === 3
                    : pre.nodeType === 1 && pre.nodeName.toLowerCase() === mut.value);
                if (matches) {
                    el = pre;
                    delete preexistingNodes[mut.nodeId];
                } else {
                    if (pre) {
                        console.error(
                            "goowee: hydration mismatch at node " + mut.nodeId +
                            " — client expected <" + mut.value + ">, server rendered <" +
                            (pre.nodeName || pre.nodeType).toString().toLowerCase() + ">. " +
                            "Render deterministic markup, or wrap non-deterministic content with h.Dynamic().");
                        delete preexistingNodes[mut.nodeId];
                    } else {
                        console.error(
                            "goowee: no server node for " + mut.nodeId + " (<" + mut.value +
                            ">). SSR/client structure diverged; creating it bare.");
                    }
                    // Best-effort recovery so the app keeps running.
                    el = wantText ? document.createTextNode("") : document.createElement(mut.value);
                }
                el._nodeID = mut.nodeId;
                nodeMap[mut.nodeId] = el;
                break;
            }
        }
    }
};
