// End-to-end smoke test: drives the real WASM app in headless Chrome over the
// DevTools Protocol. No npm dependencies — Node (>=21) has global fetch and
// WebSocket. Expects an SSR server already running at $URL (see run.sh).
//
// Verifies:
//   - hydration reuses the server-rendered DOM (no duplicated text, marker
//     comments removed) and the app is interactive (Count: 0 -> click -> 1),
//   - client routing + history: a nav link pushState-navigates and swaps the
//     route; browser back pops to the previous route and restores it,
//   - refs both ways: ref.Focus reaches the node, ref.Get reads a value back,
//     and a picked file's bytes reach Go through e.Files()[i].Bytes().
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, writeFileSync } from "node:fs";
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
  // CI runners and containers need --no-sandbox; harmless locally. Opt out with CHROME_NO_SANDBOX=0.
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
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
  // Auto-wait for the target to exist before clicking — a just-navigated route
  // may not have rendered its links yet, and CI timing is slower than local.
  const clickText = async (tag, t) => {
    await waitFor(`[...document.querySelectorAll('${tag}')].some(e=>e.textContent.trim()===${JSON.stringify(t)})`, `${tag} with text ${JSON.stringify(t)}`);
    return evalJS(`(()=>{const el=[...document.querySelectorAll('${tag}')].find(e=>e.textContent.trim()===${JSON.stringify(t)}); el.click();})()`);
  };
  const clickLinkContaining = async (sub) => {
    await waitFor(`[...document.querySelectorAll('a')].some(a=>a.textContent.includes(${JSON.stringify(sub)}))`, `link containing ${JSON.stringify(sub)}`);
    return evalJS(`(()=>{const el=[...document.querySelectorAll('a')].find(a=>a.textContent.includes(${JSON.stringify(sub)})); el.click();})()`);
  };
  const markerCount = `(()=>{const root=document.getElementById('root'); if(!root) return -1; let n=0;const w=document.createTreeWalker(root,NodeFilter.SHOW_COMMENT);while(w.nextNode())if(/^g\\d+$/.test(w.currentNode.data))n++;return n;})()`;
  const heroCount = `(document.querySelector('.count')?.textContent.trim())`;
  const hydrationReady = `(()=>{const m=${markerCount}; return m>=0&&m===0&&${heroCount}==='0'&&[...document.querySelectorAll('button')].some(b=>b.textContent.trim()==='increment');})()`;

  const fail = [];
  const check = (cond, msg) => { if (!cond) fail.push(msg); };

  // --- Hydration on the landing page ---
  // Wait for a real hydration signal: the first applyMutations removes markers.
  await waitFor(hydrationReady, "landing hydration complete");
  check(await evalJS(`(document.body.innerText.match(/Reactive web UIs, written in Go\\./g)||[]).length`) === 1, "landing headline duplicated (hydration)");
  check(await evalJS(`document.querySelectorAll('.count').length`) === 1, "hero demo duplicated");
  // Inline SVG (h.Svg) must carry the SVG namespace end-to-end, root and descendants.
  check(await evalJS(`(()=>{const s=document.querySelector('.logo');const ns='http://www.w3.org/2000/svg';return !!s && s.namespaceURI===ns && s.querySelector('rect')?.namespaceURI===ns;})()`), "inline SVG logo namespaced correctly");

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

  // --- Refs (h.Ref → ref.Focus): imperative DOM command reaches the node ---
  await clickText("a", "goowee");           // brand → landing
  await clickText("a", "Tutorial");
  await clickLinkContaining("Form");
  await waitFor(`!!document.querySelector('input[name="name"]')`, "form page");
  await clickText("button", "Focus name");
  await waitFor(`document.activeElement && document.activeElement.name === 'name'`, "ref.Focus focused the name input");

  // --- Ref reads (ref.Get): a value comes back from the node on the next frame ---
  await clickText("button", "Measure name");
  await waitFor(`/Name input is [1-9]\\d*px wide/.test(document.body.innerText)`, "ref.Get returned the input's offsetWidth");

  // --- File input (e.Files + File.Bytes): the picked file's bytes reach Go ---
  const filePath = join(userDir, "note.txt");
  writeFileSync(filePath, "hello from e2e");
  const doc = await send("DOM.getDocument", { depth: 0 });
  const { nodeId: fileInput } = await send("DOM.querySelector", { nodeId: doc.root.nodeId, selector: 'input[name="attachment"]' });
  check(fileInput > 0, "file input rendered");
  await send("DOM.setFileInputFiles", { nodeId: fileInput, files: [filePath] });
  await waitFor(`/note\\.txt: read 14 bytes: "hello from e2e"/.test(document.body.innerText)`, "File.Bytes delivered the picked file's contents");

  // --- Async data (hooks.UseResource): loading → loaded via a goroutine ---
  await clickText("a", "goowee");
  await clickText("a", "Tutorial");
  await clickLinkContaining("Async");
  await waitFor(`/Loading…/.test(document.body.innerText)`, "async page shows loading");
  await waitFor(`/Loaded at /.test(document.body.innerText)`, "async resource resolved");

  // --- Error boundary: a panicking subtree shows a fallback; page keeps working ---
  await clickText("a", "goowee");
  await clickText("a", "Tutorial");
  await clickLinkContaining("Error boundary");
  await waitFor(`/Recovered:/.test(document.body.innerText)`, "error boundary rendered its fallback");
  check(await evalJS(`/This line still renders/.test(document.body.innerText)`), "page kept rendering around the failed boundary");

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

main().catch((e) => {
  console.error("E2E ERROR:", e.message);
  if (logs.length) console.error("--- browser logs ---\n" + logs.join("\n"));
  process.exitCode = 2;
}).finally(() => { try { ws?.close(); } catch {} chrome.kill("SIGKILL"); });
