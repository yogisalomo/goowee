package app

import (
	"testing"

	"github.com/yogisalomo/goowee/gooweetest"
	"github.com/yogisalomo/goowee/router"
)

// The example pages, tested the way an app author would: through the public
// gooweetest harness (#76).

func TestCounterPageWithGooweetest(t *testing.T) {
	s := gooweetest.Render(t, counterPage(router.New("/counter")))
	s.Click(s.FindByText("Count: 0"))
	s.Click(s.FindByText("Count: 1"))
	s.FindByText("Count: 2")
	if s.Query("p.greeting") == nil {
		t.Fatal("greeting should start visible")
	}
	s.Click(s.FindByText("Toggle"))
	if s.Query("p.greeting") != nil || s.Query("p.hidden") == nil {
		t.Fatalf("Toggle should swap the greeting for the hidden note:\n%s", s.Find(".demo-card-body").HTML())
	}
}

func TestEventsPageWithGooweetest(t *testing.T) {
	s := gooweetest.Render(t, eventsPage(router.New("/events")))
	s.Click(s.FindByText("Inner button"))
	s.Click(s.FindByText("Stops propagation"))
	if got := s.Find(".event-counts").Text(); got != "Card clicks: 1 · inner: 1 · stopped: 1" {
		t.Fatalf("got %q", got)
	}
	area := s.Find("textarea.enter-send")
	s.Input(area, "hello")
	if !s.KeyDown(area, "Enter") {
		t.Fatal("Enter should be prevented")
	}
	if got := s.Find(".sent").Text(); got != "Sent: hello" {
		t.Fatalf("got %q", got)
	}
}

func TestAsyncPageWithGooweetest(t *testing.T) {
	s := gooweetest.Render(t, asyncPage(router.New("/async")))
	s.FindByText("Loading…")
	s.WaitForText("Loaded at ")
	s.Click(s.FindByText("Reload"))
	s.FindByText("Loading…")
	s.WaitForText("Loaded at ")
}

func TestWholeAppNavigationWithGooweetest(t *testing.T) {
	r := router.New("/")
	s := gooweetest.Render(t, App(r))
	s.Click(s.Find(`a[href="/tutorial"]`))
	s.FindByText("Learn goowee by example")
	s.Click(s.FindAll("ul.steps a")[1]) // Counter
	s.FindByText("Count: 0")
	if r.Path.Get() != "/counter" {
		t.Fatalf("path %q", r.Path.Get())
	}
}
