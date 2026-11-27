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
const goowee = (globalThis.goowee = globalThis.goowee || {});

// The same file is the Web Worker's script in worker mode (#158): there is no
// document, and the worker section at the end of the file runs instead of
// the page's boot. runtimeURL is this script's URL, for starting the worker.
const inWorker = typeof document === "undefined";
const runtimeURL = inWorker ? null : (document.currentScript && document.currentScript.src) || null;

// The mutation protocol this runtime understands. bridge.Run compares it with
// core.ProtocolVersion before mounting: a mismatch means this page serves an
// older goowee.js, which would misapply mutations from the linked binary.
goowee.protocolVersion = 2;
goowee.protocolMismatch = function protocolMismatch(appVersion, appProtocol) {
    const page = goowee.protocolVersion;
    const err = bootError(
        "this page serves goowee.js with protocol " + (page === undefined ? "no version marker" : page) +
        ", but main.wasm was built with goowee " + appVersion + " (protocol " + appProtocol +
        "). Copy the matching assets with: go run github.com/yogisalomo/goowee/cmd/goowee assets ./web"
    );
    if (inWorker) { postMessage({t: "boot-error", message: err.message}); return; }
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
    if (typeof goowee.inspectGo !== "function" && !goowee._workerInspector) {
        console.warn(
            "[goowee] inspector unavailable — enable dev mode with ?goowee-dev or localStorage.setItem('goowee-dev','1') and reload."
        );
        return null;
    }
    if (goowee.worker) {
        // Go runs in the worker: the answer comes back in a message, so
        // this returns a promise of the snapshot (#158).
        return askWorker("snapshot").then(function (snap) {
            console.log("[goowee] inspector snapshot:", snap);
            return snap;
        });
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
    if (typeof goowee.changesGo !== "function" && !goowee._workerInspector) {
        console.warn(
            "[goowee] change log unavailable — enable dev mode with ?goowee-dev or localStorage.setItem('goowee-dev','1') and reload."
        );
        return null;
    }
    const clear = !!(opts && opts.clear);
    if (goowee.worker) {
        return askWorker(clear ? "clear" : "changes").then(function (rows) {
            if (!clear) console.table(rows);
            return rows;
        });
    }
    const rows = goowee.changesGo(clear);
    if (!clear) console.table(rows);
    return rows;
};

// askWorker asks the worker's dev tools for a snapshot or the change log.
const inspectWaiting = {};
let inspectSeq = 0;
function askWorker(what) {
    const id = ++inspectSeq;
    return new Promise(function (resolve) {
        inspectWaiting[id] = resolve;
        goowee.worker.postMessage({t: "inspect", id: id, what: what});
    });
}

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
// markup (inside a container that opens with <!--goowee-->) are queued (up to
// 100), and a submit is held back.
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
    if (!early.active || !t || !inServerMarkup(t)) return;
    if (e.type === "submit") e.preventDefault();
    if (early.queue.length < 100) early.queue.push(e);
}

