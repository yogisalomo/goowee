// Drives test/e2e/transitions in headless Chrome (#156):
//   - a removed node stays in the DOM with "<name>-leave" and `inert` until
//     its transition ends, then is removed and forgotten (no nodeCount leak);
//   - a click on the leaving node does not reach Go;
//   - prefers-reduced-motion removes it immediately, without the leave class.
// Usage: node test/e2e/transitions.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8158), DP = Number(process.env.CDP_PORT || 9458);
const dir = mkdtempSync(join(tmpdir(), "goowee-transitions-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/transitions"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}
// The leave class is caught by a MutationObserver, but keep the animation
// long enough that the inert-node click is exercised even on a busy machine.
const css = `.panel { opacity: 1; transition: opacity 2000ms linear; }
.panel.fade-enter { opacity: 0; transition: none; }
.panel.fade-leave { opacity: 0; }`;
const index = `<!doctype html><html><head><meta charset="utf-8"><style>${css}</style><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const types = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (types[path]) { res.writeHead(200, { "content-type": types[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(200, { "content-type": "text/html" }); res.end(index);
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-transitions-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const watchdog = setTimeout(() => { console.log("TRANSITIONS CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000);
watchdog.unref();
let target;
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") errors.push("exception: " + (m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text)); };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;
const waitFor = async (expr, label) => { for (let i = 0; i < 200; i++) { if (await ev(expr).catch(() => false)) return; await sleep(50); } throw new Error(`timeout: ${label}`); };
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };

try {
  await send("Runtime.enable"); await send("Page.enable");
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  await waitFor(`!!document.getElementById('panel') && goowee.nodeCount() > 0`, "panel mounted");
  // Count handleEvent calls: a click on a leaving node must not reach Go.
  await ev(`(()=>{const h=window.handleEvent;window.goCalls=0;window.handleEvent=function(...a){window.goCalls++;return h.apply(this,a);};})()`);
  const before = await ev(`goowee.nodeCount()`);
  check(await ev(`document.getElementById('panel').getAttribute('data-goowee-transition')`) === "fade", "Transition item did not render its data attribute");

  // A MutationObserver records the leave class the moment it is added: a CDP
  // poll can miss a short-lived class on a busy machine.
  await ev(`(()=>{window.leaveSeen=null;
    window.leaveObserver=new MutationObserver(ms=>{for(const m of ms){
      if(m.type==='attributes'&&m.attributeName==='class'&&m.target.classList.contains('fade-leave'))
        window.leaveSeen={inert:m.target.hasAttribute('inert'),connected:m.target.isConnected};
    }});
    window.leaveObserver.observe(document.getElementById('root'),{subtree:true,attributes:true,attributeFilter:['class']});
  })()`);
  await ev(`document.getElementById('toggle').click()`);
  await waitFor(`!!window.leaveSeen`, "leave class applied while the node is still present");
  check(await ev(`window.leaveSeen.connected`), "leaving node disappeared before its animation ended");
  check(await ev(`window.leaveSeen.inert`), "leaving node is not inert");
  // The toggle's own click legitimately called into Go; reset before testing
  // the inert node.
  await ev(`window.goCalls = 0`);
  await ev(`document.getElementById('panel').click()`);
  check(await ev(`window.goCalls`) === 0, `a click on the inert leaving node called into Go ${await ev("window.goCalls")} times`);
  await ev(`window.leaveObserver.disconnect()`);

  await waitFor(`!document.getElementById('panel')`, "leaving node removed after the animation");
  const after = await ev(`goowee.nodeCount()`);
  check(after === before - 2, `nodeCount leak: before ${before}, after ${after} (want ${before - 2})`);

  // Re-show, then emulate reduced motion: removal must be immediate with no
  // leave class and no inert attribute.
  await ev(`document.getElementById('toggle').click()`);
  await waitFor(`!!document.getElementById('panel')`, "panel re-shown");
  await send("Emulation.setEmulatedMedia", { features: [{ name: "prefers-reduced-motion", value: "reduce" }] });
  await ev(`document.getElementById('toggle').click()`);
  await waitFor(`!document.getElementById('panel')`, "reduced-motion removal");
  check(await ev(`goowee.nodeCount()`) === before - 2, "reduced-motion removal leaked nodes");
  check(errors.length === 0, "console errors: " + errors.join("\n"));
} catch (e) {
  fail.push(String(e));
} finally {
  // The checks are over: never let cleanup time turn a pass into a failure.
  clearTimeout(watchdog);
  if (fail.length) { console.log("TRANSITIONS CHECK FAIL:\n" + fail.join("\n")); process.exitCode = 1; }
  else console.log("TRANSITIONS CHECK OK");
  ws.close(); server.close(); chrome.kill();
  // Chrome's child processes can keep writing to the profile after kill(), so
  // a lingering file must not turn a passing check into a red CI job.
  const rmOpts = { recursive: true, force: true, maxRetries: 2, retryDelay: 50 };
  for (const path of [dir, profile]) {
    try { rmSync(path, rmOpts); }
    catch (e) { console.log(`WARN: could not remove ${path}: ${e.code}`); }
  }
}
