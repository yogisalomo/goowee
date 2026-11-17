package highlight

import (
	"strings"
	"testing"
)

// render writes tokens as [kind:text] for non-plain tokens.
func render(ts []Token) string {
	var b strings.Builder
	for _, t := range ts {
		if t.Kind == Plain {
			b.WriteString(t.Text)
		} else {
			b.WriteString("[" + t.Kind.String() + ":" + t.Text + "]")
		}
	}
	return b.String()
}

func TestGo(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`func main() {}`, `[keyword:func] [func:main]() {}`},
		{`count, setCount := hooks.UseState(0) // a signal`,
			`count, setCount := hooks.[func:UseState]([number:0]) [comment:// a signal]`},
		{`x := "a \"q\" b" + 'c' + ` + "`raw\nline`", `x := [string:"a \"q\" b"] + [string:'c'] + [string:` + "`raw\nline`" + `]`},
		{`var n int = len(s)`, `[keyword:var] n [builtin:int] = [builtin:len](s)`},
		{`v.len()`, `v.[func:len]()`},
		{`if err != nil { return }`, `[keyword:if] err != [builtin:nil] { [keyword:return] }`},
		{`a := 0x1F + 1e-9 + 3.5 + 1_000 - 2`, `a := [number:0x1F] + [number:1e-9] + [number:3.5] + [number:1_000] - [number:2]`},
		{"/* block\ncomment */ x", "[comment:/* block\ncomment */] x"},
		{`s := "unterminated` + "\n" + `y`, `s := [string:"unterminated]` + "\n" + `y`},
		{`Text("héllo…")`, `[func:Text]([string:"héllo…"])`},
		{`for i, v := range xs {`, `[keyword:for] i, v := [keyword:range] xs {`},
		{`[]core.Node{}`, `[]core.Node{}`},
	} {
		if got := render(Go(tc.src)); got != tc.want {
			t.Errorf("Go(%q)\n got %s\nwant %s", tc.src, got, tc.want)
		}
	}
}

func TestHTML(t *testing.T) {
	src := `<!doctype html>
<!-- shell -->
<script src="wasm_exec.js"></script>
<div id='root'>Loading…</div>`
	want := `<![keyword:doctype] [attr:html]>
[comment:<!-- shell -->]
<[keyword:script] [attr:src]=[string:"wasm_exec.js"]></[keyword:script]>
<[keyword:div] [attr:id]=[string:'root']>Loading…</[keyword:div]>`
	if got := render(HTML(src)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestShell(t *testing.T) {
	src := `GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/app
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/ # the glue
cd web && python3 -m http.server 8080`
	want := `[attr:GOOS=js] [attr:GOARCH=wasm] [keyword:go] build [attr:-o] web/main.wasm ./cmd/app
[keyword:cp] [string:"$(go env GOROOT)/lib/wasm/wasm_exec.js"] web/ [comment:# the glue]
[keyword:cd] web && [keyword:python3] [attr:-m] http.server 8080`
	if got := render(Shell(src)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFor(t *testing.T) {
	if got := For("txt", "a // b"); len(got) != 1 || got[0].Kind != Plain {
		t.Fatalf("unknown language: %v", got)
	}
	if got := For(".go", "x := 1"); render(got) != "x := [number:1]" {
		t.Fatalf(".go: %s", render(got))
	}
	if Kind(200).String() != "plain" {
		t.Fatal("out-of-range kind")
	}
}

// Joining the tokens always gives back the input, whatever it is, and no two
// neighbours share a kind.
func TestLossless(t *testing.T) {
	inputs := []string{"", "\"", "`", "'", "/*", "<", "<a", "<a b='", "$", "#", "0x", "1e", "1e+", "…", "\xff\xfe",
		"x := `a", "<!--", "a <b> c", "echo $HOME ${X} $(pwd)", "func(\n", "1.2.3", ".5"}
	for _, in := range inputs {
		for name, lex := range map[string]func(string) []Token{"go": Go, "html": HTML, "shell": Shell} {
			ts := lex(in)
			var b strings.Builder
			for i, tok := range ts {
				b.WriteString(tok.Text)
				if tok.Text == "" {
					t.Errorf("%s(%q): empty token", name, in)
				}
				if i > 0 && ts[i-1].Kind == tok.Kind {
					t.Errorf("%s(%q): unmerged neighbours", name, in)
				}
			}
			if b.String() != in {
				t.Errorf("%s(%q) lost text: %q", name, in, b.String())
			}
		}
	}
}

func FuzzLossless(f *testing.F) {
	f.Add("func main() { fmt.Println(\"hi\") } // x")
	f.Add("<a href='x'>y</a>")
	f.Add("GOOS=js go build -o x . # c")
	f.Fuzz(func(t *testing.T, in string) {
		for _, lex := range []func(string) []Token{Go, HTML, Shell} {
			var b strings.Builder
			for _, tok := range lex(in) {
				b.WriteString(tok.Text)
			}
			if b.String() != in {
				t.Fatalf("lost text for %q", in)
			}
		}
	})
}
