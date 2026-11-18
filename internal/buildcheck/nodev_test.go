package buildcheck

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// #127: a goowee_nodev build of the minimal app links no goowee/devtools
// code, and a default build does (so the check can't pass vacuously).
func TestNodevBuildLinksNoDevtools(t *testing.T) {
	if testing.Short() {
		t.Skip("builds wasm twice")
	}
	const devtools = "github.com_yogisalomo_goowee_devtools."
	for _, tc := range []struct {
		tags string
		want bool
	}{{"", true}, {"goowee_nodev", false}} {
		names := buildNames(t, tc.tags)
		got := 0
		for _, n := range names {
			if strings.HasPrefix(n, devtools) {
				got++
			}
		}
		if (got > 0) != tc.want {
			t.Errorf("tags %q: %d goowee/devtools functions linked, want any=%v", tc.tags, got, tc.want)
		}
	}
}

// buildNames builds test/e2e/hello for js/wasm with tags and returns the
// function names in its name section.
func buildNames(t *testing.T, tags string) []string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "hello.wasm")
	args := []string{"build", "-o", out}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	cmd := exec.Command("go", append(args, "./test/e2e/hello")...)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build -tags %q: %v\n%s", tags, err, msg)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	names, err := functionNames(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 1000 {
		t.Fatalf("only %d function names: no name section?", len(names))
	}
	return names
}

// functionNames reads the "name" custom section's function-name subsection.
func functionNames(b []byte) ([]string, error) {
	if len(b) < 8 || !bytes.Equal(b[:4], []byte("\x00asm")) {
		return nil, errors.New("not a wasm file")
	}
	r := &leb{b: b, p: 8}
	for r.p < len(b) {
		id := b[r.p]
		r.p++
		size := int(r.u32())
		end := r.p + size
		if id == 0 && r.name() == "name" {
			for r.p < end {
				sub := b[r.p]
				r.p++
				subEnd := int(r.u32()) + r.p
				if sub == 1 {
					n := int(r.u32())
					names := make([]string, 0, n)
					for i := 0; i < n; i++ {
						r.u32()
						names = append(names, r.name())
					}
					return names, nil
				}
				r.p = subEnd
			}
		}
		r.p = end
	}
	return nil, errors.New("no function names")
}

type leb struct {
	b []byte
	p int
}

func (r *leb) u32() uint32 {
	v, n := binary.Uvarint(r.b[r.p:])
	r.p += n
	return uint32(v)
}

func (r *leb) name() string {
	n := int(r.u32())
	s := string(r.b[r.p : r.p+n])
	r.p += n
	return s
}
