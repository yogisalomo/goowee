package ssr

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"

	"github.com/yogisalomo/goowee/core"
)

// IslandAttr marks an island's container: its value is the island's name,
// which the client looks up in the map it gives bridge.Islands.
const IslandAttr = "data-goowee-island"

// IslandPropsAttr carries an island's props, as JSON, from the server render
// to the client's factory.
const IslandPropsAttr = "data-goowee-props"

// Island renders node, a goowee component, as a piece of any server-rendered
// page (html/template, templ, a string): a <div data-goowee-island> holding
// the component's markup and its props as JSON. In the browser,
// bridge.Islands finds each island and hydrates it with the factory
// registered under name, which builds the same component from the props.
//
//	// Server, in a handler or a template func:
//	cart, err := ssr.Island("cart", CartWidget(props), props)
//
//	// Client (cmd/islands/main.go, GOOS=js):
//	bridge.Islands(map[string]func(json.RawMessage) core.Node{
//	    "cart": func(raw json.RawMessage) core.Node {
//	        var p CartProps
//	        json.Unmarshal(raw, &p)
//	        return CartWidget(p)
//	    },
//	})
//	bridge.Wait()
//
// Each island hydrates on its own, so several can sit in one page, between
// server markup goowee doesn't own. props must marshal to JSON (nil is
// "null"); node must render from props alone, so server and client agree.
func Island(name string, node core.Node, props any) (template.HTML, error) {
	data, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("ssr.Island %q: props: %w", name, err)
	}
	body, _ := New().Render(node)
	return template.HTML(`<div ` + IslandAttr + `="` + html.EscapeString(name) + `" ` +
		IslandPropsAttr + `="` + html.EscapeString(string(data)) + `">` + body + `</div>`), nil
}
