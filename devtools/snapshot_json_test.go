package devtools_test

import (
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/devtools"
	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
	"github.com/yogisalomo/goowee/internal/jsonlite"
)

// In the browser the inspector writes its snapshot with jsonlite, without
// encoding/json (#126): everything in it must be a plain value.
func TestSnapshotEncodesWithoutEncodingJSON(t *testing.T) {
	devtools.Enable()
	app := core.Component("App", func() core.Node {
		n, _ := hooks.UseState(0)
		n.Named("hero.count")
		return h.Div(h.TextS(n))
	})
	gooweetest.Render(t, app)
	devtools.SetRoot(app)
	prev := core.JSONMarshaler()
	core.UseJSON(nil)
	defer core.UseJSON(prev)
	snap := devtools.Snapshot()
	for k, v := range snap {
		if _, ok := jsonlite.Append(nil, v); !ok {
			t.Errorf("field %s (%T) can't be encoded", k, v)
			var walk func(path string, v any)
			walk = func(path string, v any) {
				switch x := v.(type) {
				case map[string]any:
					for k2, v2 := range x {
						if _, ok := jsonlite.Append(nil, v2); !ok {
							walk(path+"."+k2, v2)
						}
					}
				case []any:
					for i, v2 := range x {
						if _, ok := jsonlite.Append(nil, v2); !ok {
							walk(path+"["+string(rune('0'+i))+"]", v2)
						}
					}
				default:
					t.Errorf("  %s: %T %#v", path, v, v)
				}
			}
			walk(k, v)
		}
	}
}
