// Drives test/e2e/islands in headless Chrome (#154): two goowee islands in a
// hand-written server page hydrate independently (each keeps its server
// markup and works on its own), the server text around them is untouched,
// and one unmounts cleanly while the other keeps working.
// Usage: node test/e2e/islands.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8157), DP = Number(process.env.CDP_PORT || 9457);
const dir = mkdtempSync(join(tmpdir(), "goowee-islands-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/islands"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}
const index = execFileSync("go", ["run", "./test/e2e/islands/render"]).toString();
const unusedIndex = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-islands-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("ISLANDS CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
// GOOWEE_WORKER=1 runs the app in a Web Worker (#158); run.sh runs both ways.
const WORKER = process.env.GOOWEE_WORKER === "1";
if (WORKER) await send("Page.addScriptToEvaluateOnNewDocument", { source: `try { localStorage.setItem("goowee-worker", "1") } catch (_) {}` });
try {
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  await waitText("#a button.inc", "3", "island a, server rendered");
  await ev(`window.__serverButtons = [...document.querySelectorAll("button.inc")]`);
  await ev(`window.bootNow()`);
  await ev(`new Promise((r) => { const t = setInterval(() => { if (goowee._interactive) { clearInterval(t); r(); } }, 20); })`);
  check(await ev(`[...document.querySelectorAll("button.inc")].every((b, i) => b === window.__serverButtons[i])`), "an island was rebuilt instead of hydrated (its server <button> was replaced)");
  check(await ev(`document.querySelector("#between").textContent === "Server text between the islands." && document.querySelector("h1").textContent === "A server-rendered shop"`), "the server page around the islands changed");
  await ev(`document.querySelector("#a button.inc").click()`);
  await waitText("#a button.inc", "4", "island a counts");
  check(await text("#b button.inc") === "10", "clicking island a changed island b");
  await ev(`document.querySelector("#b button.inc").click()`);
  await waitText("#b button.inc", "11", "island b counts");
  check(await text("#a button.inc") === "4", "clicking island b changed island a");

  // Unmount island b: its nodes go, island a's stay tracked and working.
  // (Not in worker mode: the test reaches Go through window.unmountIsland,
  // which the app sets with syscall/js; in a worker that lands on the
  // worker's global, not the page's — #158, limit 7.)
  if (!WORKER) {
  const tracked = `(() => { let n = 0; const w = (el) => { if (el._nodeID !== undefined) n++; for (let c = el.firstChild; c; c = c.nextSibling) w(c); }; w(document.documentElement); return n; })()`;
  const before = await ev(`goowee.nodeCount()`);
  await ev(`window.unmountIsland(1)`);
  await ev(`new Promise((r) => setTimeout(r, 100))`);
  const after = await ev(`goowee.nodeCount()`), live = await ev(tracked);
  check(after < before && after === live, `after unmounting island b the runtime tracks ${after} nodes (was ${before}), ${live} live`);
  check(await ev(`document.querySelector("#b [data-goowee-island]").children.length === 0`), "island b's content is still in the page");
  await ev(`document.querySelector("#a button.inc").click()`);
  await waitText("#a button.inc", "5", "island a after b unmounted");
  }
  check(await ev(`!!goowee.worker`) === WORKER, `worker mode is ${await ev(`!!goowee.worker`)}, want ${WORKER}`);
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "ISLANDS CHECK FAIL:\n- " + fail.join("\n- ") : "ISLANDS CHECK PASS" + (WORKER ? " (worker)" : "") + ": two islands in a server page hydrate and work independently; one unmounts cleanly");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
