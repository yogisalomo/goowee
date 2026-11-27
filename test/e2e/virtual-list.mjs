// Drives test/e2e/virtual-list in headless Chrome (#157): a 100,000-row keyed
// VirtualList renders only its window, measured row sizes drive the spacers,
// row state survives an insert above the viewport, ScrollToIndex works, and
// 120 range recomputations over 100k rows stay well under a frame budget.
// Usage: node test/e2e/virtual-list.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8157), DP = Number(process.env.CDP_PORT || 9457);
const dir = mkdtempSync(join(tmpdir(), "goowee-vlist-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/virtual-list"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}
const index = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-vlist-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("VIRTUAL-LIST CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target;
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") errors.push("exception: " + (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text)); };
const send = (method, params = {}) => Promise.race([
  new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); }),
  sleep(10000).then(() => { throw new Error("CDP timeout: " + method + " " + JSON.stringify(params).slice(0, 120)); }),
]);
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;
const waitFor = async (expr, label) => { for (let i = 0; i < 200; i++) { if (await ev(expr)) return; await sleep(50); } throw new Error("timeout: " + label); };
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");
// GOOWEE_WORKER=1 runs the app in a Web Worker (#158); run.sh runs both ways.
const WORKER = process.env.GOOWEE_WORKER === "1";
if (WORKER) await send("Page.addScriptToEvaluateOnNewDocument", { source: `try { localStorage.setItem("goowee-worker", "1") } catch (_) {}` });
// Count observer targets before goowee.js runs: the row scanner must watch
// only the opted-in list, and unmounting must unobserve its rows.
await send("Page.addScriptToEvaluateOnNewDocument", { source: `
  window.__obs = {mo: [], roObserved: 0, roUnobserved: 0};
  const RealMO = window.MutationObserver;
  window.MutationObserver = class extends RealMO {
    observe(target, opts) {
      const kind = target === document ? "document"
        : target.getAttribute && target.hasAttribute("data-goowee-virtual-list") ? "vlist" : "other";
      window.__obs.mo.push(kind);
      return super.observe(target, opts);
    }
  };
  const RealRO = window.ResizeObserver;
  window.ResizeObserver = class extends RealRO {
    observe(el) { window.__obs.roObserved++; return super.observe(el); }
    unobserve(el) { window.__obs.roUnobserved++; return super.unobserve(el); }
  };
` });
try {
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  await waitFor(`document.querySelector("#status")?.textContent === "items=100000"`, "virtual list boot");

  const mounted = await ev(`document.querySelectorAll(".vlist-row").length`);
  check(mounted > 0 && mounted < 100, `expected a small window, got ${mounted} rows`);

  const moTargets = await ev(`window.__obs.mo`);
  check(!moTargets.includes("document"), `row scanner watched the document: ${moTargets}`);
  check(moTargets.includes("vlist"), `row scanner did not watch the list: ${moTargets}`);
  check((await ev(`window.__obs.roObserved`)) > 0, "no rows were observed for measurement");

  // A stateful row keeps its counter when 100 items are inserted above it.
  await ev(`document.querySelector('.vl-row[data-id="0"] .bump').click()`);
  await waitFor(`document.querySelector('.vl-row[data-id="0"]')?.textContent.includes("#1")`, "row counter");
  await ev(`document.querySelector("#prepend").click()`);
  await waitFor(`document.querySelector("#status").textContent === "items=100001"`, "prepend");
  const state = await ev(`document.querySelector('.vl-row[data-id="0"]')?.textContent`);
  check(state && state.includes("#1"), `row state was lost on insert: ${state}`);

  // ScrollToIndex jumps into the middle of a 100k list.
  await ev(`document.querySelector("#scroll").click()`);
  await waitFor(`Number(document.querySelector('.vlist-row')?.getAttribute('data-row-index')) >= 49990`, "ScrollToIndex");

  // Measure 120 range recomputations over 100k rows (Fenwick prefix sums,
  // keyed row cache). Headless Chrome reports the page as hidden and throttles
  // requestAnimationFrame, so drive the benchmark from Go instead of rAF.
  await ev(`document.querySelector("#bench").click()`);
  await waitFor(`/bench=[1-9]/.test(document.querySelector("#benchresult").textContent)`, "scroll benchmark");
  const benchText = await ev(`document.querySelector("#benchresult").textContent`);
  const us = Number(benchText.match(/bench=([\d.]+)us/)?.[1] || Infinity);
  check(us < 2000, `100k range recomputation took ${benchText}`);
  console.log(`VIRTUAL-LIST 100k scroll: ${mounted} mounted rows, ${benchText} per range update`);

  // Unmounting the list must release every observed row.
  const beforeHide = await ev(`window.__obs.roUnobserved`);
  await ev(`document.querySelector("#hide").click()`);
  await waitFor(`!document.querySelector(".vlist")`, "list unmount");
  await waitFor(`window.__obs.roUnobserved > ${beforeHide}`, "rows unobserved on unmount");
  check(errors.length === 0, "console errors:\n" + errors.join("\n"));
} catch (e) { fail.push(e.message); }
console.log(fail.length ? "VIRTUAL-LIST CHECK FAIL:\n- " + fail.join("\n- ") : "VIRTUAL-LIST CHECK PASS: keyed rows keep state, ScrollToIndex works, and 100k rows scroll within frame budget");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
