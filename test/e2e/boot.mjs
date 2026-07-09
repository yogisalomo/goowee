// Boot-latency / TTI measurement harness (roadmap 3.2). Drives the real WASM app
// in headless Chromium over the DevTools Protocol — same no-dependency CDP
// pattern as smoke.mjs — and reads the `goowee:*` performance marks that
// runtime/goowee.js emits during boot. See docs/plans/boot-latency-measurement.md.
//
// For each scenario it does N cold-cache loads (Network cache disabled) and
// reports the median phase split:
//   - download+compile : fetch(main.wasm) → instantiateStreaming resolved
//   - go boot+render   : go.run → first applyMutations (interactive)
//   - hydrate          : the one-time SSR-claim pass (subset of go boot)
//   - TTI              : navigation start → interactive
// plus the wasm bytes-on-the-wire from Resource Timing.
//
// Env: URL (SSR server base, default http://localhost:8137),
//      ITERS (default 7), CHROME (browser path), CDP_PORT (default 9346),
//      GOOWEE_TTI_BUDGET_MS (optional; non-zero exit if median TTI exceeds it),
//      THROTTLE (off | 4g | fast3g | slow3g — emulate a real network so the
//      download phase, and thus the compression win, is visible; loopback is
//      too fast for it to show).
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const URL = process.env.URL || "http://localhost:8137";
const DP = Number(process.env.CDP_PORT || 9346);
const BUDGET = process.env.GOOWEE_TTI_BUDGET_MS ? Number(process.env.GOOWEE_TTI_BUDGET_MS) : 0;

// Network profiles (throughput in bytes/s, latency in ms), matching Chrome
// DevTools / Puppeteer presets. THROTTLE=off (default) leaves the loopback
// unthrottled — fine for the phase split, but the download win only shows under
// a bandwidth-limited profile.
const THROTTLES = {
  off: null,
  "4g": { download: (4 * 1000 * 1000 / 8) * 0.9, upload: (3 * 1000 * 1000 / 8) * 0.9, latency: 70 },
  fast3g: { download: (1.6 * 1000 * 1000 / 8) * 0.9, upload: (750 * 1000 / 8) * 0.9, latency: 150 * 3.75 },
  slow3g: { download: (500 * 1000 / 8) * 0.8, upload: (500 * 1000 / 8) * 0.8, latency: 400 * 5 },
};
const THROTTLE = (process.env.THROTTLE || "off").toLowerCase();
const netProfile = THROTTLES[THROTTLE];
if (netProfile === undefined) { console.error(`unknown THROTTLE "${THROTTLE}" (want: ${Object.keys(THROTTLES).join(", ")})`); process.exit(2); }
// Slow profiles need fewer iterations and a longer interactivity ceiling.
const ITERS = Number(process.env.ITERS || (netProfile ? 3 : 7));
const MAXWAIT_MS = netProfile ? 90000 : 15000;

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const cands = [
    "/opt/pw-browsers/chromium",
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  ];
  for (const c of cands) if (existsSync(c)) return c;
  return "google-chrome"; // Linux/CI on PATH
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const median = (xs) => {
  const v = xs.filter((x) => x != null).sort((a, b) => a - b);
  if (!v.length) return null;
  const m = Math.floor(v.length / 2);
  return v.length % 2 ? v[m] : (v[m - 1] + v[m]) / 2;
};
const min = (xs) => { const v = xs.filter((x) => x != null); return v.length ? Math.min(...v) : null; };
const ms = (x) => (x == null ? "   —  " : x.toFixed(1).padStart(6));
const kb = (x) => (x == null ? "  —  " : (x / 1024).toFixed(0).padStart(5));

