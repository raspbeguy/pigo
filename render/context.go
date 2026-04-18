// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package render

import (
	"net/http"
	"sort"
	"strings"

	"github.com/raspbeguy/pigo/content"
	"github.com/raspbeguy/pigo/router"
	"github.com/raspbeguy/pigo/tree"
)

// Filters holds state needed by Pico-compatible template filters/functions.
// A fresh Filters is built per HTTP request so url_param/form_param work.
type Filters struct {
	Pages        []*content.Page
	Config       map[string]any
	Meta         map[string]any
	BaseURL      string
	Rewrite      bool
	Tree         *tree.Node
	Markdown     *content.Markdown
	Placeholders router.PlaceholderMap
	Req          *http.Request // nil-safe for url_param/form_param
}

// BuildContext assembles the template-facing map that Pico themes expect.
// Variables match the list Pico populates in Pico::getTwigVariables.
func BuildContext(
	cfg map[string]any,
	pages []*content.Page,
	currentPage, prevPage, nextPage *content.Page,
	pageTree *tree.Node,
	baseURL, themeURL, themesURL, assetsURL, pluginsURL, version, siteTitle string,
	meta map[string]any,
	content string,
) map[string]any {
	// Pages is emitted as an ordered []any so {% for p in pages %} yields the
	// configured sort order. A separate pages_by_id map supports direct lookup.
	pagesList := make([]any, 0, len(pages))
	pagesByID := make(map[string]any, len(pages))
	for _, p := range pages {
		m := p.AsMap()
		pagesList = append(pagesList, m)
		pagesByID[p.ID] = m
	}
	return map[string]any{
		"config":        cfg,
		"base_url":      baseURL,
		"theme_url":     themeURL,
		"themes_url":    themesURL,
		"assets_url":    assetsURL,
		"plugins_url":   pluginsURL,
		"site_title":    siteTitle,
		"version":       version,
		"meta":          meta,
		"content":       content,
		"pages":         pagesList,
		"pages_by_id":   pagesByID,
		"current_page":  currentPage.AsMap(),
		"previous_page": prevPage.AsMap(),
		"next_page":     nextPage.AsMap(),
		"page_tree":     pageTree.AsMap(),
	}
}

// MarkdownFilter parses a Markdown string with optional placeholder
// substitution. Matches Pico's {{ text|markdown(meta) }} filter.
func (f *Filters) MarkdownFilter(text string, meta map[string]any, singleLine bool) string {
	if meta != nil {
		pm := f.Placeholders
		pm.Meta = meta
		text = pm.Substitute(text)
	}
	if f.Markdown == nil {
		return text
	}
	var out string
	var err error
	if singleLine {
		out, err = f.Markdown.RenderLine(text)
	} else {
		out, err = f.Markdown.Render(text)
	}
	if err != nil {
		return ""
	}
	return out
}

// URLFilter resolves Pico URL placeholders (%base_url%, %meta.*%, ...).
func (f *Filters) URLFilter(s string) string { return f.Placeholders.Substitute(s) }

// LinkFilter returns the public URL for a given page id.
func (f *Filters) LinkFilter(pageID string) string {
	return router.PageURL(f.BaseURL, pageID, f.Rewrite)
}

// ContentFilter returns the rendered HTML of another page by id.
func (f *Filters) ContentFilter(pageID string) string {
	for _, p := range f.Pages {
		if p.ID == pageID {
			if p.Content != "" {
				return p.Content
			}
			// Lazily render if not already parsed.
			if f.Markdown != nil {
				pm := f.Placeholders
				pm.Meta = p.Meta
				html, err := f.Markdown.Render(pm.Substitute(p.RawContent))
				if err == nil {
					p.Content = html
					return html
				}
			}
			return p.RawContent
		}
	}
	return ""
}

// SortByFilter sorts a list of maps by a dotted key path, per Pico semantics.
// fallback: "top" | "bottom" | "keep" | "remove".
func (f *Filters) SortByFilter(items []any, keyPath, fallback string) []any {
	type entry struct {
		idx   int
		value any
		key   any
		has   bool
	}
	entries := make([]entry, 0, len(items))
	for i, it := range items {
		k, ok := dig(it, keyPath)
		entries = append(entries, entry{idx: i, value: it, key: k, has: ok})
	}
	// Stable sort among entries that have the key.
	sort.SliceStable(entries, func(i, j int) bool {
		// Handle fallback positioning.
		if !entries[i].has && entries[j].has {
			return fallback == "top"
		}
		if entries[i].has && !entries[j].has {
			return fallback == "bottom"
		}
		if !entries[i].has && !entries[j].has {
			return entries[i].idx < entries[j].idx
		}
		return less(entries[i].key, entries[j].key)
	})
	out := make([]any, 0, len(entries))
	for _, e := range entries {
		if !e.has && fallback == "remove" {
			continue
		}
		if !e.has && fallback == "keep" {
			// Preserve original index placement: emit where it was.
		}
		out = append(out, e.value)
	}
	return out
}

// MapFilter extracts values at keyPath from each item.
func (f *Filters) MapFilter(items []any, keyPath string) []any {
	out := make([]any, 0, len(items))
	for _, it := range items {
		if v, ok := dig(it, keyPath); ok {
			out = append(out, v)
		}
	}
	return out
}

// URLParam returns the first GET query param matching name, or "".
func (f *Filters) URLParam(name string) string {
	if f.Req == nil {
		return ""
	}
	return f.Req.URL.Query().Get(name)
}

// FormParam returns the first POST form param matching name, or "".
func (f *Filters) FormParam(name string) string {
	if f.Req == nil {
		return ""
	}
	_ = f.Req.ParseForm()
	return f.Req.PostFormValue(name)
}

// PagesQuery mirrors Pico's pages(start, depth, depthOffset, offset) function.
func (f *Filters) PagesQuery(start string, depth, depthOffset, offset int) []map[string]any {
	if f.Tree == nil {
		return nil
	}
	pages := f.Tree.Query(start, depth, depthOffset, offset)
	out := make([]map[string]any, 0, len(pages))
	for _, p := range pages {
		out = append(out, p.AsMap())
	}
	return out
}

// dig walks dotted paths on map/struct-like values.
func dig(v any, path string) (any, bool) {
	cur := v
	if path == "" {
		return cur, true
	}
	for _, seg := range strings.Split(path, ".") {
		switch x := cur.(type) {
		case map[string]any:
			val, ok := x[seg]
			if !ok {
				return nil, false
			}
			cur = val
		case map[any]any:
			val, ok := x[seg]
			if !ok {
				return nil, false
			}
			cur = val
		default:
			return nil, false
		}
	}
	return cur, true
}

func less(a, b any) bool {
	switch av := a.(type) {
	case string:
		bv, _ := b.(string)
		return av < bv
	case int:
		bv, _ := b.(int)
		return av < bv
	case int64:
		bv, _ := b.(int64)
		return av < bv
	case float64:
		bv, _ := b.(float64)
		return av < bv
	}
	return false
}
