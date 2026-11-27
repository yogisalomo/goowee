package core

import "testing"

func TestEventFilterMatchAndSpec(t *testing.T) {
	enter := KeyIs("Enter").Without(ModShift)
	saveKey := KeyIs("s", "S").With(ModCtrl)
	cases := []struct {
		name    string
		f       EventFilter
		payload string
		want    bool
	}{
		{"zero matches nothing", EventFilter{}, `{"button":0}`, false},
		{"plain click", PlainClick(), `{"button":0}`, true},
		{"ctrl-click", PlainClick(), `{"button":0,"ctrlKey":true}`, false},
		{"cmd-click", PlainClick(), `{"button":0,"metaKey":true}`, false},
		{"middle click", PlainClick(), `{"button":1}`, false},
		{"enter", enter, `{"key":"Enter"}`, true},
		{"shift+enter", enter, `{"key":"Enter","shiftKey":true}`, false},
		{"enter while composing", enter, `{"key":"Enter","isComposing":true}`, false},
		{"other key", enter, `{"key":"a"}`, false},
		{"ctrl+s", saveKey, `{"key":"s","ctrlKey":true}`, true},
		{"s without ctrl", saveKey, `{"key":"s"}`, false},
		{"ctrl+shift+S", saveKey, `{"key":"S","ctrlKey":true,"shiftKey":true}`, true},
		{"files", CarriesFiles(), `{"types":["Files","text/uri-list"]}`, true},
		{"text drag", CarriesFiles(), `{"types":["text/plain"]}`, false},
	}
	for _, c := range cases {
		if got := c.f.Match(NewEventDataJSON("x", 1, c.payload, nil)); got != c.want {
			t.Errorf("%s: Match = %v, want %v", c.name, got, c.want)
		}
	}
	for f, want := range map[EventFilter]string{
		{}:                                    "",
		PlainClick():                          "plain",
		CarriesFiles():                        "files",
		enter:                                 "key\n0\n4\nEnter",
		saveKey:                               "key\n1\n0\ns\nS",
		KeyIs("Escape").With(ModMeta, ModAlt): "key\n10\n0\nEscape",
	} {
		if got := f.Spec(); got != want {
			t.Errorf("Spec = %q, want %q", got, want)
		}
	}
}
