// Mounting a fresh subtree three ways (#139), in headless Chrome: the same
// list of rows (5 nodes, 3 attributes each) as today's per-node binary batch,
// as one HTML string claimed the way hydration claims server markup, and as
// a cloned row template with only its dynamic parts set. Each mount runs in a
// fresh page; apply is the DOM work, layout the style and layout it causes.
// Env: CHROME, CDP_PORT (default 9357), PORT (default 8149), ROUNDS (5).
import { execFileSync, spawn } from "node:child_process";
import { copyFileSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, extname, join } from "node:path";
import { fileURLToPath } from "node:url";

const DP = Number(process.env.CDP_PORT || 9357);
const PORT = Number(process.env.PORT || 8149);
const ROUNDS = Number(process.env.ROUNDS || 5);
const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
const dir = mkdtempSync(join(tmpdir(), "goowee-mount-"));

console.log("writing the mount frames…");
execFileSync("go", ["test", "-count=1", "-run", "^TestWriteMountFrames$", "./ssr/"], {
  cwd: root, stdio: ["ignore", "ignore", "inherit"],
  env: { ...process.env, GOOWEE_MOUNT_FRAMES: join(dir, "frames.json") },
});
const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js")
  : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync(join(root, "runtime/goowee.js"), join(dir, "goowee.js"));
writeFileSync(join(dir, "mount.html"), `<!doctype html><meta charset="utf-8"><title>mount</title>
<script src="wasm_exec.js"></script><script src="goowee.js"></script><div id="root"></div>`);

const types = { ".html": "text/html", ".js": "text/javascript", ".wasm": "application/wasm", ".json": "application/json" };
const server = createServer((req, res) => {
  const file = join(dir, new URL(req.url, "http://x").pathname.replace(/^\/+/, "") || "index.html");
  if (!file.startsWith(dir) || !existsSync(file)) { res.writeHead(404).end(); return; }
  res.writeHead(200, {
    "Content-Type": types[extname(file)] || "application/octet-stream",
    "Cross-Origin-Opener-Policy": "same-origin", "Cross-Origin-Embedder-Policy": "require-corp",
  });
  res.end(readFileSync(file));
}).listen(PORT);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const profile = mkdtempSync(join(tmpdir(), "goowee-mount-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check", "--js-flags=--no-liftoff",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });

let ws, nextId = 1;
const pending = new Map();
const send = (method, params = {}) => {
  const id = nextId++;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((res, rej) => pending.set(id, { res, rej }));
};
const evalJS = async (expr) => {
  const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || "eval error");
  return r.result.value;
};
const load = async (path, ready) => {
  await send("Page.navigate", { url: `http://localhost:${PORT}/${path}` });
  for (let i = 0; i < 300; i++) {
    if (await evalJS(ready).catch(() => false)) return;
    await sleep(100);
  }
  throw new Error(`timeout loading ${path}`);
};


