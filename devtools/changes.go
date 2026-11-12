package devtools

import (
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yogisalomo/goowee/core"
)

// Change is one signal write in the change log: which signal, its value
// before and after, and the code that wrote it.
type Change struct {
	Seq    int     `json:"seq"`            // 1 for the first change since Enable
	MS     float64 `json:"ms"`             // milliseconds since Enable
	Signal int     `json:"signal"`         // inspector id (Snapshot's signals)
	Name   string  `json:"name,omitempty"` // from Signal.Named
	From   string  `json:"from"`
	To     string  `json:"to"`
	At     string  `json:"at"`   // file:line of the write
	Func   string  `json:"func"` // the function that wrote it, e.g. app.(*Store).AddToCart
}

// MaxChanges is how many changes the log keeps; older ones are dropped.
const MaxChanges = 500

// maxValueLen caps a value's text in the log, so a big list doesn't fill it.
const maxValueLen = 200

var (
	changes []Change
	seq     int
	started time.Time
)

// Changes returns the logged signal writes, oldest first: every Set that
// changed a value since devtools were enabled (the last MaxChanges of them).
// Computed values aren't written, so they don't appear; their sources do.
func Changes() []Change {
	mu.RLock()
	defer mu.RUnlock()
	return append([]Change(nil), changes...)
}

// ClearChanges empties the change log.
func ClearChanges() {
	mu.Lock()
	changes = nil
	mu.Unlock()
}

// recordChange is core's write hook while devtools are enabled.
func recordChange(sig core.SignalAccessor, from, to any) {
	at, fn := writeSite()
	c := Change{
		From: truncate(formatValue(from)),
		To:   truncate(formatValue(to)),
		At:   at,
		Func: fn,
	}
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		return
	}
	m := register(sig, "signal") // a signal no hook registered shows up once written
	seq++
	c.Seq, c.Signal, c.Name = seq, m.ID, m.Name
	c.MS = float64(time.Since(started).Microseconds()) / 1000
	if len(changes) == MaxChanges {
		copy(changes, changes[1:])
		changes = changes[:MaxChanges-1]
	}
	changes = append(changes, c)
}

// writeSite finds the code behind a write: the first caller outside goowee
// (the handler or store method that set the signal), or, for a write goowee
// made on its own (a two-way binding, a router link), the first frame outside
// core.
func writeSite() (at, fn string) {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:]) // from core's Set: skip Callers, writeSite, recordChange
	frames := runtime.CallersFrames(pcs[:n])
	var fallback runtime.Frame
	for {
		f, more := frames.Next()
		switch pkg := packageOf(f.Function); {
		case !framework(pkg):
			return site(f)
		case fallback.Function == "" && pkg != modulePath+"core" && !stdlib(pkg):
			fallback = f
		}
		if !more {
			break
		}
	}
	if fallback.Function != "" {
		return site(fallback)
	}
	return "unknown", ""
}

const modulePath = "github.com/yogisalomo/goowee/"

// framework reports whether pkg is goowee's own (not an app or example built
// in this module) or the standard library's.
func framework(pkg string) bool {
	if stdlib(pkg) {
		return true
	}
	rest, ok := strings.CutPrefix(pkg, modulePath)
	if !ok {
		return false
	}
	switch rest {
	case "core", "h", "hooks", "router", "net", "storage", "bridge", "devtools", "gooweetest", "ssr":
		return true
	}
	return strings.HasPrefix(rest, "internal/")
}

// stdlib covers the standard packages a write's stack passes through: the
// runtime, a JavaScript callback, a test runner.
func stdlib(pkg string) bool {
	switch pkg {
	case "runtime", "syscall/js", "reflect", "sync", "testing":
		return true
	}
	return false
}

// packageOf returns the import path of a function's package from its full
// name, e.g. "github.com/me/app" for "github.com/me/app.(*Store[go.shape.int]).Add".
func packageOf(function string) string {
	name := function
	if i := strings.IndexAny(name, "(["); i >= 0 {
		name = name[:i] // a receiver or type parameters can contain dots and slashes
	}
	slash := strings.LastIndexByte(name, '/')
	if dot := strings.IndexByte(name[slash+1:], '.'); dot >= 0 {
		return function[:slash+1+dot]
	}
	return name
}

func site(f runtime.Frame) (at, fn string) {
	at = fmt.Sprintf("%s:%d", shortFile(f.File), f.Line)
	fn = f.Function
	if i := strings.LastIndexByte(packageOf(fn), '/'); i >= 0 {
		fn = fn[i+1:]
	}
	return at, fn
}

// shortFile keeps a file's last two path elements, e.g. "app/store.go".
func shortFile(file string) string {
	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return file
}

func truncate(s string) string {
	if len(s) <= maxValueLen {
		return s
	}
	cut := maxValueLen
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
