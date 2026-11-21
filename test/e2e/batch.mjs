// Binary transport check: the runtime's binary decoder must see exactly what
// JSON.parse sees for the same batch. Go writes a corpus of batches in both
// encodings (TestWriteBatchCorpus in internal/dom: random batches over awkward
// strings and numbers, every sample value, a 5,000-update layout tick, an
// empty batch); this script loads runtime/goowee.js and compares every
// mutation field by field, with Object.is, so -0 and 0 or undefined and null
// differ. Each batch is decoded from a reused, larger buffer with stale bytes
// past its end, as the bridge sends it.
//
// No browser needed: node test/e2e/batch.mjs
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
const corpusPath = join(mkdtempSync(join(tmpdir(), "goowee-batch-")), "corpus.json");
execFileSync("go", ["test", "-count=1", "-run", "^TestWriteBatchCorpus$", "./internal/dom/"], {
  cwd: root, stdio: ["ignore", "ignore", "inherit"],
  env: { ...process.env, GOOWEE_BATCH_CORPUS: corpusPath },
});
const corpus = JSON.parse(readFileSync(corpusPath, "utf8"));

globalThis.window = globalThis;
vm.runInThisContext(readFileSync(join(root, "runtime/goowee.js"), "utf8"), { filename: "goowee.js" });
const decode = globalThis.goowee._decodeBatch;

const FIELDS = ["type", "nodeId", "key", "value", "childId", "refId", "ns"];
function same(a, b) {
  if (Object.is(a, b)) return true;
  if (typeof a !== "object" || typeof b !== "object" || a === null || b === null) return false;
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  const ka = Object.keys(a), kb = Object.keys(b);
  return ka.length === kb.length && ka.every((k) => Object.hasOwn(b, k) && same(a[k], b[k]));
}

let buf = new Uint8Array(0), muts = 0, failures = 0;
for (const [i, entry] of corpus.entries()) {
  const want = JSON.parse(entry.json);
  const bin = Buffer.from(entry.binary, "base64");
  if (buf.length < bin.length + 64) buf = new Uint8Array(2 * (bin.length + 64));
  buf.fill(0xab);
  buf.set(bin);
  const got = [];
  decode(buf, bin.length, (m) => got.push(Object.fromEntries(FIELDS.map((f) => [f, m[f]]))));
  const ok = got.length === want.length && got.every((m, j) => FIELDS.every((f) => same(m[f], want[j][f])));
  if (!ok && failures++ < 5) {
    const j = got.findIndex((m, j) => !want[j] || FIELDS.some((f) => !same(m[f], want[j][f])));
    console.error(`batch ${i}: binary decodes ${got.length} mutations, JSON ${want.length}; first difference at ${j}`);
    console.error("  binary:", got[j]);
    console.error("  json:  ", want[j]);
  }
  muts += want.length;
}
if (failures) {
  console.error(`batch: ${failures} of ${corpus.length} batches decode differently from JSON`);
  process.exit(1);
}
console.log(`batch: ${corpus.length} batches, ${muts} mutations: the binary decoder matches JSON.parse field for field`);
