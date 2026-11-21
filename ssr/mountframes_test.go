package ssr_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/yogisalomo/goowee/core"
	. "github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/internal/dom"
	"github.com/yogisalomo/goowee/ssr"
)

// TestWriteMountFrames writes, for test/e2e/mount.mjs (#139), the same list
// of rows two ways: the binary batch the client renderer sends to mount it,
// and the HTML the server renders for it (ids and text markers included).
func TestWriteMountFrames(t *testing.T) {
	out := os.Getenv("GOOWEE_MOUNT_FRAMES")
	if out == "" {
		t.Skip("set GOOWEE_MOUNT_FRAMES to write the frames")
	}
	rows := func(n int) core.Node {
		items := make([]core.Item, n)
		for i := range items {
			// 5 nodes, 3 attributes, 2 texts: a typical list row.
			items[i] = Li(Class("row"), Attr("data-id", strconv.Itoa(i)),
				Span(Class("label"), Text("Row number "+strconv.Itoa(i))),
				A(Href("#"), Text("remove")))
		}
		return Ul(items...)
	}
	type frame struct {
		Rows     int    `json:"rows"`
		Muts     int    `json:"muts"`
		Binary   string `json:"binary"`
		HTML     string `json:"html"`
		BinBytes int    `json:"binBytes"`
	}
	var frames []frame
	for _, n := range []int{1000, 10000} {
		r := dom.New()
		muts, _ := r.Render(rows(n))
		var e dom.BatchEncoder
		data, _ := e.Encode(muts)
		body, _ := ssr.New().Render(rows(n))
		frames = append(frames, frame{n, len(muts), base64.StdEncoding.EncodeToString(data), body, len(data)})
	}
	data, _ := json.Marshal(frames)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
