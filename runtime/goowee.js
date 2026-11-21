const nodeMap = {};
let rootNode = null;

function getRoot() {
    if (!rootNode) rootNode = rootContainers[0] || document.getElementById("root");
    return rootNode;
}

// Roots (#154). Go mounts each root into a container it registers with
// goowee.mountRoot under a parent id: 0 for the app's main root (#root by
// default), negative ids for the others (islands, extra mounts). Mutations
// that attach to a root name that id.
const rootContainers = {};
let rootsRegistered = false;
const pendingClear = []; // client-rendered roots, emptied by the first frame that paints them

function containerOf(id) {
    if (id > 0) return nodeMap[id];
    if (id === 0) return getRoot();
    return rootContainers[id] || null;
}

// Boot instrumentation (roadmap 3.2). All marks are one-shot and namespaced so
// they never collide with app or browser marks, and SPA route changes don't
// overwrite them. See docs/plans/boot-latency-measurement.md.
const goowee = (window.goowee = window.goowee || {});

// The mutation protocol this runtime understands. bridge.Run compares it with
// core.ProtocolVersion before mounting: a mismatch means this page serves an
// older goowee.js, which would misapply mutations from the linked binary.
goowee.protocolVersion = 1;
goowee.protocolMismatch = function protocolMismatch(appVersion, appProtocol) {
    const page = goowee.protocolVersion;
    const err = bootError(
        "this page serves goowee.js with protocol " + (page === undefined ? "no version marker" : page) +
        ", but main.wasm was built with goowee " + appVersion + " (protocol " + appProtocol +
        "). Copy the matching assets with: go run github.com/yogisalomo/goowee/cmd/goowee assets ./web"
    );
    goowee.bootError = err;
    releaseEarly();
    console.error(err.message);
    window.dispatchEvent(new CustomEvent("goowee:boot-error", {detail: err}));
};

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
    if (goowee.noDevTools) {
        console.warn("[goowee] built without dev tools (-tags goowee_nodev): no inspector in this build.");
        return null;
    }
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

