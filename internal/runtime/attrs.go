package runtime

import (
	"strings"

	"github.com/yogisalomo/goowee/core"
)

// urlAttrs are attributes (and same-named properties) whose value is a URL
// the browser may navigate to or load — where a javascript: URL executes.
var urlAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true,
	"xlink:href": true, "poster": true, "cite": true, "background": true,
	"data": true, "manifest": true, "codebase": true, "longdesc": true,
	"usemap": true, "ping": true,
}

// BlockedURL replaces a blocked javascript:/vbscript: URL.
const BlockedURL = "about:blank#blocked"

// SafeURL returns value unchanged unless name is a URL-valued attribute and
// value uses a script scheme (javascript:, vbscript:), in which case it logs
// and returns BlockedURL. Rendering user data into href/src is common; a
// script URL there is a stored-XSS vector, so goowee blocks it on both the
// server and the client, like React does.
func SafeURL(name, value string) string {
	if !urlAttrs[strings.ToLower(name)] || !scriptURL(value) {
		return value
	}
	core.Log(core.LogWarn, "blocked a script URL in a URL attribute", map[string]any{"attr": name})
	return BlockedURL
}

// SafeURLValue is SafeURL for a property value of any type (only strings can
// carry a URL).
func SafeURLValue(name string, v any) any {
	if s, ok := v.(string); ok {
		return SafeURL(name, s)
	}
	return v
}

// scriptURL reports whether v's scheme is javascript: or vbscript:, the way a
// browser parses it: leading C0 controls and spaces are ignored, and tabs and
// newlines are removed anywhere.
func scriptURL(v string) bool {
	var b strings.Builder
	started := false
	for _, r := range v {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if !started && r <= 0x20 {
			continue
		}
		started = true
		b.WriteRune(r)
		if b.Len() >= len("javascript:") {
			break
		}
	}
	s := strings.ToLower(b.String())
	return strings.HasPrefix(s, "javascript:") || strings.HasPrefix(s, "vbscript:")
}

// ValidAttrName reports whether name is an attribute name both the HTML
// serializer and the DOM's setAttribute accept: it starts with a letter, '_'
// or ':' and continues with letters, digits, '-', '_', '.', or ':' (non-ASCII
// letters allowed). xlink:href, xml:lang, data-x_y, aria-* all pass; names
// with spaces, quotes, '=', '>', '/', '<' or '@' don't — they would break the
// markup on the server and throw in the browser.
func ValidAttrName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == ':', r >= 0x80 && r != 0xFFFD:
		case i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '.'):
		default:
			return false
		}
	}
	return true
}

// WarnInvalidAttr logs a skipped attribute.
func WarnInvalidAttr(name string) {
	core.Log(core.LogWarn, "skipping invalid attribute name", map[string]any{"name": name})
}
