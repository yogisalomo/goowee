package core

// RenderScope runs a scope's render function. In dev mode it also reports
// signals the render read but the scope doesn't list in Deps — the scope
// wouldn't re-render when they change. Renderers call it instead of
// sn.Render().
func RenderScope(sn *ScopeNode) Node {
	var n Node
	checkedEval("reactive region (Show/For/Switch/Route/UseScope)", sn.Deps, func() { n = sn.Render() })
	return n
}
