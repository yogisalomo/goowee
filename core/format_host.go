//go:build !(js && wasm)

package core

import (
	"encoding/json"
	"fmt"
)

// Outside the browser fmt and encoding/json cost nothing worth saving: composite values
// format as %v does, so server-rendered text matches what fmt would write.
func init() {
	UseFmt(fmt.Sprintf)
	UseJSON(json.Marshal)
}
