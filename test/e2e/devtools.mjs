// Dev-mode inspector + structured recover logging (roadmap 4.2).
// Expects an SSR server at $URL (see run.sh). No npm deps — Node CDP only.
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
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
  // CI runners and containers need --no-sandbox; harmless locally. Opt out with CHROME_NO_SANDBOX=0.
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
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
  for (let i = 0; i < 200 && !target; i++) { // up to 20s: Chrome can take ~8s to open its debug port
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
  // Auto-wait for the target to exist before clicking (see smoke.mjs).
  const clickText = async (tag, t) => {
    await waitFor(`[...document.querySelectorAll('${tag}')].some(e=>e.textContent.trim()===${JSON.stringify(t)})`, `${tag} with text ${JSON.stringify(t)}`);
    return evalJS(`(()=>{const el=[...document.querySelectorAll('${tag}')].find(e=>e.textContent.trim()===${JSON.stringify(t)}); el.click();})()`);
  };
  const clickLinkContaining = async (sub) => {
    await waitFor(`[...document.querySelectorAll('a')].some(a=>a.textContent.includes(${JSON.stringify(sub)}))`, `link containing ${JSON.stringify(sub)}`);
    return evalJS(`(()=>{const el=[...document.querySelectorAll('a')].find(a=>a.textContent.includes(${JSON.stringify(sub)})); el.click();})()`);
  };
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
  // Named signals: the router's, named in main before bridge.Run (held until
  // dev mode was known), and the hero counter's, named in its component.
  const named = (n) => snap?.signals?.find((sg) => sg.name === n);
  check(named("router.path")?.value === "/", "snapshot lists the router's named path signal: " + JSON.stringify(named("router.path")));
  check(named("hero.count")?.kind === "state" && named("hero.count")?.value === "0", "snapshot lists hero.count: " + JSON.stringify(named("hero.count")));

  // --- Hydration + interactivity still work with dev mode on ---
  await waitFor(hydrationReady, "landing hydrated with dev mode");
  await clickText("button", "increment");
  await waitFor(`${heroCount} === '1'`, "hero increment with dev mode");
  check(await evalJS(heroCount) === "1", "counter did not update with dev mode");

  // --- Change log: the write, its values, and the code behind it ---
  const lastChange = (name) => evalJS(`(goowee.changes() || []).filter((c) => c.name === ${JSON.stringify(name)}).pop() || null`);
  const inc = await lastChange("hero.count");
  check(inc && inc.from === "0" && inc.to === "1", "change log has hero.count 0 → 1: " + JSON.stringify(inc));
  check(/^app\/site\.go:\d+$/.test(inc?.at || "") && /^app\..*\bheroDemo\./.test(inc?.func || ""), "the write is traced to the click handler in site.go: " + JSON.stringify(inc));

  // --- Routing smoke with dev mode ---
  await clickText("a", "Tutorial");
  await waitFor(`/Learn goowee by example/.test(document.body.innerText)`, "tutorial with dev mode");
  check(await evalJS(`location.pathname`) === "/tutorial", "tutorial path with dev mode");
  const nav = await lastChange("router.path");
  check(nav && nav.from === "/" && nav.to === "/tutorial", "change log has the navigation: " + JSON.stringify(nav));
  check(Array.isArray(await evalJS(`goowee.changes({ clear: true })`)) && (await evalJS(`goowee.changes().length`)) === 0, "goowee.changes({ clear: true }) empties the log");

  // --- Structured recover.error_boundary log on /error ---
  consoleLogs.length = 0;
  await clickLinkContaining("Error boundary");
  await waitFor(`/Recovered:/.test(document.body.innerText)`, "error boundary fallback");

  const mentionsBoundary = (l) =>
    l.text.includes("recover.error_boundary") ||
    l.args.some((a) => a.description?.includes("recover.error_boundary"));
  check(consoleLogs.some(mentionsBoundary), "console received a recover.error_boundary log");
  // core.Log emits to the Go host logger (surfaces as a console.log passthrough)
  // AND the structured sink, which routes recover.* to console.error. Assert the
  // structured error path specifically, not just the first line mentioning it.
  const boundaryError = consoleLogs.find((l) => l.type === "error" && mentionsBoundary(l));
  check(!!boundaryError, "recover.error_boundary routed to console.error");

  if (consoleLogs.length) console.log("--- captured console ---\n" + consoleLogs.map((l) => `[${l.type}] ${l.text}`).join("\n"));
  if (fail.length) { console.log("DEVTOOLS E2E FAIL:\n- " + fail.join("\n- ")); process.exitCode = 1; }
  else console.log("DEVTOOLS E2E PASS: inspector snapshot, named signals, change log, dev mode, structured error-boundary log.");
}

main().catch((e) => {
  console.error("DEVTOOLS E2E ERROR:", e.message);
  if (consoleLogs.length) console.error("--- captured console ---\n" + consoleLogs.map((l) => `[${l.type}] ${l.text}`).join("\n"));
  process.exitCode = 2;
}).finally(() => {
  try { ws?.close(); } catch {}
  // Delete the Chrome profile (tens of MB) once Chrome has exited.
  chrome.once("exit", () => { try { rmSync(userDir, { recursive: true, force: true, maxRetries: 3 }); } catch {} });
  chrome.kill("SIGKILL");
});
