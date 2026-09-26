package dom

import (
	"bytes"
	"encoding/json"

	"github.com/yogisalomo/goowee/core"
)

// EncodeBatch serializes a mutation batch for the JS runtime. When the batch as
// a whole can't be encoded — typically a float NaN/±Inf in a property value,
// which JSON can't carry — it falls back to encoding mutations one by one and
// returns the ones it had to drop, so a single bad value costs one mutation
// instead of the whole frame's DOM updates. The caller logs the dropped ones.
func EncodeBatch(muts []core.Mutation) (data []byte, dropped []core.Mutation) {
	data, err := json.Marshal(muts)
	if err == nil {
		return data, nil
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	first := true
	for _, m := range muts {
		b, err := json.Marshal(m)
		if err != nil {
			dropped = append(dropped, m)
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(b)
	}
	buf.WriteByte(']')
	return buf.Bytes(), dropped
}

// LogDropped reports mutations EncodeBatch could not serialize.
func LogDropped(dropped []core.Mutation) {
	for _, m := range dropped {
		core.Log(core.LogWarn, "dropped a DOM update whose value can't be encoded (NaN/Inf or non-JSON value)", map[string]any{
			"type":   m.Type.String(),
			"nodeId": m.NodeID,
			"key":    m.Key,
			"value":  m.Value,
		})
	}
}
