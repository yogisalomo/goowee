// Drives test/e2e/net (goowee/net) in headless Chrome against this file's
// server: JSON endpoints and a WebSocket endpoint (a minimal RFC 6455 server,
// since Node has no WebSocket server built in). Checks Fetch (JSON both ways,
// headers, an error status, a binary body, abort on cancel), the socket
// (subprotocol, text and binary both ways, reconnection from a cursor after a
// drop, Close), and that updates still apply while the page is hidden, when
// the browser stops animation frames.
// Usage: node test/e2e/net.mjs   (run.sh runs it)
import { createServer } from "node:http";
import { createHash } from "node:crypto";
import { readFileSync, existsSync, mkdtempSync, copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn, execFileSync } from "node:child_process";

const PORT = Number(process.env.PORT || 8141), DP = Number(process.env.CDP_PORT || 9441);
const dir = mkdtempSync(join(tmpdir(), "goowee-net-"));
execFileSync("go", ["build", "-o", join(dir, "main.wasm"), "./test/e2e/net"], { env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
const goroot = execFileSync("go", ["env", "GOROOT"]).toString().trim();
copyFileSync(existsSync(join(goroot, "lib/wasm/wasm_exec.js")) ? join(goroot, "lib/wasm/wasm_exec.js") : join(goroot, "misc/wasm/wasm_exec.js"), join(dir, "wasm_exec.js"));
copyFileSync("runtime/goowee.js", join(dir, "goowee.js"));

function chromePath() {
  if (process.env.CHROME) return process.env.CHROME;
  const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
  if (existsSync(mac)) return mac;
  return "google-chrome"; // Linux/CI on PATH
}

// ---- HTTP ----
const seen = { echoHeader: null, echoType: null, slowStarted: false, slowAborted: false };
const index = `<!doctype html><html><head><meta charset="utf-8"><script src="/wasm_exec.js"></script><script src="/goowee.js"></script></head><body><div id="root"></div><script>goowee.boot()</script></body></html>`;
const files = { "/wasm_exec.js": "text/javascript", "/goowee.js": "text/javascript", "/main.wasm": "application/wasm" };
const server = createServer((req, res) => {
  const path = new URL(req.url, "http://x").pathname;
  if (path === "/") { res.writeHead(200, { "content-type": "text/html" }); return res.end(index); }
  if (files[path]) { res.writeHead(200, { "content-type": files[path] }); return res.end(readFileSync(join(dir, path))); }
  if (path === "/api/user") { res.writeHead(200, { "content-type": "application/json" }); return res.end(`{"Name":"Ada"}`); }
  if (path === "/api/missing") { res.writeHead(404, { "content-type": "application/json" }); return res.end(`{"error":"nope"}`); }
  if (path === "/api/bytes") { res.writeHead(200, { "content-type": "application/octet-stream" }); return res.end(Buffer.from([...Array(256).keys()])); }
  if (path === "/api/echo") {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      seen.echoHeader = req.headers["x-goowee"];
      res.writeHead(201, { "content-type": "application/json", "x-echo-method": req.method, "x-echo-type": req.headers["content-type"] });
      res.end(Buffer.concat(chunks));
    });
    return;
  }
  if (path === "/api/slow") {
    seen.slowStarted = true;
    const t = setTimeout(() => { res.writeHead(200); res.end("late"); }, 15000);
    res.on("close", () => { if (!res.writableEnded) { seen.slowAborted = true; clearTimeout(t); } });
    return;
  }
  res.writeHead(404); res.end();
});