// One mount, measured in a fresh page: way is "nodes" (today's per-node
// batch, through the runtime's applyBatch), "html" (one HTML string parsed
// through a <template>, then its nodes claimed by data-node-id and text
// markers, as hydration does) or "clone" (a row template cloned per row,
// with its dynamic parts set and its nodes recorded).
const mount = (way, index) => `(async () => {
  const F = (await (await fetch("frames.json")).json())[${index}];
  const root = document.getElementById("root");
  const nodes = {};
  let apply;
  if (${JSON.stringify(way)} === "nodes") {
    const u8 = Uint8Array.from(atob(F.binary), (c) => c.charCodeAt(0));
    apply = () => goowee.applyBatch(u8, u8.length);
  } else if (${JSON.stringify(way)} === "html") {
    apply = () => {
      const tpl = document.createElement("template");
      tpl.innerHTML = F.html;
      for (const el of tpl.content.querySelectorAll("[data-node-id]")) nodes[+el.getAttribute("data-node-id")] = el;
      const w = document.createTreeWalker(tpl.content, NodeFilter.SHOW_COMMENT), marks = [];
      while (w.nextNode()) marks.push(w.currentNode);
      for (const c of marks) { const m = /^g(\\d+)$/.exec(c.data); if (m) { nodes[+m[1]] = c.nextSibling; c.remove(); } }
      root.appendChild(tpl.content);
    };
  } else {
    // The dynamic values a clone needs: what the batch would carry per row.
    const labels = [], ids = [];
    for (let i = 0; i < F.rows; i++) { labels.push("Row number " + i); ids.push(String(i)); }
    apply = () => {
      const t = document.createElement("template");
      t.innerHTML = '<li class="row"><span class="label"> </span><a href="#">remove</a></li>';
      const row = t.content.firstChild;
      const ul = document.createElement("ul");
      let id = 1; nodes[id++] = ul;
      for (let i = 0; i < F.rows; i++) {
        const li = row.cloneNode(true), span = li.firstChild, a = li.lastChild;
        li.setAttribute("data-id", ids[i]);
        span.firstChild.data = labels[i];
        nodes[id++] = li; nodes[id++] = span; nodes[id++] = span.firstChild; nodes[id++] = a; nodes[id++] = a.firstChild;
        ul.appendChild(li);
      }
      root.appendChild(ul);
    };
  }
  await new Promise((r) => requestAnimationFrame(() => r()));
  const t0 = performance.now();
  apply();
  const t1 = performance.now();
  document.body.offsetHeight;
  const t2 = performance.now();
  const li = document.querySelectorAll("li").length;
  return { apply: t1 - t0, layout: t2 - t1, li };
})()`;

async function main() {
  let target;
  for (let i = 0; i < 200 && !target; i++) {
    try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); }
    catch {}
    if (!target) await sleep(100);
  }
  if (!target) throw new Error("could not reach Chrome DevTools endpoint");
  ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id && pending.has(m.id)) {
      const p = pending.get(m.id); pending.delete(m.id);
      m.error ? p.rej(new Error(m.error.message)) : p.res(m.result);
    }
  };
  await send("Runtime.enable");
  await send("Page.enable");
  const frames = JSON.parse(readFileSync(join(dir, "frames.json"), "utf8"));
  const med = (a) => { a = [...a].sort((p, q) => p - q); return a[a.length >> 1]; };
  const ms = (v) => (v >= 1 ? v.toFixed(1) + " ms" : (v * 1000).toFixed(0) + " µs").padStart(9);
  for (let index = 0; index < frames.length; index++) {
    const f = frames[index];
    const res = { nodes: [], html: [], clone: [] };
    for (let round = 0; round < ROUNDS; round++) {
      for (const way of round % 2 ? ["clone", "html", "nodes"] : ["nodes", "html", "clone"]) {
        await load("mount.html", "typeof goowee === 'object'");
        const r = await evalJS(mount(way, index));
        if (r.li !== f.rows) throw new Error(`${way}: ${r.li} rows in the page, want ${f.rows}`);
        res[way].push(r);
      }
    }
    console.log(`\n${f.rows} rows: ${f.muts} mutations today; ${(f.binBytes / 1024).toFixed(0)} KiB binary batch, ${(f.html.length / 1024).toFixed(0)} KiB HTML`);
    console.log("  way          apply   layout    total");
    for (const way of ["nodes", "html", "clone"]) {
      const a = med(res[way].map((r) => r.apply)), l = med(res[way].map((r) => r.layout)), t = med(res[way].map((r) => r.apply + r.layout));
      console.log(`  ${way.padEnd(7)} ${ms(a)} ${ms(l)} ${ms(t)}`);
    }
  }
  console.log(`\n(${ROUNDS} fresh page loads per way and size; medians)`);
}

main().catch((e) => { console.error("mount:", e.message); process.exitCode = 2; })
  .finally(() => {
    try { ws?.close(); } catch {}
    try { server.close(); } catch {}
    rmSync(dir, { recursive: true, force: true });
    chrome.once("exit", () => { try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {} });
    chrome.kill("SIGKILL");
  });
