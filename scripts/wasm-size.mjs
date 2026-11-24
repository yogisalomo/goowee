// wasm-size: what a Go wasm binary is made of, and whether it grew (#129).
//
//   node scripts/wasm-size.mjs app.wasm [more.wasm …]   report each file
//   node scripts/wasm-size.mjs --check docs/perf/size-budget.json name=app.wasm …
//
// For each file: raw, gzip -9 and brotli -q 11 sizes, the section totals,
// and the code bytes per Go package, read from the code section and the
// "name" custom section (present in default and -s -w builds). No
// dependencies: Node's zlib does the compression.
//
// --check reports each named program's brotli size against its reference in
// the budget file. Growth never fails the check: more than `tolerance` (3%)
// over the reference prints a warning (a CI annotation) so it is visible in
// review. The check fails only when a program links a package on its deny
// list, or more of a package's code than its maxCode allows: those enforce
// which packages client code may use, not a byte count.
import { readFileSync } from "node:fs";
import { gzipSync, brotliCompressSync, constants } from "node:zlib";

const SECTION = { 0: "custom", 1: "type", 2: "import", 3: "function", 4: "table", 5: "memory", 6: "global",
  7: "export", 8: "start", 9: "element", 10: "code", 11: "data", 12: "datacount" };

function reader(buf, pos = 0) {
  return {
    get pos() { return pos; },
    set pos(p) { pos = p; },
    u8() { return buf[pos++]; },
    u32() { // unsigned LEB128
      let result = 0, shift = 0, b;
      do { b = buf[pos++]; result += (b & 0x7f) * 2 ** shift; shift += 7; } while (b & 0x80);
      return result;
    },
    bytes(n) { const s = buf.subarray(pos, pos + n); pos += n; return s; },
    name() { const n = this.u32(); return new TextDecoder().decode(this.bytes(n)); },
  };
}

// parse returns the sections (id, name, size), the size of each function
// body in index order, the number of imported functions (they come first in
// the index space and have no body), and the function names.
export function parse(buf) {
  if (buf.readUInt32LE(0) !== 0x6d736100) throw new Error("not a wasm file");
  const r = reader(buf, 8);
  const sections = [], bodies = [], names = new Map();
  let imported = 0;
  while (r.pos < buf.length) {
    const id = r.u8(), size = r.u32(), start = r.pos, end = start + size;
    let label = SECTION[id] || "unknown";
    if (id === 0) {
      const name = r.name();
      label = "custom:" + name;
      if (name === "name") {
        while (r.pos < end) {
          const sub = r.u8(), subSize = r.u32(), subEnd = r.pos + subSize;
          if (sub === 1) { // function names
            for (let n = r.u32(); n > 0; n--) names.set(r.u32(), r.name());
          }
          r.pos = subEnd;
        }
      }
    } else if (id === 2) {
      for (let n = r.u32(); n > 0; n--) {
        r.name(); r.name();
        const kind = r.u8();
        if (kind === 0) { r.u32(); imported++; }
        else if (kind === 1) { r.u8(); const f = r.u8(); r.u32(); if (f & 1) r.u32(); }
        else if (kind === 2) { const f = r.u8(); r.u32(); if (f & 1) r.u32(); }
        else if (kind === 3) { r.u8(); r.u8(); }
      }
    } else if (id === 10) {
      for (let n = r.u32(); n > 0; n--) { const s = r.u32(); bodies.push(s); r.pos += s; }
    }
    sections.push({ id, label, size });
    r.pos = end;
  }
  return { sections, bodies, imported, names };
}