// ---- WebSocket ----
const conns = [];
function wsSend(conn, op, data) {
  const head = data.length < 126 ? Buffer.from([0x80 | op, data.length])
    : Buffer.from([0x80 | op, 126, data.length >> 8, data.length & 0xff]);
  conn.socket.write(Buffer.concat([head, data]));
}
function onFrame(conn, op, data) {
  if (op === 1) { const t = data.toString(); conn.received.push(t); if (t === "ping") wsSend(conn, 1, Buffer.from("pong")); }
  else if (op === 2) { conn.received.push([...data]); wsSend(conn, 2, Buffer.from([...data].reverse())); }
  else if (op === 8) { conn.closeCode = data.length >= 2 ? data.readUInt16BE(0) : 1005; wsSend(conn, 8, data.subarray(0, 2)); conn.socket.end(); }
  else if (op === 9) wsSend(conn, 10, data);
}
server.on("upgrade", (req, socket) => {
  const url = new URL(req.url, "http://x");
  if (url.pathname !== "/ws") return socket.destroy();
  const accept = createHash("sha1").update(req.headers["sec-websocket-key"] + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").digest("base64");
  const protocols = (req.headers["sec-websocket-protocol"] || "").split(",").map((s) => s.trim()).filter(Boolean);
  socket.write(["HTTP/1.1 101 Switching Protocols", "Upgrade: websocket", "Connection: Upgrade", `Sec-WebSocket-Accept: ${accept}`,
    ...(protocols.includes("goowee-e2e") ? ["Sec-WebSocket-Protocol: goowee-e2e"] : [])].join("\r\n") + "\r\n\r\n");
  const conn = { socket, since: url.searchParams.get("since"), protocols, received: [], closeCode: null };
  conns.push(conn);
  socket.on("error", () => {});
  let buf = Buffer.alloc(0);
  socket.on("data", (chunk) => {
    buf = Buffer.concat([buf, chunk]);
    while (buf.length >= 2) {
      let len = buf[1] & 0x7f, off = 2;
      if (len === 126) { if (buf.length < 4) return; len = buf.readUInt16BE(2); off = 4; }
      else if (len === 127) { if (buf.length < 10) return; len = Number(buf.readBigUInt64BE(2)); off = 10; }
      const mask = buf[1] & 0x80 ? buf.subarray(off, off + 4) : null;
      if (mask) off += 4;
      if (buf.length < off + len) return;
      const data = Buffer.from(buf.subarray(off, off + len));
      if (mask) for (let i = 0; i < len; i++) data[i] ^= mask[i & 3];
      const op = buf[0] & 0x0f;
      buf = buf.subarray(off + len);
      onFrame(conn, op, data);
    }
  });
  wsSend(conn, 1, Buffer.from(`hello since=${conn.since}`));
});
server.listen(PORT);

// ---- Chrome ----
const chrome = spawn(chromePath(), [
  "--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
  ...(process.env.CHROME_NO_SANDBOX === "0" ? [] : ["--no-sandbox"]),
  `--remote-debugging-port=${DP}`, `--user-data-dir=${mkdtempSync(join(tmpdir(), "goowee-net-chrome-"))}`, "about:blank",
], { stdio: "ignore" });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// A CDP reply that never comes (a crashed page, say) must fail the run, not hang it.
setTimeout(() => { console.log("NET CHECK FAIL: timed out after 3 minutes"); chrome.kill(); process.exit(1); }, 180000).unref();
let target; // up to 20s: Chrome can take ~8s to open its debug port
for (let i = 0; i < 200 && !target; i++) { try { target = (await (await fetch(`http://localhost:${DP}/json`)).json()).find((t) => t.type === "page"); } catch {} if (!target) await sleep(100); }
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => (ws.onopen = r));
let id = 0; const pending = new Map(); const errors = [];
ws.onmessage = (ev) => { const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") errors.push(m.params.args.map((a) => a.value ?? a.description).join(" "));
  if (m.method === "Runtime.exceptionThrown") errors.push("exception: " + m.params.exceptionDetails.text); };
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })); });
const ev = async (expr) => (await send("Runtime.evaluate", { expression: expr, returnByValue: true, awaitPromise: true })).result.result.value;
const text = (sel) => ev(`document.querySelector(${JSON.stringify(sel)})?.textContent`);
const waitFor = async (cond, label) => { for (let i = 0; i < 100; i++) { if (await cond()) return; await sleep(50); } throw new Error(`timeout: ${label} (status ${await text("#status")}, msgs ${await text("#msgs")})`); };
const waitText = (sel, want, label) => waitFor(async () => { const t = await text(sel); return typeof want === "string" ? t === want : want.test(t ?? ""); }, label || `${sel} = ${want}`);
const click = (sel) => ev(`document.querySelector(${JSON.stringify(sel)}).click()`);
const fail = []; const check = (c, m) => { if (!c) fail.push(m); };
await send("Runtime.enable"); await send("Page.enable");
try {
  await send("Page.navigate", { url: `http://localhost:${PORT}/` });
  // Fetch.
  await waitText("#user", "user: Ada", "UseResource + Fetch + JSON");
  await click("#post");
  await waitText("#echo", `201 POST application/json {"n":1,"s":"héllo"}`, "JSON POST echo");
  check(seen.echoHeader === "e2e", `custom header reached the server as ${seen.echoHeader}`);
  await click("#missing");
  await waitText("#missing-out", /^ok=false goowee\/net: http:\/\/localhost:\d+\/api\/missing: 404 Not Found$/, "404 as StatusError");
  await click("#bytes");
  await waitText("#bytes-out", "bytes 256 sum 32640", "binary body");
  await click("#slow");
  await waitFor(() => seen.slowStarted, "slow request reached the server");
  await click("#cancel");
  await waitText("#slow-out", "error: context canceled", "cancelled Fetch returns ctx.Err()");
  await waitFor(() => seen.slowAborted, "the cancelled request was aborted on the wire");

  // Socket.
  await waitText("#status", "open", "socket open");
  await waitText("#msgs", "hello since=0");
  check(conns.length === 1 && conns[0].protocols.join() === "goowee-e2e", `subprotocol: ${conns[0]?.protocols}`);
  await click("#send");
  await waitText("#msgs", "hello since=0|pong|bin[3 2 1]", "text and binary round trip");
  check(JSON.stringify(conns[0].received) === `["ping",[1,2,3]]`, `server received ${JSON.stringify(conns[0].received)}`);

  // A hidden page gets no animation frames; updates must still apply.
  const hide = `Object.defineProperty(document, "hidden", {configurable: true, get: () => true})`;
  const show = `delete document.hidden; window.requestAnimationFrame = window.__raf`;
  await ev(`window.__raf = window.requestAnimationFrame; window.requestAnimationFrame = () => 0; ${hide}`);
  wsSend(conns[0], 1, Buffer.from("while-hidden"));
  await waitText("#msgs", /\|while-hidden$/, "message applied while the page is hidden");
  // A frame requested while visible, then the page hides before it runs.
  await ev(`delete document.hidden`);
  wsSend(conns[0], 1, Buffer.from("before-hide"));
  await sleep(200);
  check(!/before-hide/.test(await text("#msgs")), "frames weren't actually stopped (the stub didn't take)");
  await ev(`${hide}; document.dispatchEvent(new Event("visibilitychange"))`);
  await waitText("#msgs", /\|before-hide$/, "frame requested before hiding runs once hidden");
  await ev(show);

  // The server drops the connection; the app reconnects from its cursor.
  conns[0].socket.destroy();
  await waitFor(() => conns.length === 2, "reconnection");
  check(conns[1].since === "5", `reconnected with since=${conns[1].since}, want 5 (messages seen)`);
  await waitText("#msgs", /\|hello since=5$/, "messages flow after reconnecting");
  await waitText("#status", "open", "open again");
  check(/^1006:true $/.test(await text("#closes")), `OnClose events: ${await text("#closes")}`);

  // Close: a 1000 close frame, no OnClose, no reconnection.
  await click("#close");
  await waitFor(() => conns[1].closeCode === 1000, "close frame 1000");
  await sleep(500);
  check(conns.length === 2, `reconnected after Close (${conns.length} connections)`);
  check(await text("#status") === "closed by app" && await text("#closes") === "1006:true ", `after Close: status ${await text("#status")}, closes ${await text("#closes")}`);
} catch (e) { fail.push(e.message); }
check(errors.length === 0, "console errors:\n" + errors.join("\n"));
console.log(fail.length ? "NET CHECK FAIL:\n- " + fail.join("\n- ") : "NET CHECK PASS: fetch (JSON, headers, 404, bytes, abort), socket (subprotocol, text+binary, reconnect from cursor, close), updates while hidden");
ws.close(); chrome.kill(); server.close(); rmSync(dir, { recursive: true, force: true });
process.exit(fail.length ? 1 : 0);
