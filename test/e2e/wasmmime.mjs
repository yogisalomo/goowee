// Boots test/e2e/hello in headless Chrome under different server behaviours
// for main.wasm (#109): served as application/wasm (the streaming fast path),
// as application/octet-stream and with no Content-Type (the download-then-
// compile fallback, with a warning), and two real failures that must produce
// one actionable error, a goowee:boot-error event and no unhandled rejection:
// an HTML page answering the wasm URL (an SPA fallback) and a 404.
// Usage: node test/e2e/wasmmime.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8145), DP = Number(process.env.CDP_PORT || 9445);
const dir = mkdtempSync(join(tmpdir(), "goowee-mime-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/hello"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));
const wasm = readFileSync(join(dir, "main.wasm"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}

const page = (c) => `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"><p class="splash">Loading…</p></div><script>goowee.boot({wasm: "/${c}/main.wasm"})</script></body></html>`;
// How each case serves its main.wasm.
const wasmFor = {
  ok: (res) => { res.writeHead(200, { "content-type": "application/wasm" }); res.end(wasm); },
  octet: (res) => { res.writeHead(200, { "content-type": "application/octet-stream" }); res.end(wasm); },
  none: (res) => { res.removeHeader("content-type"); res.writeHead(200); res.end(wasm); },
  html: (res) => { res.writeHead(200, { "content-type": "text/html" }); res.end(page("html")); },
  missing: (res) => { res.writeHead(404, { "content-type": "text/plain" }); res.end("not found"); },
};
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/wasm_exec.js" || path === "/goowee.js") { res.writeHead(200, { "content-type": "text/javascript" }); return res.end(readFileSync(join(dir, path))); }
  const [, c, file] = path.split("/");
  if (wasmFor[c] && file === "main.wasm") return wasmFor[c](res);
  if (wasmFor[c]) { res.writeHead(200, { "content-type": "text/html" }); return res.end(page(c)); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-mime-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("WASM MIME CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
// Count which WebAssembly entry point boot used, and record boot-error events.
await send("Page.addScriptToEvaluateOnNewDocument", { source: `
  window.__calls = {streaming: 0, bytes: 0}; window.__bootErrors = [];
  const s = WebAssembly.instantiateStreaming, b = WebAssembly.instantiate;
  WebAssembly.instantiateStreaming = function () { __calls.streaming++; return s.apply(this, arguments); };
  WebAssembly.instantiate = function () { __calls.bytes++; return b.apply(this, arguments); };
  addEventListener("goowee:boot-error", (e) => __bootErrors.push(e.detail.message));` });

async function load(c) {
  log = [];
  await send("Page.navigate", { url: `http://localhost:${PORT}/${c}/` });
  for (let i = 0; i < 100; i++) {
    if (await ev(`!!document.querySelector("#hello") || (window.__bootErrors || []).length > 0`)) break;
    await sleep(100);
  }
  await sleep(300); // let late console messages arrive
  return { calls: await ev(`JSON.stringify(__calls)`), errors: await ev(`JSON.stringify(__bootErrors)`), started: await ev(`!!document.querySelector("#hello")`) };
}
try {
  // The fast path: streaming only, silently.
  let r = await load("ok");
  check(r.started && r.calls === `{"streaming":1,"bytes":0}` && log.length === 0, `application/wasm: ${JSON.stringify(r)} ${log.join(" | ")}`);
  await ev(`document.querySelector("#inc").click()`);
  await sleep(200);
  check(await ev(`document.querySelector("#hello").textContent`) === "clicked 1", "app not interactive after a streaming boot");
  // Wrong or missing MIME type: compiled from the bytes, with one warning.
  for (const c of ["octet", "none"]) {
    r = await load(c);
    const warned = log.length === 1 && /^warning: goowee: .*main\.wasm is served as .*not application\/wasm.*Content-Type: application\/wasm/.test(log[0]);
    check(r.started && r.calls === `{"streaming":0,"bytes":1}` && warned, `${c}: ${JSON.stringify(r)} ${log.join(" | ")}`);
  }
  // Real failures: one actionable error, the event, no unhandled rejection.
  for (const [c, want] of [["html", /isn't a WebAssembly module.*HTML page.*index\.html fallback/], ["missing", /downloading .*\/missing\/main\.wasm failed: HTTP 404/]]) {
    r = await load(c);
    const errs = log.filter((l) => l.startsWith("error:")), uncaught = log.filter((l) => l.startsWith("exception:"));
    check(!r.started && errs.length === 1 && want.test(errs[0]) && JSON.parse(r.errors).length === 1 && uncaught.length === 0,
      `${c}: ${JSON.stringify(r)} log: ${log.join(" | ")}`);
  }
} catch (e) { fail.push(e.message); }
console.log(fail.length ? "WASM MIME CHECK FAIL:\n- " + fail.join("\n- ") : "WASM MIME CHECK PASS: streaming with application/wasm; octet-stream and no type fall back with a warning; HTML and 404 give one actionable error");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
// Delete the Chrome profile (tens of MB) once Chrome has exited.
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