// pkgOf is the Go package a function belongs to. TinyGo writes names as Go
// spells them ("(*github.com/yogisalomo/goowee/core.Signal[int]).Set"). The Go linker writes names
// with "/" replaced by "_" ("github.com_yogisalomo_goowee_core.Batch",
// "internal_runtime_maps.(*Map).grow"), so the package is the text before the
// first "." that isn't in a leading domain, with "_" turned back into "/".
// (A package whose own name has "_" is shown with "/"; none in std does.)
// Compiler-generated "type_.eq.T" functions are "type:"; assembly helpers
// without a package ("memeqbody") are "(asm)".
export function pkgOf(fn) {
  // TinyGo keeps "/" and writes methods as "(*pkg.T).M" or "(pkg.T).M".
  fn = fn.replace(/^\(\*?/, "");
  const generic = fn.indexOf("[");
  const base = generic >= 0 ? fn.slice(0, generic) : fn;
  if (base.includes("/")) {
    const slash = base.lastIndexOf("/");
    const dot = base.indexOf(".", slash + 1);
    return dot < 0 ? base : base.slice(0, dot);
  }
  if (base.startsWith("type_.")) return "type:";
  const under = base.indexOf("_"), firstDot = base.indexOf(".");
  if (firstDot < 0) return "(asm)";
  let dot = firstDot;
  if (under > 0 && firstDot < under && /^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(base.slice(0, under))) {
    dot = base.indexOf(".", under); // skip the domain's dots: github.com_…
    if (dot < 0) return base.replaceAll("_", "/");
  }
  return base.slice(0, dot).replaceAll("_", "/");
}

export function measure(path) {
  const buf = readFileSync(path);
  const { sections, bodies, imported, names } = parse(buf);
  const pkgs = new Map();
  bodies.forEach((size, i) => {
    const pkg = pkgOf(names.get(imported + i) || "(unnamed)");
    pkgs.set(pkg, (pkgs.get(pkg) || 0) + size);
  });
  return {
    raw: buf.length,
    gzip: gzipSync(buf, { level: 9 }).length,
    brotli: brotliCompressSync(buf, { params: { [constants.BROTLI_PARAM_QUALITY]: 11, [constants.BROTLI_PARAM_SIZE_HINT]: buf.length } }).length,
    sections,
    code: bodies.reduce((a, b) => a + b, 0),
    pkgs: [...pkgs].sort((a, b) => b[1] - a[1]),
  };
}

const fmt = (n) => n.toLocaleString("en-US");
const kb = (n) => (n / 1024).toFixed(1).padStart(8) + " KB";

export function report(name, m, top = 25) {
  const out = [];
  out.push(`${name}: ${fmt(m.raw)} raw, ${fmt(m.gzip)} gzip -9, ${fmt(m.brotli)} brotli -q 11`);
  out.push("  sections: " + m.sections.filter((s) => s.size > 1024).map((s) => `${s.label} ${kb(s.size).trim()}`).join(", "));
  const goowee = m.pkgs.filter(([p]) => p.startsWith("github.com/yogisalomo/goowee/")).reduce((a, [, s]) => a + s, 0);
  out.push(`  code ${kb(m.code).trim()}, of which goowee ${kb(goowee).trim()} (${(100 * goowee / m.code).toFixed(1)}%)`);
  out.push(`  top ${Math.min(top, m.pkgs.length)} packages by code bytes:`);
  for (const [pkg, size] of m.pkgs.slice(0, top)) {
    out.push(`  ${kb(size)} ${(100 * size / m.code).toFixed(1).padStart(5)}%  ${pkg.replace("github.com/yogisalomo/goowee", "goowee")}`);
  }
  return out.join("\n");
}

function check(budgetPath, programs) {
  const budget = JSON.parse(readFileSync(budgetPath, "utf8"));
  const tolerance = budget.tolerance ?? 0.03;
  const problems = [];
  for (const [name, path] of programs) {
    const rec = budget.programs?.[name];
    const m = measure(path);
    console.log(report(name, m) + "\n");
    if (!rec) { console.log(`${name}: brotli ${fmt(m.brotli)} (no reference in ${budgetPath})`); continue; }
    const pct = (m.brotli - rec.brotli) / rec.brotli * 100;
    const delta = (Math.abs(pct) < 0.05 ? 0 : pct).toFixed(1);
    const line = `${name}: brotli ${fmt(m.brotli)} vs reference ${fmt(rec.brotli)} (${Number(delta) >= 0 ? "+" : ""}${delta}%)`;
    if (m.brotli > rec.brotli * (1 + tolerance)) {
      // Informational only: bundle size is not a hard limit.
      console.log((process.env.GITHUB_ACTIONS ? "::warning::" : "note: ") + line + `, more than ${tolerance * 100}% over; update "brotli" in ${budgetPath} if you want a new reference`);
    } else {
      console.log(line);
    }
    const code = new Map(m.pkgs);
    for (const pkg of rec.deny || []) {
      if (code.has(pkg)) problems.push(`${name}: links ${pkg}, which is on its deny list in ${budgetPath}`);
    }
    // maxCode caps a package's code bytes: a package whose functions an app
    // doesn't call can still be linked for its init when a linked package
    // imports it (encoding/json for hooks.Load and h.JSONLD, #126).
    for (const [pkg, max] of Object.entries(rec.maxCode || {})) {
      const n = code.get(pkg) || 0;
      if (n > max) problems.push(`${name}: links ${fmt(n)} bytes of ${pkg} code, over its cap of ${fmt(max)} in ${budgetPath} (a function of it is reachable, not just its init)`);
    }
  }
  if (problems.length) {
    for (const p of problems) console.log((process.env.GITHUB_ACTIONS ? "::error::" : "FAIL: ") + p);
    process.exit(1);
  }
  console.log("\nsize report: OK (deny lists hold)");
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const args = process.argv.slice(2);
  if (args[0] === "--check") {
    check(args[1], args.slice(2).map((a) => { const i = a.indexOf("="); return [a.slice(0, i), a.slice(i + 1)]; }));
  } else if (args.length) {
    for (const path of args) console.log(report(path, measure(path)) + "\n");
  } else {
    console.error("usage: node scripts/wasm-size.mjs app.wasm … | --check budget.json name=app.wasm …");
    process.exit(2);
  }
}
