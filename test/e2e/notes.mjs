// Drives examples/notes in headless Chrome (#151): a list, a detail page and a
// form talking to their server through goowee/rpc, server-rendered and
// hydrated. Typed calls load the list, a note and a filtered list; a form
// error comes back as rpc.Error fields; a saved note navigates to its page; a
// missing note shows the server's not-found error.
// Usage: node test/e2e/notes.mjs   (run.sh runs it)
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8159), DP = Number(process.env.CDP_PORT || 9459);
const dir = mkdtempSync(join(tmpdir(), "goowee-notes-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./examples/notes"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
execFileSync("go", ["build", "-o", join(dir, "server"), "./examples/notes/server"], { stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));
const server = spawn(join(dir, "server"), ["-addr", `localhost:${PORT}`, "-static", dir], { stdio: "ignore" });
const base = `http://localhost:${PORT}`;
for (let i = 0; i < 100; i++) { try { if ((await fetch(base + "/")).ok) break; } catch {} await new Promise((r) => setTimeout(r, 100)); }

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}
const profile = mkdtempSync(join(tmpdir(), "goowee-notes-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("NOTES CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target; // up to 20s: Chrome can take ~8s to open its debug port
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
const text = (sel) => ev(`document.querySelector(${JSON.stringify(sel)})?.textContent`);
const waitText = async (sel, want, label) => { let t; for (let i = 0; i < 100; i++) { t = await text(sel); if (typeof want === "string" ? t === want : want.test(t ?? "")) return; await sleep(50); } throw new Error(`timeout: ${label || sel} — got ${JSON.stringify(t)}, want ${want}`); };
const type = (value) => ev(`(() => { const el = document.querySelector("#draft"); el.value = ${JSON.stringify(value)}; el.dispatchEvent(new InputEvent("input", {bubbles: true})); })()`);
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
const click = (sel) => ev(`document.querySelector(${JSON.stringify(sel)}).click()`);
const clickLink = (t) => ev(`[...document.querySelectorAll("a")].find((a) => a.textContent === ${JSON.stringify(t)}).click()`);
const mismatches = () => errors.filter((e) => /hydration mismatch|no server node/.test(e));
await send("Runtime.enable"); await send("Page.enable");
try {
  // Server-rendered: the page's markup arrives with the HTML.
  const html = await (await fetch(base + "/")).text();
  check(/<h1[^>]*>.*Notes.*<\/h1>/.test(html) && html.includes("<!--goowee-->"), "/ isn't server-rendered");

  await send("Page.navigate", { url: base + "/" });
  await waitText("#notes", /Groceries.*Ideas/s, "the list loads through rpc");
  await clickLink("Groceries");
  await waitText("#note", /Milk, eggs, bread\./, "a note loads by id");

  await ev(`document.querySelector("#new-link").click()`);
  await waitText("h1", "New note", "the form");
  const typeInto = (sel, value) => ev(`(() => { const t = document.querySelector(${JSON.stringify(sel)}); t.value = ${JSON.stringify(value)}; t.dispatchEvent(new InputEvent("input", {bubbles: true})); })()`);
  // The form runs api.AddReq.Validate in the browser (goowee/form, #150)...
  await click("button[type=submit]");
  await waitText("#title-error", "Give the note a title.", "the form's own check, before any request");
  // ...and shows the server's field errors (rpc.Invalid) on their fields.
  await typeInto("#title", "Groceries");
  await waitText("#title-error", "", "the error clears as the title is fixed");
  await click("button[type=submit]");
  await waitText("#title-error", "A note with this title already exists.", "a field error from rpc.Invalid");
  await typeInto("#title", "Plan");
  await typeInto("#body", "Ship #151.");
  await click("button[type=submit]");
  await waitText("#note", /Plan.*Ship #151\./s, "saving navigates to the new note");
  check(await ev(`location.pathname`) === "/notes/3", "the new note's URL: " + await ev(`location.pathname`));

  await clickLink("Notes");
  await waitText("#notes", /Groceries.*Ideas.*Plan/s, "the list has the new note");
  await ev(`(() => { const s = document.querySelector("#search"); s.value = "gro"; s.dispatchEvent(new InputEvent("input", {bubbles: true})); })()`);
  await waitText("#notes", "Groceries", "the filter refetches with the new request");

  await send("Page.navigate", { url: base + "/notes/99" });
  await waitText("#note-error", "no note 99", "a server not-found error reaches the page");
} catch (e) { fail.push(e.message); }
check(mismatches().length === 0, "hydration mismatches:\n" + mismatches().join("\n"));
check(errors.filter((e) => !/404|Not Found/.test(e)).length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "NOTES CHECK FAIL:\n- " + fail.join("\n- ") : "NOTES CHECK PASS: SSR + hydration, list/detail/filter through goowee/rpc, form field errors, save and navigate, not-found");
ws.close(); server.kill(); rmSync(dir, { recursive: true, force: true });
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
