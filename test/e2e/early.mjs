// Drives test/e2e/early in headless Chrome (#131): on a server-rendered page,
// the user types, ticks a checkbox, clicks a button and submits a form
// before the WASM has booted. Once it boots, Go must hold what the page
// shows, and each click and submit must be handled once.
// Usage: node test/e2e/early.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8156), DP = Number(process.env.CDP_PORT || 9456);
const dir = mkdtempSync(join(tmpdir(), "goowee-early-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/early"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}
const index = execFileSync("go", ["run", "./test/e2e/early/render"]).toString();
const unusedIndex = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-early-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("EARLY CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  await waitText("#state", "name=initial agreed=false clicks=0 submits=0", "server-rendered page");
  // Before boot: type into the bound input, tick the box, click, submit.
  await ev(`window.samePage = true`);
  await ev(`(() => { const el = document.querySelector("#name"); el.focus(); el.select(); })()`);
  await send("Input.insertText", { text: "Ada" });
  await ev(`document.querySelector("#agree").click()`);
  await ev(`document.querySelector("#go").click()`);
  await ev(`document.querySelector("#send").click()`);
  await sleep(200);
  check(await ev(`window.samePage === true`), "the form submitted natively before boot: the page reloaded and lost what was typed");
  check(await ev(`document.documentElement.hasAttribute("data-goowee-pending")`), "no data-goowee-pending before boot");
  await ev(`window.bootNow()`);
  await waitText("#state", /clicks=/, "boot");
  await ev(`new Promise((r) => setTimeout(r, 300))`);
  const state = await text("#state");
  check(state === "name=Ada agreed=true clicks=1 submits=1", `after boot Go holds ${JSON.stringify(state)}, want "name=Ada agreed=true clicks=1 submits=1"`);
  check(await ev(`document.querySelector("#name").value`) === "Ada", "the typed text was overwritten");
  check(await ev(`!document.documentElement.hasAttribute("data-goowee-pending")`), "data-goowee-pending stayed after boot");
  // After boot, events go straight to Go.
  await ev(`document.querySelector("#go").click()`);
  await waitText("#state", "name=Ada agreed=true clicks=2 submits=1", "a click after boot");

  // If the WASM never loads, a held submit goes through natively, as it
  // would have without goowee.
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  await waitText("#state", "name=initial agreed=false clicks=0 submits=0", "page again");
  await ev(`window.samePage = true`);
  await ev(`document.querySelector("#send").click()`);
  await sleep(100);
  check(await ev(`window.samePage === true`), "the submit wasn't held before boot");
  await ev(`window.bootBroken()`);
  let navigated = false;
  for (let i = 0; i < 50 && !navigated; i++) { navigated = await ev(`window.samePage !== true`); await sleep(100); }
  check(navigated, "a held submit didn't go through after the WASM failed to load");
  errors.splice(0, errors.length, ...errors.filter((e) => !/missing\.wasm/.test(e)));
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "EARLY CHECK FAIL:\n- " + fail.join("\n- ") : "EARLY CHECK PASS: typing, a checkbox, a click and a submit before boot reach Go once after it boots; a held submit goes through natively when the WASM fails");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
