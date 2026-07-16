package runtime

import "github.com/yogisalomo/goowee/core"

func CollectIDs(n core.Node) []int {
	var ids []int
	switch v := n.(type) {
	case *core.ElementNode:
		if v.ID > 0 {
			ids = append(ids, v.ID)
		}
		for _, child := range v.Children {
			ids = append(ids, CollectIDs(child)...)
		}
	case *core.TextNode:
		if v.ID > 0 {
			ids = append(ids, v.ID)
		}
	case *core.RawNode:
		if v.ID > 0 {
			ids = append(ids, v.ID)
		}
	case *core.FragmentNode:
		for _, child := range v.Children {
			ids = append(ids, CollectIDs(child)...)
		}
	case *core.PortalNode:
		for _, child := range v.Children {
			ids = append(ids, CollectIDs(child)...)
		}
	case *core.ErrorBoundaryNode:
		if v.Prev != nil {
			ids = append(ids, CollectIDs(v.Prev)...)
		}
	case *core.ComponentNode:
		if v.Prev != nil {
			ids = append(ids, CollectIDs(v.Prev)...)
		}
	case *core.ScopeNode:
		if v.Prev != nil {
			ids = append(ids, CollectIDs(v.Prev)...)
		}
	}
	return ids
}
