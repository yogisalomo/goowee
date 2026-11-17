// Mutation-transport measurement (roadmap 3.4): the binary transport against
// the JSON one, in headless Chrome, same no-dependency CDP pattern as
// smoke.mjs. Two parts:
//
//  1. Replay. Go writes real frames in both encodings (TestWriteTransportFrames
//     in internal/dom): the layout graph's mount, 90 layout ticks of 5,000
//     attribute updates and 400 nudges of 10. A page with runtime/goowee.js
//     applies them, alternating formats frame by frame, and times
//       - transport: getting the batch to the runtime and decoding it — for
//         JSON the UTF-8 decode syscall/js does to pass the string plus
//         JSON.parse, for binary the copy CopyBytesToJS does plus the decode
//       - apply:     the transport plus the DOM writes
//       - layout:    the style + layout the writes cause (forced right after)
//  2. End to end. The real WASM app (test/e2e/graph) runs over each transport,
//     in fresh page loads that alternate which transport goes first, and the
//     page times every frame Go renders: its requestAnimationFrame callback
//     (flush, encode, send, the runtime's apply) and the runtime's apply
//     within it, after warm-up frames. Chrome compiles the WASM with V8's
//     optimizing tier only (--no-liftoff), so every load measures the
//     steady-state code a long-running app reaches, not a first load's
//     baseline-tier code.
//
// The pages are served cross-origin isolated, so performance.now() is precise
// to 5 µs rather than 0.1 ms. Timings compare on one machine only, and read
// best on an idle one.
//
// Env: CHROME (browser path), CDP_PORT (default 9347), PORT (default 8139),
// ROUNDS (page loads per transport, default 3).
import { execFileSync, spawn } from "node:child_process";
import { copyFileSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, extname, join } from "node:path";
import { fileURLToPath } from "node:url";

const DP = Number(process.env.CDP_PORT || 9347);
const PORT = Number(process.env.PORT || 8139);
const ROUNDS = Number(process.env.ROUNDS || 3);
const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
const dir = mkdtempSync(join(tmpdir(), "goowee-transport-"));

console.log("building the frames and the graph app…");
execFileSync("go", ["test", "-count=1", "-run", "^TestWriteTransportFrames$", "./internal/dom/"], {
  cwd: root, stdio: ["ignore", "ignore", "inherit"],
  env: { ...process.env, GOOWEE_TRANSPORT_FRAMES: join(dir, "frames.json") },
});
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/graph"], {
  cwd: root, stdio: "inherit", env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
});
const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js")
  : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync(join(root, "runtime/goowee.js"), join(dir, "goowee.js"));
writeFileSync(join(dir, "replay.html"), `<!doctype html><meta charset="utf-8"><title>replay</title>
<script src="goowee.js"></script><div id="root"></div>`);
// Every requestAnimationFrame callback is timed (Go's is the only one), and so
// is the runtime's apply, whichever transport calls it.
writeFileSync(join(dir, "index.html"), `<!doctype html><meta charset="utf-8"><title>graph</title>
<div id="root"></div>
<script src="wasm_exec.js"></script>
<script src="goowee.js"></script>
<script>
  window.__raf = window.requestAnimationFrame.bind(window);
  window.__frames = []; window.__applies = [];
  window.requestAnimationFrame = (cb) => __raf((ts) => { const t0 = performance.now(); cb(ts); __frames.push(performance.now() - t0); });
  const timed = (obj, name) => { const f = obj[name]; obj[name] = function () {
    const t0 = performance.now(); const r = f.apply(this, arguments); __applies.push(performance.now() - t0); return r; }; };
  timed(goowee, "applyBatch"); timed(window, "applyMutations");
  goowee.boot({transport: new URLSearchParams(location.search).get("transport")});
</script>`);

const types = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm", ".json": "application/json" };
const server = createServer((req, res) => {
  const file = join(dir, new URL(req.url, "http://x").pathname.replace(/^\/+/, "") || "index.html");
  if (!file.startsWith(dir) || !existsSync(file)) { res.writeHead(404).end(); return; }
  res.writeHead(200, {
    "Content-Type": types[extname(file)] || "application/octet-stream",
    "Cross-Origin-Opener-Policy": "same-origin", "Cross-Origin-Embedder-Policy": "require-corp",
  });
  res.end(readFileSync(file));
}).listen(PORT);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--js-flags=--no-liftoff",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${mkdtempSync(join(tmpdir(), "goowee-transport-chrome-"))}`, "about:blank",
], { stdio: "ignore" });

let ws, nextId = 1;
const pending = new Map();
const send = (method, params = {}) => {
  const id = nextId++;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((res, rej) => pending.set(id, { res, rej }));
};
const evalJS = async (expr) => {
  const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || "eval error");
  return r.result.value;
};
const load = async (path, ready) => {
  await send("Page.navigate", { url: `http://localhost:${PORT}/${path}` });
  for (let i = 0; i < 300; i++) {
    if (await evalJS(ready).catch(() => false)) return;
    await sleep(100);
  }
  throw new Error(`timeout loading ${path}`);
};

