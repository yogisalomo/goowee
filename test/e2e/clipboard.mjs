// Drives test/e2e/clipboard in headless Chrome (#106): paste and drop with
// files and text through the real runtime (payloads, file handles,
// File.Bytes), the OnPaste/OnCopy helpers, ref.Call placing the caret after
// the value it positions (focused or not), and a throwing ref.Call not
// stopping its frame. Events are synthetic ClipboardEvent/DragEvent objects
// carrying a constructed DataTransfer, which the page handles like real ones.
// Usage: node test/e2e/clipboard.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8143), DP = Number(process.env.CDP_PORT || 9443);
const dir = mkdtempSync(join(tmpdir(), "goowee-clipboard-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/clipboard"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}

const index = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const files = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (files[path]) { res.writeHead(200, { "content-type": files[path] }); return res.end(readFileSync(join(dir, path))); }
  res.writeHead(404); res.end();
}).listen(PORT);

const profile = mkdtempSync(join(tmpdir(), "goowee-clipboard-chrome-"));
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${profile}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("CLIPBOARD CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
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
const ev = async (expr) => { const r = (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result; if (r.exceptionDetails) throw new Error(`${expr}: ${r.exceptionDetails.exception?.description}`); return r.result.value; };
const text = (sel) => ev(`document.querySelector(${JSON.stringify(sel)})?.textContent`);
const waitText = async (sel, want, label) => { let t; for (let i = 0; i < 100; i++) { t = await text(sel); if (typeof want === "string" ? t === want : want.test(t ?? "")) return; await sleep(50); } throw new Error(`timeout: ${label || sel} — got ${JSON.stringify(t)}, want ${want}`); };
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");

// In-page helpers: a DataTransfer with files ([name, type, bytes]) and text,
// dispatched as a paste/copy or a drag event; each returns defaultPrevented.
const helpers = `
window.dt = (files, text) => { const d = new DataTransfer();
  for (const [name, type, bytes] of files) d.items.add(new File([new Uint8Array(bytes)], name, {type}));
  if (text) d.setData("text/plain", text); return d; };
window.paste = (sel, files, text) => { const e = new ClipboardEvent("paste", {clipboardData: dt(files, text), bubbles: true, cancelable: true});
  document.querySelector(sel).dispatchEvent(e); return e.defaultPrevented; };
window.drag = (type, sel, files, text) => { const e = new DragEvent(type, {dataTransfer: dt(files, text), bubbles: true, cancelable: true, clientX: 10, clientY: 20});
  document.querySelector(sel).dispatchEvent(e); return e.defaultPrevented; };
window.type = (sel, value) => { const el = document.querySelector(sel); el.value = value; el.dispatchEvent(new InputEvent("input", {bubbles: true})); };
`;
try {
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  for (let i = 0; i < 200 && !(await ev(`!!document.querySelector("#box")`)); i++) await sleep(50);
  await ev(helpers);

  // Paste: an image is taken over by the app; text goes through.
  check(await ev(`paste("#box", [["shot.png", "image/png", [1, 2, 3, 250]]], "")`) === true, "image paste not prevented");
  await waitText("#files", "shot.png image/png 4 256 <nil>", "pasted image read through File.Bytes");
  check(await text("#paste") === "text: types:Files files:1", `paste payload: ${await text("#paste")}`);
  check(await ev(`paste("#box", [], "hello")`) === false, "text paste prevented");
  await waitText("#paste", "text:hello types:text/plain files:0", "text paste payload");
  await ev(`paste("#plain", [], "pasted words")`);
  await waitText("#plain-out", "pasted words", "OnPaste gets the pasted text");

  // Copy hands OnCopy the selection.
  await ev(`type("#box", "copy me please"); document.querySelector("#box").setSelectionRange(5, 7)`);
  await ev(`document.querySelector("#box").dispatchEvent(new ClipboardEvent("copy", {bubbles: true, cancelable: true}))`);
  await waitText("#copied", "me", "OnCopy gets the selection");

  // ref.Call after the value it positions: unfocused (a write moves the caret
  // to the end) and focused (a write restores the old caret).
  await ev(`document.querySelector("#box").blur(); type("#box", "hi 🎉 @a there")`);
  await ev(`document.querySelector("#complete").click()`);
  for (let i = 0; i < 100 && (await ev(`document.querySelector("#box").value`)) !== "hi 🎉 @ada  there"; i++) await sleep(50);
  const caret1 = await ev(`[document.querySelector("#box").selectionStart, document.querySelector("#box").selectionEnd].join()`);
  check(caret1 === "11,11", `unfocused: caret at ${caret1}, want 11,11`);
  await ev(`const b = document.querySelector("#box"); b.focus(); type("#box", "yo @a"); b.setSelectionRange(0, 0)`);
  await ev(`document.querySelector("#complete").click()`);
  for (let i = 0; i < 100 && (await ev(`document.querySelector("#box").value`)) !== "yo @ada "; i++) await sleep(50);
  const caret2 = await ev(`[document.querySelector("#box").selectionStart, document.querySelector("#box").selectionEnd].join()`);
  check(caret2 === "8,8", `focused: caret at ${caret2}, want 8,8`);

  // A throwing call is logged and doesn't stop the frame.
  await ev(`document.querySelector("#bad").click()`);
  await waitText("#status", "after bad call", "frame applied despite a throwing ref.Call");
  const callErrors = errors.filter((e) => e.startsWith("goowee: ref.Call setSelectionRange"));
  check(callErrors.length === 1, `ref.Call error logs: ${JSON.stringify(errors)}`);
  errors.splice(0, errors.length, ...errors.filter((e) => !callErrors.includes(e)));

  // Drag and drop: types (no files) on dragover, files and text on drop.
  check(await ev(`drag("dragover", "#target", [["a.txt", "text/plain", [104, 101, 108, 108, 111]]], "")`) === true, "dragover with files not accepted");
  await waitText("#over", "Files files:0", "dragover payload");
  check(await ev(`drag("dragover", "#target", [], "words")`) === false, "dragover with text accepted");
  check(await ev(`drag("drop", "#target", [["a.txt", "text/plain", [104, 101, 108, 108, 111]]], "note")`) === true, "drop not prevented");
  await waitText("#files", /\|a\.txt text\/plain 5 532 <nil>$/, "dropped file read through File.Bytes");
  check(await text("#dropped") === "text:note at:true,true", `drop payload: ${await text("#dropped")}`);
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "CLIPBOARD CHECK FAIL:\n- " + fail.join("\n- ") : "CLIPBOARD CHECK PASS: paste files+text, OnPaste/OnCopy, drop zone, ref.Call caret (focused + unfocused), throwing call");
ws.close(); server.close(); rmSync(dir, { recursive: true, force: true });
// Delete the Chrome profile (tens of MB) once Chrome has exited.
await new Promise((r) => { chrome.once("exit", r); chrome.kill(); setTimeout(r, 5000).unref(); });
try { rmSync(profile, { recursive: true, force: true, maxRetries: 3 }); } catch {}
process.exit(fail.length ? 1 : 0);
