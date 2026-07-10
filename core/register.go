package core

// RegisterDisposer queues fn to run when the current component unmounts. It is
// a function variable set by internal/runtime during init; the default no-op
// allows core tests (which don't load the runtime package) to compile cleanly.
// The real implementation is lifecycle-aware: it appends fn to the current
// component frame's disposer list.
var RegisterDisposer = func(func()) {}

// CurrentComponentPath returns the mounted component path when a render context
// is active. Wired by internal/runtime during init; devtools uses it to label
// signals without importing internal packages.
var CurrentComponentPath func() string
