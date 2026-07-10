// Dev-mode inspector + structured recover logging (roadmap 4.2).
// Expects an SSR server at $URL (see run.sh). No npm deps — Node CDP only.
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const URL = process.env.URL || "http://localhost:8137";
const DP = Number(process.env.CDP_PORT || 9346);

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const userDir = mkdtempSync(join(tmpdir(), "goowee-devtools-e2e-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  `--remote-debugging-port=${DP}`, `--user-data-dir=${userDir}`, "about:blank",
], { stdio: "ignore" });

let ws, nextId = 1;
const pending = new Map();
const consoleLogs = [];

const send = (method, params = {}) => {
  const id = nextId++;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((res, rej) => pending.set(id, { res, rej }));
};

function consoleText(args) {
  return args.map((a) => {
    if (a.type === "string") return a.value;
    if (a.type === "object" && a.preview) {
      return a.preview.properties?.map((p) => `${p.name}=${p.value}`).join(" ") || "[object]";
    }
    return a.description || a.value || "";
  }).join(" ");
}

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
    } else if (m.method === "Runtime.consoleAPICalled") {
      consoleLogs.push({
        type: m.params.type,
        text: consoleText(m.params.args || []),
        args: m.params.args || [],
      });
    }
  };

  await send("Runtime.enable");
  await send("Page.enable");
  await send("Page.navigate", { url: URL + "/?goowee-dev" });

  const evalJS = async (expr) => {
    const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || "eval error");
    return r.result.value;
  };
  const waitFor = async (expr, label) => {
    for (let i = 0; i < 200; i++) { if (await evalJS(expr).catch(() => false)) return; await sleep(100); }
    throw new Error("timeout waiting for: " + label);
  };
  const clickText = (tag, t) => evalJS(`(()=>{const el=[...document.querySelectorAll('${tag}')].find(e=>e.textContent.trim()===${JSON.stringify(t)}); if(!el) throw new Error('no ${tag} with text '+${JSON.stringify(t)}); el.click();})()`);
  const clickLinkContaining = (sub) => evalJS(`(()=>{const el=[...document.querySelectorAll('a')].find(a=>a.textContent.includes(${JSON.stringify(sub)})); if(!el) throw new Error('no link containing '+${JSON.stringify(sub)}); el.click();})()`);
  const heroCount = `(document.querySelector('.count')?.textContent.trim())`;
  const hydrationReady = `(()=>{const root=document.getElementById('root'); if(!root) return false; let n=0;const w=document.createTreeWalker(root,NodeFilter.SHOW_COMMENT);while(w.nextNode())if(/^g\\d+$/.test(w.currentNode.data))n++;return n===0&&${heroCount}==='0'&&[...document.querySelectorAll('button')].some(b=>b.textContent.trim()==='increment');})()`;

  const fail = [];
  const check = (cond, msg) => { if (!cond) fail.push(msg); };

  // --- Dev mode enabled + inspector API ---
  await waitFor(`typeof goowee.inspect === 'function' && typeof goowee.inspectGo === 'function'`, "devtools wired");
  check(await evalJS(`goowee.devEnabled()`) === true, "goowee.devEnabled() should be true with ?goowee-dev");

  const snap = await evalJS(`goowee.inspect()`);
  check(snap && typeof snap === "object", "goowee.inspect() returns a snapshot object");
  check(snap?.tree?.type === "component", "snapshot tree root is a component");
  check(Array.isArray(snap?.signals) && snap.signals.length > 0, "snapshot lists registered signals");

  // --- Hydration + interactivity still work with dev mode on ---
  const heroCount = `(document.querySelector('.count')?.textContent.trim())`;
  await waitFor(hydrationReady, "landing hydrated with dev mode");
  await clickText("button", "increment");
  await waitFor(`${heroCount} === '1'`, "hero increment with dev mode");
  check(await evalJS(heroCount) === "1", "counter did not update with dev mode");

  // --- Routing smoke with dev mode ---
  await clickText("a", "Tutorial");
  await waitFor(`/Learn goowee by example/.test(document.body.innerText)`, "tutorial with dev mode");
  check(await evalJS(`location.pathname`) === "/tutorial", "tutorial path with dev mode");

  // --- Structured recover.error_boundary log on /error ---
  consoleLogs.length = 0;
  await clickLinkContaining("Error boundary");
  await waitFor(`/Recovered:/.test(document.body.innerText)`, "error boundary fallback");

  const boundaryLog = consoleLogs.find((l) =>
    l.text.includes("recover.error_boundary") ||
    l.args.some((a) => a.description?.includes("recover.error_boundary"))
  );
  check(!!boundaryLog, "console received recover.error_boundary structured log");
  check(boundaryLog?.type === "error", "recover.error_boundary routed to console.error");

  if (consoleLogs.length) console.log("--- captured console ---\n" + consoleLogs.map((l) => `[${l.type}] ${l.text}`).join("\n"));
  if (fail.length) { console.log("DEVTOOLS E2E FAIL:\n- " + fail.join("\n- ")); process.exitCode = 1; }
  else console.log("DEVTOOLS E2E PASS: inspector snapshot, dev mode, structured error-boundary log.");
}

main().catch((e) => {
  console.error("DEVTOOLS E2E ERROR:", e.message);
  if (consoleLogs.length) console.error("--- captured console ---\n" + consoleLogs.map((l) => `[${l.type}] ${l.text}`).join("\n"));
  process.exitCode = 2;
}).finally(() => { try { ws?.close(); } catch {} chrome.kill("SIGKILL"); });