// goowee.changes() prints and returns dev mode's change log: each signal write
// (oldest first, the last 500) with the value before and after and the code
// that made it. goowee.changes({ clear: true }) empties it.
goowee.changes = function changes(opts) {
    if (goowee.noDevTools) {
        console.warn("[goowee] built without dev tools (-tags goowee_nodev): no change log in this build.");
        return null;
    }
    if (typeof goowee.changesGo !== "function") {
        console.warn(
            "[goowee] change log unavailable — enable dev mode with ?goowee-dev or localStorage.setItem('goowee-dev','1') and reload."
        );
        return null;
    }
    const rows = goowee.changesGo(!!(opts && opts.clear));
    if (!(opts && opts.clear)) console.table(rows);
    return rows;
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
// "main.wasm", resolved against <base> like the other assets). `opts.transport`
// sets goowee.transport, how Go sends DOM updates: "binary" (the default) or
// "json", the earlier format, kept as a fallback. The bridge reads it once at
// startup and sets it to the transport it actually uses.
//
// A module served as application/wasm is compiled while it downloads
// (instantiateStreaming). Browsers refuse that for any other Content-Type, so
// for a host that doesn't send it the bytes are downloaded first and compiled
// after, with a warning (#109). A failure (network, HTTP status, a body that
// isn't WebAssembly) is reported once, as an actionable console error, a
// "goowee:boot-error" event on window, and goowee.bootError. The returned
// promise still rejects for a caller that chains on it, e.g. to replace the
// loading placeholder with a message.
// Early interaction (#131). A server-rendered page looks ready before the
// WASM has booted: clicks and keystrokes there would reach nobody, a form
// would submit natively, and text typed into a bound input would stay out of
// its signal. From the moment goowee.js loads (in <head>, before the body is
// parsed) until the first frame, clicks, keydowns and submits on server
// markup ([data-node-id]) are queued (up to 100), and a submit is held back.
// After the first frame, form controls whose state differs from the server's
// are announced to Go as input and change events, so bound signals take what
// the user typed or ticked, and then the queued events are dispatched in
// order: each reaches Go's handlers once. A held submit is resubmitted with
// requestSubmit, so the app's OnSubmit handles it, or the form submits
// natively if no handler does. data-goowee-pending is on <html> meanwhile,
// for styling a page that isn't interactive yet.
const early = {active: false, scheduled: false, queue: []};
const EARLY_TYPES = ["click", "keydown", "submit"];

function captureEarly(e) {
    const t = e.target;
    if (!early.active || !t || typeof t.closest !== "function" || !t.closest("[data-node-id]")) return;
    if (e.type === "submit") e.preventDefault();
    if (early.queue.length < 100) early.queue.push(e);
}

function startEarly() {
    early.active = true;
    for (const type of EARLY_TYPES) document.addEventListener(type, captureEarly, true);
    document.documentElement.setAttribute("data-goowee-pending", "");
}

function stopEarly() {
    early.active = false;
    for (const type of EARLY_TYPES) document.removeEventListener(type, captureEarly, true);
    document.documentElement.removeAttribute("data-goowee-pending");
    const queue = early.queue;
    early.queue = [];
    return queue;
}

function finishEarly() {
    const queue = stopEarly();
    const roots = new Set(Object.values(rootContainers));
    if (getRoot()) roots.add(getRoot());
    for (const root of roots) if (root.querySelector("[data-node-id]")) adoptFormState(root);
    for (const e of queue) {
        const t = e.target;
        if (!t || !t.isConnected) continue; // the page it belonged to is gone
        if (e.type === "submit") {
            resubmit(t, e.submitter);
            continue;
        }
        dispatchToGo(e.type, e, false);
    }
}

// adoptFormState tells Go about form controls the user changed before the
// app ran: a text field whose value isn't its server-rendered value, a box or
// radio whose checked state isn't, a select whose selection isn't.
function adoptFormState(root) {
    for (const el of root.querySelectorAll("input, textarea, select")) {
        if (el._nodeID === undefined || el.type === "file") continue;
        let changed;
        if (el.type === "checkbox" || el.type === "radio") changed = el.checked !== el.defaultChecked;
        else if (el.nodeName === "SELECT") changed = Array.prototype.some.call(el.options, function (o) { return o.selected !== o.defaultSelected; });
        else changed = el.value !== el.defaultValue;
        if (!changed) continue;
        for (const type of ["input", "change"]) {
            dispatchToGo(type, {type: type, target: el, cancelable: false, inputType: "", isComposing: false,
                preventDefault: function () {}, stopPropagation: function () {}}, false);
        }
    }
}

function resubmit(form, submitter) {
    if (typeof form.requestSubmit === "function") {
        form.requestSubmit(submitter && submitter.isConnected && submitter.form === form ? submitter : undefined);
    } else {
        form.submit();
    }
}

// The page never became interactive (the WASM failed to load): let held
// submits go through natively, as they would have without goowee.
function releaseEarly() {
    if (!early.active) return;
    for (const e of stopEarly()) {
        if (e.type === "submit" && e.target.isConnected) resubmit(e.target, e.submitter);
    }
}

if (typeof document !== "undefined") startEarly();

// opts.when says when a server-rendered page starts the download (#130): the
// page is readable before the WASM arrives, and a page that is mostly read
// can keep the download from competing with its CSS, fonts and images.
//   "load"         at once (the default)
//   "idle"         after the load event, when the browser is idle
//                  (requestIdleCallback, at most opts.idleTimeout ms, 2000 by
//                  default, after load)
//   "visible"      when opts.target (an element or a selector; #root by
//                  default) scrolls into view
//   "interaction"  at the first pointerdown, keydown or focusin anywhere
// Until then, and while the module downloads, clicks, keys and submits on the
// server markup are queued and replayed once the app is live (#131), so the
// interaction that starts the boot isn't lost. A client-only page (nothing
// server-rendered to show meanwhile) always boots at once. goowee.bootWhen
// reports the mode in effect.
goowee.boot = function boot(opts) {
    opts = opts || {};
    let when = opts.when || "load";
    // ?goowee-boot=idle (or load, visible, interaction) overrides it, to try
    // or measure a mode on a live page (make boot does).
    const forced = /[?&]goowee-boot=(load|idle|visible|interaction)(?:&|$)/.exec(location.search);
    if (forced) when = forced[1];
    if (when !== "load" && !goowee.serverRendered()) when = "load";
    goowee.bootWhen = when;
    if (when === "load") return startBoot(opts);
    const started = new Promise(function (resolve) { bootTrigger(when, opts, resolve); })
        .then(function () { return startBoot(opts); });
    started.catch(function () {}); // reported by startBoot, as for "load"
    return started;
};

// bootTrigger calls go once the moment opts.when names has come.
function bootTrigger(when, opts, go) {
    let done = false;
    const fire = function () {
        if (done) return;
        done = true;
        mark("goowee:boot-trigger");
        go();
    };
    switch (when) {
        case "idle": {
            const idle = function () {
                if (typeof requestIdleCallback === "function") requestIdleCallback(fire, {timeout: opts.idleTimeout || 2000});
                else setTimeout(fire, 1);
            };
            if (document.readyState === "complete") idle();
            else window.addEventListener("load", idle, {once: true});
            return;
        }
        case "visible": {
            let el = opts.target || "#root";
            if (typeof el === "string") el = document.querySelector(el);
            if (!el || typeof IntersectionObserver !== "function") { fire(); return; }
            const io = new IntersectionObserver(function (entries) {
                if (entries.some(function (e) { return e.isIntersecting; })) { io.disconnect(); fire(); }
            });
            io.observe(el);
            return;
        }
        case "interaction": {
            const types = ["pointerdown", "keydown", "focusin"];
            const on = function () {
                for (const t of types) document.removeEventListener(t, on, true);
                fire();
            };
            for (const t of types) document.addEventListener(t, on, {capture: true, passive: true});
            return;
        }
    }
    console.warn('goowee: goowee.boot: unknown when "' + when + '"; booting now');
    fire();
}

function startBoot(opts) {
    if (opts.transport) goowee.transport = opts.transport;
    // calls: "callbacks" keeps every call into Go on syscall/js callbacks,
    // as before 0.4 (for comparing, or a problem with the exports).
    const wasmURL = opts.wasm || "main.wasm";
    const go = new Go();
    mark("goowee:fetch-start");
    const started = fetch(wasmURL)
        .catch(function (err) {
            throw bootError("couldn't download " + wasmURL + " (" + err + ")", err);
        })
        .then(function (resp) {
            if (!resp.ok) {
                throw bootError("downloading " + resp.url + " failed: HTTP " + resp.status + " " + resp.statusText);
            }
            const type = (resp.headers.get("content-type") || "").split(";")[0].trim().toLowerCase();
            if (type === "application/wasm" && typeof WebAssembly.instantiateStreaming === "function") {
                return WebAssembly.instantiateStreaming(resp, go.importObject)
                    .catch(function (err) { throw wasmError(resp.url, type, err); });
            }
            console.warn("goowee: " + resp.url + " is served as " + (type ? "'" + type + "'" : "no Content-Type") +
                ", not application/wasm, so it is compiled after downloading instead of while it downloads. " +
                "Configure the server to send Content-Type: application/wasm.");
            return resp.arrayBuffer()
                .then(function (bytes) { return WebAssembly.instantiate(bytes, go.importObject); })
                .catch(function (err) { throw wasmError(resp.url, type, err); });
        })
        .then(function (result) {
            mark("goowee:instantiated");
            mark("goowee:run");
            if (opts.calls !== "callbacks") goowee.useInstance(go, result.instance);
            go.run(result.instance);
        })
        .catch(function (err) {
            if (!err || !err.gooweeBoot) err = bootError("starting " + wasmURL + " failed (" + err + ")", err);
            goowee.bootError = err;
            releaseEarly();
            console.error(err.message);
            window.dispatchEvent(new CustomEvent("goowee:boot-error", {detail: err}));
            throw err;
        });
    // Reported above; don't also surface as an unhandled rejection.
    started.catch(function () {});
    return started;
};

function bootError(message, cause) {
    const err = new Error("goowee: " + message);
    err.gooweeBoot = true;
    if (cause) err.cause = cause;
    return err;
}

// wasmError explains a compile or instantiate failure; the usual cause is a
// server answering the wasm URL with an HTML page (an SPA fallback).
function wasmError(url, type, err) {
    const hint = type === "text/html"
        ? " The server answered with an HTML page: check that " + url + " serves the built main.wasm and isn't caught by an index.html fallback."
        : "";
    return bootError(url + " isn't a WebAssembly module this page can run (" + err + ")." + hint, err);
}

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
        trigger: at("goowee:boot-trigger"), // a lazy boot's moment (#130)
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
            // The page's load event, which a download started at once delays.
            load: (performance.getEntriesByType("navigation")[0] || {}).loadEventEnd || null,
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
// For a pure client app (no SSR) there are no nodes to claim, and #root
// belongs to the app: whatever the page put there (a "Loading…" splash shown
// while the WASM downloads) is removed here, in the same task that appends
// the app's first frame, so nothing paints in between. "Server-rendered" is
// decided exactly as bridge.Run decides it: a [data-node-id] in the document.
// serverRendered reports whether the document holds server markup for this
// page, to hydrate: goowee's node ids are present, and a prerendered page
// (ssr.Export marks it with <meta name="goowee-prerendered">) was rendered
// for the current path. A static host serves its 404.html, prerendered for a
// path nothing matches, for every URL it has no file for, including a valid
// page nobody prerendered; that markup is another page's, so the app renders
// on the client instead. bridge.Run asks the same question.
goowee.serverRendered = function serverRendered() {
    if (!document.querySelector("[data-node-id]")) return false;
    const meta = document.querySelector('meta[name="goowee-prerendered"]');
    if (!meta) return true;
    const norm = function (p) {
        p = p.replace(/\/index\.html$/, "/");
        return p.length > 1 && p.endsWith("/") ? p.slice(0, -1) : p;
    };
    let here = location.pathname;
    try { here = decodeURI(here); } catch (_) {}
    return norm(meta.getAttribute("content") || "") === norm(here);
};

// claimServerNodes collects the server-rendered nodes inside container so the
// first frame's Hydrate mutations can claim them. base is added to the ids
// the markup carries: an island is server-rendered on its own (ids from 1),
// while the client numbers every root's nodes from one counter (#154).
function claimServerNodes(container, base) {
    container.querySelectorAll("[data-node-id]").forEach(el => {
        preexistingNodes[base + parseInt(el.getAttribute("data-node-id"), 10)] = el;
    });

    // Text nodes can't carry attributes; SSR marks each with a preceding
    // <!--g{id}--> comment. Claim the comment's next sibling, then drop the
    // marker so the live DOM matches the client tree. Scope end anchors are
    // <!--/{id}--> comments that stay in the DOM: claim the comment itself.
    const walker = document.createTreeWalker(container, NodeFilter.SHOW_COMMENT);
    const markers = [];
    while (walker.nextNode()) {
        const data = walker.currentNode.data;
        let m = /^g(\d+)$/.exec(data);
        if (m) { markers.push([base + parseInt(m[1], 10), walker.currentNode]); continue; }
        m = /^\/(\d+)$/.exec(data);
        if (m) preexistingNodes[base + parseInt(m[1], 10)] = walker.currentNode;
    }
    for (const [id, comment] of markers) {
        const next = comment.nextSibling;
        if (next && next.nodeType === 3 /* TEXT_NODE */) {
            preexistingNodes[id] = next;
            comment.remove();
        } else {
            // An empty text node has no markup: the marker is all the server
            // wrote (TextS of ""). Claim an empty text node in its place.
            const empty = document.createTextNode("");
            comment.replaceWith(empty);
            preexistingNodes[id] = empty;
        }
    }
}

// goowee.mountRoot registers a root's container before Go renders into it.
// With hydrate, the container's server markup is claimed (ids offset by
// base); otherwise its content (a placeholder) is removed by the first frame
// that paints the root, so nothing blank shows in between.
goowee.mountRoot = function mountRoot(parentId, container, base, hydrate) {
    rootsRegistered = true;
    rootContainers[parentId] = container;
    if (parentId === 0) rootNode = container;
    if (hydrate) {
        mark("goowee:hydrate-start");
        claimServerNodes(container, base);
        mark("goowee:hydrate-end");
        return;
    }
    if (parentId === 0) {
        // Server markup for another page (a static host's 404.html, say)
        // goes too, with its head metadata.
        for (const el of document.head.querySelectorAll("[data-node-id]")) el.remove();
    }
    pendingClear.push(container);
};

// goowee.unmountRoot forgets a root's container once its content is removed.
goowee.unmountRoot = function unmountRoot(parentId) {
    if (parentId !== 0) delete rootContainers[parentId];
};

function flushPendingClear() {
    while (pendingClear.length > 0) pendingClear.shift().replaceChildren();
}

function hydrateOnce() {
    if (hydrated) return;
    hydrated = true;
    flushPendingClear();
    if (rootsRegistered) return; // the Go side registers its roots itself (goowee.mountRoot)
    mark("goowee:hydrate-start");
    const root = getRoot();
    if (!root) { mark("goowee:hydrate-end"); return; }
    if (!goowee.serverRendered()) {
        // Server markup for another page (a static host's 404.html, say)
        // goes too, so the app's first frame starts from an empty page.
        for (const el of document.querySelectorAll("[data-node-id]")) el.remove();
        root.replaceChildren();
        mark("goowee:hydrate-end");
        return;
    }

    claimServerNodes(root, 0);
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

// Files from a file input's change/input, a paste or a drop: the payload
// carries each file's metadata plus a handle, and the File objects themselves
// are parked here so Go can read their bytes later (goowee.readFile, via
// core.File.Bytes). A node's handles are released when its next selection,
// paste or drop replaces them, or when the node is removed, mirroring the
// lifetime of input.files; a File that survives a re-selection keeps its
// handle, so an in-flight read is not cut off by the second event of the same
// pick (input then change).
const fileStore = {};          // handle -> File
const nodeFiles = new WeakMap(); // element -> Map(File -> handle)
let nextFileHandle = 1;

function stashFiles(el, files) {
    const prev = nodeFiles.get(el) || new Map();
    const next = new Map();
    const out = [];
    for (const f of files) {
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
    if (node.nodeName === "CANVAS") forgetCanvas(node);
    for (let c = node.firstChild; c; c = c.nextSibling) forget(c);
}

// nodeCount reports how many live nodes the runtime tracks (E2E leak check).
goowee.nodeCount = function nodeCount() {
    return Object.keys(nodeMap).length;
};

// Transitions (#156). A node with data-goowee-transition animates on insert
// and on removal. The Go side tells us about removals with RemoveNodeAfter;
// enter classes are discovered from the attribute when the node is inserted.
// goowee.transitionTimeout is the fallback when neither transitionend nor
// animationend fires (e.g. the class has no CSS rule).
goowee.transitionTimeout = 1000;
const reducedMotionQuery = typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-reduced-motion: reduce)") : null;
function motionReduced() {
    return !!(reducedMotionQuery && reducedMotionQuery.matches);
}

// startEnterTree adds "<name>-enter" to every transitionable node in a newly
// inserted subtree, then removes it on the next frame so CSS transitions begin
// from that state. Hydrated nodes are marked _goHydrated and stay untouched.
function startEnterTree(node) {
    if (!node || node.nodeType !== 1) return;
    const all = node.hasAttribute("data-goowee-transition")
        ? [node].concat(Array.prototype.slice.call(node.querySelectorAll("[data-goowee-transition]")))
        : Array.prototype.slice.call(node.querySelectorAll("[data-goowee-transition]"));
    for (const el of all) {
        if (el._goEntered || el._goInert || el._goHydrated || motionReduced()) continue;
        const name = el.getAttribute("data-goowee-transition");
        if (!name) continue;
        el._goEntered = true;
        el.classList.add(name + "-enter");
        requestAnimationFrame(function () {
            requestAnimationFrame(function () {
                if (el.isConnected) el.classList.remove(name + "-enter");
            });
        });
    }
}

// removeAfterTransition keeps a removed node in DOM for its leave animation.
// Go has already disposed the subtree's reactive state, so the node is made
// inert before the animation starts and is forgotten once it ends.
function removeAfterTransition(node, name) {
    if (!node) return;
    const finish = function () {
        if (node.parentNode) node.parentNode.removeChild(node);
        forget(node);
        delete nodeMap[node._nodeID];
    };
    if (!name || motionReduced()) { finish(); return; }
    node.setAttribute("inert", "");
    node._goInert = true;
    node.classList.add(name + "-leave");
    let done = false;
    const end = function (e) {
        if (e && e.target !== node) return;
        if (done) return;
        done = true;
        node.removeEventListener("transitionend", end);
        node.removeEventListener("animationend", end);
        clearTimeout(timer);
        finish();
    };
    node.addEventListener("transitionend", end);
    node.addEventListener("animationend", end);
    const timer = setTimeout(end, goowee.transitionTimeout);
}

function releaseFiles(el) {
    const prev = nodeFiles.get(el);
    if (!prev) return;
    for (const h of prev.values()) delete fileStore[h];
    nodeFiles.delete(el);
}

// fileValue is the parked File itself (undefined once released), so Go can
// hand it to fetch/FormData or a JS library without copying its bytes
// (bridge.FileValue, goowee/net's Form).
goowee.fileValue = function fileValue(handle) {
    return fileStore[handle];
};

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

// fileOwner is the element a paste's or drop's files are parked under: the
// nearest goowee node (so they're released with it), else the target.
function fileOwner(target) {
    for (let el = target; el; el = el.parentNode) {
        if (el._nodeID !== undefined) return el;
    }
    return target;
}

// selectedText is what a cut or copy takes: a text field's selection, else
// the document's.
function selectedText(t) {
    if (t && typeof t.value === "string" && typeof t.selectionStart === "number") {
        return t.value.slice(t.selectionStart, t.selectionEnd);
    }
    const sel = document.getSelection();
    return sel ? sel.toString() : "";
}

function touchList(list) {
    const out = [];
    for (const t of list || []) out.push({identifier: t.identifier, clientX: t.clientX, clientY: t.clientY});
    return out;
}

function isPlainObject(v) {
    if (v === null || typeof v !== "object") return false;
    const p = Object.getPrototypeOf(v);
    return p === Object.prototype || p === null;
}

function buildPayload(type, e) {
    switch (type) {
        case "click": case "dblclick": case "auxclick": case "contextmenu":
        case "mousedown": case "mouseup": case "mousemove":
        case "mouseenter": case "mouseleave": case "mouseover": case "mouseout":
            return pointerish(e);
        case "dragstart": case "drag": case "dragend": case "dragenter":
        case "dragover": case "dragleave": case "drop": {
            // The browser exposes a drag's types throughout, but its data
            // and files only on drop.
            const p = pointerish(e), dt = e.dataTransfer;
            if (dt) {
                p.types = Array.from(dt.types || []);
                if (type === "drop") {
                    p.text = dt.getData("text/plain");
                    p.files = stashFiles(fileOwner(e.target), dt.files || []);
                }
            }
            return p;
        }
        case "paste": {
            const cd = e.clipboardData;
            if (!cd) return {};
            return {text: cd.getData("text/plain"), types: Array.from(cd.types || []),
                files: stashFiles(fileOwner(e.target), cd.files || [])};
        }
        case "cut": case "copy":
            return {text: selectedText(e.target)};
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
                return {value: t.value, files: stashFiles(t, t.files)};
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
            // A custom event's detail, when it's a plain object, becomes the
            // handler's e.Data (goowee:canvasresize carries the new size).
            if (typeof CustomEvent !== "undefined" && e instanceof CustomEvent && isPlainObject(e.detail))
                return e.detail;
            return {};
    }
}

// dispatchToGo delivers e to the Go handlers for it: every ancestor of the
// target that has a handler, innermost first (bubbling), until one stops
// propagation — or, for a non-bubbling event, the target's handler only.
// Go marks the nodes that have a handler with a Listen mutation (el._goEvents,
// a Set of event types), and only those are asked: a click whose path has no
// handler costs no call into Go and builds no payload (#132). The payload is
// built once, at the first node that has a handler. goowee.listenSets is set
// by a Go side that sends Listen; an older one is asked about every node, as
// before.
// goowee.eventCalls counts the calls into Go's event handlers, either way.
goowee.eventCalls = 0;
function dispatchToGo(type, e, targetOnly) {
    let payload = null;
    for (let el = e.target; el; el = targetOnly ? null : el.parentNode) {
        // A node kept in DOM for its leave animation is inert: no event on or
        // inside it reaches Go, even before browsers that support `inert` do.
        if (el._goInert) return 0;
        if (el._nodeID === undefined) continue;
        if (goowee.listenSets && !(el._goEvents && el._goEvents.has(type))) continue;
        if (payload === null) payload = JSON.stringify(buildPayload(type, e));
        goowee.eventCalls++;
        const r = viaExports ? callEvent(el._nodeID, type, payload) : decodeDispatch(handleEvent(el._nodeID, type, payload));
        if (!(r & 1)) continue;
        if ((r & 2) && e.cancelable) e.preventDefault();
        if ((r & 8) && type === "focus" && el.select) el.select();
        if (r & 4) {
            e.stopPropagation();
            return;
        }
    }
}

// Calling Go through its exports (#133, ADR-031). syscall/js callbacks
// (handleEvent, the frame js.Func) cost about 18 µs a call; the goowee_event
// and goowee_frame exports cost about 20 ns. goowee.boot hands the instance
// over with useInstance; a page that instantiates the module itself can call
// goowee.useInstance(go, instance) before go.run(instance) to get the same,
// and otherwise everything goes through syscall/js as before. Go opts in at
// startup with goowee.useExports(version).
let goInst = null, goRunner = null, wasmExports = null, viaExports = false;
let resumeQueued = false;
goowee.calls = "callbacks"; // "exports" once Go opts in
goowee.useInstance = function useInstance(go, instance) {
    goRunner = go;
    goInst = instance;
};
goowee.useExports = function useExports(version) {
    const ex = goInst && goInst.exports;
    if (version !== 1 || !ex || typeof ex.goowee_event !== "function" || typeof ex.goowee_frame !== "function" ||
        typeof ex.goowee_scratch !== "function" || !(ex.mem instanceof WebAssembly.Memory) ||
        !goRunner || typeof goRunner._resume !== "function") return false;
    wasmExports = ex;
    viaExports = true;
    goowee.calls = "exports";
    return true;
};

// resumeGo lets goroutines run that a call into an export started or woke: an
// export returns without running the Go scheduler. Once per burst of calls, in
// a microtask, so they run before the browser renders or handles the next
// event, as they did when callbacks ran the scheduler themselves.
function resumeGo() {
    if (resumeQueued) return;
    resumeQueued = true;
    queueMicrotask(function () {
        resumeQueued = false;
        if (!goRunner.exited) goRunner._resume();
    });
}

// goowee.requestFrame is how Go asks for a frame when it uses the exports.
function runFrame() {
    wasmExports.goowee_frame();
    resumeGo();
}
goowee.requestFrame = function requestFrame(hidden) {
    if (hidden) setTimeout(runFrame, 0);
    else requestAnimationFrame(runFrame);
};

// callEvent is handleEvent through goowee_event: the type and the payload go
// into Go's scratch buffer as UTF-8, one after the other.
const eventText = new TextEncoder();
let scratchPtr = 0, scratchLen = 0;
function callEvent(id, type, payload) {
    // At most three UTF-8 bytes per UTF-16 code unit.
    const need = 3 * (type.length + payload.length);
    if (need > scratchLen || scratchPtr === 0) {
        scratchPtr = wasmExports.goowee_scratch(need) >>> 0;
        scratchLen = need;
    }
    // A fresh view: Go's memory may have grown since the last call.
    const u8 = new Uint8Array(wasmExports.mem.buffer, scratchPtr, scratchLen);
    const t = eventText.encodeInto(type, u8).written;
    const p = eventText.encodeInto(payload, u8.subarray(t)).written;
    const r = wasmExports.goowee_event(id, t, p) >>> 0;
    resumeGo();
    return r;
}

// handleEvent may answer with packed bits (Go reads this flag at startup).
goowee.dispatchBits = true;

// decodeDispatch turns handleEvent's result into bits: handled 1,
// preventDefault 2, stopPropagation 4, selectOnFocus 8. Go returns the bits;
// an older Go side returned an object.
function decodeDispatch(r) {
    if (typeof r === "number") return r;
    if (!r || !r.handled) return 0;
    return 1 | (r.preventDefault ? 2 : 0) | (r.stopPropagation ? 4 : 0) | (r.selectOnFocus ? 8 : 0);
}

// claimTitle makes the app's <title> the document's: document.title reads the
// first <title> in the document, so the page's own (the HTML shell's, or the
// server-rendered one, neither created by goowee) would shadow the app's. It
// removes those; titles goowee rendered stay, in tree order, as on the server.
function claimTitle() {
    for (const t of document.head.querySelectorAll(":scope > title")) {
        if (t._nodeID === undefined) t.remove();
    }
}

// moveNode inserts child into parent before ref (null = at the end). A node
// already in the document is moved with moveBefore where the browser has it —
// an atomic move that keeps focus, text selection, scroll position and running
// animations. It reports whether it fell back to a plain remove + insert that
// moved a live node, which drops focus (restoreFocus repairs that).
function moveNode(parent, child, ref) {
    const live = child.isConnected;
    if (live && parent.isConnected && typeof parent.moveBefore === "function") {
        try { parent.moveBefore(child, ref); return false; } catch (_) { /* fall through */ }
    }
    parent.insertBefore(child, ref);
    return live;
}

// saveFocus records the focused element and its text selection before a batch,
// for restoreFocus.
function saveFocus() {
    const el = document.activeElement;
    if (!el || el === document.body || el === document.documentElement) return null;
    const s = {el: el, start: null, end: null, dir: null};
    try {
        if (typeof el.selectionStart === "number") {
            s.start = el.selectionStart; s.end = el.selectionEnd; s.dir = el.selectionDirection;
        }
    } catch (_) {} // some input types have no selection
    return s;
}

// restoreFocus puts focus and the text selection back on an element a batch
// moved without moveBefore (a keyed-list reorder), when it is still in the
// document and nothing else took focus. A removed element stays unfocused.
function restoreFocus(s) {
    if (!s || !s.el.isConnected) return;
    const now = document.activeElement;
    if (now === s.el || (now && now !== document.body && now !== document.documentElement)) return;
    try { s.el.focus({preventScroll: true}); } catch (_) { return; }
    if (s.start !== null) try { s.el.setSelectionRange(s.start, s.end, s.dir || undefined); } catch (_) {}
}

// A batch of DOM updates is applied in one pass, whichever transport carried
// it: startBatch, applyMutation for each update in order, then finishBatch.
function startBatch() {
    hydrateOnce();
    flushPendingClear();
    // First flush = first paint of app-produced DOM = interactive (roadmap 3.2).
    if (!goowee._interactive) {
        goowee._interactive = true;
        mark("goowee:interactive");
    }
    // Reads (ref.Get) are answered after every write in the batch has been
    // applied, so a handler that updates state and measures sees the result.
    // Calls with arguments (ref.Call) also wait for the batch's writes: a
    // setSelectionRange must land after the value it positions the caret in.
    return {reads: [], calls: [], focus: saveFocus(), movedLive: false, focusCommand: false};
}

function finishBatch(b) {
    // An explicit ref.Focus/Blur in the batch decides focus; otherwise a move
    // that dropped it is undone.
    if (b.movedLive && !b.focusCommand) restoreFocus(b.focus);
    // The first frame has claimed the server's nodes and told the runtime
    // which have handlers: hand Go what the user did before it (#131), once
    // this call has returned to Go.
    if (early.active && !early.scheduled) {
        early.scheduled = true;
        setTimeout(finishEarly, 0);
    }
    for (const mut of b.calls) {
        // Node 0 is the document: core.ScrollWindow uses it for window
        // scrolling (h.VirtualList's handle).
        if (mut.nodeId === 0 && mut.key === "scrollTo") {
            try { window.scrollTo.apply(window, Array.isArray(mut.value) ? mut.value : []); }
            catch (err) { console.error("goowee: ScrollWindow", err); }
            continue;
        }
        // h.VirtualList opts in to row measurement after mount; apps without
        // one never create a MutationObserver or ResizeObserver.
        if (mut.key === "gooweeObserveRows") {
            watchVirtualRows(nodeMap[mut.nodeId]);
            continue;
        }
        if (mut.key === "gooweeUnobserveRows") {
            stopVirtualRows(mut.nodeId);
            continue;
        }
        const el = nodeMap[mut.nodeId];
        if (!el || typeof el[mut.key] !== "function") continue;
        // A throwing method (setSelectionRange on an email input, say) must
        // not abort the batch.
        try { el[mut.key].apply(el, Array.isArray(mut.value) ? mut.value : []); }
        catch (err) { console.error("goowee: ref.Call", mut.key, err); }
    }
    if (b.reads.length === 0) return undefined;
    return JSON.stringify(b.reads.map(function (r) {
        return {req: r.value, value: readProp(nodeMap[r.nodeId], r.key)};
    }));
}

// applyMutations applies a batch sent as JSON: the fallback transport
// (goowee.transport = "json"), and what a wasm built before the binary format
// sends.
window.applyMutations = function applyMutations(json) {
    const b = startBatch();
    for (const mut of JSON.parse(json)) applyMutation(mut, b);
    return finishBatch(b);
};

// goowee/canvas: Go records drawing commands into a buffer of 32-bit words
// and sends the whole buffer per frame (ref.Draw, canvas.View); goowee.draw
// replays it onto the <canvas>'s 2D context. The format is versioned:
// goowee.canvasVersion must match core.DrawVersion in Go. Each command is one
// word, op | operandCount << 8, then its operands (float32s, integers, or a
// string as a byte length plus UTF-8 padded to 4 bytes). Unknown ops are
// skipped, so an older runtime ignores newer commands instead of failing.
goowee.canvasVersion = 1;
const canvasState = new WeakMap(); // <canvas> -> {ctx, last, follow, cw, ch}
const liveCanvases = new Set();    // canvases drawn and still in the document
const canvasText = new TextDecoder();
const CANVAS_CAPS = ["butt", "round", "square"];
const CANVAS_JOINS = ["miter", "round", "bevel"];
const CANVAS_ALIGNS = ["start", "end", "left", "right", "center"];
const CANVAS_BASELINES = ["alphabetic", "top", "hanging", "middle", "ideographic", "bottom"];
const TAU = Math.PI * 2;
let canvasVersionWarned = false;

goowee.draw = function draw(u8, nbytes, nodeId) {
    const el = nodeMap[nodeId];
    if (!el || el.nodeName !== "CANVAS") return;
    let st = canvasState.get(el);
    if (!st) {
        // follow: the canvas follows its element's size (canvas.Fill); cw and
        // ch are that size in CSS pixels, from the ResizeObserver.
        st = {ctx: el.getContext("2d"), last: null, follow: false, cw: 0, ch: 0};
        canvasState.set(el, st);
    }
    if (!st.ctx) return; // the context couldn't be created (another context type owns the canvas)
    st.last = u8.slice(0, nbytes); // a copy: the caller reuses u8, and a resize can replay it
    if (!liveCanvases.has(el)) {
        liveCanvases.add(el);
        watchPixelRatio();
    }
    if (nbytes >= 8 && (new Uint32Array(st.last.buffer, 0, 2)[1] & 2) && !st.follow) {
        st.follow = true;
        const r = el.getBoundingClientRect();
        st.cw = r.width; st.ch = r.height;
        canvasResizer().observe(el);
    }
    replayCanvas(el, st);
};

// One ResizeObserver for every following canvas. On a size change it resizes
// the backing store, replays the last picture at once (so the canvas is never
// blank while Go redraws), and tells Go the new size with a
// goowee:canvasresize event (detail {width, height}, CSS pixels).
let canvasObserver = null;
function canvasResizer() {
    if (!canvasObserver) {
        canvasObserver = new ResizeObserver(function (entries) {
            for (const entry of entries) {
                const el = entry.target, st = canvasState.get(el);
                if (!st || !st.follow) continue;
                const w = entry.contentRect.width, h = entry.contentRect.height;
                if (w === st.cw && h === st.ch && st.told) continue;
                st.cw = w; st.ch = h; st.told = true;
                if (st.last) replayCanvas(el, st);
                el.dispatchEvent(new CustomEvent("goowee:canvasresize", {detail: {width: w, height: h}}));
            }
        });
    }
    return canvasObserver;
}

function forgetCanvas(el) {
    if (!liveCanvases.delete(el)) return;
    if (canvasObserver) canvasObserver.unobserve(el);
    canvasState.delete(el); // a canvas drawn again later starts afresh
}

// h.VirtualList: one ResizeObserver watches every mounted .vlist-row inside a
// [data-goowee-virtual-list] container. The real size is sent back to Go as a
// batched goowee:rowmeasure CustomEvent ({rows: [[index, width, height], …]})
// on the list element. A MutationObserver attaches to rows as they appear, so
// this works for rows mounted by the first frame as well as later window
// changes. Older runtimes simply never send the event; Go falls back to the
// configured fixed item height.
let rowObserver = null;
let rowScan = null;
const watchedVirtualLists = new Map(); // node id -> list element
function virtualRowResizer() {
    if (!rowObserver && typeof ResizeObserver === "function") {
        rowObserver = new ResizeObserver(function (entries) {
            const pending = new Map();
            for (const entry of entries) {
                const el = entry.target;
                if (!el.isConnected) continue;
                const list = el.closest && el.closest("[data-goowee-virtual-list]");
                if (!list) continue;
                const index = Number(el.getAttribute("data-row-index"));
                if (!Number.isFinite(index)) continue;
                const box = el.getBoundingClientRect();
                let rows = pending.get(list);
                if (!rows) { rows = []; pending.set(list, rows); }
                rows.push([index, box.width, box.height]);
            }
            for (const [list, rows] of pending) {
                list.dispatchEvent(new CustomEvent("goowee:rowmeasure", {
                    detail: {rows: rows}, bubbles: true
                }));
            }
        });
    }
    return rowObserver;
}
function virtualRowsIn(root, out) {
    if (root.nodeType !== 1) return out;
    if (root.classList && root.classList.contains("vlist-row")) out.push(root);
    if (root.querySelectorAll) out.push(...root.querySelectorAll(".vlist-row"));
    return out;
}
function watchVirtualRows(list) {
    const observer = virtualRowResizer();
    if (!observer || !list || list._nodeID === undefined || watchedVirtualLists.has(list._nodeID)) return;
    watchedVirtualLists.set(list._nodeID, list);
    // One observer for the opted-in lists only: apps without a VirtualList
    // never create it, and the scan never watches the whole document.
    if (!rowScan && typeof MutationObserver === "function") {
        rowScan = new MutationObserver(function (records) {
            for (const rec of records) {
                for (const node of rec.removedNodes) unobserveVirtualRows(node);
                for (const node of rec.addedNodes) {
                    for (const row of virtualRowsIn(node, [])) observeVirtualRow(row);
                }
            }
        });
    }
    if (rowScan) rowScan.observe(list, {childList: true, subtree: true});
    for (const row of virtualRowsIn(list, [])) observeVirtualRow(row);
}
function observeVirtualRow(row) {
    const observer = virtualRowResizer();
    if (!observer || row._goRowWatched) return;
    row._goRowWatched = true;
    observer.observe(row);
}
function unobserveVirtualRows(root) {
    if (rowObserver) {
        for (const row of virtualRowsIn(root, [])) {
            if (!row._goRowWatched) continue;
            rowObserver.unobserve(row);
            row._goRowWatched = false;
        }
    }
    if (root.nodeType === 1 && root.hasAttribute && root.hasAttribute("data-goowee-virtual-list")) {
        if (rowScan) rowScan.unobserve(root);
        watchedVirtualLists.delete(root._nodeID);
    }
}
function stopVirtualRows(id) {
    const list = watchedVirtualLists.get(id);
    if (list) unobserveVirtualRows(list);
}

// A VirtualList with VirtualListWindow(0) (or a window taller than itself)
// listens on window scroll; forward one coalesced scroll event to those lists
// so the Go handler reads the same window.scrollY the browser just applied.
let windowScrollQueued = false;
if (typeof window !== "undefined" && typeof window.addEventListener === "function") {
    window.addEventListener("scroll", function () {
        if (windowScrollQueued) return;
        windowScrollQueued = true;
        requestAnimationFrame(function () {
            windowScrollQueued = false;
            for (const list of document.querySelectorAll("[data-goowee-scroll='window']")) {
                list.dispatchEvent(new CustomEvent("goowee:windowscroll", {
                    detail: {scrollTop: window.scrollY, scrollLeft: window.scrollX}, bubbles: true
                }));
            }
        });
    }, {passive: true});
}

// The device pixel ratio changes with browser zoom and when the window moves
// to another screen. A media query on the current ratio fires once it no
// longer matches: replay every canvas at the new ratio, then watch the next.
let ratioWatched = false;
function watchPixelRatio() {
    if (ratioWatched || typeof matchMedia !== "function") return;
    ratioWatched = true;
    const mq = matchMedia("(resolution: " + (globalThis.devicePixelRatio || 1) + "dppx)");
    const changed = function () {
        mq.removeEventListener("change", changed);
        ratioWatched = false;
        for (const el of liveCanvases) {
            const st = canvasState.get(el);
            if (st && st.last) replayCanvas(el, st);
        }
        if (liveCanvases.size > 0) watchPixelRatio();
    };
    mq.addEventListener("change", changed);
}

function replayCanvas(el, st) {
    const bytes = st.last;
    const words = bytes.byteLength >> 2;
    const u = new Uint32Array(bytes.buffer, 0, words);
    const f = new Float32Array(bytes.buffer, 0, words);
    if (words < 4 || u[0] !== goowee.canvasVersion) {
        if (!canvasVersionWarned) {
            canvasVersionWarned = true;
            console.error("goowee: canvas buffer format " + u[0] + " is not " + goowee.canvasVersion +
                "; serve the goowee.js of the same goowee version as the app");
        }
        return;
    }
    // The runtime owns the backing store's size: the buffer's CSS size times
    // the device pixel ratio. Compared with the element itself, so anything
    // else that resized it (a width attribute) is corrected on the next draw.
    // A following canvas (flag bit 1) takes its size from the element, not
    // the header: the observed size is the latest.
    const follow = (u[1] & 2) !== 0 && st.follow;
    const w = follow ? st.cw : f[2], h = follow ? st.ch : f[3], dpr = globalThis.devicePixelRatio || 1;
    const bw = Math.round(w * dpr), bh = Math.round(h * dpr);
    if (el.width !== bw) el.width = bw; // also clears the canvas and resets its state
    if (el.height !== bh) el.height = bh;
    const ctx = st.ctx;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0); // app coordinates are CSS pixels
    if (u[1] & 1) ctx.clearRect(0, 0, w, h);
    ctx.save();
    let depth = 1;
    const str = function (i) { return canvasText.decode(bytes.subarray(i * 4 + 4, i * 4 + 4 + u[i])); };
    for (let i = 4; i < words;) {
        const word = u[i++], op = word & 0xff, n = word >>> 8;
        if (i + n > words) break; // truncated: Go never sends one
        switch (op) {
            case 1: ctx.save(); depth++; break;
            case 2: if (depth > 1) { ctx.restore(); depth--; } break;
            case 3: ctx.translate(f[i], f[i + 1]); break;
            case 4: ctx.scale(f[i], f[i + 1]); break;
            case 5: ctx.rotate(f[i]); break;
            case 6: ctx.transform(f[i], f[i + 1], f[i + 2], f[i + 3], f[i + 4], f[i + 5]); break;
            case 10: ctx.fillStyle = str(i); break;
            case 11: ctx.strokeStyle = str(i); break;
            case 12: ctx.lineWidth = f[i]; break;
            case 13: ctx.lineCap = CANVAS_CAPS[u[i]] || "butt"; break;
            case 14: ctx.lineJoin = CANVAS_JOINS[u[i]] || "miter"; break;
            case 15: ctx.globalAlpha = f[i]; break;
            case 16: ctx.setLineDash(Array.from(f.subarray(i, i + n))); break;
            case 17: ctx.font = str(i); break;
            case 18: ctx.textAlign = CANVAS_ALIGNS[u[i]] || "start"; break;
            case 19: ctx.textBaseline = CANVAS_BASELINES[u[i]] || "alphabetic"; break;
            case 30: ctx.beginPath(); break;
            case 31: ctx.closePath(); break;
            case 32: ctx.moveTo(f[i], f[i + 1]); break;
            case 33: ctx.lineTo(f[i], f[i + 1]); break;
            case 34: ctx.arc(f[i], f[i + 1], f[i + 2], f[i + 3], f[i + 4]); break;
            case 35: ctx.rect(f[i], f[i + 1], f[i + 2], f[i + 3]); break;
            case 36: ctx.quadraticCurveTo(f[i], f[i + 1], f[i + 2], f[i + 3]); break;
            case 37: ctx.bezierCurveTo(f[i], f[i + 1], f[i + 2], f[i + 3], f[i + 4], f[i + 5]); break;
            case 38: ctx.fill(); break;
            case 39: ctx.stroke(); break;
            case 40: ctx.clip(); break;
            case 50: ctx.clearRect(f[i], f[i + 1], f[i + 2], f[i + 3]); break;
            case 51: ctx.fillRect(f[i], f[i + 1], f[i + 2], f[i + 3]); break;
            case 52: ctx.strokeRect(f[i], f[i + 1], f[i + 2], f[i + 3]); break;
            case 53: ctx.fillText(str(i + 2), f[i], f[i + 1]); break;
            case 54: ctx.strokeText(str(i + 2), f[i], f[i + 1]); break;
            case 60: // Segments: x1 y1 x2 y2 …
                for (let j = i, e = i + n; j < e; j += 4) { ctx.moveTo(f[j], f[j + 1]); ctx.lineTo(f[j + 2], f[j + 3]); }
                break;
            case 61: // Circles: x y r …
                for (let j = i, e = i + n; j < e; j += 3) { ctx.moveTo(f[j] + f[j + 2], f[j + 1]); ctx.arc(f[j], f[j + 1], f[j + 2], 0, TAU); }
                break;
            case 62: // Rects: x y w h …
                for (let j = i, e = i + n; j < e; j += 4) ctx.rect(f[j], f[j + 1], f[j + 2], f[j + 3]);
                break;
            case 63: // Polyline: x y …
                if (n >= 2) {
                    ctx.moveTo(f[i], f[i + 1]);
                    for (let j = i + 2, e = i + n; j < e; j += 2) ctx.lineTo(f[j], f[j + 1]);
                }
                break;
            case 64: // FillRects: x y w h …
                for (let j = i, e = i + n; j < e; j += 4) ctx.fillRect(f[j], f[j + 1], f[j + 2], f[j + 3]);
                break;
        }
        i += n;
    }
    while (depth-- > 0) ctx.restore(); // the next frame starts from the default state
}

// goowee.applyBatch applies a binary batch: the first len bytes of u8, which
// the bridge copies in from Go. It is the default transport; the format is
// documented with its encoder, internal/dom/batch.go. batchVersion is the
// format this runtime reads, and the bridge sends JSON instead when it doesn't
// match its own.
goowee.batchVersion = 1;
goowee.applyBatch = function applyBatch(u8, len) {
    const b = startBatch();
    decodeBatch(u8, len, applyMutation, b);
    return finishBatch(b);
};

// goowee.applyBatchAt applies a binary batch that Go hands over in place: len
// bytes at ptr in the module's memory, with no copy (#133). A handler that
// runs during the batch (a focus event, say) may grow that memory, which
// detaches the view; the reader takes a new one when it sees that.
goowee.applyBatchAt = function applyBatchAt(ptr, len) {
    const mem = wasmExports.mem;
    const b = startBatch();
    decodeBatch(new Uint8Array(mem.buffer, ptr, len), len, applyMutation, b, function () {
        return new Uint8Array(mem.buffer, ptr, len);
    });
    return finishBatch(b);
};

// One decoder for every batch. ignoreBOM keeps a string that starts with
// U+FEFF intact: by default a TextDecoder drops a leading byte order mark,
// which would shift every string after it.
const batchText = new TextDecoder("utf-8", {ignoreBOM: true});

// decodeBatch calls apply(mut, ctx) for each update in a binary batch, with
// the fields JSON.parse would have produced for it: absent ones undefined.
// mut is one object reused for the whole batch.
function decodeBatch(u8, len, apply, ctx, reattach) {
    const r = new BatchReader(u8, len);
    const mut = {type: 0, nodeId: 0, key: undefined, value: undefined, childId: undefined, refId: undefined, ns: undefined};
    while (r.pos < r.end) {
        // A view of a WebAssembly memory that grew reads as empty.
        if (reattach && r.u8.byteLength === 0) {
            r.u8 = reattach();
            r.view = null;
        }
        u8 = r.u8;
        const type = u8[r.pos], flags = u8[r.pos + 1];
        r.pos += 2;
        mut.type = type;
        mut.nodeId = r.varint();
        mut.key = flags & 1 ? r.string() : undefined;
        switch ((flags >> 4) & 7) {
            case 0: mut.value = undefined; break;
            case 1:
                mut.value = r.string();
                // The DOM copies text and markup; a property a page keeps in
                // JS (a custom element's, an expando) gets a string of its own.
                if (type === 3 && mut.key !== "textContent" && mut.key !== "innerHTML") mut.value = ownString(mut.value);
                break;
            case 2: mut.value = r.varint(); break;
            case 3: mut.value = r.float(); break;
            case 4: mut.value = false; break;
            case 5: mut.value = true; break;
            case 6: mut.value = JSON.parse(r.string()); break;
            default: throw new Error("goowee: unknown value kind in a DOM update batch");
        }
        mut.childId = flags & 2 ? r.varint() : undefined;
        mut.refId = flags & 4 ? r.varint() : undefined;
        mut.ns = flags & 8 ? r.string() : undefined;
        apply(mut, ctx);
    }
}

// BatchReader reads one binary batch: the header, varints and floats from the
// op section, and strings sliced in turn out of the string section, decoded
// once up front.
class BatchReader {
    constructor(u8, len) {
        this.u8 = u8;
        this.end = 4 + ((u8[0] | u8[1] << 8 | u8[2] << 16 | u8[3] << 24) >>> 0);
        this.pos = 4;
        this.strs = this.end < len ? batchText.decode(u8.subarray(this.end, len)) : "";
        this.spos = 0;
        this.view = null;
    }
    uvarint() {
        const u8 = this.u8;
        let b = u8[this.pos++];
        if (b < 0x80) return b;
        // Past 31 bits, bit operations would overflow: accumulate in doubles,
        // exact below 2^53 (the encoder keeps varints below it).
        let v = b & 0x7f, scale = 0x80;
        do {
            b = u8[this.pos++];
            v += (b & 0x7f) * scale;
            scale *= 0x80;
        } while (b >= 0x80);
        return v;
    }
    varint() { // zigzag: 0, -1, 1, -2, … as 0, 1, 2, 3, …
        const u = this.uvarint();
        return u % 2 === 0 ? u / 2 : -(u + 1) / 2;
    }
    string() { // its length in UTF-16 code units, then the next that many
        const n = this.uvarint(), s = this.strs.substring(this.spos, this.spos + n);
        this.spos += n;
        return s;
    }
    float() {
        if (!this.view) this.view = new DataView(this.u8.buffer, this.u8.byteOffset);
        const f = this.view.getFloat64(this.pos, true);
        this.pos += 8;
        return f;
    }
}

// ownString returns s as a string that doesn't share memory with the batch's
// string section. A substring of 13 or more characters is a view into the
// string it was cut from, so a value the page keeps would keep the whole
// section alive; concatenating and slicing flattens it into a copy.
function ownString(s) {
    return s.length < 13 ? s : (" " + s).slice(1);
}

// For test/e2e/batch.mjs, which checks decodeBatch against JSON.parse.
goowee._decodeBatch = decodeBatch;

// applyMutation applies one update. mut is only valid during the call: the
// binary decoder reuses one object for the whole batch.
function applyMutation(mut, b) {
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
        case 17: // RemoveNodeAfter — keep the node for its leave animation
            removeAfterTransition(nodeMap[mut.nodeId], mut.key);
            break;
        case 2: // SetAttribute
            el = nodeMap[mut.nodeId];
            // Go validates names; a throw here must not abort the batch.
            if (el) try { el.setAttribute(mut.key, mut.value); } catch (err) { console.error("goowee: setAttribute", mut.key, err); }
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
            const parent = containerOf(mut.nodeId);
            const child = nodeMap[mut.childId];
            if (parent && child && !child.parentNode) {
                parent.appendChild(child);
                startEnterTree(child);
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
                : containerOf(mut.nodeId);
            if (parent && child) {
                b.movedLive = moveNode(parent, child, ref && ref.parentNode === parent ? ref : null) || b.movedLive;
                startEnterTree(child);
            }
            break;
        }
        case 6: { // RemoveAttribute
            el = nodeMap[mut.nodeId];
            if (el) try { el.removeAttribute(mut.key); } catch (err) { console.error("goowee: removeAttribute", mut.key, err); }
            break;
        }
        case 8: { // Invoke — call a method on a node (refs: focus/blur/…)
            el = nodeMap[mut.nodeId];
            if (mut.key === "focus" || mut.key === "blur") b.focusCommand = true;
            if (el && typeof el[mut.key] === "function") el[mut.key]();
            break;
        }
        case 9: { // PortalAppend — append child to a container by selector
            const parent = document.querySelector(mut.value);
            const child = nodeMap[mut.childId];
            if (parent && child && child.parentNode !== parent) {
                b.movedLive = moveNode(parent, child, null) || b.movedLive;
                if (child.nodeName === "TITLE" && parent === document.head) claimTitle();
                startEnterTree(child);
            }
            break;
        }
        case 10: // Read — deferred to after the batch (see below)
            b.reads.push({nodeId: mut.nodeId, key: mut.key, value: mut.value});
            break;
        case 11: // Call — a method with arguments, after the batch (see below)
            b.calls.push({nodeId: mut.nodeId, key: mut.key, value: mut.value});
            break;
        case 13: // Listen — the node has (value true) or lost a Go handler for key
            el = nodeMap[mut.nodeId];
            if (el) {
                if (mut.value) (el._goEvents || (el._goEvents = new Set())).add(mut.key);
                else if (el._goEvents) el._goEvents.delete(mut.key);
            }
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
            el._goHydrated = true; // don't replay an enter animation on hydration
            nodeMap[mut.nodeId] = el;
            break;
        }
    }
}

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
