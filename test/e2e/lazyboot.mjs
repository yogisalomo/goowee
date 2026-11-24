// goowee.boot({when}) in headless Chrome (#130), on test/e2e/early's
// server-rendered page: "interaction" downloads nothing until the user
// interacts, and the click that started the boot reaches Go once it is live;
// "idle" boots after load with no interaction; "visible" boots when its
// target scrolls into view. The WASM requests are counted by the server.
// Usage: node test/e2e/lazyboot.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8158), DP = Number(process.env.CDP_PORT || 9458);
const dir = mkdtempSync(join(tmpdir(), "goowee-lazy-"));
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
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
let wasmRequests = 0;
const server = createServer((req, res) => {
  const u = new URL(req.url, "http://x");
  if (u.pathname === "/") {
    // The page as the server renders it, booting itself in the asked-for mode.
    const when = u.searchParams.get("when");
    const below = `<div style="height:3000px"></div><p id="below">below the fold</p>`;
    const boot = `<script>goowee.boot({when: ${JSON.stringify(when)}, target: "#below"});</script>`;
    res.writeHead(200, { "content-type": "text/html" });
    return res.end(index.replace("</body>", below + boot + "</body>"));
  }
  if (u.pathname === "/main.wasm") wasmRequests++;
  if (types[u.pathname]) { res.writeHead(200, { "content-type": types[u.pathname] }); return res.end(readFileSync(join(dir, u.pathname))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-lazy-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("LAZYBOOT CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
const text = (sel) => ev(`document.querySelector(${JSON.stringify(sel)})?.textContent`);
const waitFor = async (expr, label) => { for (let i = 0; i < 200; i++) { if (await ev(expr)) return; await sleep(50); } throw new Error("timeout: " + label); };
const live = `!document.documentElement.hasAttribute("data-goowee-pending")`;
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
// A real mouse click: pointerdown, mousedown, pointerup, mouseup, click.
const clickAt = async (sel) => {
  const r = await ev(`(() => { const b = document.querySelector(${JSON.stringify(sel)}).getBoundingClientRect(); return [b.x + b.width / 2, b.y + b.height / 2]; })()`);
  for (const type of ["mousePressed", "mouseReleased"]) await send("Input.dispatchMouseEvent", { type, x: r[0], y: r[1], button: "left", clickCount: 1 });
};
const load = async (when) => {
  wasmRequests = 0;
  await send("Page.navigate", { url: `http://localhost:${PORT}/?when=${when}` });
  await waitFor(`document.querySelector("#state")?.textContent === "name=initial agreed=false clicks=0 submits=0"`, when + ": server-rendered page");
};
await send("Runtime.enable"); await send("Page.enable");
try {
  // interaction: nothing downloads until the user acts; the click that
  // starts the boot is handled once the app is live.
  await load("interaction");
  await sleep(1500);
  check(await ev(`goowee.bootWhen`) === "interaction", "goowee.bootWhen isn't interaction");
  check(wasmRequests === 0, `interaction: the WASM was requested ${wasmRequests} times before any interaction`);
  await clickAt("#go");
  await waitFor(live, "interaction: boot after the click");
  await waitFor(`document.querySelector("#state").textContent === "name=initial agreed=false clicks=1 submits=0"`, "interaction: the starting click reached Go once");
  check(wasmRequests === 1, `interaction: ${wasmRequests} WASM requests`);
  await clickAt("#go");
  await waitFor(`document.querySelector("#state").textContent.includes("clicks=2")`, "interaction: a click after boot");

  // idle: boots after load with nobody touching the page.
  await load("idle");
  await waitFor(live, "idle: boot without interaction");
  check(wasmRequests === 1, `idle: ${wasmRequests} WASM requests`);
  await clickAt("#go");
  await waitFor(`document.querySelector("#state").textContent.includes("clicks=1")`, "idle: a click after boot");

  // visible: boots when #below scrolls into view.
  await load("visible");
  await sleep(1500);
  check(wasmRequests === 0, `visible: the WASM was requested ${wasmRequests} times before #below was visible`);
  await ev(`document.querySelector("#below").scrollIntoView()`);
  await waitFor(live, "visible: boot after scrolling");
  check(wasmRequests === 1, `visible: ${wasmRequests} WASM requests`);
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "LAZYBOOT CHECK FAIL:\n- " + fail.join("\n- ") : "LAZYBOOT CHECK PASS: when interaction downloads nothing until a click and replays it; idle boots unattended; visible boots on scroll");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
