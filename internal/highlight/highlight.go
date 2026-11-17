// Package highlight splits source code into tokens for syntax highlighting:
// Go, HTML and shell. It is a lexer, not a parser: it works on fragments as
// well as whole files, never fails, and joining the tokens' Text gives back
// the input exactly.
//
// It has no dependencies outside the standard library and doesn't import
// goowee, so it can move to its own module later. A renderer maps each Kind
// to a style:
//
//	for _, t := range highlight.Go(src) {
//	    if t.Kind == highlight.Plain {
//	        out = append(out, Text(t.Text))
//	    } else {
//	        out = append(out, Span(Class("tok-"+t.Kind.String()), Text(t.Text)))
//	    }
//	}
package highlight

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind classifies a token.
type Kind uint8

const (
	Plain   Kind = iota // identifiers, operators, punctuation, whitespace
	Keyword             // Go keywords; HTML tag names; shell commands
	String              // string, raw string and rune literals; HTML attribute values
	Comment             // line and block comments
	Number              // numeric literals
	Builtin             // Go predeclared types, constants and functions
	Func                // a name being called or declared: Println in fmt.Println(…)
	Attr                // HTML attribute names; shell flags
)

var kindNames = [...]string{"plain", "keyword", "string", "comment", "number", "builtin", "func", "attr"}

// String is the kind's lowercase name, suitable for a CSS class.
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "plain"
}

// Token is a run of source text of one kind.
type Token struct {
	Kind Kind
	Text string
}

// For picks a lexer by language name or file extension: "go", "html",
// "sh"/"shell"/"bash". Any other language gets one Plain token.
func For(lang, src string) []Token {
	switch strings.ToLower(strings.TrimPrefix(lang, ".")) {
	case "go":
		return Go(src)
	case "html", "htm":
		return HTML(src)
	case "sh", "shell", "bash", "console":
		return Shell(src)
	}
	return []Token{{Plain, src}}
}

// tokens accumulates tokens, merging neighbours of the same kind.
type tokens []Token

func (ts *tokens) add(k Kind, s string) {
	if s == "" {
		return
	}
	if n := len(*ts); n > 0 && (*ts)[n-1].Kind == k {
		(*ts)[n-1].Text += s
		return
	}
	*ts = append(*ts, Token{k, s})
}

var goKeywords = set("break", "case", "chan", "const", "continue", "default", "defer", "else",
	"fallthrough", "for", "func", "go", "goto", "if", "import", "interface", "map", "package",
	"range", "return", "select", "struct", "switch", "type", "var")

var goBuiltins = set("any", "bool", "byte", "comparable", "complex64", "complex128", "error",
	"float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint",
	"uint8", "uint16", "uint32", "uint64", "uintptr", "true", "false", "iota", "nil",
	"append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make",
	"max", "min", "new", "panic", "print", "println", "real", "recover")

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// Go tokenizes Go source.
func Go(src string) []Token {
	var ts tokens
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case strings.HasPrefix(src[i:], "//"):
			end := lineEnd(src, i)
			ts.add(Comment, src[i:end])
			i = end
		case strings.HasPrefix(src[i:], "/*"):
			end := len(src)
			if j := strings.Index(src[i+2:], "*/"); j >= 0 {
				end = i + 2 + j + 2
			}
			ts.add(Comment, src[i:end])
			i = end
		case c == '"' || c == '\'':
			end := quoted(src, i, c)
			ts.add(String, src[i:end])
			i = end
		case c == '`':
			end := len(src)
			if j := strings.IndexByte(src[i+1:], '`'); j >= 0 {
				end = i + 1 + j + 1
			}
			ts.add(String, src[i:end])
			i = end
		case isDigit(c) || (c == '.' && i+1 < len(src) && isDigit(src[i+1])):
			end := number(src, i)
			ts.add(Number, src[i:end])
			i = end
		case isIdentStart(src, i):
			end := ident(src, i)
			word := src[i:end]
			switch {
			case goKeywords[word]:
				ts.add(Keyword, word)
			case goBuiltins[word] && !afterDot(src, i):
				ts.add(Builtin, word)
			case end < len(src) && src[end] == '(':
				ts.add(Func, word)
			default:
				ts.add(Plain, word)
			}
			i = end
		default:
			_, size := utf8.DecodeRuneInString(src[i:])
			ts.add(Plain, src[i:i+size])
			i += size
		}
	}
	return ts
}

// HTML tokenizes HTML markup: tag names, attribute names and values, and
// comments. Text between tags and the content of <script> and <style> are
// Plain.
func HTML(src string) []Token {
	var ts tokens
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(src[i:], "<!--"):
			end := len(src)
			if j := strings.Index(src[i+4:], "-->"); j >= 0 {
				end = i + 4 + j + 3
			}
			ts.add(Comment, src[i:end])
			i = end
		case src[i] == '<' && i+1 < len(src) && (isASCIILetter(src[i+1]) || src[i+1] == '/' || src[i+1] == '!'):
			i = htmlTag(src, i, &ts)
		default:
			end := strings.IndexByte(src[i+1:], '<')
			if end < 0 {
				end = len(src)
			} else {
				end += i + 1
			}
			ts.add(Plain, src[i:end])
			i = end
		}
	}
	return ts
}

