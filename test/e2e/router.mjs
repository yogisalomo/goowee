// Drives test/e2e/router in headless Chrome (#142, #155):
//   - back and forward set the router's path and query together: a Watch on
//     both never sees the new path with the old query;
//   - the fragment follows Navigate and history;
//   - SetQueryParam writes sorted parameters, keeps the fragment, and leaves
//     no "?" once the query is empty;
//   - a push scrolls to the top, back restores the saved position, a fragment
//     scrolls to its target, focus lands on the new heading, and NavLink
//     follows the active route.
// Usage: node test/e2e/router.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8155), DP = Number(process.env.CDP_PORT || 9455);
const dir = mkdtempSync(join(tmpdir(), "goowee-router-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/router"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
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
  if (!types[path]) { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-router-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("ROUTER CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
const click = (sel) => ev(`document.querySelector(${JSON.stringify(sel)}).click()`);
const loc = () => ev(`location.pathname + location.search + location.hash`);
const waitLoc = async (want, label) => { let t; for (let i = 0; i < 100; i++) { t = await loc(); if (t === want) return; await sleep(50); } throw new Error(`timeout: ${label}: location ${t}, want ${want}`); };
const cls = async (sel) => (await ev(`document.querySelector(${JSON.stringify(sel)})?.className`)) ?? "";
const attr = (sel, name) => ev(`document.querySelector(${JSON.stringify(sel)})?.getAttribute(${JSON.stringify(name)})`);
const scrollY = () => ev(`window.scrollY`);
const waitScroll = async (min, max, label) => { let y; for (let i = 0; i < 100; i++) { y = await scrollY(); if (y >= min && y <= max) return; await sleep(50); } throw new Error(`timeout: ${label}: scrollY ${y}, want ${min}..${max}`); };
const focusText = () => ev(`document.activeElement?.textContent`);
const routeStatus = () => ev(`document.querySelector("[data-goowee-route-status]")?.textContent`);
try {
  const base = `http://localhost:${PORT}`;
  await send("Page.navigate", { url: base + "/a?x=1" });
  await waitText("#log", "/a|1", "first render");
  await waitText("#heading", "Page A", "heading on /a");
  check((await cls("#nav-a")).split(" ").includes("active"), "A link active on /a");
  check((await attr("#nav-a", "aria-current")) === "page", "A link aria-current on /a");
  check(!(await cls("#nav-docs")).split(" ").includes("active"), "Docs link inactive on /a");

  await ev(`window.scrollTo(0, 600)`);
  await waitScroll(550, 650, "scrolled down on /a");
  await click("#go-b"); await waitLoc("/b?x=2", "Navigate /b");
  await waitScroll(0, 5, "push scrolls to top");
  await waitText("#heading", "Page B", "heading on /b");
  check((await focusText()) === "Page B", "focus moved to the new page's heading");
  check(((await routeStatus()) ?? "").includes("/b"), "aria-live region announced the route");
  check(!(await cls("#nav-a")).split(" ").includes("active"), "A link inactive on /b");

  await click("#go-c"); await waitLoc("/c?x=3#end", "Navigate /c");
  await waitText("#fragment", "end", "Fragment after Navigate");
  await waitScroll(1500, 2500, "fragment scrolled to #end");
  check((await focusText()) === "Page C", "focus moved to the heading without undoing fragment scroll");
  await ev(`history.back()`); await waitLoc("/b?x=2", "back to /b");
  await waitText("#fragment", "", "fragment cleared by back");
  await waitScroll(0, 5, "back to /b top");
  await ev(`history.back()`); await waitLoc("/a?x=1", "back to /a");
  await waitScroll(550, 650, "back restored the /a scroll position");
  check((await cls("#nav-a")).split(" ").includes("active"), "A link active again after back");
  await ev(`history.forward()`); await waitLoc("/b?x=2", "forward to /b");
  await waitScroll(0, 5, "forward to /b top");
  await waitText("#log", /\/a\|1,\/b\|2$/, "the watcher saw forward to /b");
  const pairs = (await text("#log")).split(",");
  const want = { "/a": "1", "/b": "2", "/c": "3" };
  const mixed = pairs.filter((p) => { const [path, x] = p.split("|"); return want[path] !== x; });
  check(mixed.length === 0, `the watcher saw mixed URLs: ${mixed.join(", ")} (all: ${pairs.join(", ")})`);
  check(pairs.at(-1) === "/b|2", "last pair " + pairs.at(-1));

  await ev(`history.forward()`); await waitLoc("/c?x=3#end", "forward to /c");
  await waitText("#fragment", "end", "fragment restored by forward");
  await waitScroll(1500, 2500, "fragment scroll restored by forward");
  await ev(`window.scrollTo(0, 300)`); await waitScroll(250, 350, "scroll before a query update");
  await click("#set-y"); await waitLoc("/c?x=3&y=1#end", "SetQueryParam keeps the fragment");
  await click("#set-a"); await waitLoc("/c?a=0&x=3&y=1#end", "SetQueryParam sorts parameters");
  await click("#clear"); await waitLoc("/c#end", "no trailing ? once the query is empty");
  await waitScroll(250, 350, "query updates do not scroll");

  await click("#go-docs"); await waitLoc("/docs#install", "Navigate /docs#install");
  await waitText("#heading", "Docs", "heading on /docs");
  await waitScroll(1300, 2200, "fragment scrolled to #install");
  check((await focusText()) === "Docs", "focus on the docs heading");
  check((await cls("#nav-docs")).split(" ").includes("active"), "Docs link active on /docs");
  check((await attr("#nav-docs", "aria-current")) === "page", "Docs link aria-current on /docs");
  check(!(await cls("#nav-a")).split(" ").includes("active"), "A link inactive on /docs");
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "ROUTER CHECK FAIL:\n- " + fail.join("\n- ") : "ROUTER CHECK PASS: back/forward never mix path and query, scroll and fragments restored, focus/announcement and active links follow the route, SetQueryParam URLs sorted, fragment kept, no trailing ?");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
