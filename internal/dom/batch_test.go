package dom

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yogisalomo/goowee/core"
)

// batchSamples extends encodeSamples with the corners of the binary format:
// a leading byte order mark (which a default TextDecoder would strip), every
// kind of invalid UTF-8, characters outside the BMP, integers around the
// varint and double limits, and values that go through json.Marshal.
var batchSamples = append(append([]any{}, encodeSamples...),
	"\xef\xbb\xbfleading BOM", "\xed\xa0\x80 encoded surrogate", "\xe2\x82a truncated", "\xc0\xaf overlong",
	"\xef\xbf\xbd literal replacement", "a\xf0\x9f\x98\x80b", "\xf0\x9f", strings.Repeat("long ascii ", 50),
	int64(maxVarintInt-1), int64(-maxVarintInt+1), int64(maxVarintInt), int64(-maxVarintInt),
	int64(1<<53), int64(1<<53+1), int64(-(1<<53)-1), uint64(1<<53+1), uint(1<<60),
	int8(-3), uint8(200), uint16(65535), uint32(4e9),
	json.Number("12.50"), (*int)(nil), float32(math.Copysign(0, -1)), struct {
		A int `json:"a"`
	}{1},
)

var batchStrings = func() (out []string) {
	for _, v := range batchSamples {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}()

func randomBatchMutation(rng *rand.Rand) core.Mutation {
	m := randomMutation(rng)
	m.Value = batchSamples[rng.Intn(len(batchSamples))]
	if rng.Intn(4) == 0 {
		m.Key = batchStrings[rng.Intn(len(batchStrings))]
	}
	if rng.Intn(8) == 0 {
		m.NS = batchStrings[rng.Intn(len(batchStrings))]
	}
	if rng.Intn(8) == 0 {
		m.NodeID = -2 // portalParent
	}
	return m
}

// jsonView decodes a JSON batch into what the runtime's JSON.parse sees: an
// object per mutation, without the fields omitempty left out.
func jsonView(t testing.TB, data []byte) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", data, err)
	}
	return out
}

// binaryView decodes a binary batch as decodeBatch in runtime/goowee.js does:
// the string section is decoded once, from UTF-8 to UTF-16, and each string is
// sliced out of it by its UTF-16 length. It fails on anything the runtime
// could misread: an invalid string section, a varint a double can't hold, a
// string past the section, or unread bytes or strings.
func binaryView(t testing.TB, data []byte) []map[string]any {
	t.Helper()
	if len(data) < 4 {
		t.Fatalf("batch of %d bytes has no header", len(data))
	}
	opsEnd := 4 + int(binary.LittleEndian.Uint32(data))
	if opsEnd > len(data) {
		t.Fatalf("op section ends at %d, past the batch (%d bytes)", opsEnd, len(data))
	}
	section := data[opsEnd:]
	if !utf8.Valid(section) {
		t.Fatalf("string section is not valid UTF-8: %q", section)
	}
	units := utf16.Encode([]rune(string(section)))
	p, sp := 4, 0
	uv := func() uint64 {
		v, n := binary.Uvarint(data[p:opsEnd])
		if n <= 0 {
			t.Fatalf("bad uvarint at %d", p)
		}
		if v >= 1<<53 {
			t.Fatalf("varint %d at %d is past what a double holds exactly", v, p)
		}
		p += n
		return v
	}
	sv := func() float64 {
		u := uv()
		return float64(int64(u>>1) ^ -int64(u&1))
	}
	str := func() string {
		n := int(uv())
		if sp+n > len(units) {
			t.Fatalf("string of %d units at %d runs past the string section (%d units)", n, sp, len(units))
		}
		s := string(utf16.Decode(units[sp : sp+n]))
		sp += n
		return s
	}
	out := []map[string]any{}
	for p < opsEnd {
		if p+2 > opsEnd {
			t.Fatalf("truncated op at %d", p)
		}
		typ, flags := data[p], data[p+1]
		p += 2
		m := map[string]any{"type": float64(typ), "nodeId": sv()}
		if flags&batchKey != 0 {
			m["key"] = str()
		}
		switch kind := flags >> batchValueShift & 7; kind {
		case valueNone:
		case valueString:
			m["value"] = str()
		case valueInt:
			m["value"] = sv()
		case valueFloat:
			if p+8 > opsEnd {
				t.Fatalf("truncated float at %d", p)
			}
			m["value"] = math.Float64frombits(binary.LittleEndian.Uint64(data[p:]))
			p += 8
		case valueFalse:
			m["value"] = false
		case valueTrue:
			m["value"] = true
		case valueJSON:
			var v any
			if err := json.Unmarshal([]byte(str()), &v); err != nil {
				t.Fatalf("JSON value: %v", err)
			}
			m["value"] = v
		default:
			t.Fatalf("unknown value kind %d at %d", kind, p)
		}
		if flags&batchChild != 0 {
			m["childId"] = sv()
		}
		if flags&batchRef != 0 {
			m["refId"] = sv()
		}
		if flags&batchNS != 0 {
			m["ns"] = str()
		}
		out = append(out, m)
	}
	if sp != len(units) {
		t.Fatalf("%d UTF-16 units of the string section were not read", len(units)-sp)
	}
	return out
}

