package host

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// browserGlobal matches Go code reading a browser global that goowee.host
// stands for. Code that needs one of them asks the host instead (#158).
var browserGlobal = regexp.MustCompile(`Get\("(document|location|history|localStorage|sessionStorage|window)"\)|Reflect"\)\.Call\("get"`)

// TestGoReachesTheBrowserThroughTheHost fails when a Go file outside the
// examples and tests reads document, location, history or storage directly.
func TestGoReachesTheBrowserThroughTheHost(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			switch rel {
			case "examples", "test", "graphify-out", "node_modules", "tools":
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if browserGlobal.MatchString(line) {
				t.Errorf("%s:%d reads a browser global directly; use internal/host: %s", rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