const userDir = mkdtempSync(join(tmpdir(), "goowee-boot-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
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

// Scenarios: both hit the one SSR server. "/" is server-rendered (hydrate path);
// "/error" is served via the index.html fallback (fresh client render, no SSR
// nodes) — a clean A/B of hydrate vs. cold render on identical assets.
const scenarios = [
  { name: "SSR + hydrate", path: "/" },
  { name: "client render", path: "/error" },
];

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
  await send("Network.enable");
  await send("Network.setCacheDisabled", { cacheDisabled: true }); // cold download every load
  if (netProfile) {
    await send("Network.emulateNetworkConditions", {
      offline: false,
      downloadThroughput: netProfile.download,
      uploadThroughput: netProfile.upload,
      latency: netProfile.latency,
    });
  }

  const evalJS = async (expr) => {
    const r = await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true });
    if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || "eval error");
    return r.result.value;
  };
  const navigate = async (url) => {
    await send("Page.navigate", { url: "about:blank" });
    await sleep(50);
    await send("Page.navigate", { url });
  };
  const waitInteractive = async () => {
    const deadline = Date.now() + MAXWAIT_MS;
    while (Date.now() < deadline) {
      if (await evalJS(`!!(window.goowee && window.goowee._interactive)`).catch(() => false)) return true;
      await sleep(50);
    }
    return false;
  };

  const results = {};
  for (const sc of scenarios) {
    const runs = [];
    for (let i = 0; i < ITERS; i++) {
      await navigate(URL + sc.path);
      if (!await waitInteractive()) {
        logs.push(`scenario "${sc.name}" iter ${i}: never became interactive`);
        continue;
      }
      runs.push(await evalJS(`window.goowee.bootTimings()`));
    }
    results[sc.name] = runs;
  }

  // --- report ---
  const col = (runs, sel) => runs.map(sel);
  const net = netProfile ? `${THROTTLE} (~${(netProfile.download / 1024).toFixed(0)} KB/s down)` : "unthrottled (loopback)";
  console.log(`\ngoowee boot latency — ${URL}  (${ITERS} cold-cache loads/scenario, median)`);
  console.log(`network: ${net}\n`);
  console.log("scenario         download+compile   go boot+render     hydrate        TTI        wasm KB");
  console.log("               " + "-".repeat(82));
  let worstTTI = 0;
  for (const sc of scenarios) {
    const runs = results[sc.name] || [];
    if (!runs.length) { console.log(`${sc.name.padEnd(15)} (no successful runs)`); continue; }
    const tti = median(col(runs, (r) => r.phases.tti));
    worstTTI = Math.max(worstTTI, tti || 0);
    const line =
      sc.name.padEnd(15) +
      "  " + ms(median(col(runs, (r) => r.phases.downloadCompile))) + " ms" +
      "      " + ms(median(col(runs, (r) => r.phases.goBoot))) + " ms" +
      "   " + ms(median(col(runs, (r) => r.phases.hydrate))) + " ms" +
      "  " + ms(tti) + " ms" +
      "   " + kb(median(col(runs, (r) => r.wasm?.transferSize)));
    console.log(line);
  }
  console.log("               " + "-".repeat(82));
  console.log(netProfile
    ? `(throttled ${THROTTLE}: download reflects bytes-on-the-wire — the compression win shows here)\n`
    : "(loopback: download is fast — read the phase *split* and wasm KB, not absolute download ms;\n run THROTTLE=4g to make the download phase, and the compression win, visible)\n");

  if (logs.length) console.log("--- notes ---\n" + logs.join("\n") + "\n");

  if (BUDGET) {
    if (worstTTI > BUDGET) { console.log(`BUDGET FAIL: median TTI ${worstTTI.toFixed(1)}ms > ${BUDGET}ms`); process.exitCode = 1; }
    else console.log(`BUDGET OK: worst median TTI ${worstTTI.toFixed(1)}ms <= ${BUDGET}ms`);
  }
}

main().catch((e) => {
  console.error("BOOT ERROR:", e.message);
  if (logs.length) console.error("--- notes ---\n" + logs.join("\n"));
  process.exitCode = 2;
}).finally(() => { try { ws?.close(); } catch {} chrome.kill("SIGKILL"); });
