// Checks the GitHub Pages build (make pages → _site) the way it is deployed:
// served under /goowee/ with GitHub Pages' behavior (unknown paths → 404.html,
// status 404), client-only. Drives it in headless Chrome: every in-app link
// carries the base path (a bare "/tutorial" href is a GitHub 404 when opened
// in a new tab), navigation, deep links, bubbling, and a few live features.
// Usage: node test/e2e/pages.mjs _site   (make pages-e2e)
//        node test/e2e/pages.mjs https://yogisalomo.github.io/goowee   (the live site)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, rmSync, statSync, readdirSync } from "node:fs";
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
  // Like GitHub Pages: a directory URL without its slash redirects to it,
  // and a directory serves its index.html.
  if (!rel.endsWith("/") && existsSync(join(SITE, rel)) && statSync(join(SITE, rel)).isDirectory()) {
    res.writeHead(301, { location: path + "/" }); return res.end();
  }
  if (rel.endsWith("/")) rel += "index.html";
  const file = join(SITE, rel);
  if (existsSync(file)) { res.writeHead(200, { "content-type": types[extname(file)] || "application/octet-stream" }); return res.end(readFileSync(file)); }
  res.writeHead(404, { "content-type": "text/html" }); res.end(readFileSync(join(SITE, "404.html")));
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-pages-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  // CI runners and containers need --no-sandbox; harmless locally.
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let target; // up to 20s: Chrome can take ~8s to open its debug port
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = []; const mismatches = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.consoleAPICalled") { const line = m.params.args.map((a) => a.value ?? a.description).join(" "); if (/hydration mismatch|no server node/.test(line)) mismatches.push(line); }
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
  // #153: pages are prerendered (ssr.Export): the first paint has the page's
  // content before main.wasm arrives. Hold the wasm back and look.
  await send("Network.enable");
  await send("Network.setBlockedURLs", { urls: ["*/main.*wasm"] });
  for (const [path, text] of [["/", "Reactive web UIs, written in Go."], ["/canvas", "Canvas"], ["/greet/alice", "alice"]]) {
    await send("Page.navigate", { url: base + path });
    await waitFor(`document.readyState === "complete"`, "prerendered " + path);
    check(await ev(`!document.querySelector('#root .splash') && document.querySelector('#root').innerText.includes(${JSON.stringify(text)}) && !goowee._interactive`),
      `prerendered ${path}: no content before the wasm loaded (${JSON.stringify(await ev(`document.querySelector('#root').innerText.slice(0, 80)`))})`);
  }
  await send("Network.setBlockedURLs", { urls: [] });
  // Those loads logged the blocked download; that's expected, not a failure.
  errors.splice(0, errors.length, ...errors.filter((e) => !/couldn't download \S*main\.(?:[0-9a-f]+\.)?wasm/.test(e)));
  const hydrated = `typeof goowee !== 'undefined' && goowee._interactive === true`;
  // #145: goowee build ships the wasm and runtime under content-hashed names,
  // and the page boots the hashed wasm (its download shows in the timings).
  await send("Page.navigate", { url: base + "/" });
  await waitFor(hydrated, "hydrated for the asset check");
  const assets = await ev(`({preload: document.querySelector('link[rel=preload]').getAttribute('href'),
    scripts: [...document.scripts].map((s) => s.getAttribute('src')).filter(Boolean),
    fetched: performance.getEntriesByType('resource').map((r) => r.name).filter((n) => /\.wasm$/.test(n))})`);
  check(/^main\.[0-9a-f]{10}\.wasm$/.test(assets.preload) && assets.fetched.some((n) => n.endsWith("/" + assets.preload)),
    "the page should preload and boot a content-hashed wasm: " + JSON.stringify(assets));
  check(assets.scripts.every((src) => /\.[0-9a-f]{10}\.js$/.test(src)), "runtime scripts should be content-hashed: " + JSON.stringify(assets.scripts));
  // Every prerendered page hydrates in place: no mismatch is logged, and the
  // server's nodes are the ones on screen.
  if (!LIVE) {
    const pages = readdirSync(SITE, { recursive: true }).filter((f) => f.endsWith("index.html"))
      .map((f) => "/" + f.replace(/\\/g, "/").replace(/index\.html$/, ""));
    for (const p of pages) {
      const html = readFileSync(join(SITE, p, "index.html"), "utf8");
      if (!html.includes("goowee-prerendered")) continue; // client-only shell
      await send("Page.navigate", { url: base + p });
      const before = mismatches.length;
      await waitFor(`${hydrated}`, "hydrate " + p);
      await sleep(200);
      check(mismatches.length === before, `${p}: hydration mismatch: ${mismatches.slice(before).join("; ")}`);
      check(await ev(`goowee.serverRendered() && document.getElementById('root')._goServer === true`), `${p} wasn't hydrated`);
    }
  }
  await send("Page.navigate", { url: base + "/" });
  await waitFor(`${hydrated} && /Reactive web UIs, written in Go\\./.test(document.body.innerText)`, "landing");
  check(await ev(hrefsOK), "landing: an in-app link lacks the /goowee base");
  // #121: the app's Metadata title replaces the shell's <title> (document.title
  // reads the first one, so the shell's used to win).
  check(await ev(`(()=>{const ts=[...document.head.querySelectorAll(':scope > title')];return ts.length===1 && ts[0]._nodeID!==undefined && document.title==='goowee — Reactive web UIs in Go';})()`), `document title: ${await ev("JSON.stringify([document.title, document.head.querySelectorAll('title').length])")}`);
  check(await ev(`goowee.serverRendered() && document.querySelector('meta[name="goowee-prerendered"]')?.content === '/goowee/'`), "landing wasn't hydrated as a prerendered page");
  // #104: a client-only page (/ai) still gets the shell, whose full-height
  // "Loading…" splash the app replaces instead of rendering below it.
  check(await ev(`fetch('${base}/ai').then(r=>r.text()).then(t=>t.includes('<div class="splash">'))`), "the client-only shell has no splash to replace");
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
  await waitFor(`!!document.querySelector('textarea.copybox') && document.querySelector('textarea.copybox').value.length > 0`, "ai page (deep link)");
  check(await ev(`document.querySelector('textarea.copybox').value.includes('func(ctx context.Context) (T, error)')`), "AI rules not updated for v0.2.0");
  check(await ev(`!document.querySelector('#root .splash') && document.querySelector('#root').firstElementChild.getBoundingClientRect().top < window.innerHeight / 2`),
    "the loading splash stayed in #root after the app mounted");
  // A directory URL without its slash redirects (/counter → /counter/), as
  // on GitHub Pages; the router treats both as /counter, so the page hydrates
  // and works.
  await send("Page.navigate", { url: base + "/counter" });
  const countBtn = `[...document.querySelectorAll('button')].find(b=>b.textContent.startsWith('Count: '))`;
  await waitFor(`${hydrated} && location.pathname === '/goowee/counter/' && !!${countBtn}`, "counter via its directory URL");
  await ev(`${countBtn}.click()`);
  await waitFor(`${countBtn}.textContent === 'Count: 1'`, "counter increments after hydration");
  // A param route nobody linked to has no file: the host serves 404.html,
  // prerendered for another path. The runtime must not hydrate it as this
  // page: it renders the greeting on the client instead.
  await send("Page.navigate", { url: base + "/greet/zed" });
  await waitFor(`${hydrated} && /zed/.test(document.querySelector('#root').innerText)`, "unprerendered param route renders on the client");
  check(await ev(`!goowee.serverRendered() && !/not found/i.test(document.querySelector('#root').innerText)`), "/greet/zed kept the 404 page's markup");
  await send("Page.navigate", { url: base + "/getting-started" });
  await waitFor(`/goowee@latest/.test(document.body.innerText)`, "getting started updated");
  await send("Page.navigate", { url: base + "/dashboard" });
  await waitFor(`document.querySelector('canvas.bars')?.dataset.sum === '215'`, "canvas chart via bridge.Element");
  await send("Page.navigate", { url: base + "/async" });
  await waitFor(`/Loaded at /.test(document.body.innerText)`, "async resource with ctx");
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
check(mismatches.length === 0, "hydration mismatches:\n" + mismatches.join("\n"));
console.log(fail.length ? "PAGES CHECK FAIL:\n- " + fail.join("\n- ") : "PAGES CHECK PASS: prerendered first paint, no hydration mismatch, 404.html not hydrated as another page, directory URLs, base-path links, splash replaced on mount, app title, navigation, deep links, v0.2.0 content, canvas, async, hashed assets");
ws.close(); server?.close();
// Delete the Chrome profile (tens of MB) once Chrome has exited.
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
