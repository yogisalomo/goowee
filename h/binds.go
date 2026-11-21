package h

import (
	"fmt"
	"sort"

	"github.com/yogisalomo/goowee/core"
)

func BindProp(name string, sig core.SignalAccessor) core.Item {
	return bindItem{core.BindToProp, name, sig, ""}
}

// AttrS binds an attribute to a signal. A false boolean signal removes the
// attribute (the HTML meaning of false); other values are written as text.
func AttrS(name string, sig core.SignalAccessor) core.Item {
	return bindItem{core.BindToAttr, name, sig, ""}
}

// BindAttr is the older spelling of AttrS.
func BindAttr(name string, sig core.SignalAccessor) core.Item { return AttrS(name, sig) }

func ClassS(sig core.SignalAccessor) core.Item    { return BindAttr("class", sig) }
func StyleS(sig core.SignalAccessor) core.Item    { return BindAttr("style", sig) }
func HrefS(sig core.SignalAccessor) core.Item     { return BindAttr("href", sig) }
func SrcS(sig core.SignalAccessor) core.Item      { return BindAttr("src", sig) }
func ValueS(sig core.SignalAccessor) core.Item    { return BindProp("value", sig) }
func CheckedS(sig core.SignalAccessor) core.Item  { return BindProp("checked", sig) }
func DisabledS(sig core.SignalAccessor) core.Item { return BindProp("disabled", sig) }

// ClassIf toggles one class from a bool signal with classList.toggle, so it
// composes with a static Class and any other ClassIf on the same element.
func ClassIf(name string, sig *core.Signal[bool]) core.Item {
	return bindItem{core.BindToClass, name, sig, ""}
}

// Classes toggles several classes at once. Keys are sorted so server and
// client produce the same initial class attribute.
func Classes(classes map[string]*core.Signal[bool]) core.Item {
	names := make([]string, 0, len(classes))
	for name := range classes {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]core.Item, 0, len(names))
	for _, name := range names {
		items = append(items, ClassIf(name, classes[name]))
	}
	return Group(items...)
}

// StyleProp binds one CSS property with style.setProperty, leaving the rest
// of the style attribute alone.
func StyleProp(name string, sig core.SignalAccessor) core.Item {
	return bindItem{core.BindToStyle, name, sig, ""}
}

// StylePx is StyleProp for numbers: the runtime appends "px", so an update
// sends a number rather than a formatted string.
func StylePx(name string, sig core.SignalAccessor) core.Item {
	return bindItem{core.BindToStyle, name, sig, "px"}
}

// HiddenS follows a bool signal with the HTML hidden attribute.
func HiddenS(sig *core.Signal[bool]) core.Item { return AttrS("hidden", sig) }

// Aria state helpers use the "true"/"false" strings ARIA requires.
func AriaHiddenS(sig *core.Signal[bool]) core.Item   { return ariaBoolS("aria-hidden", sig) }
func AriaExpandedS(sig *core.Signal[bool]) core.Item { return ariaBoolS("aria-expanded", sig) }
func AriaPressedS(sig *core.Signal[bool]) core.Item  { return ariaBoolS("aria-pressed", sig) }
func AriaSelectedS(sig *core.Signal[bool]) core.Item { return ariaBoolS("aria-selected", sig) }
func AriaInvalidS(sig *core.Signal[bool]) core.Item  { return ariaBoolS("aria-invalid", sig) }
func AriaRequiredS(sig *core.Signal[bool]) core.Item { return ariaBoolS("aria-required", sig) }
func AriaModalS(sig *core.Signal[bool]) core.Item    { return ariaBoolS("aria-modal", sig) }
func AriaAtomicS(sig *core.Signal[bool]) core.Item   { return ariaBoolS("aria-atomic", sig) }
func AriaBusyS(sig *core.Signal[bool]) core.Item     { return ariaBoolS("aria-busy", sig) }
func AriaCheckedS(sig *core.Signal[bool]) core.Item  { return ariaBoolS("aria-checked", sig) }
func AriaDisabledS(sig *core.Signal[bool]) core.Item { return ariaBoolS("aria-disabled", sig) }

