package core

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
)

// Dev-mode dependency checking (#74). Dependencies are declared by hand
// (ADR-002), and a missing one is a silent bug: the region or Computed simply
// doesn't update. In dev mode, reads made while a scope renders or a Computed
// computes are recorded and compared with the declared deps; each undeclared
// read site is reported once. Off by default — Get pays one nil check.

var (
	devChecks   bool
	readTracker *[]trackedRead // non-nil while a tracked evaluation runs

	warnedReadsMu sync.Mutex
	warnedReads   = map[uintptr]bool{}
)

type trackedRead struct {
	sig SignalAccessor
	pc  uintptr // where the read happened (first frame outside core)
}

// SetDevChecks turns dev-mode dependency checking on or off. devtools.Enable
// turns it on (?goowee-dev). It is a client (single render loop) feature: it
// tracks reads in a global, so don't enable it in a server doing concurrent
// SSR.
func SetDevChecks(on bool) { devChecks = on }

// DevChecks reports whether dev-mode dependency checking is on.
func DevChecks() bool { return devChecks }

func recordRead(s SignalAccessor) {
	var pcs [8]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	var pc uintptr
	for {
		f, more := frames.Next()
		if !strings.HasPrefix(f.Function, "github.com/yogisalomo/goowee/core.") {
			pc = f.PC
			break
		}
		if !more {
			break
		}
	}
	*readTracker = append(*readTracker, trackedRead{sig: s, pc: pc})
}

// untracked runs fn with read tracking suspended (nested Computed
// evaluations and subscriber callbacks aren't the caller's reads). With
// tracking off the global is never written, so concurrent server renders
// don't contend on it.
func untracked(fn func()) {
	if readTracker == nil {
		fn()
		return
	}
	saved := readTracker
	readTracker = nil
	defer func() { readTracker = saved }()
	fn()
}

// checkedEval runs fn; in dev mode it records fn's reads and reports the ones
// not among deps. kind names the construct for the message.
func checkedEval(kind string, deps []SignalAccessor, fn func()) {
	if !devChecks {
		untracked(fn)
		return
	}
	saved := readTracker
	var reads []trackedRead
	readTracker = &reads
	defer func() {
		readTracker = saved
		reportUndeclared(kind, deps, reads)
	}()
	fn()
}

func reportUndeclared(kind string, deps []SignalAccessor, reads []trackedRead) {
	for _, rd := range reads {
		declared := false
		for _, d := range deps {
			if d == rd.sig {
				declared = true
				break
			}
		}
		if declared {
			continue
		}
		warnedReadsMu.Lock()
		seen := warnedReads[rd.pc]
		warnedReads[rd.pc] = true
		warnedReadsMu.Unlock()
		if seen {
			continue
		}
		where := "unknown location"
		if fn := runtime.FuncForPC(rd.pc); fn != nil {
			file, line := fn.FileLine(rd.pc)
			where = fmt.Sprintf("%s:%d", trimPath(file), line)
		}
		Log(LogWarn, "signal read that isn't a declared dependency: this won't update when it changes", map[string]any{
			"in":    kind,
			"read":  where,
			"deps":  len(deps),
			"fix":   "add the signal to the deps, bind it (TextS, ClassS, …), or read it with Peek() if a snapshot is intended",
			"sigId": inspectIDOf(rd.sig),
		})
	}
}

func trimPath(file string) string {
	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return file
}

func inspectIDOf(s SignalAccessor) int {
	if signalInspectID != nil {
		return signalInspectID(s)
	}
	return 0
}

// RenderScope runs a scope's render function. In dev mode it also reports
// signals the render read but the scope doesn't list in Deps — the scope
// wouldn't re-render when they change. Renderers call it instead of
// sn.Render().
func RenderScope(sn *ScopeNode) Node {
	var n Node
	checkedEval("reactive region (Show/For/Switch/Route/UseScope)", sn.Deps, func() { n = sn.Render() })
	return n
}
