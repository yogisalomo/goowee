package app

import (
	"strings"
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

// The live site is served under /goowee/ (GitHub Pages). Every in-app link
// must carry that base — a bare "/tutorial" href opens GitHub's 404 when the
// link is copied or opened in a new tab — and must leave modifier clicks to
// the browser.
func TestSiteLinksCarryBasePathAndAllowNewTabs(t *testing.T) {
	r := router.New("/")
	r.SetBasePath("/goowee")
	s := gooweetest.Render(t, App(r))
	pages := []string{"/", "/tutorial"}
	for _, st := range tutorialSteps {
		pages = append(pages, st.path)
	}
	checked := 0
	for _, p := range pages {
		r.Navigate(p)
		s.Flush()
		for _, a := range s.FindAll("a") {
			href := a.Attr("href")
			if strings.HasPrefix(href, "http") {
				continue // external (GitHub)
			}
			if !strings.HasPrefix(href, "/goowee/") {
				t.Fatalf("on %s: link %q has href %q without the /goowee base", p, a.Text(), href)
			}
			if s.Dispatch(a, "click", map[string]any{"button": 0, "metaKey": true}) {
				t.Fatalf("on %s: cmd-click on %q was prevented (hijacks open-in-new-tab)", p, a.Text())
			}
			if r.Path.Get() != p {
				t.Fatalf("on %s: cmd-click on %q navigated in-app to %s", p, a.Text(), r.Path.Get())
			}
			checked++
		}
	}
	if checked < 20 {
		t.Fatalf("only %d links checked — the walk missed pages", checked)
	}
}

func TestCanvasPageWithGooweetest(t *testing.T) {
	s := gooweetest.Render(t, canvasPage(router.New("/canvas")))
	bars := s.Find("canvas.bars-canvas")
	cmds := bars.Canvas()
	if len(cmds) != 8 || cmds[0].String() != `FillStyle("#00728f")` || cmds[1].String() != "FillRect(4, 80, 32, 40)" {
		t.Fatalf("bars: %v", cmds)
	}
	s.Click(s.FindByText("Shuffle"))
	if got := bars.Canvas()[1].String(); got != "FillRect(4, 42, 32, 78)" || bars.DrawCount() != 2 {
		t.Fatalf("after Shuffle: %s (%d draws)", got, bars.DrawCount())
	}

	field := s.Find("canvas.field-canvas")
	dots := 0
	for _, c := range field.Canvas() {
		if c.Op.String() == "Circles" {
			dots += len(c.Args) / 3
		}
	}
	if dots != 10000 || !strings.HasPrefix(s.Find(".field-stats").Text(), "10,000 dots · 117 KB per frame") {
		t.Fatalf("field: %d dots, stats %q", dots, s.Find(".field-stats").Text())
	}
	s.Input(s.Find(`input[type="range"]`), "20000")
	if !strings.HasPrefix(s.Find(".field-stats").Text(), "20,000 dots") || field.DrawCount() != 2 {
		t.Fatalf("after the slider: %q, %d draws", s.Find(".field-stats").Text(), field.DrawCount())
	}
}
