package router

import (
	"net/url"
	"testing"
)

// The hand-written query escaping matches net/url's (#126).
func TestQueryEscapingMatchesNetURL(t *testing.T) {
	for _, s := range []string{"", "plain", "a b", "a+b", "100%", "é", "😀", "a&b=c", "?#/", "-_.~", "\x00\xff", "%zz", "%4", "%41%42", "+%20+"} {
		if got, want := escapeQuery(s), url.QueryEscape(s); got != want {
			t.Errorf("escapeQuery(%q) = %q, net/url gives %q", s, got, want)
		}
		want, err := url.QueryUnescape(s)
		if err != nil {
			want = s // the router keeps a malformed escape as text
		}
		if got := unescapeQuery(s); got != want {
			t.Errorf("unescapeQuery(%q) = %q, want %q", s, got, want)
		}
	}
	m := map[string]string{"b": "2 3", "a": "x&y", "é": "ü"}
	vals := url.Values{}
	for k, v := range m {
		vals.Set(k, v)
	}
	if got, want := encodeQuery(m), vals.Encode(); got != want {
		t.Errorf("encodeQuery = %q, net/url gives %q", got, want)
	}
}
