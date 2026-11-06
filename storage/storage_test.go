package storage

import (
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/internal/runtime"
)

func TestStore(t *testing.T) {
	defer Local.Clear()
	if _, ok := Local.Get("draft"); ok {
		t.Fatal("empty store has a value")
	}
	Local.Set("draft", "hello")
	Local.Set("theme", "dark")
	Session.Set("tab", "2")
	if v, ok := Local.Get("draft"); !ok || v != "hello" {
		t.Fatalf("Get after Set: %q %v", v, ok)
	}
	if got := strings.Join(Local.Keys(), ","); got != "draft,theme" {
		t.Fatalf("Keys %s (Session's keys must stay apart)", got)
	}
	Local.Remove("draft")
	if _, ok := Local.Get("draft"); ok {
		t.Fatal("Remove left the value")
	}
	Session.Clear()
	if len(Session.Keys()) != 0 {
		t.Fatal("Clear left keys")
	}
}

// During a server render there's no browser storage: nothing is read, and
// nothing written leaks to the next request.
func TestStoreIsEmptyDuringServerRender(t *testing.T) {
	defer Local.Clear()
	Local.Set("seen", "client")
	runtime.ServerRender(func() {
		if v, ok := Local.Get("seen"); ok {
			t.Errorf("a server render read %q", v)
		}
		Local.Set("leak", "request A")
		if len(Local.Keys()) != 0 {
			t.Error("a server render listed keys")
		}
	})
	if _, ok := Local.Get("leak"); ok {
		t.Fatal("a server render wrote to the store")
	}
}
