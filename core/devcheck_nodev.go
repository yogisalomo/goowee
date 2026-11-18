//go:build goowee_nodev

package core

// Built with -tags goowee_nodev (#127): dependency checking is compiled out.
// SetDevChecks is a no-op, DevChecks is always false, and evaluations run
// untracked, so no read is recorded and runtime.Callers and FuncForPC aren't
// linked. See devcheck.go for the checks themselves.

// readTracker stays nil: Signal.Get's one nil check is all that's left.
var readTracker *[]trackedRead

type trackedRead struct{}

// SetDevChecks does nothing in a goowee_nodev build.
func SetDevChecks(bool) {}

// DevChecks is always false in a goowee_nodev build.
func DevChecks() bool { return false }

func recordRead(SignalAccessor) {}

func untracked(fn func()) { fn() }

func checkedEval(_ string, _ []SignalAccessor, fn func()) { fn() }