// Part 1, in the page: replay Go's frames through both transports.
const replay = `(async () => {
  const F = await (await fetch("frames.json")).json();
  const enc = new TextEncoder(), dec = new TextDecoder();
  const bytes = (b64) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
  const prep = (f) => ({json: enc.encode(f.json), bin: bytes(f.binary)});
  const mount = prep(F.mount), ticks = F.ticks.map(prep), nudges = F.nudge.map(prep);
  let buf = new Uint8Array(1 << 20);
  const copy = (f) => { if (buf.length < f.bin.length) buf = new Uint8Array(2 * f.bin.length); buf.set(f.bin); return f.bin.length; };
  const way = {
    json: {apply: (f) => applyMutations(dec.decode(f.json)), transport: (f) => JSON.parse(dec.decode(f.json)).length},
    binary: {apply: (f) => goowee.applyBatch(buf, copy(f)), transport: (f) => { let n = 0; goowee._decodeBatch(buf, copy(f), () => n++); return n; }},
  };
  way.json.apply(mount);
  const out = {};
  for (const k of ["json", "binary"]) out[k] = {bytes: ticks[0][k === "json" ? "json" : "bin"].length,
    nudgeBytes: nudges[0][k === "json" ? "json" : "bin"].length, transport: [], apply: [], layout: [], nudgeTransport: 0, nudgeApply: 0};
  // Transport alone: decode every tick, alternating formats.
  for (let i = 0; i < ticks.length; i++) {
    const k = i % 2 ? "binary" : "json", t0 = performance.now();
    way[k].transport(ticks[i]);
    if (i >= 10) out[k].transport.push(performance.now() - t0);
  }
  // Transport and DOM writes, then the layout they cause.
  for (let i = 0; i < ticks.length; i++) {
    const k = i % 2 ? "binary" : "json";
    const t0 = performance.now();
    way[k].apply(ticks[i]);
    const t1 = performance.now();
    document.querySelector("svg").getBoundingClientRect(); document.body.offsetHeight;
    if (i >= 10) { out[k].apply.push(t1 - t0); out[k].layout.push(performance.now() - t1); }
  }
  // Small frames, in blocks of 100 per format: per-frame averages.
  const N = 2000;
  for (let round = 0; round < 2; round++) for (const k of ["json", "binary"]) {
    let t0 = performance.now();
    for (let i = 0; i < N; i++) way[k].transport(nudges[i % nudges.length]);
    out[k].nudgeTransport += (performance.now() - t0) / N / 2;
  }
  for (let block = 0; block < 4; block++) {
    const k = block % 2 ? "binary" : "json", t0 = performance.now();
    for (let i = block * 100; i < (block + 1) * 100; i++) way[k].apply(nudges[i]);
    out[k].nudgeApply += (performance.now() - t0) / 100 / 2;
  }
  const med = (a) => { a.sort((p, q) => p - q); return a[a.length >> 1]; };
  for (const k in out) for (const m of ["transport", "apply", "layout"]) out[k][m] = med(out[k][m]);
  return {out, isolated: crossOriginIsolated};
})()`;

// Part 2, in the page: drive the real app; one sample per frame Go renders.
const drive = (base) => `(async () => {
  const afterFrame = () => new Promise((r) => __raf(() => __raf(r)));
  const run = async (fn, n, warm) => {
    const s = {go: [], frame: [], apply: [], gcs: 0};
    for (let i = 1; i <= n + warm; i++) {
      if (i === warm + 1) s.gcs = -graphGCs();
      __frames.length = 0; __applies.length = 0;
      const t0 = performance.now(); window[fn](${base} + i); const t1 = performance.now();
      await afterFrame();
      if (i > warm && __frames.length === 1 && __applies.length === 1) {
        s.go.push(t1 - t0); s.frame.push(__frames[0]); s.apply.push(__applies[0]);
      }
    }
    s.gcs += graphGCs();
    return s;
  };
  return {transport: goowee.transport, tick: await run("graphTick", 50, 20),
          nudge: await run("graphNudge", 200, 20), label: await run("graphLabel", 200, 20)};
})()`;

