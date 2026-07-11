package h

import (
	"encoding/json"
	"strings"

	"github.com/yogisalomo/goowee/core"
)

// Metadata declares page-level head metadata. Place it anywhere in a component
// tree; its children (<title>, <meta>, <link>, <script>, <style>) are collected
// during rendering and injected into the document <head>. Use like:
//
//	Metadata(
//	    Title("My Page"),
//	    Meta(Name("description"), Content("…")),
//	    Link(Rel("canonical"), Href("https://…")),
//	    Script(Type("application/ld+json"), Text(`{"@context":"…"}`)),
//	)
//
// See docs/devtools.md for the LLM-specific helpers.
func Metadata(children ...core.Node) *core.MetadataNode {
	return &core.MetadataNode{Children: children}
}

// Title declares the document title (<title>).
func Title(text string) *core.ElementNode { return El("title", Text(text)) }

// Meta adds a <meta> tag. Pass attribute items like Name(), Content(), etc.
func Meta(items ...core.Item) *core.ElementNode { return El("meta", items...) }

// Link adds a <link> tag (e.g. stylesheet, canonical, preload).
func Link(items ...core.Item) *core.ElementNode { return El("link", items...) }

// Script adds a <script> tag (e.g. JSON-LD structured data).
func Script(items ...core.Item) *core.ElementNode { return El("script", items...) }

// StyleEl adds an inline <style> tag. Named StyleEl to avoid collision with
// the Style attribute helper (which sets the element-level CSS style attr).
func StyleEl(items ...core.Item) *core.ElementNode { return El("style", items...) }

// NoScript adds a <noscript> tag.
func NoScript(items ...core.Item) *core.ElementNode { return El("noscript", items...) }

// Base adds a <base> tag.
func Base(items ...core.Item) *core.ElementNode { return El("base", items...) }

// JSONLD returns a <script type="application/ld+json"> tag with the JSON
// encoding of data. Use for schema.org structured data and LLM metadata.
// Panics if data cannot be marshalled (caught by ErrorBoundary at runtime).
func JSONLD(data any) *core.ElementNode {
	b, err := json.Marshal(data)
	if err != nil {
		panic("metadata: json.Marshal: " + err.Error())
	}
	return Script(Type("application/ld+json"), Text(string(b)))
}

// LLM declares LLM-related metadata: a machine-readable description of the
// page's purpose, structure, and content for LLM consumption. Emits:
//
//	<meta name="llm" content="purpose=…;context=…;keywords=…">
//
// And a corresponding JSON-LD block. Use on every page so LLMs can quickly
// determine whether the page is relevant to a user's query without parsing
// the full DOM.
func LLM(purpose, context string, keywords ...string) *core.ElementNode {
	content := "purpose=" + purpose + ";context=" + context
	for _, k := range keywords {
		content += ";keywords=" + k
	}
	return Meta(Name("llm"), Content(content))
}

// PageMeta configures high-level page metadata for the Page() constructor.
// A single PageMeta generates the full set of <title>, <meta>, <link>,
// JSON-LD, and LLM elements, propagated across Open Graph, Twitter Cards,
// and structured data automatically.
type PageMeta struct {
	// Title is the document title. It also populates og:title, twitter:title,
	// JSON-LD name, and LLM purpose.
	Title string
	// Description populates <meta name="description">, og:description,
	// twitter:description, JSON-LD description, and LLM context.
	Description string
	// Canonical URL for <link rel="canonical">.
	Canonical string
	// Author for <meta name="author">.
	Author string
	// Keywords for <meta name="keywords"> and LLM keywords.
	Keywords []string
	// Robots content for <meta name="robots"> (e.g. "index, follow").
	Robots string
	// Locale for og:locale (default "en_US").
	Locale string
	// SiteName for og:site_name.
	SiteName string
	// Image URL for og:image and twitter:image. When set, twitter:card
	// defaults to "summary_large_image".
	Image string
	// TwitterCard overrides the twitter:card type (default "summary_large_image"
	// when Image is set, otherwise not emitted).
	TwitterCard string
	// Charset for <meta charset>. Defaults to "utf-8".
	Charset string
	// Viewport for <meta name="viewport">. Defaults to
	// "width=device-width, initial-scale=1".
	Viewport string
}

// Page generates the full set of metadata elements from a config.
// Place it inside Metadata:
//
//	Metadata(
//	    Page(PageMeta{
//	        Title:       "My Page",
//	        Description: "A description of my page",
//	        Canonical:   "https://example.com/my-page",
//	        Keywords:    []string{"go", "wasm"},
//	    }),
//	)
func Page(cfg PageMeta) *core.FragmentNode {
	charset := cfg.Charset
	if charset == "" {
		charset = "utf-8"
	}
	viewport := cfg.Viewport
	if viewport == "" {
		viewport = "width=device-width, initial-scale=1"
	}

	var nodes []core.Node

	nodes = append(nodes, Meta(Attr("charset", charset)))
	nodes = append(nodes, Meta(Name("viewport"), Content(viewport)))

	if cfg.Title != "" {
		nodes = append(nodes,
			Title(cfg.Title),
			Meta(Attr("property", "og:title"), Content(cfg.Title)),
			Meta(Name("twitter:title"), Content(cfg.Title)),
		)
	}
	if cfg.Description != "" {
		nodes = append(nodes,
			Meta(Name("description"), Content(cfg.Description)),
			Meta(Attr("property", "og:description"), Content(cfg.Description)),
			Meta(Name("twitter:description"), Content(cfg.Description)),
		)
	}
	if cfg.Canonical != "" {
		nodes = append(nodes, Link(Rel("canonical"), Href(cfg.Canonical)))
	}
	if cfg.Author != "" {
		nodes = append(nodes, Meta(Name("author"), Content(cfg.Author)))
	}
	if len(cfg.Keywords) > 0 {
		nodes = append(nodes, Meta(Name("keywords"), Content(strings.Join(cfg.Keywords, ", "))))
	}
	if cfg.Robots != "" {
		nodes = append(nodes, Meta(Name("robots"), Content(cfg.Robots)))
	}
	locale := cfg.Locale
	if locale == "" {
		locale = "en_US"
	}
	nodes = append(nodes, Meta(Attr("property", "og:locale"), Content(locale)))
	if cfg.SiteName != "" {
		nodes = append(nodes, Meta(Attr("property", "og:site_name"), Content(cfg.SiteName)))
	}
	if cfg.Image != "" {
		nodes = append(nodes,
			Meta(Attr("property", "og:image"), Content(cfg.Image)),
			Meta(Name("twitter:image"), Content(cfg.Image)),
		)
		card := cfg.TwitterCard
		if card == "" {
			card = "summary_large_image"
		}
		nodes = append(nodes, Meta(Name("twitter:card"), Content(card)))
	}

	// JSON-LD structured data.
	if cfg.Title != "" || cfg.Description != "" {
		ld := map[string]any{
			"@context": "https://schema.org",
			"@type":    "WebPage",
		}
		if cfg.Title != "" {
			ld["name"] = cfg.Title
		}
		if cfg.Description != "" {
			ld["description"] = cfg.Description
		}
		nodes = append(nodes, JSONLD(ld))
	}

	// LLM meta.
	if cfg.Title != "" || cfg.Description != "" || len(cfg.Keywords) > 0 {
		nodes = append(nodes, LLM(cfg.Title, cfg.Description, cfg.Keywords...))
	}

	return Fragment(nodes...)
}
