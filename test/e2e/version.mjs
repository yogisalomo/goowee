// Drives test/e2e/hello in headless Chrome (#144): a page that serves a stale
// goowee.js must get one clear mismatch error naming both protocols and the
// command that copies the matching assets, and must not mount the app.
// Usage: node test/e2e/version.mjs
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8158), DP = Number(process.env.CDP_PORT || 9458);
const dir = mkdtempSync(join(tmpdir(), "goowee-version-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/hello"], {
  env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit",
});
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
const stale = readFileSync("runtime/goowee.js", "utf8").replace(
  "goowee.protocolVersion = 2;", "goowee.protocolVersion = 0; // stale, #144 e2e"
);
if (!stale.includes("// stale, #144 e2e")) throw new Error("protocolVersion marker not found in runtime/goowee.js");

const index = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"><p id="splash">Loading…</p></div>
<script>
window.bootErrs = [];
window.addEventListener("goowee:boot-error", function (e) { window.bootErrs.push(e.detail.message); });
goowee.boot();
</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (path === "/goowee.js") { res.writeHead(200, { "content-type": types[path] }); return res.end(stale); }
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}
const profile = mkdtempSync(join(tmpdir(), "goowee-version-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("VERSION CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target;
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
// The version the binary reports is internal/version.Module, bumped at each release.
const moduleVersion = readFileSync("internal/version/version.go", "utf8").match(/const Module = "([^"]+)"/)[1];
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");
// GOOWEE_WORKER=1 runs the app in a Web Worker (#158); run.sh runs both ways.
const WORKER = process.env.GOOWEE_WORKER === "1";
if (WORKER) await send("Page.addScriptToEvaluateOnNewDocument", { source: `try { localStorage.setItem("goowee-worker", "1") } catch (_) {}` });
try {
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  let message;
  for (let i = 0; i < 200 && !message; i++) { message = await ev(`window.goowee && goowee.bootError && goowee.bootError.message`); if (!message) await sleep(50); }
  check(!!message, "no goowee.bootError was set on a protocol mismatch");
  if (message) {
    check(message.includes("protocol 2"), `error doesn't name the app protocol: ${message}`);
    check(message.includes("protocol 0"), `error doesn't name the page protocol: ${message}`);
    check(message.includes(`goowee ${moduleVersion}`), `error doesn't name the app version (${moduleVersion}): ${message}`);
    check(message.includes("go run github.com/yogisalomo/goowee/cmd/goowee assets ./web"), `error lacks the fix command: ${message}`);
  }
  check((await ev(`window.bootErrs.length`)) === 1, "expected exactly one goowee:boot-error event");
  check((await ev(`!!document.querySelector("#hello")`)) === false, "the app mounted against the stale runtime");
  check((await ev(`document.querySelector("#splash") && document.querySelector("#splash").textContent`)) === "Loading…", "the splash was replaced");
  check(errors.some((e) => e.includes("goowee:") && e.includes("protocol")), "no console error explaining the mismatch");
} catch (e) { fail.push(e.message); }
console.log(fail.length ? "VERSION CHECK FAIL:\n- " + fail.join("\n- ") : "VERSION CHECK PASS: a stale goowee.js reports both protocols and the fix command, and the app does not mount");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
