package h

import (
	"fmt"
	"goowee/core"
)

func Text(s string) *core.TextNode { return &core.TextNode{Value: s} }

func TextS(sig core.SignalAccessor) *core.TextNode {
	return &core.TextNode{Value: sig}
}

func Textf(format string, args ...any) core.Node {
	var deps []core.SignalAccessor
	for _, a := range args {
		if s, ok := a.(core.SignalAccessor); ok {
			deps = append(deps, s)
		}
	}
	if len(deps) == 0 {
		return &core.TextNode{Value: sprintff(format, args...)}
	}
	compute := func() string {
		resolved := make([]any, len(args))
		for i, a := range args {
			if s, ok := a.(core.SignalAccessor); ok {
				resolved[i] = s.Value()
			} else {
				resolved[i] = a
			}
		}
		return sprintff(format, resolved...)
	}
	return &core.TextNode{Value: core.Computed(deps, compute)}
}

//go:noinline
func sprintff(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