func AriaLabelS(sig *core.Signal[string]) core.Item       { return AttrS("aria-label", sig) }
func AriaCurrentS(sig *core.Signal[string]) core.Item     { return AttrS("aria-current", sig) }
func AriaDescribedByS(sig *core.Signal[string]) core.Item { return AttrS("aria-describedby", sig) }
func AriaLabelledByS(sig *core.Signal[string]) core.Item  { return AttrS("aria-labelledby", sig) }
func AriaControlsS(sig *core.Signal[string]) core.Item    { return AttrS("aria-controls", sig) }
func AriaHaspopupS(sig *core.Signal[string]) core.Item    { return AttrS("aria-haspopup", sig) }
func AriaLiveS(sig *core.Signal[string]) core.Item        { return AttrS("aria-live", sig) }

func ariaBoolS(name string, sig *core.Signal[bool]) core.Item {
	return AttrS(name, core.Computed([]core.SignalAccessor{sig}, func() string {
		return fmtBool(sig.Get())
	}))
}

func BindValue(sig *core.Signal[string]) core.Item {
	return Group(
		ValueS(sig),
		OnInputE(func(e core.EventData) { sig.Set(e.Value()) }),
	)
}

// BindNumber binds a numeric input or range slider to a numeric signal. The
// browser's valueAsNumber does the parsing (including empty-input handling),
// so no Go-side strconv runs on every keystroke.
func BindNumber[T bindNumber](sig *core.Signal[T]) core.Item {
	return Group(
		ValueS(core.Computed([]core.SignalAccessor{sig}, func() string { return fmt.Sprint(sig.Get()) })),
		OnInputE(func(e core.EventData) { sig.Set(T(e.ValueAsNumber())) }),
	)
}

type bindNumber interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

func BindChecked(sig *core.Signal[bool]) core.Item {
	return Group(
		CheckedS(sig),
		OnInputE(func(e core.EventData) { sig.Set(e.Checked()) }),
	)
}

// BindRadio binds one radio of a group to a string signal. The radio is
// checked when the signal equals value; choosing it sets that value.
func BindRadio(sig *core.Signal[string], value string) core.Item {
	return Group(
		CheckedS(core.Computed([]core.SignalAccessor{sig}, func() bool { return sig.Get() == value })),
		OnChangeE(func(e core.EventData) {
			if e.Checked() {
				sig.Set(value)
			}
		}),
	)
}

// BindSelect binds a signal to a <select> element's value. Uses the "change"
// event (not "input") so the value only updates on explicit selection.
func BindSelect(sig *core.Signal[string]) core.Item {
	return Group(
		ValueS(sig),
		OnChangeE(func(e core.EventData) { sig.Set(e.Value()) }),
	)
}

// BindSelectMulti binds a <select multiple> to all of its selected option
// values. It sets the select's multiple property too, so a plain <select>
// doesn't silently keep one value.
func BindSelectMulti(sig *core.Signal[[]string]) core.Item {
	return Group(
		propItem{"multiple", true},
		bindItem{core.BindToSelectMulti, "", sig, ""},
		OnChangeE(func(e core.EventData) { sig.Set(e.SelectedValues()) }),
	)
}

// BindFiles keeps a file input's picked files in a signal. The files carry
// metadata; call File.Bytes in a goroutine to read their contents.
func BindFiles(sig *core.Signal[[]core.File]) core.Item {
	return OnChangeE(func(e core.EventData) { sig.Set(e.Files()) })
}

// BindValueLazy binds a signal to an <input>'s value, updating on "change"
// rather than "input". Use it when you want to batch updates until the user
// finishes editing (e.g. search-after-typing-stops, debounced at the signal
// consumer level).
func BindValueLazy(sig *core.Signal[string]) core.Item {
	return Group(
		ValueS(sig),
		OnChangeE(func(e core.EventData) { sig.Set(e.Value()) }),
	)
}
