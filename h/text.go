package h

import (
	"fmt"
	"goowee/core"
)

func Text(s string) *core.TextNode { return &core.TextNode{Value: s} }

func TextS(sig core.SignalAccessor) *core.TextNode {
	return &core.TextNode{Value: sig}
}

// Textf formats text like fmt.Sprintf. Any argument implementing
// core.SignalAccessor is treated as reactive: its current value is
// substituted, and the text recomputes whenever that signal changes.
// With no signal arguments it is a plain static text node.
//
// Because signals stand in for the values their verbs format (a
// *Signal[int] for %d, etc.), args are resolved before formatting; the
// raw variadic is never forwarded to fmt, which also keeps `go vet`'s
// printf check from misreading signal arguments as type mismatches.
func Textf(format string, args ...any) core.Node {
	var deps []core.SignalAccessor
	for _, a := range args {
		if s, ok := a.(core.SignalAccessor); ok {
			deps = append(deps, s)
		}
	}
	if len(deps) == 0 {
		return &core.TextNode{Value: fmt.Sprintf(format, resolveArgs(args)...)}
	}
	compute := func() string {
		return fmt.Sprintf(format, resolveArgs(args)...)
	}
	return &core.TextNode{Value: core.Computed(deps, compute)}
}

func resolveArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		if s, ok := a.(core.SignalAccessor); ok {
			out[i] = s.Value()
		} else {
			out[i] = a
		}
	}
	return out
}
