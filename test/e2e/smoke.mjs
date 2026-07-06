// End-to-end smoke test: drives the real WASM app in headless Chrome over the
// DevTools Protocol. No npm dependencies — Node (>=21) has global fetch and
// WebSocket. Expects an SSR server already running at $URL (see run.sh).
//
// Verifies:
//   - hydration reuses the server-rendered DOM (no duplicated text, marker
//     comments removed) and the app is interactive (Count: 0 -> click -> 1),
//   - client routing + history: a nav link pushState-navigates and swaps the
//     route; browser back pops to the previous route and restores it.
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const URL = process.env.URL || "http://localhost:8137";
const DP = Number(process.env.CDP_PORT || 9345);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const userDir = mkdtempSync(join(tmpdir(), "goowee-e2e-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  `--remote-debugging-port=${DP}`, `--user-data-dir=${userDir}`, "about:blank",
], { stdio: "ignore" });

let ws, nextId = 1;
const pending = new Map();
const logs = [];
const send = (method, params = {}) => {
  const id = nextId++;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((res, rej) => pending.set(id, { res, rej }));
};

async function main() {
  let target;
  for (let i = 0; i < 50 && !target; i++) {
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
    } else if (m.method === "Runtime.exceptionThrown") {
      logs.push("exception: " + (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text));
    }
  };

  await send("Runtime.enable");
  await send("Page.enable");
  await send("Page.navigate", { url: URL + "/" }); // the server-rendered landing page

  const evalJS = async (expr) => {
    const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || "eval error");
    return r.result.value;
  };
  const waitFor = async (expr, label) => {
    for (let i = 0; i < 200; i++) { if (await evalJS(expr).catch(() => false)) return; await sleep(100); }
    throw new Error("timeout waiting for: " + label + (logs.length ? "\n" + logs.join("\n") : ""));
  };
  // click a link/button by exact trimmed text, or (…Containing) by substring
  const clickText = (tag, t) => evalJS(`[...document.querySelectorAll('${tag}')].find(e=>e.textContent.trim()===${JSON.stringify(t)}).click()`);
  const clickLinkContaining = (sub) => evalJS(`[...document.querySelectorAll('a')].find(a=>a.textContent.includes(${JSON.stringify(sub)})).click()`);
  const markerCount = `(()=>{let n=0;const w=document.createTreeWalker(document.getElementById('root'),NodeFilter.SHOW_COMMENT);while(w.nextNode())if(/^g\\d+$/.test(w.currentNode.data))n++;return n;})()`;
  const heroCount = `(document.querySelector('.count')?.textContent.trim())`;

  const fail = [];
  const check = (cond, msg) => { if (!cond) fail.push(msg); };

  // --- Hydration on the landing page ---
  // Wait for a real hydration signal: the first applyMutations removes markers.
  await waitFor(`(${markerCount}) === 0 && ${heroCount} === '0'`, "landing hydration complete");
  check(await evalJS(`(document.body.innerText.match(/Reactive web UIs, written in Go\\./g)||[]).length`) === 1, "landing headline duplicated (hydration)");
  check(await evalJS(`document.querySelectorAll('.count').length`) === 1, "hero demo duplicated");

  // --- Interactivity: the live hero demo is a real goowee component ---
  await clickText("button", "increment");
  await waitFor(`${heroCount} === '1'`, "hero demo increment");
  check(await evalJS(heroCount) === "1", "hero counter did not update");

  // --- Routing + history ---
  await clickText("a", "Tutorial");
  await waitFor(`/Learn goowee by example/.test(document.body.innerText)`, "tutorial index");
  check(await evalJS(`location.pathname`) === "/tutorial", "Tutorial nav path");
  await clickLinkContaining("Counter");
  await waitFor(`[...document.querySelectorAll('button')].some(b=>/Count: 0/.test(b.textContent))`, "counter example");
  check(await evalJS(`location.pathname`) === "/counter", "counter route path");
  await evalJS(`history.back()`);
  await waitFor(`/Learn goowee by example/.test(document.body.innerText)`, "back to tutorial");
  check(await evalJS(`location.pathname`) === "/tutorial", "history back path");

  // --- URL params (preserved component reads the param reactively) ---
  await clickLinkContaining("Greeting");
  await waitFor(`/Hello, alice!/.test(document.body.innerText)`, "greet alice");
  check(await evalJS(`location.pathname`) === "/greet/alice", "greet path");
  await clickText("a", "Bob");
  await waitFor(`/Hello, bob!/.test(document.body.innerText)`, "greet bob (reactive param)");
  check(!(await evalJS(`/Hello, alice!/.test(document.body.innerText)`)), "stale 'alice' after param change");

  // --- Off-loop updates (core.Schedule): the stopwatch ticks from a goroutine ---
  const disp = `(()=>{const p=[...document.querySelectorAll('p')].find(p=>/^\\d\\d:\\d\\d\\.\\d$/.test(p.textContent.trim()));return p?p.textContent.trim():'';})()`;
  await clickText("a", "goowee");           // brand → landing
  await clickText("a", "Tutorial");
  await clickLinkContaining("Stopwatch");
  await waitFor(`(${disp}) === '00:00.0'`, "stopwatch page (display at 00:00.0)");
  await clickText("button", "Start");
  // The 100ms ticker goroutine posts updates via core.Schedule; the display
  // should advance past 00:00.0 within a couple of seconds.
  await waitFor(`(${disp}) !== '' && (${disp}) !== '00:00.0'`, "stopwatch advanced via goroutine (core.Schedule)");

  if (logs.length) console.log("--- browser logs ---\n" + logs.join("\n"));
  if (fail.length) { console.log("E2E FAIL:\n- " + fail.join("\n- ")); process.exitCode = 1; }
  else console.log("E2E PASS: hydration reuses SSR DOM, interactive, routing + history work.");
}

main().catch((e) => { console.error("E2E ERROR:", e.message); process.exitCode = 2; })
  .finally(() => { try { ws?.close(); } catch {} chrome.kill("SIGKILL"); });
