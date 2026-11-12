package core

import (
	"runtime"
	"sync"
)

// SubscriberCount reports how many active subscriptions this signal has.
// Used by the devtools inspector to show the signal graph.
func (s *Signal[T]) SubscriberCount() int {
	if s == nil {
		return 0
	}
	return len(s.subs) - s.deadSubs
}

// InspectID returns the devtools inspector id when devtools are enabled and
// this signal was registered; otherwise 0.
func (s *Signal[T]) InspectID() int {
	// devtools registers signals by pointer; avoid an import cycle by using
	// the optional registration hook below.
	if s == nil {
		return 0
	}
	if signalInspectID != nil {
		return signalInspectID(s)
	}
	return 0
}

var signalInspectID func(SignalAccessor) int

// SetSignalInspectID wires devtools.SignalID without an import cycle.
func SetSignalInspectID(fn func(SignalAccessor) int) {
	signalInspectID = fn
}

// RegisterSignalHook is called when a signal is created in a component hook.
// devtools sets this to register signals for the inspector.
var RegisterSignalHook func(sig SignalAccessor, kind string)

// RegisterSignal records sig for the devtools inspector when enabled.
func RegisterSignal(sig SignalAccessor, kind string) {
	if RegisterSignalHook != nil {
		RegisterSignalHook(sig, kind)
	}
}

// NameSignalHook gives the name from Named to devtools, which reports whether
// it took it (only in dev mode).
var NameSignalHook func(sig SignalAccessor, name string) bool

// NamedSignal is a signal and the name Named gave it.
type NamedSignal struct {
	Signal SignalAccessor
	Name   string
}

// A browser app's package-level signals (a store in a var) are named before
// bridge.Run knows whether dev mode is on, so a browser build holds those
// names until TakeEarlyNames: devtools.Enable registers them, and bridge.Run
// drops them when dev mode is off. Elsewhere (a server, a test) none are held.
var (
	earlyNamesMu sync.Mutex
	holdingNames = runtime.GOOS == "js"
	earlyNames   []NamedSignal
)

func nameSignal(sig SignalAccessor, name string) {
	if NameSignalHook != nil && NameSignalHook(sig, name) {
		return
	}
	earlyNamesMu.Lock()
	if holdingNames {
		earlyNames = append(earlyNames, NamedSignal{Signal: sig, Name: name})
	}
	earlyNamesMu.Unlock()
}

// TakeEarlyNames returns the names given before the app started and stops
// holding new ones.
func TakeEarlyNames() []NamedSignal {
	earlyNamesMu.Lock()
	defer earlyNamesMu.Unlock()
	names := earlyNames
	earlyNames, holdingNames = nil, false
	return names
}

var signalWriteHook func(sig SignalAccessor, from, to any)

// SetSignalWriteHook makes each Set that changes a signal's value call fn with
// the old and new values first. devtools uses it for dev mode's change log;
// apps watch signals instead (hooks.Watch). nil turns it off.
func SetSignalWriteHook(fn func(sig SignalAccessor, from, to any)) {
	signalWriteHook = fn
}
