// Measures what worker mode is for (#158): test/e2e/busy runs about two
// seconds of pure Go computation in a click handler. During it, the longest
// gap between the page's animation frames shows whether the page kept
// painting (main thread: one gap of about two seconds; worker: frames keep
// coming). Also the click -> DOM latency of a counter at rest, both ways.
// Usage: node test/e2e/busy.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8147), DP = Number(process.env.CDP_PORT || 9447);
const dir = mkdtempSync(join(tmpdir(), "goowee-busy-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/busy"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

const page = (worker) => `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot({worker: ${worker}})</script></body></html>`;
const server = createServer((req, res) => {
  const u = new URL(req.url, "http://x");
  if (u.pathname === "/wasm_exec.js" || u.pathname === "/goowee.js") { res.writeHead(200, { "content-type": "text/javascript" }); return res.end(readFileSync(join(dir, u.pathname))); }
  if (u.pathname === "/main.wasm") { res.writeHead(200, { "content-type": "application/wasm" }); return res.end(readFileSync(join(dir, "main.wasm"))); }
  if (u.pathname === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(page(u.searchParams.get("worker") === "1")); }
  res.writeHead(404); res.end();
}).listen(PORT);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}

const profile = mkdtempSync(join(tmpdir(), "goowee-busy-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("BUSY CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target; // up to 20s: Chrome can take ~8s to open its debug port
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); let log = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && (m.params.type === "error" || m.params.type === "warning"))
    log.push(m.params.type + ": " + m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") log.push("exception: " + (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text)); };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");

const sleep2 = (ms) => new Promise((r) => setTimeout(r, ms));
async function measure(worker) {
  await send("Page.navigate", { url: `http://localhost:${PORT}/?worker=${worker ? 1 : 0}` });
  for (let i = 0; i < 100 && (await ev(`document.querySelector("#count")?.textContent`)) !== "0"; i++) await sleep2(100);
  check(await ev(`!!goowee.worker`) === worker, `worker mode is ${await ev(`!!goowee.worker`)}, want ${worker}`);
  // Click -> DOM updated for the counter, at rest (the issue's spike).
  const latency = await ev(`(async () => {
    const p = document.querySelector("#count"), b = document.querySelector("#inc"), out = [];
    for (let i = 0; i < 30; i++) {
      await new Promise((res) => {
        let t0;
        const mo = new MutationObserver(() => { mo.disconnect(); out.push(performance.now() - t0); res(); });
        mo.observe(p, {childList: true, characterData: true, subtree: true});
        t0 = performance.now();
        b.click();
      });
      await new Promise((r) => setTimeout(r, 23 + (i % 7) * 3));
    }
    out.sort((a, b) => a - b);
    return {p50: out[15], p90: out[27]};
  })()`);
  // The longest gap between animation frames while Go computes.
  const gap = await ev(`(async () => {
    let last = performance.now(), max = 0, run = true;
    const tick = (t) => { max = Math.max(max, t - last); last = t; if (run) requestAnimationFrame(tick); };
    requestAnimationFrame(tick);
    await new Promise((r) => setTimeout(r, 100));
    const t0 = performance.now();
    document.querySelector("#work").click();
    while (!/^done/.test(document.querySelector("#result").textContent)) await new Promise((r) => setTimeout(r, 20));
    const took = performance.now() - t0;
    await new Promise((r) => setTimeout(r, 100));
    run = false;
    return {maxGap: max, took: took};
  })()`);
  return { latency, gap };
}
const f = (x) => x.toFixed(1).padStart(7);
try {
  const main = await measure(false), work = await measure(true);
  console.log("                           main thread     worker");
  console.log(`  longest frame gap (ms)  ${f(main.gap.maxGap)}      ${f(work.gap.maxGap)}`);
  console.log(`  computation took (ms)   ${f(main.gap.took)}      ${f(work.gap.took)}`);
  console.log(`  click -> DOM p50 (ms)   ${f(main.latency.p50)}      ${f(work.latency.p50)}`);
  console.log(`  click -> DOM p90 (ms)   ${f(main.latency.p90)}      ${f(work.latency.p90)}`);
  // The measurement is meaningful only if the main thread visibly blocked.
  check(main.gap.maxGap > 1000, `the main-thread computation didn't block frames (${main.gap.maxGap.toFixed(0)} ms): the benchmark measures nothing`);
  check(work.gap.maxGap < 250, `worker mode: frames stopped for ${work.gap.maxGap.toFixed(0)} ms while Go computed`);
} catch (e) { fail.push(e.message); }
console.log(fail.length ? "BUSY CHECK FAIL:\n- " + fail.join("\n- ") : "BUSY CHECK PASS: the page keeps painting while Go computes in a worker");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
