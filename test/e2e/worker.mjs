// Runs test/e2e/hello in a Web Worker (#158, phase 4): the page boots with
// goowee.boot({worker: true}) and has no wasm_exec.js script tag; the worker
// loads it, runs the Go program, and sends its batches to the page, which
// forwards events back. Also checks that a missing wasm rejects the boot.
// Usage: node test/e2e/worker.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8146), DP = Number(process.env.CDP_PORT || 9446);
const dir = mkdtempSync(join(tmpdir(), "goowee-worker-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/hello"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

// No wasm_exec.js on the page: only the worker loads it.
const page = (wasm) => `<!doctype html><html><head><meta charset="utf-8"><title>Shell title</title><script src="/goowee.js"></script></head><body><div id="root"><p class="splash">Loading…</p></div><script>window.__boot = goowee.boot({wasm: "${wasm}", wasmExec: "/wasm_exec.js", worker: true}); window.__boot.catch(function (e) { window.__rejected = e.message; });</script></body></html>`;
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/wasm_exec.js" || path === "/goowee.js") { res.writeHead(200, { "content-type": "text/javascript" }); return res.end(readFileSync(join(dir, path))); }
  if (path === "/main.wasm") { res.writeHead(200, { "content-type": "application/wasm" }); return res.end(readFileSync(join(dir, "main.wasm"))); }
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(page("/main.wasm")); }
  if (path === "/missing/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(page("/missing.wasm")); }
  res.writeHead(404, { "content-type": "text/plain" }); res.end("not found");
}).listen(PORT);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}

const profile = mkdtempSync(join(tmpdir(), "goowee-worker-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("WORKER CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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

async function waitFor(expr, ms = 10000) {
  for (let i = 0; i < ms / 100; i++) { if (await ev(expr)) return true; await sleep(100); }
  return false;
}
try {
  log = [];
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  check(await waitFor(`document.querySelector("#hello")?.textContent === "clicked 0"`), "the app didn't render from the worker: " + await ev(`document.body.innerHTML.slice(0, 200)`));
  check(await ev(`goowee.worker instanceof Worker`), "goowee.worker isn't a Worker");
  check(await ev(`typeof Go === "undefined"`), "Go was defined on the page: wasm_exec.js ran there");
  await ev(`document.querySelector("#inc").click()`);
  check(await waitFor(`document.querySelector("#hello").textContent === "clicked 1"`), "a click didn't reach the worker's handler: " + await ev(`document.querySelector("#hello").textContent`));
  check(await waitFor(`document.title === "(1) hello"`), "document title: " + await ev(`document.title`));
  for (let i = 0; i < 5; i++) await ev(`document.querySelector("#inc").click()`);
  check(await waitFor(`document.querySelector("#hello").textContent === "clicked 6"`), "five quick clicks: " + await ev(`document.querySelector("#hello").textContent`));
  check(log.length === 0, "console errors or warnings: " + log.join(" | "));

  log = [];
  await send("Page.navigate", { url: `http://localhost:${PORT}/missing/` });
  check(await waitFor(`!!window.__rejected`), "a missing wasm didn't reject the boot");
  const err = await ev(`JSON.stringify([window.__rejected, goowee.bootError && goowee.bootError.message])`);
  check(/missing\.wasm failed: HTTP 404/.test(err) && JSON.parse(err)[1] === JSON.parse(err)[0], "boot error: " + err);
} catch (e) { fail.push(e.message); }
console.log(fail.length ? "WORKER CHECK FAIL:\n- " + fail.join("\n- ") : "WORKER CHECK PASS: hello runs in a worker (render, clicks, reactive title, no wasm_exec.js on the page); a missing wasm rejects the boot");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
