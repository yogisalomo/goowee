// Drives test/e2e/canvas in headless Chrome (#159): goowee/canvas buffers sent
// with ref.Draw and canvas.View are replayed onto real canvases.
//   - pixels land where the Go code drew them, at device scale factors 1 and 2;
//   - every op of canvas/testdata/scene.json reaches CanvasRenderingContext2D
//     as the call the plan specifies, with the same arguments (the Go recorder
//     and the JS replayer agree);
//   - a canvas.View redraws when its signal changes;
//   - a canvas.Fill View follows its box's size, at both scale factors;
//   - a canvas.Animate View draws every frame, stops while the page is hidden
//     and resumes when it's shown;
//   - a device pixel ratio change replays every canvas at the new ratio.
// Usage: node test/e2e/canvas.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8153), DP = Number(process.env.CDP_PORT || 9453);
const dir = mkdtempSync(join(tmpdir(), "goowee-canvas-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/canvas"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));
const scene = JSON.parse(readFileSync("canvas/testdata/scene.json", "utf8"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}
const index = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

// Before the app boots, record every call and property write the #scene
// canvas's context receives.
const recorder = `(() => {
  window.sceneLog = [];
  window.animDraws = 0;
  window.resolutionQueries = []; // the media queries the runtime watches for ratio changes
  const mm = window.matchMedia;
  window.matchMedia = function (q) { const m = mm.call(window, q); if (q.includes("resolution")) resolutionQueries.push(m); return m; };
  const P = CanvasRenderingContext2D.prototype;
  const mine = (ctx) => ctx.canvas && ctx.canvas.id === "scene";
  for (const name of Object.getOwnPropertyNames(P)) {
    const d = Object.getOwnPropertyDescriptor(P, name);
    if (typeof d.value === "function" && name !== "constructor") {
      const orig = d.value;
      P[name] = function (...args) {
        if (name === "setTransform" && this.canvas && this.canvas.id === "anim") animDraws++; // once per replayed picture
        if (mine(this)) sceneLog.push([name, ...args.map((a) => Array.isArray(a) ? [...a] : a)]);
        return orig.apply(this, args);
      };
    } else if (d.set) {
      const set = d.set;
      Object.defineProperty(P, name, { ...d, set(v) { if (mine(this)) sceneLog.push(["set " + name, v]); set.call(this, v); } });
    }
  }
})();`;

// What scene.json's commands must turn into, per the buffer format.
const TAU = Math.PI * 2;
const CAPS = ["butt", "round", "square"], JOINS = ["miter", "round", "bevel"];
const ALIGNS = ["start", "end", "left", "right", "center"], BASELINES = ["alphabetic", "top", "hanging", "middle", "ideographic", "bottom"];
const simple = { Save: "save", Restore: "restore", Translate: "translate", Scale: "scale", Rotate: "rotate", Transform: "transform",
  BeginPath: "beginPath", ClosePath: "closePath", MoveTo: "moveTo", LineTo: "lineTo", Arc: "arc", Rect: "rect",
  QuadTo: "quadraticCurveTo", CubicTo: "bezierCurveTo", Fill: "fill", Stroke: "stroke", Clip: "clip",
  ClearRect: "clearRect", FillRect: "fillRect", StrokeRect: "strokeRect" };
function expected(cmds) {
  const out = [];
  for (const { op, args = [], text } of cmds) {
    if (simple[op]) out.push([simple[op], ...args]);
    else if (op === "FillStyle") out.push(["set fillStyle", text]);
    else if (op === "StrokeStyle") out.push(["set strokeStyle", text]);
    else if (op === "Font") out.push(["set font", text]);
    else if (op === "LineWidth") out.push(["set lineWidth", args[0]]);
    else if (op === "GlobalAlpha") out.push(["set globalAlpha", args[0]]);
    else if (op === "LineCap") out.push(["set lineCap", CAPS[args[0]]]);
    else if (op === "LineJoin") out.push(["set lineJoin", JOINS[args[0]]]);
    else if (op === "TextAlign") out.push(["set textAlign", ALIGNS[args[0]]]);
    else if (op === "TextBaseline") out.push(["set textBaseline", BASELINES[args[0]]]);
    else if (op === "LineDash") out.push(["setLineDash", args]);
    else if (op === "FillText") out.push(["fillText", text, args[0], args[1]]);
    else if (op === "StrokeText") out.push(["strokeText", text, args[0], args[1]]);
    else if (op === "Segments") for (let j = 0; j < args.length; j += 4) out.push(["moveTo", args[j], args[j + 1]], ["lineTo", args[j + 2], args[j + 3]]);
    else if (op === "Circles") for (let j = 0; j < args.length; j += 3) out.push(["moveTo", args[j] + args[j + 2], args[j + 1]], ["arc", args[j], args[j + 1], args[j + 2], 0, TAU]);
    else if (op === "Rects") for (let j = 0; j < args.length; j += 4) out.push(["rect", ...args.slice(j, j + 4)]);
    else if (op === "Polyline") { out.push(["moveTo", args[0], args[1]]); for (let j = 2; j < args.length; j += 2) out.push(["lineTo", args[j], args[j + 1]]); }
    else if (op === "FillRects") for (let j = 0; j < args.length; j += 4) out.push(["fillRect", ...args.slice(j, j + 4)]);
    else throw new Error("canvas.mjs doesn't know op " + op + " — add it here and to the replayer");
  }
  return out;
}

const profile = mkdtempSync(join(tmpdir(), "goowee-canvas-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("CANVAS CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target; // up to 20s: Chrome can take ~8s to open its debug port
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && (m.params.type === "error" || m.params.type === "warning")) errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") errors.push("exception: " + (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text)); };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;
const waitFor = async (expr, label) => { for (let i = 0; i < 100; i++) { if (await ev(expr)) return; await sleep(50); } throw new Error("timeout: " + label); };
// A pixel of a canvas's backing store, as [r, g, b, a].
const pixel = (sel, x, y) => ev(`Array.from(document.querySelector(${JSON.stringify(sel)}).getContext("2d").getImageData(${x}, ${y}, 1, 1).data)`);
const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");
await send("Page.addScriptToEvaluateOnNewDocument", { source: recorder });
try {
  const base = `http://localhost:${PORT}`;
  await send("Page.navigate", { url: base + "/" });
  await waitFor(`document.querySelector("#pixels")?.width === 100 && window.sceneLog.length > 0`, "first draw");

  // Pixels at device scale factor 1.
  check(same(await pixel("#pixels", 15, 15), [255, 0, 0, 255]), "dpr 1: (15, 15) isn't red: " + await pixel("#pixels", 15, 15));
  check(same(await pixel("#pixels", 50, 50), [0, 0, 0, 0]), "dpr 1: (50, 50) isn't transparent");
  check(await ev(`document.querySelector("#pixels").height`) === 80, "dpr 1: backing store height");

  // Every op, in order: the runtime's frame setup, the scene, the final restore.
  const got = await ev(`sceneLog`);
  const want = [["setTransform", 1, 0, 0, 1, 0, 0], ["clearRect", 0, 0, 120, 90], ["save"], ...expected(scene), ["restore"]];
  if (!same(got, want)) {
    let i = 0; while (i < want.length && same(got[i], want[i])) i++;
    fail.push(`scene: call ${i} is ${JSON.stringify(got[i])}, want ${JSON.stringify(want[i])} (${got.length} calls, want ${want.length})`);
  }

  // canvas.View: sized by its style, redrawn when its signal changes.
  check(await ev(`document.querySelector("#view").getAttribute("style")`) === "width:60px;height:40px", "View style");
  check(await ev(`document.querySelector("#view").getAttribute("aria-label")`) === "Bars", "View label");
  check(same(await pixel("#view", 5, 5), [0, 255, 0, 255]), "View: first bar missing");
  check(same(await pixel("#view", 15, 5), [0, 0, 0, 0]), "View: drew a second bar too early");
  await ev(`document.querySelector("#grow").click()`);
  await waitFor(`document.querySelector("#view").getContext("2d").getImageData(15, 5, 1, 1).data[3] === 255`, "View redraw after the click");
  check(same(await pixel("#view", 25, 5), [0, 0, 0, 0]), "View: the redraw didn't clear first");

  // canvas.Fill: the canvas follows its box, and so does the picture.
  await waitFor(`document.querySelector("#fill").width === 300`, "Fill: first layout");
  check(await ev(`document.querySelector("#fill").height`) === 200, "Fill: backing store height");
  check(same(await pixel("#fill", 295, 195), [255, 0, 0, 255]), "Fill: the picture doesn't cover the canvas");
  await ev(`document.querySelector("#fillbox").style.width = "400px"`);
  await waitFor(`document.querySelector("#fill").width === 400`, "Fill: backing store after the box grew");
  await waitFor(`document.querySelector("#fill").getContext("2d").getImageData(395, 100, 1, 1).data[3] === 255`, "Fill: redraw at the new size");

  // canvas.Animate: a picture every frame while the page is shown, none
  // while it's hidden, and again once it's shown.
  const drawsIn = async (ms) => { const a = await ev(`animDraws`); await sleep(ms); return (await ev(`animDraws`)) - a; };
  const shown = await drawsIn(500);
  check(shown >= 10, `Animate: ${shown} pictures in 500ms`);
  await ev(`Object.defineProperty(document, "hidden", {configurable: true, get: () => true}); document.dispatchEvent(new Event("visibilitychange"))`);
  await sleep(100); // a frame already requested may still run
  const hidden = await drawsIn(500);
  check(hidden === 0, `Animate: ${hidden} pictures while hidden`);
  await ev(`delete document.hidden; document.dispatchEvent(new Event("visibilitychange"))`);
  const again = await drawsIn(500);
  check(again >= 10, `Animate: ${again} pictures in 500ms after the page was shown again`);

  // Device scale factor 2: the backing store doubles, the drawing follows.
  await send("Emulation.setDeviceMetricsOverride", { width: 800, height: 600, deviceScaleFactor: 2, mobile: false });
  await send("Page.reload");
  await waitFor(`devicePixelRatio === 2 && document.querySelector("#pixels")?.width === 200`, "dpr 2 backing store");
  check(await ev(`document.querySelector("#pixels").height`) === 160, "dpr 2: backing store height");
  check(await ev(`document.querySelector("#pixels").getBoundingClientRect().width`) === 100, "dpr 2: CSS width changed");
  check(same(await pixel("#pixels", 30, 30), [255, 0, 0, 255]), "dpr 2: device pixel (30, 30) isn't red");
  check(same(await pixel("#pixels", 62, 62), [0, 0, 0, 0]), "dpr 2: device pixel (62, 62) isn't transparent");
  check(same((await ev(`sceneLog`))[0], ["setTransform", 2, 0, 0, 2, 0, 0]), "dpr 2: the scene isn't scaled");
  await waitFor(`document.querySelector("#fill").width === 600`, "dpr 2: Fill backing store");
  await waitFor(`document.querySelector("#fill").getContext("2d").getImageData(590, 390, 1, 1).data[3] === 255`, "dpr 2: Fill picture");

  // A ratio change without a reload (zoom, another screen) replays every
  // canvas at the new ratio.
  await send("Emulation.setDeviceMetricsOverride", { width: 800, height: 600, deviceScaleFactor: 1, mobile: false });
  // Headless Chrome changes the ratio without firing the media query's
  // change event, so fire it on the runtime's query, as a zoom would.
  check(await ev(`resolutionQueries.length > 0 && resolutionQueries.at(-1).media.includes("2dppx")`), "no resolution media query watched at dpr 2");
  await ev(`resolutionQueries.at(-1).dispatchEvent(new Event("change"))`);
  await waitFor(`devicePixelRatio === 1 && document.querySelector("#pixels").width === 100`, "replay after the ratio changed");
  check(same(await pixel("#pixels", 15, 15), [255, 0, 0, 255]), "ratio change: (15, 15) isn't red");
  check(await ev(`document.querySelector("#fill").width`) === 300, "ratio change: Fill backing store");
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors or warnings:\n" + errors.join("\n"));
console.log(fail.length ? "CANVAS CHECK FAIL:\n- " + fail.join("\n- ") : "CANVAS CHECK PASS: pixels at dpr 1 and 2 and after a ratio change, every op of the golden scene, canvas.View redraws, Fill follows its box, Animate pauses while hidden");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