async function main() {
  let target;
  for (let i = 0; i < 200 && !target; i++) {
    try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); }
    catch {}
    if (!target) await sleep(100);
  }
  if (!target) throw new Error("could not reach Chrome DevTools endpoint");
  ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id && pending.has(m.id)) {
      const p = pending.get(m.id); pending.delete(m.id);
      m.error ? p.rej(new Error(m.error.message)) : p.res(m.result);
    }
  };
  await send("Runtime.enable");
  await send("Page.enable");

  const ms = (v) => (v >= 1 ? v.toFixed(2) + " ms" : (v * 1000).toFixed(1) + " µs").padStart(10);
  const vs = (j, b) => `${ms(j)}  ${ms(b)}  ${((b / j - 1) * 100).toFixed(0).padStart(5)}%`;
  const head = (t) => console.log(`\n${t.padEnd(44)}      json     binary  change`);

  await load("replay.html", "typeof goowee === 'object'");
  const { out: r, isolated } = await evalJS(replay);
  if (!isolated) console.log("(page not cross-origin isolated: timings are only precise to 0.1 ms)");
  head("Replay: layout tick, 5,000 updates (median)");
  console.log(`  payload                                     ${(r.json.bytes / 1024).toFixed(0).padStart(6)} KiB ${(r.binary.bytes / 1024).toFixed(0).padStart(6)} KiB  ${((r.binary.bytes / r.json.bytes - 1) * 100).toFixed(0).padStart(5)}%`);
  console.log(`  transport (to the runtime + decode)       ${vs(r.json.transport, r.binary.transport)}`);
  console.log(`  apply (transport + DOM writes)            ${vs(r.json.apply, r.binary.apply)}`);
  console.log(`  layout                                    ${vs(r.json.layout, r.binary.layout)}`);
  head("Replay: nudge, 10 updates (mean)");
  console.log(`  payload                                     ${String(r.json.nudgeBytes).padStart(6)} B   ${String(r.binary.nudgeBytes).padStart(6)} B    ${((r.binary.nudgeBytes / r.json.nudgeBytes - 1) * 100).toFixed(0).padStart(5)}%`);
  console.log(`  transport                                 ${vs(r.json.nudgeTransport, r.binary.nudgeTransport)}`);
  console.log(`  apply                                     ${vs(r.json.nudgeApply, r.binary.nudgeApply)}`);

  const pool = { json: [], binary: [] };
  for (let round = 0; round < ROUNDS; round++) {
    for (const t of round % 2 ? ["json", "binary"] : ["binary", "json"]) {
      await load(`index.html?transport=${t}`, "document.querySelectorAll('line').length === 1000 && typeof graphTick === 'function'");
      const s = await evalJS(drive(round * 10000 + (t === "json" ? 5000 : 0)));
      if (s.transport !== t) throw new Error(`asked for the ${t} transport, the bridge used ${s.transport}`);
      pool[t].push(s);
    }
  }
  const med = (a) => { a = [...a].sort((p, q) => p - q); return a[a.length >> 1]; };
  const derived = { side: (s, i) => s.frame[i] - s.apply[i], whole: (s, i) => s.go[i] + s.frame[i] };
  const series = (s, m) => derived[m] ? s.frame.map((_, i) => derived[m](s, i)) : s[m];
  const stat = (t, w, m) => med(pool[t].flatMap((s) => series(s[w], m)));
  const loads = (t, w, m) => pool[t].map((s) => ms(med(series(s[w], m))).trim()).join(", ");
  for (const [w, title] of [["tick", "layout tick, 5,000 updates"], ["nudge", "nudge, 10 updates"], ["label", "label, 1 text update"]]) {
    head(`End to end (real WASM): ${title}`);
    console.log(`  signal writes (Go, before the frame)      ${vs(stat("json", w, "go"), stat("binary", w, "go"))}`);
    console.log(`  frame: flush + encode + send + apply      ${vs(stat("json", w, "frame"), stat("binary", w, "frame"))}`);
    console.log(`    the runtime's apply                     ${vs(stat("json", w, "apply"), stat("binary", w, "apply"))}`);
    console.log(`    Go's side (frame − apply)               ${vs(stat("json", w, "side"), stat("binary", w, "side"))}`);
    console.log(`  whole update (writes + frame)             ${vs(stat("json", w, "whole"), stat("binary", w, "whole"))}`);
    const gcs = (t) => pool[t].reduce((a, s) => a + s[w].gcs, 0) / pool[t].reduce((a, s) => a + s[w].frame.length, 0) * 100;
    console.log(`  Go garbage collections per 100 updates   ${gcs("json").toFixed(1).padStart(10)}  ${gcs("binary").toFixed(1).padStart(9)}`);
    console.log(`    frame per page load: json ${loads("json", w, "frame")}; binary ${loads("binary", w, "frame")}`);
  }
  const n = pool.binary.reduce((a, s) => a + s.tick.frame.length + s.nudge.frame.length + s.label.frame.length, 0);
  console.log(`\n(${ROUNDS} page loads per transport, ${n} frames per transport; medians)`);
}

main().catch((e) => { console.error("transport:", e.message); process.exitCode = 2; })
  .finally(() => { try { ws?.close(); } catch {} chrome.kill("SIGKILL"); server.close(); rmSync(dir, { recursive: true, force: true }); });
