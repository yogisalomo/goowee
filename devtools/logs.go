package devtools

import (
	"sync"

	"github.com/yogisalomo/goowee/core"
)

// The dev overlay (#148) shows goowee's log: contained panics and warnings,
// which otherwise reach only the console.

// MaxLogs is how many log entries dev mode keeps; older ones are dropped.
const MaxLogs = 200

var (
	logMu   sync.Mutex
	logs    []core.LogEntry
	logSeq  int
	logsSig *core.Signal[int] // bumped (on the render loop) when an entry arrives

	silenced = map[uintptr]bool{}
)

// RecordLog keeps e for the overlay. The bridge's log sink calls it in dev
// mode; it may run on any goroutine.
func RecordLog(e core.LogEntry) {
	logMu.Lock()
	if len(logs) == MaxLogs {
		copy(logs, logs[1:])
		logs = logs[:MaxLogs-1]
	}
	logs = append(logs, e)
	logSeq++
	n := logSeq
	logMu.Unlock()
	core.Schedule(func() { LogsVersion().Set(n) })
}

// Logs returns the kept entries, oldest first.
func Logs() []core.LogEntry {
	logMu.Lock()
	defer logMu.Unlock()
	return append([]core.LogEntry(nil), logs...)
}

// ClearLogs forgets the kept entries.
func ClearLogs() {
	logMu.Lock()
	logs = nil
	logSeq++
	n := logSeq
	logMu.Unlock()
	core.Schedule(func() { LogsVersion().Set(n) })
}

// LogsVersion changes whenever an entry is kept or the log is cleared: a
// view lists it as a dependency to follow the log.
func LogsVersion() *core.Signal[int] {
	logMu.Lock()
	defer logMu.Unlock()
	if logsSig == nil {
		logsSig = core.NewSignal(0)
		Silence(logsSig)
	}
	return logsSig
}

// Silence keeps sig out of the change log and the inspector's signal list:
// for the overlay's own state, which would otherwise log its every click.
func Silence(sig core.SignalAccessor) {
	mu.Lock()
	silenced[signalKey(sig)] = true
	delete(signals, signalKey(sig))
	mu.Unlock()
}

func isSilenced(sig core.SignalAccessor) bool { return silenced[signalKey(sig)] }
