package core

import (
	"strconv"
	"strings"
)

// EventFilter is a condition on an event that the browser runtime checks by
// itself, before any Go code runs: h.PreventDefaultWhen uses one to cancel
// an event's default action without asking Go (#158). The zero value matches
// nothing. Build one with PlainClick, KeyIs or CarriesFiles.
//
//	Textarea(OnKeyDownE(send), h.PreventDefaultWhen(core.KeyIs("Enter").Without(core.ModShift)))
type EventFilter struct {
	kind    uint8  // 0 none, 1 plain click, 2 key, 3 carries files
	keys    string // kind 2: e.key values joined by "\n"
	with    uint8  // kind 2: modifiers that must be down
	without uint8  // kind 2: modifiers that must be up
}

const (
	filterNone uint8 = iota
	filterPlainClick
	filterKey
	filterFiles
)

// Modifier is a modifier key for EventFilter.With and Without.
type Modifier uint8

const (
	ModCtrl Modifier = 1 << iota
	ModAlt
	ModShift
	ModMeta
)

// PlainClick matches a click with the primary button and no modifier key:
// one the browser would otherwise handle in the same tab (a link's
// navigation), as opposed to Ctrl/Cmd-click opening a new tab.
func PlainClick() EventFilter { return EventFilter{kind: filterPlainClick} }

// KeyIs matches a key event whose e.key is one of keys. It never matches
// while an input method is composing (IME), so Enter that confirms a
// composition isn't taken.
func KeyIs(keys ...string) EventFilter {
	return EventFilter{kind: filterKey, keys: strings.Join(keys, "\n")}
}

// CarriesFiles matches a drag, drop or paste whose data types include
// "Files". A drop zone's dragover handler must cancel such an event for the
// browser to allow the drop; a paste handler that takes the files cancels it
// so the browser doesn't insert them itself.
func CarriesFiles() EventFilter { return EventFilter{kind: filterFiles} }

// With narrows a KeyIs filter to events with all of mods down.
func (f EventFilter) With(mods ...Modifier) EventFilter {
	for _, m := range mods {
		f.with |= uint8(m)
	}
	return f
}

// Without narrows a KeyIs filter to events with none of mods down.
func (f EventFilter) Without(mods ...Modifier) EventFilter {
	for _, m := range mods {
		f.without |= uint8(m)
	}
	return f
}

// IsZero reports whether f is the zero filter, which matches nothing.
func (f EventFilter) IsZero() bool { return f.kind == filterNone }

// Match reports whether e matches f, exactly as the runtime decides it.
func (f EventFilter) Match(e EventData) bool {
	switch f.kind {
	case filterPlainClick:
		return e.Button() == 0 && !e.CtrlKey() && !e.MetaKey() && !e.ShiftKey() && !e.AltKey()
	case filterFiles:
		for _, t := range e.DataTypes() {
			if t == "Files" {
				return true
			}
		}
		return false
	case filterKey:
		if e.IsComposing() {
			return false
		}
		var m uint8
		if e.CtrlKey() {
			m |= uint8(ModCtrl)
		}
		if e.AltKey() {
			m |= uint8(ModAlt)
		}
		if e.ShiftKey() {
			m |= uint8(ModShift)
		}
		if e.MetaKey() {
			m |= uint8(ModMeta)
		}
		if m&f.with != f.with || m&f.without != 0 {
			return false
		}
		key := e.Key()
		for _, k := range strings.Split(f.keys, "\n") {
			if k == key {
				return true
			}
		}
	}
	return false
}

// Spec is f's wire form for the runtime: "plain", "files", or
// "key\n<with>\n<without>\n<key>\n<key>…". It is "" for the zero value.
func (f EventFilter) Spec() string {
	switch f.kind {
	case filterPlainClick:
		return "plain"
	case filterFiles:
		return "files"
	case filterKey:
		return "key\n" + strconv.Itoa(int(f.with)) + "\n" + strconv.Itoa(int(f.without)) + "\n" + f.keys
	}
	return ""
}