// inServerMarkup reports whether node is inside a server-rendered container.
function inServerMarkup(node) {
    for (let el = node; el; el = el.parentNode) {
        if (el._goServer || (el.nodeType === 1 && hasServerMarkup(el))) return true;
    }
    return false;
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
    for (const root of roots) if (root._goServer) adoptFormState(root);
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

// goowee.host is everything the Go side asks of the browser other than
// applying batches and receiving events (#158, ADR-005): the page, the URL
// and history, scrolling and focus for the router, storage. Go reaches the
// browser only through it (internal/host), so a worker can stand in for the
// page later with the same methods over messages. These are the page's.
if (!inWorker) goowee.host = {
    worker: false,
    hydrating: function () { return goowee.serverRendered(); },
    root: function () { return document.getElementById("root"); },
    query: function (selector) { return document.querySelector(selector); },
    queryAll: function (selector) { return Array.from(document.querySelectorAll(selector)); },
    body: function () { return document.body; },
    // overlayContainer is the element the dev overlay mounts into (#148).
    overlayContainer: function () {
        let el = document.getElementById("goowee-devtools");
        if (!el) {
            el = document.createElement("div");
            el.id = "goowee-devtools";
            document.body.appendChild(el);
        }
        return el;
    },
    hidden: function () { return document.hidden; },
    onVisibilityChange: function (cb) { document.addEventListener("visibilitychange", function () { cb(document.hidden); }); },
    baseURI: function () { return document.baseURI; },
    baseHref: function () {
        const b = document.querySelector("base");
        return b ? (b.getAttribute("href") || "") : "";
    },
    location: function () {
        return {pathname: location.pathname, search: location.search, hash: location.hash, href: location.href};
    },
    historyState: function () { return history.state; },
    pushState: function (state, url) { history.pushState(state, "", url); },
    replaceState: function (state, url) { history.replaceState(state, "", url); },
    back: function () { history.back(); },
    forward: function () { history.forward(); },
    // The router restores scroll positions itself, after a route's frame.
    manualScrollRestoration: function () { history.scrollRestoration = "manual"; },
    // on listens on window: popstate, scroll, beforeunload.
    on: function (type, cb, passive) { addEventListener(type, cb, passive ? {passive: true} : undefined); },
    scrollPosition: function (container) {
        const el = container ? document.querySelector(container) : null;
        return el ? {x: el.scrollLeft, y: el.scrollTop} : {x: scrollX, y: scrollY};
    },
    scrollTo: function (container, x, y) {
        const el = container ? document.querySelector(container) : null;
        if (el) { el.scrollLeft = x; el.scrollTop = y; } else scrollTo(x, y);
    },
    // scrollToFragment scrolls the fragment's target (an id, or a named
    // anchor) to the top of the page or container; false when there is none.
    scrollToFragment: function (fragment, container) {
        if (!fragment) return false;
        let target = document.getElementById(fragment);
        if (!target) {
            const name = typeof CSS !== "undefined" && CSS.escape ? CSS.escape(fragment) : fragment;
            target = document.querySelector('[name="' + name + '"]');
        }
        if (!target) return false;
        const el = container ? document.querySelector(container) : null;
        let top = target.getBoundingClientRect().top;
        if (el) { el.scrollTop = top + el.scrollTop - el.getBoundingClientRect().top; el.scrollLeft = 0; }
        else scrollTo(0, top + scrollY);
        return true;
    },
    // focusRoute moves focus to the new page's heading after a navigation.
    focusRoute: function () {
        const el = document.querySelector("[data-route-focus], h1, main");
        if (!el) return;
        if (el.tabIndex < 0) el.tabIndex = -1; // headings and <main> aren't focusable by default
        el.focus({preventScroll: true});
    },
    // announceRoute tells screen readers the page changed, by its title.
    announceRoute: function () {
        let el = document.querySelector("[data-goowee-route-status]");
        if (!el) {
            el = document.createElement("div");
            el.setAttribute("data-goowee-route-status", "");
            el.setAttribute("aria-live", "polite");
            el.setAttribute("role", "status");
            Object.assign(el.style, {position: "absolute", width: "1px", height: "1px",
                overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap"});
            document.body.appendChild(el);
        }
        el.textContent = "Navigated to " + (document.title || location.pathname);
    },
    // storage is localStorage or sessionStorage, or null where it is
    // unavailable (disabled, a sandboxed frame).
    storage: function (area) { try { return globalThis[area] || null; } catch (_) { return null; } },
    // dataScript is the text of the <script> ssr embeds resource data in.
    dataScript: function (id) { const el = document.getElementById(id); return el ? el.textContent : null; },
};

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

// bootWasmURL is the absolute URL startBoot fetched the module from.
let bootWasmURL = null;

function startBoot(opts) {
    if (opts.transport) goowee.transport = opts.transport;
    // calls: "callbacks" keeps every call into Go on syscall/js callbacks,
    // as before 0.4 (for comparing, or a problem with the exports).
    const wasmURL = opts.wasm || "main.wasm";
    // The resolved URL, for bootMetrics to find the download among the
    // resource timings (goowee build gives it a hashed name).
    try { bootWasmURL = new URL(wasmURL, document.baseURI).href; } catch (e) { bootWasmURL = null; }
    if (workerMode(opts)) return bootWorker(opts);
    const go = new Go();
    mark("goowee:fetch-start");
    const started = loadWasm(wasmURL, go.importObject)
        .then(function (instance) {
            mark("goowee:instantiated");
            mark("goowee:run");
            if (opts.calls !== "callbacks") goowee.useInstance(go, instance);
            go.run(instance);
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

// Worker mode (#158): the Go program runs in a dedicated Web Worker started
// from this same file, and the page applies its batches and forwards events.
// goowee.boot({worker: true}) asks for it; localStorage "goowee-worker" = "1"
// does too when opts.worker isn't given (tests use it).
function workerMode(opts) {
    if (opts.worker !== undefined) return opts.worker === true;
    try { return localStorage.getItem("goowee-worker") === "1"; } catch (_) { return false; }
}

let workerBoot = null; // {resolve, reject} until the worker says it runs
let workerRunning = false, workerFail = null;
function bootWorker(opts) {
    const abs = function (u) { return new URL(u, document.baseURI).href; };
    const src = opts.runtime ? abs(opts.runtime) : runtimeURL;
    const fail = (workerFail = function (err) {
        goowee.bootError = err;
        releaseEarly();
        console.error(err.message);
        window.dispatchEvent(new CustomEvent("goowee:boot-error", {detail: err}));
        return err;
    });
    if (!src) {
        const err = fail(bootError('worker mode needs the URL of goowee.js: pass {runtime: ".../goowee.js"} to goowee.boot'));
        const p = Promise.reject(err);
        p.catch(function () {});
        return p;
    }
    mark("goowee:fetch-start");
    const started = new Promise(function (resolve, reject) {
        workerBoot = {resolve: resolve, reject: reject};
        const w = (goowee.worker = new Worker(src));
        // goowee._delayMessages (a test knob, #158) handles each worker
        // message that many ms late, in order, as a slow page would.
        let delayed = Promise.resolve();
        w.onmessage = function (e) {
            const ms = goowee._delayMessages;
            if (typeof ms !== "number") { onWorkerMessage(e.data); return; }
            const due = performance.now() + ms;
            delayed = delayed.then(function () {
                return new Promise(function (r) {
                    setTimeout(function () { onWorkerMessage(e.data); r(); }, Math.max(0, due - performance.now()));
                });
            });
        };
        w.onerror = function (e) { reject(bootError("the worker failed to start (" + (e.message || "error") + ")")); };
        w.postMessage({t: "init", wasm: abs(opts.wasm || "main.wasm"), wasmExec: abs(opts.wasmExec || pageWasmExec() || "wasm_exec.js"),
            dev: goowee.devEnabled(), hydrating: goowee.host.hydrating(), hidden: document.hidden,
            baseURI: document.baseURI, baseHref: goowee.host.baseHref(),
            location: goowee.host.location(), historyState: cloneable(history.state),
            storage: {localStorage: snapshotStorage("localStorage"), sessionStorage: snapshotStorage("sessionStorage")},
            // The data ssr embedded for hydration (internal/data), so the
            // worker doesn't fetch it again.
            dataScripts: dataScripts(),
            islands: islandStandIns(),
            calls: opts.calls || ""});
        // What the router in the worker hears from the browser: back and
        // forward, and the window's scroll position (one per frame).
        addEventListener("popstate", function () {
            w.postMessage({t: "popstate", location: goowee.host.location(), state: cloneable(history.state)});
        });
        let scrollQueued = false;
        addEventListener("scroll", function () {
            if (scrollQueued) return;
            scrollQueued = true;
            requestAnimationFrame(function () {
                scrollQueued = false;
                w.postMessage({t: "scroll", x: scrollX, y: scrollY});
            });
        }, {passive: true});
        document.addEventListener("visibilitychange", function () {
            w.postMessage({t: "visibility", hidden: document.hidden});
        });
    }).catch(function (err) {
        if (!err || !err.gooweeBoot) err = bootError(String((err && err.message) || err), err);
        throw fail(err);
    });
    started.catch(function () {});
    return started;
}

// islandStandIns describes the page's islands (ssr.Island) for the worker,
// which can't look at the DOM: a selector that finds each again, the
// attributes bridge.Islands reads, and whether it holds server markup.
function islandStandIns() {
    const out = [];
    document.querySelectorAll("[data-goowee-island]").forEach(function (el, i) {
        el.setAttribute("data-goowee-mount", String(i));
        out.push({id: el.id, selector: '[data-goowee-mount="' + i + '"]', serverMarkup: hasServerMarkup(el),
            attrs: {"data-goowee-island": el.getAttribute("data-goowee-island"),
                "data-goowee-props": el.getAttribute("data-goowee-props")}});
    });
    return out;
}

// pageWasmExec is the URL of the page's own wasm_exec.js script, if it has
// one: the loader the worker should import too.
function pageWasmExec() {
    for (const s of document.scripts) if (/(^|\/)wasm_exec\.js(\?|$)/.test(s.getAttribute("src") || "")) return s.src;
    return null;
}

// snapshotStorage is every entry of a storage area, for the worker's copy,
// or null where the area is unavailable.
function snapshotStorage(area) {
    try {
        const s = globalThis[area];
        if (!s) return null;
        const out = [];
        for (let i = 0; i < s.length; i++) { const k = s.key(i); out.push([k, s.getItem(k)]); }
        return out;
    } catch (_) { return null; }
}

// dataScripts is the text of each <script type="application/json" id=…>.
function dataScripts() {
    const out = {};
    for (const el of document.querySelectorAll('script[type="application/json"][id]')) out[el.id] = el.textContent;
    return out;
}

// cloneable is v if postMessage can copy it, else null (history.state may
// hold anything a page put there).
function cloneable(v) {
    try { structuredClone(v); return v; } catch (_) { return null; }
}

// onWorkerMessage is the page's half of worker mode: what the worker's Go
// program asks of the DOM.
function onWorkerMessage(d) {
    switch (d.t) {
        case "running": workerRunning = true; mark("goowee:run"); if (workerBoot) workerBoot.resolve(); break;
        case "boot-error": {
            const err = new Error(d.message);
            err.gooweeBoot = true;
            // After "running" the boot promise has settled (a protocol
            // mismatch is found by the running program): report it directly.
            if (workerRunning) workerFail(err);
            else if (workerBoot) workerBoot.reject(err);
            break;
        }
        case "console": console[d.level].apply(console, d.args); break;
        case "draw": goowee.draw(new Uint8Array(d.buf), d.buf.byteLength, d.nodeId); break;
        case "mark": mark(d.name); break;
        case "listen": goListen(d.type, d.capture); break;
        case "mount": goowee.mountRoot(d.parentId, document.querySelector(d.selector), d.base, d.hydrate); break;
        case "unmount": goowee.unmountRoot(d.parentId); break;
        case "batch": {
            batchAck = d.ack;
            let replies;
            try {
                replies = d.bin ? goowee.applyBatch(new Uint8Array(d.bin), d.bin.byteLength) : applyMutations(d.json);
            } finally {
                batchAck = Infinity;
            }
            applyDeferredInput(d.ack);
            // ref.Get answers go back to Go, which runs their callbacks.
            if (typeof replies === "string") goowee.worker.postMessage({t: "replies", value: replies});
            break;
        }
        case "ack": applyDeferredInput(d.seq); break;
        case "log": goowee.log(d.entry); break;
        case "inspect-result": {
            const done = inspectWaiting[d.id];
            delete inspectWaiting[d.id];
            if (done) done(d.value);
            break;
        }
        case "history":
            switch (d.op) {
                case "push": history.pushState(d.state, "", d.url); break;
                case "replace": history.replaceState(d.state, "", d.url); break;
                case "back": history.back(); break;
                case "forward": history.forward(); break;
            }
            break;
        // The router's DOM work (scrolling, focus, the announcement).
        case "host": goowee.host[d.call].apply(goowee.host, d.args || []); break;
        case "storage":
            try {
                const area = globalThis[d.area];
                if (d.op === "set") area.setItem(d.key, d.value);
                else if (d.op === "remove") area.removeItem(d.key);
                else if (d.op === "clear") area.clear();
            } catch (err) {
                // The worker's Store.Set already returned: report it here.
                goowee.log({kind: "warn", message: "goowee/storage: " + d.area + " " + d.op + " failed in worker mode: " + err});
            }
            break;
        case "set": goowee[d.key] = d.value; break;
    }
}

// dispatchToWorker is dispatchToGo in worker mode: the page decides what must
// be decided while the event is dispatched (cancelling, stopping, selecting)
// from what Go told it with Listen, and posts the event with the ids of the
// nodes whose handlers should run, innermost first. A handler's dynamic
// e.PreventDefault() can't take effect here; Go warns about it.
let eventSeq = 0;
function dispatchToWorker(type, e, targetOnly) {
    const targets = [];
    for (let el = e.target; el; el = targetOnly ? null : el.parentNode) {
        if (el._goInert) return;
        if (el._nodeID === undefined || !(el._goEvents && el._goEvents.has(type))) continue;
        targets.push(el._nodeID);
        const flags = (el._goFlags && el._goFlags[type]) || 0;
        const filter = el._goFilters && el._goFilters[type];
        if (((flags & 1) || (filter && filterMatches(filter, e))) && e.cancelable) e.preventDefault();
        if ((flags & 4) && type === "focus" && el.select) el.select();
        if (flags & 2) { e.stopPropagation(); break; }
    }
    if (targets.length === 0) return;
    goowee.eventCalls++;
    const seq = ++eventSeq;
    // The field the user typed in: a batch computed before Go saw this event
    // must not put the old text back (inputGuarded).
    if ((type === "input" || type === "change") && e.target) e.target._gInputSeq = seq;
    const payload = buildPayload(type, e);
    // Parked files travel with the event (a File is cloned, not its bytes),
    // so the worker can read them (goowee.readFile there).
    const files = Array.isArray(payload.files)
        ? payload.files.map(function (f) { return [f.handle, fileStore[f.handle]]; }) : undefined;
    flushReleasedFiles();
    goowee.worker.postMessage({t: "event", seq: seq, type: type, targets: targets,
        payload: JSON.stringify(payload), files: files});
}

// Typed input in worker mode (#158). The worker answers events a little
// later, so a frame it computed before it handled the user's latest keystroke
// would write the old value back into the field and lose characters.
// batchAck is the last event the worker had handled when it computed the
// batch being applied (Infinity outside worker mode: nothing is held). A
// value or checked write to a field the user has typed in since is held in
// deferredInput, and applied once the worker has caught up with that field.
let batchAck = Infinity;
const deferredInput = new Map(); // element -> {key: value}
function inputGuarded(el, key, value) {
    if (key !== "value" && key !== "checked") return false;
    if (el._gInputSeq !== undefined && el._gInputSeq > batchAck) {
        const w = deferredInput.get(el) || {};
        w[key] = value;
        deferredInput.set(el, w);
        return true;
    }
    const w = deferredInput.get(el);
    if (w) { delete w[key]; if (!Object.keys(w).length) deferredInput.delete(el); }
    return false;
}
function applyDeferredInput(ack) {
    for (const [el, w] of deferredInput) {
        if (!(el._gInputSeq <= ack)) continue;
        deferredInput.delete(el);
        for (const k in w) if (el[k] !== w[k]) el[k] = w[k];
    }
}

// loadWasm downloads and instantiates the module at wasmURL, streaming when
// the server sends application/wasm. It rejects with a bootError that says
// what went wrong. The page and the worker both use it.
function loadWasm(wasmURL, importObject) {
    return fetch(wasmURL)
        .catch(function (err) {
            throw bootError("couldn't download " + wasmURL + " (" + err + ")", err);
        })
        .then(function (resp) {
            if (!resp.ok) {
                throw bootError("downloading " + resp.url + " failed: HTTP " + resp.status + " " + resp.statusText);
            }
            const type = (resp.headers.get("content-type") || "").split(";")[0].trim().toLowerCase();
            if (type === "application/wasm" && typeof WebAssembly.instantiateStreaming === "function") {
                return WebAssembly.instantiateStreaming(resp, importObject)
                    .catch(function (err) { throw wasmError(resp.url, type, err); });
            }
            console.warn("goowee: " + resp.url + " is served as " + (type ? "'" + type + "'" : "no Content-Type") +
                ", not application/wasm, so it is compiled after downloading instead of while it downloads. " +
                "Configure the server to send Content-Type: application/wasm.");
            return resp.arrayBuffer()
                .then(function (bytes) { return WebAssembly.instantiate(bytes, importObject); })
                .catch(function (err) { throw wasmError(resp.url, type, err); });
        })
        .then(function (result) { return result.instance; });
}

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
        .find(function (r) {
            const name = r.name.split("?")[0];
            return bootWasmURL ? name === bootWasmURL : name.endsWith("main.wasm");
        });
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

// hydrationRoots are the server-rendered containers being claimed, each with
// a cursor through its nodes in document order (ADR-033). A root's node ids
// are the ones after its base: a Hydrate's id says which root it belongs to.
const hydrationRoots = [];
let hydrated = false;

// Collect server-rendered nodes for reuse, the first time mutations are
// applied. This is deferred rather than run at module load because this
// script sits in <head>, before <div id="root"> (and its SSR content) exists;
// by the first applyMutations the WASM app has booted and the DOM is present.
// For a pure client app (no SSR) there are no nodes to claim, and #root
// belongs to the app: whatever the page put there (a "Loading…" splash shown
// while the WASM downloads) is removed here, in the same task that appends
// the app's first frame, so nothing paints in between. "Server-rendered" is
// decided exactly as bridge.Run decides it: #root opens with <!--goowee-->.
// serverRendered reports whether the document holds server markup for this
// page, to hydrate: #root opens with goowee's marker comment, and a
// prerendered page (ssr.Export marks it with <meta name="goowee-prerendered">)
// was rendered for the current path. A static host serves its 404.html,
// prerendered for a path nothing matches, for every URL it has no file for,
// including a valid page nobody prerendered; that markup is another page's,
// so the app renders on the client instead. bridge.Run asks the same question.
goowee.serverRendered = function serverRendered() {
    const root = getRoot();
    // After hydration the marker is gone; the claimed root remembers.
    if (!hasServerMarkup(root) && !(root && root._goServer)) return false;
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

// hasServerMarkup reports whether container holds a server render: its first
// node, after any whitespace, is the <!--goowee--> marker ssr writes.
function hasServerMarkup(container) {
    for (let n = container && container.firstChild; n; n = n.nextSibling) {
        if (n.nodeType === 3 && !n.data.trim()) continue;
        return n.nodeType === 8 && n.data === "goowee";
    }
    return false;
}
goowee._hasServerMarkup = hasServerMarkup; // bridge.Mount asks it of an island

// claimServerNodes starts claiming the server-rendered nodes in container.
// Go walks the same tree the server rendered and sends one Hydrate per node,
// in document order; each claims the next server node (claimNext). ids from
// base on belong to this root (an island's, #154).
function claimServerNodes(container, base) {
    container._goServer = true; // its form controls may hold early input (#131)
    hydrationRoots.push({container: container, base: base, last: container, skip: false});
    hydrationRoots.sort(function (a, b) { return a.base - b.base; });
}

// nextInOrder is the node after n in document order within container,
// without entering n's children when skip is set.
function nextInOrder(n, container, skip) {
    if (!skip && n.firstChild) return n.firstChild;
    while (n && n !== container) {
        if (n.nextSibling) return n.nextSibling;
        n = n.parentNode;
    }
    return null;
}

// claimNext claims the next server node for a Hydrate of id: the root's
// cursor moves past the markers ssr writes (the opening <!--goowee-->, an
// empty <!----> between adjacent texts, which the parser would otherwise
// merge) and whitespace the page put around the render, and stops at the
// node the client expects. <!--t--> stands for an empty text node. A node of
// the wrong kind is a mismatch: it is reported and passed over, and the
// client gets a bare node so the app keeps running.
function claimNext(id, want, opaque) {
    let root = null;
    // A root's own ids start after its base (base + 1 is its first node).
    for (const r of hydrationRoots) if (r.base < id) root = r;
    const wantText = want === "#text", wantComment = want === "#comment";
    let n = root ? nextInOrder(root.last, root.container, root.skip) : null;
    while (n) {
        const next = nextInOrder(n, root.container, true);
        if (n.nodeType === 8 && (n.data === "goowee" || n.data === "")) {
            n.remove(); n = next; continue;
        }
        if (n.nodeType === 8 && n.data === "t" && wantText) {
            const empty = document.createTextNode("");
            n.replaceWith(empty);
            n = empty;
            break;
        }
        if (n.nodeType === 3 && !wantText && !n.data.trim()) { n = next; continue; }
        break;
    }
    const matches = n && (wantText ? n.nodeType === 3
        : wantComment ? n.nodeType === 8 && n.data.charAt(0) === "/"
        : n.nodeType === 1 && n.nodeName.toLowerCase() === want);
    if (n) {
        root.last = n;
        root.skip = opaque || !matches;
    }
    if (matches) return n;
    if (n) {
        console.error(
            "goowee: hydration mismatch at node " + id +
            " — client expected <" + want + ">, server rendered <" +
            (n.nodeType === 3 ? "#text" : n.nodeType === 8 ? "#comment" : n.nodeName.toLowerCase()) + ">. " +
            "Render deterministic markup, or wrap non-deterministic content with h.Dynamic().");
    } else {
        console.error("goowee: no server node for " + id + " (<" + want +
            ">). SSR/client structure diverged; creating it bare.");
    }
    return wantText ? document.createTextNode("")
        : wantComment ? document.createComment("")
        : document.createElement(want);
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
globalThis.goListen = function (type, capture) {
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

// releasedFiles are handles dropped since the worker was last told (#158):
// the worker keeps its own copy of each event's files.
let releasedFiles = [];
function dropFile(h) {
    delete fileStore[h];
    if (goowee.worker) releasedFiles.push(h);
}
function flushReleasedFiles() {
    if (releasedFiles.length === 0) return;
    goowee.worker.postMessage({t: "files-release", handles: releasedFiles});
    releasedFiles = [];
}

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
        if (!next.has(f)) dropFile(h);
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
const reducedMotionQuery = typeof matchMedia === "function"
    ? matchMedia("(prefers-reduced-motion: reduce)") : null;
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
    for (const h of prev.values()) dropFile(h);
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

// MEDIA_EVENTS are the HTMLMediaElement events whose payload is the
// element's playback state (#210).
const MEDIA_EVENTS = new Set(["loadstart", "progress", "suspend", "abort", "emptied", "stalled",
    "loadedmetadata", "loadeddata", "canplay", "canplaythrough", "playing", "waiting",
    "seeking", "seeked", "ended", "durationchange", "timeupdate", "play", "pause",
    "ratechange", "volumechange"]);

function isMedia(t) {
    return typeof HTMLMediaElement !== "undefined" && t instanceof HTMLMediaElement;
}

// mediaState is a media element's playback state. JSON has no NaN or
// Infinity: an unknown duration (NaN) becomes null and a live stream's
// (Infinity) the string "Infinity", which EventData.Duration reads as +Inf.
function mediaState(t) {
    const d = t.duration, buffered = [];
    for (let i = 0; t.buffered && i < t.buffered.length; i++) {
        buffered.push([t.buffered.start(i), t.buffered.end(i)]);
    }
    const s = {currentTime: t.currentTime, duration: d === Infinity ? "Infinity" : d,
        paused: t.paused, ended: t.ended, seeking: t.seeking, muted: t.muted,
        volume: t.volume, playbackRate: t.playbackRate, readyState: t.readyState, buffered: buffered};
    if (t.error) { s.errorCode = t.error.code; s.errorMessage = t.error.message; }
    return s;
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
            const extra = type === "input" ? {inputType: e.inputType, isComposing: e.isComposing, data: e.data} : {};
            if (t.type === "checkbox" || t.type === "radio")
                return Object.assign({value: t.value, checked: t.checked}, extra);
            if (t.type === "file" && t.files)
                return {value: t.value, files: stashFiles(t, t.files)};
            // JSON turns NaN (empty/invalid number input) into null; Go reads
            // that as 0, which is what a numeric signal should hold.
            if (t.nodeName === "SELECT")
                return Object.assign({value: t.value,
                    selected: Array.prototype.map.call(t.selectedOptions, function (o) { return o.value; })}, extra);
            if (typeof t.valueAsNumber === "number")
                return Object.assign({value: t.value, valueAsNumber: t.valueAsNumber}, extra);
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
        case "scroll": case "scrollend":
            return {scrollTop: e.target.scrollTop, scrollLeft: e.target.scrollLeft};
        case "beforeinput":
            return {data: e.data, inputType: e.inputType, isComposing: e.isComposing};
        case "compositionstart": case "compositionupdate": case "compositionend":
            return {data: e.data};
        case "toggle": case "beforetoggle": {
            // A ToggleEvent carries the states; where the browser fires a plain
            // Event (older <details>), the element's open flag says the new one.
            if (typeof e.newState === "string") return {newState: e.newState, oldState: e.oldState};
            const open = !!e.target.open;
            return {newState: open ? "open" : "closed", oldState: open ? "closed" : "open"};
        }
        case "close": case "cancel":
            return {returnValue: typeof e.target.returnValue === "string" ? e.target.returnValue : ""};
        case "transitionrun": case "transitionstart": case "transitionend": case "transitioncancel":
            return {propertyName: e.propertyName, elapsedTime: e.elapsedTime, pseudoElement: e.pseudoElement};
        case "animationstart": case "animationiteration": case "animationend": case "animationcancel":
            return {animationName: e.animationName, elapsedTime: e.elapsedTime, pseudoElement: e.pseudoElement};
        case "load": case "error": {
            const t = e.target;
            if (isMedia(t)) return mediaState(t);
            if (type === "load" && t && t.nodeName === "IMG")
                return {naturalWidth: t.naturalWidth, naturalHeight: t.naturalHeight};
            return {};
        }
        default:
            if (MEDIA_EVENTS.has(type) && isMedia(e.target)) return mediaState(e.target);
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

// filterMatches is core.EventFilter.Match, on the browser's event: f is the
// spec split on newlines ("plain", "files", or "key", with, without, keys…).
function filterMatches(f, e) {
    switch (f[0]) {
        case "plain": return e.button === 0 && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey;
        case "files": {
            const dt = e.dataTransfer || e.clipboardData; // a drag or drop, or a paste
            return !!dt && Array.from(dt.types || []).includes("Files");
        }
        case "key": {
            if (e.isComposing) return false;
            const m = (e.ctrlKey ? 1 : 0) | (e.altKey ? 2 : 0) | (e.shiftKey ? 4 : 0) | (e.metaKey ? 8 : 0);
            return (m & +f[1]) === +f[1] && (m & +f[2]) === 0 && f.indexOf(e.key, 3) !== -1;
        }
    }
    return false;
}

function dispatchToGo(type, e, targetOnly) {
    if (goowee.worker) return dispatchToWorker(type, e, targetOnly);
    let payload = null;
    for (let el = e.target; el; el = targetOnly ? null : el.parentNode) {
        // A node kept in DOM for its leave animation is inert: no event on or
        // inside it reaches Go, even before browsers that support `inert` do.
        if (el._goInert) return 0;
        if (el._nodeID === undefined) continue;
        if (goowee.listenSets && !(el._goEvents && el._goEvents.has(type))) continue;
        // PreventDefaultWhen: decided here, before Go runs (#158).
        const filter = el._goFilters && el._goFilters[type];
        if (filter && e.cancelable && filterMatches(filter, e)) e.preventDefault();
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
globalThis.applyMutations = function applyMutations(json) {
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
        watchedVirtualLists.delete(root._nodeID);
        // A MutationObserver has no unobserve: disconnect, then watch the
        // lists that remain again.
        if (rowScan) {
            rowScan.disconnect();
            for (const list of watchedVirtualLists.values()) rowScan.observe(list, {childList: true, subtree: true});
        }
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
goowee._buildPayload = buildPayload; // for test/e2e (event data, #210)

// applyMutation applies one update. mut is only valid during the call: the
// binary decoder reuses one object for the whole batch.
function applyMutation(mut, b) {
    let el;
    switch (mut.type) {
        case 0: // CreateElement
            if (mut.value === "#text") {
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
            if (el && !inputGuarded(el, mut.key, mut.value)) {
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
                // A string value is the handler's static options as flags
                // (1 prevent default, 2 stop propagation, 4 select on focus),
                // then its PreventDefaultWhen spec, if any (#158).
                if (typeof mut.value === "string" && mut.value) {
                    const parts = mut.value.split("\n");
                    (el._goFlags || (el._goFlags = {}))[mut.key] = +parts[0] || 0;
                    if (parts.length > 1) (el._goFilters || (el._goFilters = {}))[mut.key] = parts.slice(1);
                    else if (el._goFilters) delete el._goFilters[mut.key];
                } else {
                    if (el._goFlags) delete el._goFlags[mut.key];
                    if (el._goFilters) delete el._goFilters[mut.key];
                }
            }
            break;
        case 14: // ToggleClass — classList.toggle(key, value), other classes stay
            el = nodeMap[mut.nodeId];
            if (el && el.classList) try { el.classList.toggle(mut.key, !!mut.value); } catch (err) { console.error("goowee: toggleClass", mut.key, err); }
            break;
        case 15: { // SetStyle — one CSS property; ns carries a unit for numbers
            el = nodeMap[mut.nodeId];
            if (el && el.style) {
                let value = mut.value;
                if (mut.ns && typeof value === "number") value = String(value) + mut.ns;
                try { el.style.setProperty(mut.key, value == null ? "" : String(value)); }
                catch (err) { console.error("goowee: setProperty", mut.key, err); }
            }
            break;
        }
        case 16: { // SetSelected — select the <option>s whose values are listed
            el = nodeMap[mut.nodeId];
            if (el && el.options) {
                const want = new Set(mut.value || []);
                for (const opt of el.options) opt.selected = want.has(opt.value);
            }
            break;
        }
        case 7: { // Hydrate — claim the next server-rendered node (ADR-033)
            el = claimNext(mut.nodeId, mut.value, mut.key === "opaque");
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

// ---------------------------------------------------------------------------
// The worker (#158). In worker mode this file is also the Web Worker's script:
// the Go program runs here, its batches and its other requests go to the page
// as messages, and the page sends events back.
if (inWorker) {
    const post = function (m, transfer) { postMessage(m, transfer || []); };
    // The worker's warnings and errors (boot warnings, Go's panics) show in
    // the page's console, where people and tests look.
    for (const level of ["warn", "error"]) {
        console[level] = function () {
            const args = Array.prototype.map.call(arguments, function (a) {
                return a instanceof Error ? (a.stack || a.message) : typeof a === "object" ? JSON.stringify(a) : String(a);
            });
            post({t: "console", level: level, args: args});
        };
    }
    let hidden = false, inEvent = false, init = null;
    // lastSeq is the last event handled; a batch says it has seen it (ack),
    // so the page can tell a stale write to a field typed in since.
    let lastSeq = 0, ackedSeq = 0, sentBatch = false;
    let visibilityCb = null;

    // Batches go to the page. A binary batch is copied out of the module's
    // memory (Go reuses it), and the copy is transferred, not cloned.
    const postBatch = function (m, transfer) {
        sentBatch = true;
        m.ack = lastSeq;
        ackedSeq = lastSeq;
        post(m, transfer);
    };
    globalThis.applyMutations = function (json) { postBatch({t: "batch", json: json}); };
    goowee.applyBatch = function (u8, len) {
        const copy = u8.slice(0, len);
        postBatch({t: "batch", bin: copy.buffer}, [copy.buffer]);
    };
    goowee.applyBatchAt = function (ptr, len) {
        const copy = new Uint8Array(wasmExports.mem.buffer, ptr, len).slice();
        postBatch({t: "batch", bin: copy.buffer}, [copy.buffer]);
    };
    globalThis.goListen = function (type, capture) { post({t: "listen", type: type, capture: !!capture}); };
    goowee.log = function (entry) { post({t: "log", entry: entry}); };
    goowee.node = function () { return undefined; }; // a worker has no DOM nodes
    // goowee/canvas buffers are replayed on the page's canvases, after the
    // batch that created them.
    goowee.draw = function (u8, nbytes, nodeId) {
        const copy = u8.slice(0, nbytes);
        post({t: "draw", buf: copy.buffer, nodeId: nodeId}, [copy.buffer]);
    };
    // A root's container is a stand-in here ({id, selector}); one the page
    // reported server markup for says so.
    goowee._hasServerMarkup = function (c) { return !!(c && c.serverMarkup); };
    // Properties Go sets on goowee that the page reports (goowee.transport),
    // mirrored there.
    for (const key of ["transport", "calls", "noDevTools"]) {
        let value = goowee[key];
        Object.defineProperty(goowee, key, {
            configurable: true,
            get: function () { return value; },
            set: function (v) { value = v; post({t: "set", key: key, value: v}); },
        });
    }
    goowee.mountRoot = function (parentId, container, base, hydrate) {
        post({t: "mount", parentId: parentId, selector: container.selector, base: base, hydrate: hydrate});
    };
    goowee.unmountRoot = function (parentId) { post({t: "unmount", parentId: parentId}); };
    // Dev tools: the page learns the inspector exists when Go registers it.
    let inspectGo;
    Object.defineProperty(goowee, "inspectGo", {
        configurable: true,
        get: function () { return inspectGo; },
        set: function (fn) { inspectGo = fn; post({t: "set", key: "_workerInspector", value: true}); },
    });

    // Frames: right after an event (the page is waiting for its result), on
    // a timer while the page is hidden, else on the worker's own animation
    // frame. Go's frame loop ignores a second callback for one request.
    // A frame that changed nothing still acknowledges the events before it,
    // so the page releases writes it held for them.
    const workerFrame = function () {
        sentBatch = false;
        runFrame();
        if (!sentBatch && lastSeq > ackedSeq) {
            ackedSeq = lastSeq;
            post({t: "ack", seq: lastSeq});
        }
    };
    goowee.requestFrame = function (wantTimer) {
        if (wantTimer || hidden || inEvent || typeof requestAnimationFrame !== "function") setTimeout(workerFrame, 0);
        else requestAnimationFrame(workerFrame);
    };

    onmessage = function (e) {
        const d = e.data;
        switch (d.t) {
            case "init": start(d); break;
            case "event": {
                if (d.files) for (const [h, f] of d.files) fileStore[h] = f;
                inEvent = true;
                try {
                    for (const id of d.targets) {
                        goowee.eventCalls++;
                        const r = viaExports ? callEvent(id, d.type, d.payload) : decodeDispatch(handleEvent(id, d.type, d.payload));
                        if (r & 4) break; // a handler stopped propagation
                    }
                } finally {
                    inEvent = false;
                    lastSeq = d.seq;
                }
                break;
            }
            case "replies":
                if (typeof goowee.resolveReads === "function") goowee.resolveReads(d.value);
                break;
            case "inspect": {
                let value = null;
                try {
                    value = d.what === "snapshot" ? goowee.inspectGo() : goowee.changesGo(d.what === "clear");
                    value = JSON.parse(JSON.stringify(value)); // plain data for postMessage
                } catch (err) {
                    value = {error: String(err)};
                }
                post({t: "inspect-result", id: d.id, value: value});
                break;
            }
            case "files-release":
                for (const h of d.handles) delete fileStore[h];
                break;
            case "popstate":
                loc = d.location;
                hstate = d.state;
                if (listeners.popstate) listeners.popstate();
                break;
            case "scroll":
                scroll = {x: d.x, y: d.y};
                if (listeners.scroll) listeners.scroll();
                break;
            case "visibility":
                hidden = d.hidden;
                if (visibilityCb) visibilityCb(hidden);
                break;
        }
    };

    let loc = null, hstate = null, scroll = {x: 0, y: 0};
    const listeners = {}, stores = {};
    const setLoc = function (url) {
        const u = new URL(url, loc.href);
        loc = {pathname: u.pathname, search: u.search, hash: u.hash, href: u.href};
    };
    const pageCall = function (call) {
        post({t: "host", call: call, args: Array.prototype.slice.call(arguments, 1)});
    };
    // workerStorage is the worker's copy of a storage area: reads answer at
    // once from the copy the page sent at start, writes update it and go to
    // the page, which writes the real area.
    function workerStorage(area, entries) {
        const m = new Map(entries);
        return {
            get length() { return m.size; },
            key: function (i) { const k = Array.from(m.keys())[i]; return k === undefined ? null : k; },
            getItem: function (k) { return m.has(k) ? m.get(k) : null; },
            setItem: function (k, v) { v = String(v); m.set(k, v); post({t: "storage", area: area, op: "set", key: k, value: v}); },
            removeItem: function (k) { m.delete(k); post({t: "storage", area: area, op: "remove", key: k}); },
            clear: function () { m.clear(); post({t: "storage", area: area, op: "clear"}); },
        };
    }

    function start(d) {
        init = d;
        hidden = d.hidden;
        loc = d.location;
        hstate = d.historyState;
        for (const area of ["localStorage", "sessionStorage"]) {
            const entries = d.storage && d.storage[area];
            if (entries) stores[area] = workerStorage(area, entries);
        }
        goowee.devEnabled = function () { return d.dev; };
        // The worker's host: what the page told it at start. History,
        // storage and the rest arrive in later phases; until then they do
        // nothing, or report themselves unavailable.
        goowee.host = {
            worker: true,
            hydrating: function () { return d.hydrating; },
            // A stand-in for #root: the page mounts into the element with
            // this selector (goowee.mountRoot posts it).
            root: function () { return {id: "root", selector: "#root"}; },
            // bridge.Mount: the page finds the element when it mounts.
            query: function (selector) { return {id: "", selector: selector, serverMarkup: false}; },
            // bridge.Islands: the islands the page described at start.
            queryAll: function (selector) {
                if (selector !== "[data-goowee-island]") return [];
                return (d.islands || []).map(function (isl) {
                    return {id: isl.id, selector: isl.selector, serverMarkup: isl.serverMarkup,
                        getAttribute: function (name) { return Object.prototype.hasOwnProperty.call(isl.attrs, name) ? isl.attrs[name] : null; }};
                });
            },
            body: function () { return null; },
            overlayContainer: function () {
                pageCall("overlayContainer");
                return {id: "goowee-devtools", selector: "#goowee-devtools"};
            },
            hidden: function () { return hidden; },
            onVisibilityChange: function (cb) { visibilityCb = cb; },
            baseURI: function () { return d.baseURI; },
            baseHref: function () { return d.baseHref; },
            location: function () { return loc; },
            historyState: function () { return hstate; },
            pushState: function (state, url) { hstate = state; setLoc(url); post({t: "history", op: "push", state: state, url: url}); },
            replaceState: function (state, url) { hstate = state; setLoc(url); post({t: "history", op: "replace", state: state, url: url}); },
            back: function () { post({t: "history", op: "back"}); },
            forward: function () { post({t: "history", op: "forward"}); },
            manualScrollRestoration: function () { pageCall("manualScrollRestoration"); },
            // popstate and scroll come from the page; beforeunload can't be
            // answered from a worker in time, so BeforeLeave doesn't guard a
            // page unload in worker mode.
            on: function (type, cb) { listeners[type] = cb; },
            // The window's position as the page last reported it; a scroll
            // container (ScrollOptions.Container) isn't tracked here.
            scrollPosition: function () { return {x: scroll.x, y: scroll.y}; },
            scrollTo: function (container, x, y) { pageCall("scrollTo", container, x, y); },
            // Can't ask the page whether the target exists: a fragment is
            // assumed to have one, and the page scrolls to it if it does.
            scrollToFragment: function (fragment, container) {
                if (!fragment) return false;
                pageCall("scrollToFragment", fragment, container);
                return true;
            },
            focusRoute: function () { pageCall("focusRoute"); },
            announceRoute: function () { pageCall("announceRoute"); },
            storage: function (area) { return stores[area] || null; },
            dataScript: function (id) {
                return d.dataScripts && Object.prototype.hasOwnProperty.call(d.dataScripts, id) ? d.dataScripts[id] : null;
            },
        };
        try {
            importScripts(d.wasmExec);
        } catch (err) {
            post({t: "boot-error", message: "goowee: couldn't load " + d.wasmExec + " in the worker (" + err + ")"});
            return;
        }
        const go = new Go();
        loadWasm(d.wasm, go.importObject)
            .then(function (instance) {
                post({t: "mark", name: "goowee:instantiated"});
                if (d.calls !== "callbacks") goowee.useInstance(go, instance);
                post({t: "running"});
                go.run(instance);
            })
            .catch(function (err) {
                post({t: "boot-error", message: (err && err.message) || ("goowee: starting " + d.wasm + " failed (" + err + ")")});
            });
    }
}