// htmlTag tokenizes one tag starting at src[i] == '<' and returns where it
// ends.
func htmlTag(src string, i int, ts *tokens) int {
	start := i
	i++ // '<'
	if i < len(src) && (src[i] == '/' || src[i] == '!') {
		i++
	}
	ts.add(Plain, src[start:i])
	name := i
	for i < len(src) && (isASCIILetter(src[i]) || isDigit(src[i]) || src[i] == '-') {
		i++
	}
	ts.add(Keyword, src[name:i])
	for i < len(src) && src[i] != '>' {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			end := strings.IndexByte(src[i+1:], c)
			if end < 0 {
				end = len(src)
			} else {
				end += i + 2
			}
			ts.add(String, src[i:end])
			i = end
		case isASCIILetter(c) || c == '-' || c == '_' || c == ':':
			end := i
			for end < len(src) && (isASCIILetter(src[end]) || isDigit(src[end]) || strings.IndexByte("-_:.", src[end]) >= 0) {
				end++
			}
			ts.add(Attr, src[i:end])
			i = end
		default:
			ts.add(Plain, src[i:i+1])
			i++
		}
	}
	if i < len(src) {
		ts.add(Plain, ">")
		i++
	}
	return i
}

// Shell tokenizes shell commands: the command word of each line, flags,
// quoted strings, $(…) and $VAR expansions (as Builtin), and # comments.
func Shell(src string) []Token {
	var ts tokens
	commandNext := true // the next word starts a command
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '#' && (i == 0 || src[i-1] == ' ' || src[i-1] == '\n' || src[i-1] == '\t'):
			end := lineEnd(src, i)
			ts.add(Comment, src[i:end])
			i = end
		case c == '"' || c == '\'':
			end := quoted(src, i, c)
			ts.add(String, src[i:end])
			i = end
			commandNext = false
		case c == '$':
			end := i + 1
			if end < len(src) && (src[end] == '(' || src[end] == '{') {
				end++
			}
			for end < len(src) && (isASCIILetter(src[end]) || isDigit(src[end]) || src[end] == '_') {
				end++
			}
			ts.add(Builtin, src[i:end])
			i = end
			commandNext = false
		case c == '\n' || c == ';' || c == '|' || c == '&':
			ts.add(Plain, src[i:i+1])
			i++
			commandNext = true
		case c == ' ' || c == '\t' || c == '\\':
			ts.add(Plain, src[i:i+1])
			i++
		default:
			end := i
			for end < len(src) && strings.IndexByte(" \t\n;|&\"'$", src[end]) < 0 {
				end++
			}
			if end == i { // a lone special character
				end = i + 1
			}
			word := src[i:end]
			switch {
			case commandNext && strings.Contains(word, "=") && !strings.HasPrefix(word, "="):
				ts.add(Attr, word) // an environment assignment: GOOS=js
			case commandNext:
				ts.add(Keyword, word)
				commandNext = false
			case strings.HasPrefix(word, "-"):
				ts.add(Attr, word)
			default:
				ts.add(Plain, word)
			}
			i = end
		}
	}
	return ts
}

func lineEnd(src string, i int) int {
	if j := strings.IndexByte(src[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(src)
}

// quoted returns the end of the quoted literal starting at src[i] == q. An
// unterminated literal ends at the end of the line.
func quoted(src string, i int, q byte) int {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case q:
			return j + 1
		case '\n':
			return j
		}
	}
	return len(src)
}

// number returns the end of the numeric literal at src[i]: digits, letters
// (0x, 1e9, 2i), '_' and '.', and the sign of an exponent (1e-9, 0x1p+4).
func number(src string, i int) int {
	hex := i+1 < len(src) && src[i] == '0' && (src[i+1] == 'x' || src[i+1] == 'X')
	j := i
	for j < len(src) {
		c := src[j]
		if c == '+' || c == '-' {
			p := src[j-1]
			if !hex && (p == 'e' || p == 'E') || hex && (p == 'p' || p == 'P') {
				j++
				continue
			}
			break
		}
		if !isDigit(c) && !isASCIILetter(c) && c != '_' && c != '.' {
			break
		}
		j++
	}
	return j
}

func isDigit(c byte) bool       { return c >= '0' && c <= '9' }
func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isIdentStart(src string, i int) bool {
	r, _ := utf8.DecodeRuneInString(src[i:])
	return r == '_' || unicode.IsLetter(r)
}

func ident(src string, i int) int {
	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		i += size
	}
	return i
}

// afterDot reports whether the identifier at src[i] is a selector (x.len),
// which is never the predeclared name.
func afterDot(src string, i int) bool { return i > 0 && src[i-1] == '.' }