// sameJS reports whether two decoded values are the same to JavaScript: equal
// strings and booleans, doubles with the same bits (so -0 and 0 differ), and
// arrays and objects whose elements are.
func sameJS(a, b any) bool {
	switch a := a.(type) {
	case float64:
		b, ok := b.(float64)
		return ok && math.Float64bits(a) == math.Float64bits(b)
	case []any:
		b, ok := b.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !sameJS(a[i], b[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		b, ok := b.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for k, v := range a {
			if w, ok := b[k]; !ok || !sameJS(v, w) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func sameView(a, b []map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameJS(a[i], b[i]) {
			return false
		}
	}
	return true
}

// checkBatch encodes batch both ways and requires the runtime to see the same
// mutations from the binary batch as from the JSON one, with the same ones
// dropped.
func checkBatch(t testing.TB, e *BatchEncoder, batch []core.Mutation) {
	t.Helper()
	jsonData, jsonDropped := EncodeBatch(batch)
	data, dropped := e.Encode(batch)
	if len(dropped) != len(jsonDropped) {
		t.Fatalf("dropped %d mutations, EncodeBatch dropped %d\nbatch: %#v", len(dropped), len(jsonDropped), batch)
	}
	if got, want := binaryView(t, data), jsonView(t, jsonData); !sameView(got, want) {
		t.Fatalf("binary batch decodes differently from JSON\n got: %#v\nwant: %#v\nbatch: %#v", got, want, batch)
	}
}

// The runtime must see exactly what JSON.parse gave it before: the binary
// format replaces the encoding, not the values.
// laterTypesBatch has the mutation types added after the binary format:
// ref.Call with its arguments (carried as JSON text), Listen on and off, and a
// head title moved by a portal.
var laterTypesBatch = []core.Mutation{
	{Type: core.MutCall, NodeID: 3, Key: "setSelectionRange", Value: []any{2, 5, "backward"}},
	{Type: core.MutCall, NodeID: 3, Key: "scrollIntoView", Value: []any{map[string]any{"block": "nearest"}}},
	{Type: core.MutCall, NodeID: 3, Key: "showModal", Value: []any{}},
	{Type: core.MutListen, NodeID: 4, Key: "click", Value: true},
	{Type: core.MutListen, NodeID: 4, Key: "goowee:canvasresize", Value: false},
	{Type: core.MutCreateElement, NodeID: 5, Key: "tag", Value: "title"},
	{Type: core.MutSetProperty, NodeID: 5, Key: "textContent", Value: "Inbox (3) · goowee"},
	{Type: core.MutPortalAppend, NodeID: 0, ChildID: 5, Value: "head"},
}

func TestBatchEncoderMatchesJSON(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var e BatchEncoder
	for i := 0; i < 3000; i++ {
		batch := make([]core.Mutation, rng.Intn(8))
		for j := range batch {
			batch[j] = randomBatchMutation(rng)
		}
		checkBatch(t, &e, batch)
	}
	for _, v := range batchSamples {
		checkBatch(t, &e, []core.Mutation{
			{Type: core.MutSetProperty, NodeID: 7, Key: "value", Value: v},
			{Type: core.MutSetAttribute, NodeID: 8, Key: "data-x", Value: v},
		})
	}
	checkBatch(t, &e, laterTypesBatch)
}

func TestBatchEncoderEmpty(t *testing.T) {
	var e BatchEncoder
	for _, batch := range [][]core.Mutation{nil, {}} {
		data, dropped := e.Encode(batch)
		if string(data) != "\x00\x00\x00\x00" || dropped != nil {
			t.Fatalf("Encode(%v) = %q (dropped %v), want an empty header", batch, data, dropped)
		}
	}
}

// Values JSON can't carry are dropped one mutation at a time, as EncodeBatch
// drops them, and leave nothing behind in either section.
func TestBatchEncoderDrops(t *testing.T) {
	ok := core.Mutation{Type: core.MutSetAttribute, NodeID: 1, Key: "x", Value: "kept é"}
	var e BatchEncoder
	for _, bad := range []any{math.NaN(), math.Inf(1), math.Inf(-1), float32(math.Inf(-1)), make(chan int)} {
		b := core.Mutation{Type: core.MutSetProperty, NodeID: 2, Key: "dropped é", Value: bad, NS: core.SVGNamespace}
		for _, batch := range [][]core.Mutation{{b, ok}, {ok, b}, {b, b}, {ok, b, ok}} {
			checkBatch(t, &e, batch)
			if _, dropped := e.Encode(batch); len(dropped) == 0 {
				t.Fatalf("%v was not dropped", bad)
			}
		}
	}
	if _, dropped := e.Encode([]core.Mutation{{Type: 300, NodeID: 1}, {Type: -1, NodeID: 1}}); len(dropped) != 2 {
		t.Fatalf("mutation types outside a byte were not dropped")
	}
}

// Reusing the encoder's buffers never leaks one batch into the next.
func TestBatchEncoderReuse(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var reused BatchEncoder
	for i := 0; i < 200; i++ {
		batch := make([]core.Mutation, rng.Intn(40))
		for j := range batch {
			batch[j] = randomBatchMutation(rng)
		}
		var fresh BatchEncoder
		want, _ := fresh.Encode(batch)
		if got, _ := reused.Encode(batch); string(got) != string(want) {
			t.Fatalf("batch %d: a reused encoder wrote %q, a fresh one %q", i, got, want)
		}
	}
}

// One huge batch doesn't pin its buffer once batches are small again.
func TestBatchEncoderShrinks(t *testing.T) {
	var e BatchEncoder
	big := make([]core.Mutation, 100_000)
	for i := range big {
		big[i] = core.Mutation{Type: core.MutSetAttribute, NodeID: i + 1, Key: "data-index", Value: "some value"}
	}
	e.Encode(big)
	if cap(e.buf) <= shrinkAbove {
		t.Fatalf("a 100,000-mutation batch left a %d-byte buffer; the test needs one past %d", cap(e.buf), shrinkAbove)
	}
	small := big[:10]
	for i := 0; i < shrinkAfter-1; i++ {
		e.Encode(small)
	}
	if cap(e.buf) <= shrinkAbove {
		t.Fatalf("the buffer shrank after %d small batches, before %d", shrinkAfter-1, shrinkAfter)
	}
	e.Encode(small)
	e.Encode(small)
	if cap(e.buf) > shrinkAbove {
		t.Fatalf("buffer still %d bytes after %d small batches", cap(e.buf), shrinkAfter+1)
	}
	checkBatch(t, &e, small)
}

func FuzzBatchString(f *testing.F) {
	for _, s := range batchSamples {
		if s, ok := s.(string); ok {
			f.Add(s)
		}
	}
	var e BatchEncoder
	f.Fuzz(func(t *testing.T, s string) {
		checkBatch(t, &e, []core.Mutation{
			{Type: core.MutSetAttribute, NodeID: 1, Key: s, Value: s, NS: s},
			{Type: core.MutSetProperty, NodeID: 2, Key: "k", Value: s},
		})
	})
}

func FuzzBatchNumber(f *testing.F) {
	for _, v := range batchSamples {
		switch v := v.(type) {
		case float64:
			f.Add(math.Float64bits(v))
		case int64:
			f.Add(uint64(v))
		}
	}
	var e BatchEncoder
	f.Fuzz(func(t *testing.T, bits uint64) {
		checkBatch(t, &e, []core.Mutation{
			{Type: core.MutSetProperty, NodeID: 1, Key: "f", Value: math.Float64frombits(bits)},
			{Type: core.MutSetProperty, NodeID: 1, Key: "f32", Value: float32(math.Float64frombits(bits))},
			{Type: core.MutSetProperty, NodeID: 1, Key: "i", Value: int64(bits)},
			{Type: core.MutSetProperty, NodeID: 1, Key: "u", Value: bits},
		})
	})
}

// TestWriteBatchCorpus writes batches in both encodings for
// test/e2e/batch.mjs, which checks that the runtime's binary decoder sees what
// its JSON.parse does. It only runs with GOOWEE_BATCH_CORPUS set to the output
// file.
func TestWriteBatchCorpus(t *testing.T) {
	out := os.Getenv("GOOWEE_BATCH_CORPUS")
	if out == "" {
		t.Skip("set GOOWEE_BATCH_CORPUS to write the corpus")
	}
	var batches [][]core.Mutation
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 3000; i++ {
		batch := make([]core.Mutation, rng.Intn(8))
		for j := range batch {
			batch[j] = randomBatchMutation(rng)
		}
		batches = append(batches, batch)
	}
	for _, v := range batchSamples {
		batches = append(batches, []core.Mutation{
			{Type: core.MutSetProperty, NodeID: 7, Key: "value", Value: v},
			{Type: core.MutSetAttribute, NodeID: 8, Key: "data-x", Value: v},
		})
	}
	batches = append(batches, laterTypesBatch)
	r := New()
	g := layoutGraph(r, 500, 1000)
	g.tick(1)
	batches = append(batches, r.Scheduler.Flush(), nil)

	type entry struct {
		JSON   string `json:"json"`
		Binary string `json:"binary"` // base64
	}
	var e BatchEncoder
	corpus := make([]entry, 0, len(batches))
	for _, b := range batches {
		j, _ := EncodeBatch(b)
		data, _ := e.Encode(b)
		corpus = append(corpus, entry{string(j), base64.StdEncoding.EncodeToString(data)})
	}
	data, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d batches to %s", len(corpus), out)
}

// TestWriteTransportFrames writes the frames test/e2e/transport.mjs replays in
// the browser, in both encodings: the layout graph's mount, 90 layout ticks
// (5,000 attribute updates each) and 400 frames that move one node (10
// updates each). It only runs with GOOWEE_TRANSPORT_FRAMES set to the output
// file.
func TestWriteTransportFrames(t *testing.T) {
	out := os.Getenv("GOOWEE_TRANSPORT_FRAMES")
	if out == "" {
		t.Skip("set GOOWEE_TRANSPORT_FRAMES to write the frames")
	}
	type frame struct {
		JSON   string `json:"json"`
		Binary string `json:"binary"` // base64
	}
	var e BatchEncoder
	encode := func(muts []core.Mutation) frame {
		j, _ := EncodeBatch(muts)
		data, _ := e.Encode(muts)
		return frame{string(j), base64.StdEncoding.EncodeToString(data)}
	}
	r := New()
	g := layoutGraph(r, 500, 1000)
	var frames struct {
		Mount frame   `json:"mount"`
		Ticks []frame `json:"ticks"`
		Nudge []frame `json:"nudge"`
	}
	frames.Mount = encode(g.mount)
	for step := 1; step <= 90; step++ {
		g.tick(step)
		frames.Ticks = append(frames.Ticks, encode(r.Scheduler.Flush()))
	}
	for step := 1; step <= 400; step++ {
		g.nudge(step)
		muts := r.Scheduler.Flush()
		if len(muts) != 10 {
			t.Fatalf("a nudge produced %d updates, want 10", len(muts))
		}
		frames.Nudge = append(frames.Nudge, encode(muts))
	}
	data, err := json.Marshal(frames)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
