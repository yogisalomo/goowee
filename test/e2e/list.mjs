// h.ForList in headless Chrome (#140): each core.List operation changes the
// rows in an <ul> and in an <svg> (circles in the SVG namespace), in order.
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8159), DP = Number(process.env.CDP_PORT || 9459);
const dir = mkdtempSync(join(tmpdir(), "goowee-list-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/list"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
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
const profile = mkdtempSync(join(tmpdir(), "goowee-list-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("LIST CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
const state = `(() => {
  const rows = [...document.querySelectorAll("#rows li")].map((l) => l.textContent).join(" ");
  const dots = [...document.querySelectorAll("#dots > *")].map((c) => c.getAttribute("data-t") + (c.namespaceURI === "http://www.w3.org/2000/svg" ? "" : "(html!)")).join(" ");
  return rows + " | " + dots;
})()`;
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
const expect = async (want, label) => { let got; for (let i = 0; i < 100; i++) { got = await ev(state); if (got === want + " | " + want) return; await sleep(30); } fail.push(`${label}: ${got}, want ${want} (both lists)`); };
await send("Runtime.enable"); await send("Page.enable");
try {
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  await expect("1a 2b 3c", "first render");
  const click = (sel) => ev(`document.querySelector("${sel}").click()`);
  await click("#append"); await expect("1a 2b 3c 4n", "append");
  await click("#prepend"); await expect("5p 1a 2b 3c 4n", "insert at 0");
  await click("#update"); await expect("5p 1a 2b! 3c 4n", "update");
  await click("#remove"); await expect("5p 2b! 3c 4n", "remove");
  await click("#move"); await expect("3c 5p 2b! 4n", "move");
  await click("#set"); await expect("2B 9z", "set");
  await click("#append"); await expect("2B 9z 6n", "append after set");
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "LIST CHECK FAIL:\n- " + fail.join("\n- ") : "LIST CHECK PASS: ForList append, insert, update, remove, move and set, in HTML and SVG");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
