// Checks the GitHub Pages build (make pages → _site) the way it is deployed:
// served under /goowee/ with GitHub Pages' behavior (unknown paths → 404.html,
// status 404), client-only. Drives it in headless Chrome: every in-app link
// carries the base path (a bare "/tutorial" href is a GitHub 404 when opened
// in a new tab), navigation, deep links, bubbling, and a few live features.
// Usage: node test/e2e/pages.mjs _site   (make pages-e2e)
//        node test/e2e/pages.mjs https://yogisalomo.github.io/goowee   (the live site)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync } from "node:fs";
import { join, extname } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";

const SITE = process.argv[2] || "_site";
const PORT = Number(process.env.PORT || 8199), DP = Number(process.env.CDP_PORT || 9399);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}
const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".wasm": "application/wasm" };
const LIVE = SITE.startsWith("http");
const server = LIVE ? null : createServer((req, res) => {
  const path = decodeURIComponent(new URL(req.url, "http://x").pathname);
  if (!path.startsWith("/goowee")) { res.writeHead(404); return res.end("GitHub 404"); }
  let rel = path.slice("/goowee".length) || "/";
  if (rel.endsWith("/")) rel += "index.html";
  const file = join(SITE, rel);
  if (existsSync(file)) { res.writeHead(200, { "content-type": types[extname(file)] || "application/octet-stream" }); return res.end(readFileSync(file)); }
  res.writeHead(404, { "content-type": "text/html" }); res.end(readFileSync(join(SITE, "404.html")));
}).listen(PORT);

const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  // CI runners and containers need --no-sandbox; harmless locally.
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${mkdtempSync(join(tmpdir(), "goowee-pages-"))}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let target; // up to 20s: Chrome can take ~8s to open its debug port
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") errors.push("exception: " + m.params.exceptionDetails.text); };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;
const waitFor = async (expr, label) => { for (let i = 0; i < 150; i++) { if (await ev(expr)) return; await sleep(100); } throw new Error("timeout: " + label); };
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");
const base = LIVE ? SITE.replace(/\/$/, "") : `http://localhost:${PORT}/goowee`;
const hrefsOK = `[...document.querySelectorAll('a')].filter(a=>!a.getAttribute('href').startsWith('http')).every(a=>a.getAttribute('href').startsWith('/goowee/'))`;
const click = (text) => ev(`(()=>{const a=[...document.querySelectorAll('a')].find(a=>a.textContent.trim()===${JSON.stringify(text)}); a.click(); return !!a})()`);
try {
  await send("Page.navigate", { url: base + "/" });
  await waitFor(`/Reactive web UIs, written in Go\\./.test(document.body.innerText)`, "landing");
  check(await ev(hrefsOK), "landing: an in-app link lacks the /goowee base");
  await click("Tutorial");
  await waitFor(`location.pathname === '/goowee/tutorial'`, "tutorial via header link");
  await waitFor(`/Learn goowee by example/.test(document.body.innerText)`, "tutorial index rendered");
  check(await ev(hrefsOK), "tutorial index: an in-app link lacks the base");
  await ev(`[...document.querySelectorAll('a')].find(a=>a.textContent.includes('Events')).click()`);
  await waitFor(`location.pathname === '/goowee/events' && !!document.querySelector('.event-card')`, "events lesson");
  await ev(`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='Inner button').click()`);
  await waitFor(`/Card clicks: 1 · inner: 1/.test(document.body.innerText)`, "bubbling on the live build");
  check(await ev(hrefsOK), "events lesson: an in-app link lacks the base");
  const prevented = await ev(`(()=>{let seen=null;window.addEventListener('click',(e)=>{seen=e.defaultPrevented;e.preventDefault();},{once:true});const a=[...document.querySelectorAll('a')].find(a=>a.textContent.trim().startsWith('10 '));a.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true,metaKey:true}));return seen;})()`);
  check(prevented === false, "step-nav link hijacked a cmd-click");
  // Deep links (refresh / new tab) land on the app via 404.html.
  const newTab = await ev(`fetch('${base}/ai').then(r=>r.text()).then(t=>t.includes('goowee.boot'))`);
  check(newTab, "deep link /goowee/ai doesn't serve the app shell");
  await send("Page.navigate", { url: base + "/ai" });
  await waitFor(`!!document.querySelector('textarea.copybox')`, "ai page (deep link)");
  check(await ev(`document.querySelector('textarea.copybox').value.includes('func(ctx context.Context) (T, error)')`), "AI rules not updated for v0.2.0");
  await send("Page.navigate", { url: base + "/getting-started" });
  await waitFor(`/goowee@latest/.test(document.body.innerText)`, "getting started updated");
  await send("Page.navigate", { url: base + "/dashboard" });
  await waitFor(`document.querySelector('canvas.bars')?.dataset.sum === '215'`, "canvas chart via bridge.Element");
  await send("Page.navigate", { url: base + "/async" });
  await waitFor(`/Loaded at /.test(document.body.innerText)`, "async resource with ctx");
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "PAGES CHECK FAIL:\n- " + fail.join("\n- ") : "PAGES CHECK PASS: base-path links, navigation, deep links, v0.2.0 content, canvas, async");
ws.close(); chrome.kill(); server?.close(); process.exit(fail.length ? 1 : 0);
