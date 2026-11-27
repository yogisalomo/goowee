// Calls into Go through its exports (#133, ADR-031), in headless Chrome. The
// app (test/e2e/calls) is loaded twice: with the exports (the default) and
// with every call on syscall/js callbacks (goowee.boot({calls: "callbacks"})).
// Each load checks:
//   - events and frames work, and goowee.calls says which way they went
//   - a goroutine a click handler starts runs with no further event
//   - a worker goroutine a handler wakes through a channel runs
//   - a goroutine a mount effect starts runs
//   - a click dispatched inside a batch (ref.Click) re-enters Go while the
//     runtime reads that batch, grows the memory by 64 MB, and the rest of the
//     batch still applies
// and reports what an event dispatch and a one-update frame cost each way.
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8151), DP = Number(process.env.CDP_PORT || 9451);
const dir = mkdtempSync(join(tmpdir(), "goowee-calls-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/calls"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}
// Every requestAnimationFrame callback is timed; Go's frame is the only one.
const index = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>
  window.__raf = window.requestAnimationFrame.bind(window); window.__frames = [];
  window.requestAnimationFrame = (cb) => __raf((ts) => { const t0 = performance.now(); cb(ts); __frames.push(performance.now() - t0); });
  goowee.boot({calls: new URLSearchParams(location.search).get("calls") || undefined});
</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  const headers = { "Cross-Origin-Opener-Policy": "same-origin", "Cross-Origin-Embedder-Policy": "require-corp" };
  if (path === "/") { res.writeHead(200, { ...headers, "content-type": "text/html" }); return res.end(index); }
  if (types[path]) { res.writeHead(200, { ...headers, "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-calls-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("CALLS CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target; // up to 20s: Chrome can take ~8s to open its debug port
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") errors.push("exception: " + (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text)); };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;
const text = (sel) => ev(`document.querySelector(${JSON.stringify(sel)})?.textContent`);
const waitText = async (sel, want, label) => { let t; for (let i = 0; i < 100; i++) { t = await text(sel); if (t === want) return; await sleep(20); } throw new Error(`timeout: ${label || sel}: got ${JSON.stringify(t)}, want ${JSON.stringify(want)}`); };
const click = (sel) => ev(`document.querySelector(${JSON.stringify(sel)}).click()`);
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");
// GOOWEE_WORKER=1 runs the app in a Web Worker (#158); run.sh runs both ways.
const WORKER = process.env.GOOWEE_WORKER === "1";
if (WORKER) await send("Page.addScriptToEvaluateOnNewDocument", { source: `try { localStorage.setItem("goowee-worker", "1") } catch (_) {}` });

// The cost of one event dispatch (click → Go handler → back), and of a frame
// that applies one text update, as medians.
const measure = `(async () => {
  const b = document.querySelector("#inc"), afterFrame = () => new Promise((r) => __raf(() => __raf(r)));
  const med = (a) => { a.sort((p, q) => p - q); return a[a.length >> 1]; };
  const ev = [], fr = [];
  for (let round = 0; round < 20; round++) {
    const t0 = performance.now();
    for (let i = 0; i < 200; i++) b.click();
    ev.push((performance.now() - t0) / 200);
    await afterFrame();
  }
  for (let i = 0; i < 120; i++) {
    __frames.length = 0; b.click(); await afterFrame();
    if (i >= 20 && __frames.length === 1) fr.push(__frames[0]);
  }
  return {event: med(ev), frame: med(fr)};
})()`;

const results = {};
try {
  for (const mode of ["exports", "callbacks"]) {
    const tag = `[${mode}] `;
    await send("Page.navigate", { url: `http://localhost:${PORT}/` + (mode === "callbacks" ? "?calls=callbacks" : "") });
    await waitText("#mounted", "mounted", tag + "a goroutine a mount effect started ran");
    check(await ev("goowee.calls") === mode, `${tag}goowee.calls is ${await ev("goowee.calls")}`);
    // No call into Go through syscall/js's handleEvent once the exports are
    // in use; with callbacks, every click goes through it.
    await ev(`(() => { const f = window.handleEvent; window.__viaCallback = 0;
      window.handleEvent = function () { __viaCallback++; return f.apply(this, arguments); }; })()`);
    await click("#inc");
    await waitText("#inc", "n 1", tag + "a click updated the count");
    const viaCallback = await ev("__viaCallback");
    // (In worker mode Go's handleEvent lives in the worker, where this page's
    // counter can't see it; goowee.calls above says which way it went.)
    if (!WORKER) check(mode === "exports" ? viaCallback === 0 : viaCallback === 1, `${tag}${viaCallback} clicks went through handleEvent`);
    await click("#spawn");
    await waitText("#spawned", "spawned", tag + "a goroutine started in a handler ran with no further event");
    await click("#wake");
    await waitText("#woken", "woken 7", tag + "a worker woken through a channel from a handler ran");
    await click("#grow");
    await waitText("#grown", "grown 1", tag + "the click inside the batch reached Go");
    await waitText("#labels span:last-child", "label 299", tag + "the batch applied past a memory growth");
    check(await ev(`[...document.querySelectorAll("#labels span")].every((s, i) => s.textContent === "label " + i)`), tag + "a label after the re-entrant click is wrong");
    results[mode] = await ev(measure);
  }
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
const us = (ms) => (ms * 1000).toFixed(1).padStart(8) + " µs";
if (results.exports && results.callbacks) {
  console.log(`                              exports    callbacks`);
  console.log(`  event dispatch (click)    ${us(results.exports.event)}  ${us(results.callbacks.event)}`);
  console.log(`  frame, one text update    ${us(results.exports.frame)}  ${us(results.callbacks.frame)}`);
}
console.log(fail.length ? "CALLS CHECK FAIL:\n- " + fail.join("\n- ") : "CALLS CHECK PASS" + (WORKER ? " (worker)" : "") + ": events and frames through the exports, goroutines resume, re-entry past a memory growth, callbacks still work");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
