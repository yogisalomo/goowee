// `goowee new` + `goowee dev` in headless Chrome (#145): the new project's
// page boots with dev mode on, its button works, and an edit to the Go code
// rebuilds and reloads the page with the change; a broken edit shows the
// build error over the page.
import { spawn, execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const PORT = Number(process.env.PORT || 8153), DP = Number(process.env.CDP_PORT || 9453);
const root = process.cwd();
const work = mkdtempSync(join(tmpdir(), "goowee-cli-"));
const bin = join(work, "goowee");
execFileSync("go", ["build", "-o", bin, "./cmd/goowee"], { stdio: "inherit" });
execFileSync(bin, ["new", join(work, "demo"), "--goowee", root], { stdio: "inherit" });
const dev = spawn(bin, ["dev", "--addr", `127.0.0.1:${PORT}`], { cwd: join(work, "demo"), stdio: ["ignore", "inherit", "inherit"] });

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome";
}
const profile = mkdtempSync(join(tmpdir(), "goowee-cli-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
setTimeout(() => { console.log("CLI CHECK FAIL: timed out after 4 minutes"); cleanup(); process.exit(1); }, 240000).unref();
let target;
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map();
ws.onmessage = (ev) => { const m = JSON.parse(ev.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result?.result?.value;
const waitFor = async (expr, label, tries = 600) => { for (let i = 0; i < tries; i++) { if (await ev(expr).catch(() => false)) return; await sleep(100); } throw new Error("timeout: " + label); };
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
function cleanup() {
  try { ws.close(); } catch {}
  dev.kill(); chrome.kill();
  try { rmSync(work, { recursive: true, force: true }); } catch {}
}

try {
  await send("Runtime.enable"); await send("Page.enable");
  // The first build runs as dev starts: wait for the server.
  for (let i = 0; i < 1200; i++) { try { if ((await fetch(`http://127.0.0.1:${PORT}/main.wasm`)).ok) break; } catch {} await sleep(100); }
  await send("Page.navigate", { url: `http://127.0.0.1:${PORT}/` });
  await waitFor(`document.querySelector("#count")?.textContent === "Clicked 0 times"`, "the app booted");
  check(await ev(`goowee.devEnabled()`) === true, "dev mode isn't on under goowee dev");
  await ev(`document.querySelector("#count").click()`);
  await waitFor(`document.querySelector("#count").textContent === "Clicked 1 times"`, "the button counts");

  await ev(`window.__beforeReload = true`);
  const app = join(work, "demo", "app", "app.go");
  const src = readFileSync(app, "utf8");
  writeFileSync(app, src.replace(`"Clicked " + strconv.Itoa`, `"Pressed " + strconv.Itoa`));
  await waitFor(`!window.__beforeReload && document.querySelector("#count")?.textContent === "Pressed 0 times"`, "the edit reloaded the page");

  writeFileSync(app, src + "\nfunc broken( {\n");
  await waitFor(`(document.querySelector("#goowee-build-error")?.textContent || "").includes("app.go")`, "the build error shows over the page");
  writeFileSync(app, src);
  await waitFor(`!document.querySelector("#goowee-build-error") && document.querySelector("#count")?.textContent === "Clicked 0 times"`, "the fix reloaded the page");
} catch (e) { fail.push(e.message); }
console.log(fail.length ? "CLI CHECK FAIL:\n- " + fail.join("\n- ") : "CLI CHECK PASS: goowee new + goowee dev: boots in dev mode, reloads on an edit, shows a build error over the page");
cleanup();
await sleep(500);
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
