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

// node returns the live DOM node for a goowee node id (bridge.Element), or
// undefined when it doesn't exist (not rendered yet, or removed).
goowee.node = function node(id) {
    return nodeMap[id];
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
    // marker so the live DOM matches the client tree. Scope end anchors are
    // <!--/{id}--> comments that stay in the DOM: claim the comment itself.
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_COMMENT);
    const markers = [];
    while (walker.nextNode()) {
        const data = walker.currentNode.data;
        let m = /^g(\d+)$/.exec(data);
        if (m) { markers.push([parseInt(m[1], 10), walker.currentNode]); continue; }
        m = /^\/(\d+)$/.exec(data);
        if (m) preexistingNodes[parseInt(m[1], 10)] = walker.currentNode;
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
// Coalesce high-frequency events: per event type *and target*, only the latest
// event is kept, and they flush on the next requestAnimationFrame — so two
// containers scrolling in the same frame each get their update.
const pendingEvents = new Map(); // type -> Map(target -> [event, targetOnly])
let rAFEventPending = false;
function queueEvent(type, e, targetOnly) {
    let byTarget = pendingEvents.get(type);
    if (!byTarget) pendingEvents.set(type, (byTarget = new Map()));
    byTarget.set(e.target, [e, targetOnly]);
    if (!rAFEventPending) {
        rAFEventPending = true;
        requestAnimationFrame(function () {
            rAFEventPending = false;
            const batch = [...pendingEvents];
            pendingEvents.clear();
            for (const [t, byT] of batch) {
                for (const [ev, only] of byT.values()) dispatchToGo(t, ev, only);
            }
        });
    }
}

// goListen registers one document-level listener per event type. Events that
// don't bubble (capture === true: focus, mouseenter, invalid, load, …) are
// listened for in the capture phase — the only way they reach the document —
// and dispatched to their target's handler only, as the DOM would.
window.goListen = function (type, capture) {
    // First registration means Go has booted through bridge.Init (roadmap 3.2).
    markOnce("goowee:first-listen");
    if (listening[type]) return;
    listening[type] = true;
    const targetOnly = !!capture;
    const handler = (type === "scroll" || type === "pointermove")
        ? function (e) { queueEvent(type, e, targetOnly); }
        : function (e) { dispatchToGo(type, e, targetOnly); };
    document.addEventListener(type, handler, capture);
};

// File inputs: the change/input payload carries each picked file's metadata
// plus a handle, and the File objects themselves are parked here so Go can
// read their bytes later (goowee.readFile, via core.File.Bytes). A node's
// handles are released when its selection changes or the node is removed,
// mirroring the lifetime of input.files; a File that survives a re-selection
// keeps its handle, so an in-flight read is not cut off by the second event
// of the same pick (input then change).
const fileStore = {};          // handle -> File
const nodeFiles = new WeakMap(); // element -> Map(File -> handle)
let nextFileHandle = 1;

function stashFiles(el) {
    const prev = nodeFiles.get(el) || new Map();
    const next = new Map();
    const out = [];
    for (const f of el.files) {
        let h = prev.get(f);
        if (h === undefined) {
            h = nextFileHandle++;
            fileStore[h] = f;
        }
        next.set(f, h);
        out.push({handle: h, name: f.name, size: f.size, type: f.type, lastModified: f.lastModified});
    }
    for (const [f, h] of prev) {
        if (!next.has(f)) delete fileStore[h];
    }
    nodeFiles.set(el, next);
    return out;
}

// forget drops a removed subtree from nodeMap (and releases any file handles
// parked for inputs inside it).
function forget(node) {
    if (node._nodeID !== undefined) delete nodeMap[node._nodeID];
    releaseFiles(node);
    for (let c = node.firstChild; c; c = c.nextSibling) forget(c);
}

// nodeCount reports how many live nodes the runtime tracks (E2E leak check).
goowee.nodeCount = function nodeCount() {
    return Object.keys(nodeMap).length;
};

function releaseFiles(el) {
    const prev = nodeFiles.get(el);
    if (!prev) return;
    for (const h of prev.values()) delete fileStore[h];
    nodeFiles.delete(el);
}

// readFile resolves to the bytes of a parked File as a Uint8Array. Called by
// the Go bridge for core.File.Bytes.
goowee.readFile = function readFile(handle) {
    const f = fileStore[handle];
    if (!f) {
        return Promise.reject(new Error("file is no longer available (selection changed or input removed)"));
    }
    return f.arrayBuffer().then(function (buf) { return new Uint8Array(buf); });
};

function modifiers(e) {
    return {ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey, metaKey: e.metaKey};
}

function pointerish(e) {
    return Object.assign({clientX: e.clientX, clientY: e.clientY,
        offsetX: e.offsetX, offsetY: e.offsetY, button: e.button, buttons: e.buttons}, modifiers(e));
}

function touchList(list) {
    const out = [];
    for (const t of list || []) out.push({identifier: t.identifier, clientX: t.clientX, clientY: t.clientY});
    return out;
}

function buildPayload(type, e) {
    switch (type) {
        case "click": case "dblclick": case "auxclick": case "contextmenu":
        case "mousedown": case "mouseup": case "mousemove":
        case "mouseenter": case "mouseleave": case "mouseover": case "mouseout":
        case "dragstart": case "drag": case "dragend": case "dragenter":
        case "dragover": case "dragleave": case "drop":
            return pointerish(e);
        case "pointerdown": case "pointerup": case "pointermove": case "pointercancel":
        case "pointerenter": case "pointerleave": case "pointerover": case "pointerout":
            return Object.assign(pointerish(e),
                {pointerId: e.pointerId, pointerType: e.pointerType, pressure: e.pressure});
        case "wheel":
            return Object.assign(pointerish(e),
                {deltaX: e.deltaX, deltaY: e.deltaY, deltaZ: e.deltaZ, deltaMode: e.deltaMode});
        case "touchstart": case "touchmove": case "touchend": case "touchcancel":
            return Object.assign({touches: touchList(e.touches), changedTouches: touchList(e.changedTouches)},
                modifiers(e));
        case "input": case "change": {
            const t = e.target;
            const extra = type === "input" ? {inputType: e.inputType, isComposing: e.isComposing} : {};
            if (t.type === "checkbox" || t.type === "radio")
                return Object.assign({value: t.value, checked: t.checked}, extra);
            if (t.type === "file" && t.files)
                return {value: t.value, files: stashFiles(t)};
            return Object.assign({value: t.value}, extra);
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
        case "keydown": case "keyup": case "keypress":
            return Object.assign({key: e.key, code: e.code, repeat: e.repeat,
                isComposing: e.isComposing}, modifiers(e));
        case "scroll":
            return {scrollTop: e.target.scrollTop, scrollLeft: e.target.scrollLeft};
        default:
            return {};
    }
}

// dispatchToGo delivers e to the Go handlers for it: every ancestor of the
// target that has a handler, innermost first (bubbling), until one stops
// propagation — or, for a non-bubbling event, the target's handler only.
function dispatchToGo(type, e, targetOnly) {
    const payload = JSON.stringify(buildPayload(type, e));
    for (let el = e.target; el; el = targetOnly ? null : el.parentNode) {
        if (el._nodeID === undefined) continue;
        const r = handleEvent(el._nodeID, type, payload);
        if (!r || !r.handled) continue;
        if (r.preventDefault && e.cancelable) e.preventDefault();
        if (r.selectOnFocus && type === "focus" && el.select) el.select();
        if (r.stopPropagation) {
            e.stopPropagation();
            return;
        }
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
    // Reads (ref.Get) are answered after every write in the batch has been
    // applied, so a handler that updates state and measures sees the result.
    const reads = [];
    for (const mut of muts) {
        let el;
        switch (mut.type) {
            case 0: // CreateElement
                if (preexistingNodes[mut.nodeId]) {
                    el = preexistingNodes[mut.nodeId];
                    delete preexistingNodes[mut.nodeId];
                } else if (mut.value === "#text") {
                    el = document.createTextNode("");
                } else if (mut.value === "#comment") {
                    el = document.createComment(""); // a scope's end anchor
                } else if (mut.ns) {
                    el = document.createElementNS(mut.ns, mut.value); // SVG etc.
                } else {
                    el = document.createElement(mut.value);
                }
                el._nodeID = mut.nodeId;
                nodeMap[mut.nodeId] = el;
                break;
            case 1: // RemoveNode — Go sends only a removed subtree's roots
                el = nodeMap[mut.nodeId];
                if (el) {
                    if (el.parentNode) el.parentNode.removeChild(el);
                    forget(el);
                }
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
                // The reference node's actual parent wins: a scope's content is
                // inserted before its anchor wherever the anchor lives (inside a
                // portal target, the root container, …).
                const child = nodeMap[mut.childId];
                const ref = mut.refId ? nodeMap[mut.refId] : null;
                const parent = ref && ref.parentNode ? ref.parentNode
                    : (mut.nodeId === 0 ? getRoot() : nodeMap[mut.nodeId]);
                if (parent && child) {
                    if (ref && ref.parentNode === parent) parent.insertBefore(child, ref);
                    else parent.appendChild(child);
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
            case 10: // Read — deferred to after the batch (see below)
                reads.push(mut);
                break;
            case 7: { // Hydrate — claim a server-rendered node by id
                const pre = preexistingNodes[mut.nodeId];
                const wantText = mut.value === "#text";
                const wantComment = mut.value === "#comment";
                // The claimed node must match what the client expects; a wrong
                // tag/type means the server and client rendered different trees.
                const matches = pre && (wantText ? pre.nodeType === 3
                    : wantComment ? pre.nodeType === 8
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
                    el = wantText ? document.createTextNode("")
                        : wantComment ? document.createComment("")
                        : document.createElement(mut.value);
                }
                el._nodeID = mut.nodeId;
                nodeMap[mut.nodeId] = el;
                break;
            }
        }
    }
    if (reads.length === 0) return undefined;
    return JSON.stringify(reads.map(function (mut) {
        return {req: mut.value, value: readProp(nodeMap[mut.nodeId], mut.key)};
    }));
};

// readProp evaluates a ref.Get: a method is called with no arguments, a
// property is read as-is. Anything JSON can't carry (undefined, a function, a
// throwing getter, a missing node) comes back as null.
function readProp(el, key) {
    if (!el) return null;
    try {
        const v = typeof el[key] === "function" ? el[key]() : el[key];
        return v === undefined ? null : v;
    } catch (_) {
        return null;
    }
}
