package core

// SubscriberCount reports how many active subscriptions this signal has.
// Used by the devtools inspector to show the signal graph.
func (s *Signal[T]) SubscriberCount() int {
	if s == nil {
		return 0
	}
	return len(s.subs)
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
