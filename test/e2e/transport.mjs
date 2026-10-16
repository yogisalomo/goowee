// Mutation-transport measurement (roadmap 3.4). Loads runtime/goowee.js into a
// blank page in headless Chrome — same no-dependency CDP pattern as smoke.mjs —
// mounts an SVG graph (500 circles, 1,000 lines) and applies layout-tick
// frames: every node moves, 5,000 SetAttribute mutations per frame, the same
// workload as BenchmarkLayoutTickFrame / BenchmarkEncodeLayoutTick in
// internal/dom. Reports medians of:
//   - payload  : the frame's JSON size
//   - decode   : UTF-8 decoding the payload into a JS string (what syscall/js
//                does when Go passes the batch as a string)
//   - parse    : JSON.parse of the payload
//   - apply    : applyMutations (parse + the DOM writes)
//   - layout   : the style + layout the writes cause (forced right after)
// Parse and decode are what a binary transport could replace; compare them to
// the whole frame. See docs/plans/roadmap-1.0.md (3.4).
//
// Env: CHROME (browser path), CDP_PORT (default 9347), TICKS (default 40).
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const DP = Number(process.env.CDP_PORT || 9347);
const TICKS = Number(process.env.TICKS || 40);
const runtimeJS = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "../../runtime/goowee.js"), "utf8");

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${mkdtempSync(join(tmpdir(), "goowee-transport-"))}`, "about:blank",
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

// Runs in the page: mount the graph, then time TICKS frames.
const measure = `(() => {
  const SVG = "http://www.w3.org/2000/svg", N = 500, E = 1000, TICKS = ${TICKS};
  document.body.replaceChildren(Object.assign(document.createElement("div"), {id: "root"}));
  const mount = [], circles = [], lines = [];
  let id = 1;
  const svg = id++;
  mount.push({type:0,nodeId:svg,key:"tag",value:"svg",ns:SVG},
             {type:2,nodeId:svg,key:"width",value:"1200"},{type:2,nodeId:svg,key:"height",value:"1200"});
  for (let i = 0; i < N; i++) {
    const c = id++; circles.push(c);
    mount.push({type:0,nodeId:c,key:"tag",value:"circle",ns:SVG},
               {type:2,nodeId:c,key:"cx",value:i},{type:2,nodeId:c,key:"cy",value:i},
               {type:2,nodeId:c,key:"r",value:"4"},{type:4,nodeId:svg,childId:c});
  }
  for (let e = 0; e < E; e++) {
    const l = id++; lines.push([l, e % N, (e * 7 + 1) % N]);
    mount.push({type:0,nodeId:l,key:"tag",value:"line",ns:SVG},
               {type:2,nodeId:l,key:"stroke",value:"#999"},{type:4,nodeId:svg,childId:l});
  }
  mount.push({type:4,nodeId:0,childId:svg});
  applyMutations(JSON.stringify(mount));

  const frame = (step) => {
    const x = (i) => i + step * 0.37, y = (i) => i - step * 0.61, m = [];
    circles.forEach((c, i) => m.push({type:2,nodeId:c,key:"cx",value:x(i)},{type:2,nodeId:c,key:"cy",value:y(i)}));
    for (const [l, a, b] of lines) {
      m.push({type:2,nodeId:l,key:"x1",value:x(a)},{type:2,nodeId:l,key:"y1",value:y(a)},
             {type:2,nodeId:l,key:"x2",value:x(b)},{type:2,nodeId:l,key:"y2",value:y(b)});
    }
    return JSON.stringify(m);
  };
  const enc = new TextEncoder(), dec = new TextDecoder();
  const t = {decode: [], parse: [], apply: [], layout: []};
  let bytes = 0, muts = 0;
  for (let s = 1; s <= TICKS + 10; s++) {
    const json = frame(s), utf8 = enc.encode(json);
    let t0 = performance.now(); dec.decode(utf8); const decode = performance.now() - t0;
    t0 = performance.now(); muts = JSON.parse(json).length; const parse = performance.now() - t0;
    t0 = performance.now(); applyMutations(json); const apply = performance.now() - t0;
    t0 = performance.now(); document.querySelector("svg").getBoundingClientRect(); document.body.offsetHeight;
    const layout = performance.now() - t0;
    if (s > 10) { t.decode.push(decode); t.parse.push(parse); t.apply.push(apply); t.layout.push(layout); }
    bytes = utf8.length;
  }
  const med = (a) => { a.sort((p, q) => p - q); return a[a.length >> 1]; };
  return {bytes, muts, decode: med(t.decode), parse: med(t.parse), apply: med(t.apply), layout: med(t.layout),
          moveBefore: typeof Element.prototype.moveBefore === "function"};
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
  await evalJS(runtimeJS + "\n;true");
  const r = await evalJS(measure);
  const ms = (v) => v.toFixed(2).padStart(7) + " ms";
  console.log(`layout tick: ${r.muts} mutations, ${(r.bytes / 1024).toFixed(0)} KiB JSON (median of ${TICKS} frames)`);
  console.log(`  decode  ${ms(r.decode)}   UTF-8 → JS string (the syscall/js crossing)`);
  console.log(`  parse   ${ms(r.parse)}   JSON.parse`);
  console.log(`  apply   ${ms(r.apply)}   applyMutations (parse + DOM writes)`);
  console.log(`  layout  ${ms(r.layout)}   style + layout of the writes`);
  const transport = r.decode + r.parse, frame = r.decode + r.apply + r.layout;
  console.log(`  transport (decode + parse) is ${(100 * transport / frame).toFixed(0)}% of the JS side of the frame (${ms(frame).trim()})`);
}

main().catch((e) => { console.error("transport:", e.message); process.exitCode = 2; })
  .finally(() => { try { ws?.close(); } catch {} chrome.kill("SIGKILL"); });
