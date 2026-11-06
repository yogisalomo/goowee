// Drives test/e2e/storage in headless Chrome (#100): goowee/storage reads and
// writes localStorage and sessionStorage, survives a reload, and degrades
// cleanly where reading localStorage throws (a sandboxed frame, storage
// disabled): nothing found, Set returns ErrUnavailable, no console errors.
// Usage: node test/e2e/storage.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8147), DP = Number(process.env.CDP_PORT || 9447);
const dir = mkdtempSync(join(tmpdir(), "goowee-storage-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/storage"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}
const index = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/" || path === "/disabled") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-storage-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("STORAGE CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
const waitText = async (sel, want, label) => { let t; for (let i = 0; i < 100; i++) { t = await text(sel); if (typeof want === "string" ? t === want : want.test(t ?? "")) return; await sleep(50); } throw new Error(`timeout: ${label || sel} — got ${JSON.stringify(t)}, want ${want}`); };
const type = (value) => ev(`(() => { const el = document.querySelector("#draft"); el.value = ${JSON.stringify(value)}; el.dispatchEvent(new InputEvent("input", {bubbles: true})); })()`);
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");
try {
  const base = `http://localhost:${PORT}`;
  await send("Page.navigate", { url: base + "/" });
  await waitText("#restored", `draft="" found=false visits="" keys=`, "first visit: nothing stored");
  await type("half-written");
  await waitText("#save", "<nil>", "saved");
  check(await ev(`localStorage.getItem("draft")`) === "half-written", "not in localStorage");
  // Reload: localStorage and sessionStorage both survive within the tab.
  await send("Page.reload");
  await waitText("#restored", `draft="half-written" found=true visits="x" keys=draft`, "restored after a reload");
  check(await ev(`document.querySelector("#draft").value`) === "half-written", "draft not put back in the textarea");
  await ev(`document.querySelector("#clear").click()`);
  await waitText("#restored", "removed");
  check(await ev(`localStorage.getItem("draft")`) === null, "Remove left the key");
  // Storage disabled: reading localStorage throws.
  await send("Page.addScriptToEvaluateOnNewDocument", { source: `if (location.pathname === "/disabled") Object.defineProperty(window, "localStorage", { get() { throw new DOMException("denied", "SecurityError"); } });` });
  await send("Page.navigate", { url: base + "/disabled" });
  await waitText("#restored", /^draft="" found=false visits="(x+)?" keys=$/, "disabled: nothing found");
  await type("lost");
  await waitText("#save", "goowee/storage: storage is unavailable in this browser context", "disabled: Set reports ErrUnavailable");
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "STORAGE CHECK FAIL:\n- " + fail.join("\n- ") : "STORAGE CHECK PASS: localStorage + sessionStorage, reload, remove, storage disabled");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
// Delete the Chrome profile (tens of MB) once Chrome has exited.
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
