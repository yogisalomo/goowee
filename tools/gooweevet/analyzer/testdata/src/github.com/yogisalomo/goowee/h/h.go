// Package h is a stub of goowee/h for the analyzer's tests.
package h

import "github.com/yogisalomo/goowee/core"

func Text(s string) core.Node               { return nil }
func TextS(s core.SignalAccessor) core.Node { return nil }
func Class(s string) core.Item              { return nil }
func P(items ...any) core.Node              { return nil }
func Button(items ...any) core.Node         { return nil }
func OnClick(fn func()) core.Item           { return nil }
