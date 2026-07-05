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
  await send("Page.navigate", { url: URL + "/counter" });

  const evalJS = async (expr) => {
    const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || "eval error");
    return r.result.value;
  };
  const waitFor = async (expr, label) => {
    for (let i = 0; i < 200; i++) { if (await evalJS(expr).catch(() => false)) return; await sleep(100); }
    throw new Error("timeout waiting for: " + label + (logs.length ? "\n" + logs.join("\n") : ""));
  };
  const hasCounter = `[...document.querySelectorAll('button')].some(b => b.textContent.includes('Count:'))`;
  const markerCount = `(()=>{let n=0;const w=document.createTreeWalker(document.getElementById('root'),NodeFilter.SHOW_COMMENT);while(w.nextNode())if(/^g\\d+$/.test(w.currentNode.data))n++;return n;})()`;

  const fail = [];
  const check = (cond, msg) => { if (!cond) fail.push(msg); };

  // --- Hydration ---
  // The button exists in the SSR HTML, so wait for a real hydration signal:
  // the WASM app's first applyMutations removes the marker comments.
  await waitFor(`(${markerCount}) === 0 && ${hasCounter}`, "hydration complete");
  check(await evalJS(`(document.body.innerText.match(/Count: 0/g)||[]).length`) === 1, "text duplicated (Count: 0 not once)");
  check(await evalJS(`(document.body.innerText.match(/Hello!/g)||[]).length`) === 1, "greeting duplicated");
  check(await evalJS(`(()=>{let n=0;const w=document.createTreeWalker(document.getElementById('root'),NodeFilter.SHOW_COMMENT);while(w.nextNode())if(/^g\\d+$/.test(w.currentNode.data))n++;return n;})()`) === 0, "hydration marker comments not removed");

  // --- Interactivity ---
  await evalJS(`[...document.querySelectorAll('button')].find(b=>b.textContent.includes('Count:')).click()`);
  let txt = "";
  for (let i = 0; i < 40; i++) { txt = await evalJS(`[...document.querySelectorAll('button')].find(b=>/Count:/.test(b.textContent)).textContent`); if (/Count: 1/.test(txt)) break; await sleep(50); }
  check(/Count: 1/.test(txt), `click did not update count (got ${JSON.stringify(txt)})`);

  // --- Routing + history ---
  await evalJS(`[...document.querySelectorAll('a')].find(a=>a.textContent.trim()==='About').click()`);
  await waitFor(`/A minimal Go WASM/.test(document.body.innerText)`, "navigate to about");
  check(await evalJS(`location.pathname`) === "/about", "nav did not pushState to /about");
  check(await evalJS(`!(${hasCounter})`), "counter still present after navigating away");

  await evalJS(`history.back()`);
  await waitFor(hasCounter, "back to counter");
  check(await evalJS(`location.pathname`) === "/counter", "back did not restore /counter");

  // --- URL params (preserved component reads the param reactively) ---
  await evalJS(`[...document.querySelectorAll('a')].find(a=>a.textContent.trim()==='Greet').click()`);
  await waitFor(`/Hello, alice!/.test(document.body.innerText)`, "greet alice");
  check(await evalJS(`location.pathname`) === "/greet/alice", "greet nav path");
  await evalJS(`[...document.querySelectorAll('a')].find(a=>a.textContent.trim()==='Bob').click()`);
  await waitFor(`/Hello, bob!/.test(document.body.innerText)`, "greet bob (reactive param change)");
  check(await evalJS(`location.pathname`) === "/greet/bob", "greet bob path");
  check(!(await evalJS(`/Hello, alice!/.test(document.body.innerText)`)), "stale 'alice' after param change");

  if (logs.length) console.log("--- browser logs ---\n" + logs.join("\n"));
  if (fail.length) { console.log("E2E FAIL:\n- " + fail.join("\n- ")); process.exitCode = 1; }
  else console.log("E2E PASS: hydration reuses SSR DOM, interactive, routing + history work.");
}

main().catch((e) => { console.error("E2E ERROR:", e.message); process.exitCode = 2; })
  .finally(() => { try { ws?.close(); } catch {} chrome.kill("SIGKILL"); });
