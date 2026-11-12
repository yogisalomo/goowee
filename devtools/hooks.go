package devtools

import "github.com/yogisalomo/goowee/core"

func init() {
	core.RegisterSignalHook = RegisterSignal
	core.NameSignalHook = NameSignal
}

// wireHooks connects devtools to core after Enable so InspectID resolves.
func wireHooks() {
	core.SetSignalInspectID(SignalID)
}
