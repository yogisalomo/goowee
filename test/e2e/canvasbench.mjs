// Measures the goowee/canvas demo (examples/graph: 10,000 nodes and 10,000
// edges redrawn every frame) in headless Chrome, and reports where a frame's
// time goes (#159):
//   - Go:     the frame callback up to the crossing: frame callbacks, recording
//             the picture, the flush, and copying the buffer into JS;
//   - record: recording alone, as the demo measures it in Go;
//   - replay: goowee.draw, issuing the canvas calls;
//   - frame:  the whole frame callback;
//   - interval: time between frames that drew.
// Budget on the reference machine (Apple M1): Go + replay at most 4 ms.
//
// Usage: node test/e2e/canvasbench.mjs   (make canvasbench)
//   SOFTWARE=1  raster in software (--disable-gpu), as on a machine without
//               a GPU; the default rasters on the GPU, like a browser
//   FRAMES=n  frames to measure (default 240)
//   STRICT=1  exit 1 when over budget
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8154), DP = Number(process.env.CDP_PORT || 9454);
const FRAMES = Number(process.env.FRAMES || 240), BUDGET_MS = 4;
const dir = mkdtempSync(join(tmpdir(), "goowee-canvasbench-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./examples/graph"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));
copyFileSync("examples/graph/index.html", join(dir, "index.html"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}
const types = { "/": "text/html", "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (!types[path]) { res.writeHead(404); return res.end(); }
  res.writeHead(200, { "content-type": types[path] });
  res.end(readFileSync(join(dir, path === "/" ? "index.html" : path)));
}).listen(PORT);

// Before anything loads: time every animation frame callback, and every
// goowee.draw inside one (goowee.js defines goowee; the bridge looks up
// goowee.draw at its first draw, after this wrapper is in place).
const probe = `(() => {
  window.bench = { frames: [] };
  let cur = null;
  const raf = window.requestAnimationFrame.bind(window);
  window.requestAnimationFrame = (cb) => raf((ts) => {
    const f = { ts, start: performance.now(), drawStart: 0, drawEnd: 0 };
    cur = f;
    try { cb(ts); } finally {
      f.end = performance.now();
      cur = null;
      if (f.drawStart) bench.frames.push(f);
    }
  });
  let g;
  Object.defineProperty(window, "goowee", { configurable: true, get: () => g, set(v) {
    g = v;
    let draw;
    Object.defineProperty(g, "draw", { configurable: true, get: () => draw && function (...a) {
      if (cur) cur.drawStart = performance.now();
      try { return draw.apply(this, a); } finally { if (cur) cur.drawEnd = performance.now(); }
    }, set(fn) { draw = fn; } });
  } });
})();`;

const profile = mkdtempSync(join(tmpdir(), "goowee-canvasbench-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", ...(process.env.SOFTWARE === "1" ? ["--disable-gpu"] : []), "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("CANVASBENCH FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target;
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && (m.params.type === "error" || m.params.type === "warning")) errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") errors.push("exception: " + (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text)); };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;

let failed = false;
try {
  await send("Runtime.enable"); await send("Page.enable");
  await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 800, deviceScaleFactor: 1, mobile: false });
  await send("Page.addScriptToEvaluateOnNewDocument", { source: probe });
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  for (let i = 0; i < 200 && !(await ev(`bench.frames.length > 60`)); i++) await sleep(50);
  await ev(`bench.frames.length = 0`); // warmed up: measure from here
  for (let i = 0; i < 600 && (await ev(`bench.frames.length`)) < FRAMES; i++) await sleep(50);
  const frames = await ev(`bench.frames.map((f) => ({ ts: f.ts, go: f.drawStart - f.start, replay: f.drawEnd - f.drawStart, total: f.end - f.start }))`);
  const stats = await ev(`document.querySelector("#graph-stats").textContent`);
  const size = await ev(`[document.querySelector("#graph").width, document.querySelector("#graph").height]`);
  if (frames.length < FRAMES / 2) throw new Error(`only ${frames.length} frames drew`);

  const q = (xs, p) => { const s = [...xs].sort((a, b) => a - b); return s[Math.min(s.length - 1, Math.floor(p * s.length))]; };
  const row = (name, xs) => `${name.padEnd(22)} ${q(xs, 0.5).toFixed(2).padStart(7)} ${q(xs, 0.95).toFixed(2).padStart(7)}`;
  const intervals = frames.slice(1).map((f, i) => f.ts - frames[i].ts);
  const record = Number((/recorded in ([\d.]+) ms/.exec(stats) || [])[1]);
  const work = frames.map((f) => f.go + f.replay);
  console.log(`goowee/canvas: ${stats}`);
  console.log(`canvas ${size[0]}×${size[1]}, ${frames.length} frames, ${process.env.SOFTWARE === "1" ? "software" : "GPU"} raster\n`);
  console.log(`${"ms per frame".padEnd(22)} ${"median".padStart(7)} ${"p95".padStart(7)}`);
  console.log(`${"record (Go)".padEnd(22)} ${(isNaN(record) ? "?" : record.toFixed(2)).padStart(7)}`);
  console.log(row("Go up to the crossing", frames.map((f) => f.go)));
  console.log(row("replay (goowee.draw)", frames.map((f) => f.replay)));
  console.log(row("Go + replay", work));
  console.log(row("frame callback", frames.map((f) => f.total)));
  console.log(row("frame interval", intervals));
  const med = q(work, 0.5);
  console.log(`\nbudget: Go + replay ≤ ${BUDGET_MS} ms: ${med <= BUDGET_MS ? "within" : "OVER"} (median ${med.toFixed(2)} ms)`);
  if (med > BUDGET_MS && process.env.STRICT === "1") failed = true;
  if (errors.length) { console.log("console errors:\n" + errors.join("\n")); failed = true; }
} catch (e) { console.log("CANVASBENCH FAIL: " + e.message); failed = true; }
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(failed ? 1 : 0);
