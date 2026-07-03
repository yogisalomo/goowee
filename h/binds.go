package h

import "goowee/core"

func BindProp(name string, sig core.SignalAccessor) core.Item {
	return bindItem{core.BindToProp, name, sig}
}

func BindAttr(name string, sig core.SignalAccessor) core.Item {
	return bindItem{core.BindToAttr, name, sig}
}

func ClassS(sig core.SignalAccessor) core.Item    { return BindAttr("class", sig) }
func StyleS(sig core.SignalAccessor) core.Item    { return BindAttr("style", sig) }
func HrefS(sig core.SignalAccessor) core.Item     { return BindAttr("href", sig) }
func ValueS(sig core.SignalAccessor) core.Item    { return BindProp("value", sig) }
func CheckedS(sig core.SignalAccessor) core.Item  { return BindProp("checked", sig) }
func DisabledS(sig core.SignalAccessor) core.Item { return BindProp("disabled", sig) }

func BindValue(sig *core.Signal[string]) core.Item {
	return Group(
		ValueS(sig),
		OnInputE(func(e core.EventData) { sig.Set(e.Value()) }),
	)
}

func BindChecked(sig *core.Signal[bool]) core.Item {
	return Group(
		CheckedS(sig),
		OnInputE(func(e core.EventData) { sig.Set(e.Checked()) }),
	)
}
