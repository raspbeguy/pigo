// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package content models pigo's pages: discovery, meta parsing, Markdown rendering.
package content

// Page is a single content file. Field layout matches the Pico page struct
// exposed to templates (see Pico.php::readPages). Templates expect these exact
// lowercased keys when accessed as map data.
type Page struct {
	ID            string         `json:"id"`             // relative path sans extension
	URL           string         `json:"url"`            // full URL
	Title         string         `json:"title"`          // meta.title (hoisted)
	Description   string         `json:"description"`    // meta.description (hoisted)
	Author        string         `json:"author"`         // meta.author (hoisted)
	Date          string         `json:"date"`           // raw meta.date
	DateFormatted string         `json:"date_formatted"` // strftime(date_format, Time)
	Time          int64          `json:"time"`           // Unix timestamp
	Hidden        bool           `json:"hidden"`
	RawContent    string         `json:"raw_content"`
	Content       string         `json:"content"` // rendered HTML; only set for current page
	Meta          map[string]any `json:"meta"`

	// ModificationTime is the Unix timestamp of the content file's mtime at
	// scan time. Mirrors Pico's $pageData['modificationTime']. Primarily
	// used by sitemap-style plugins.
	ModificationTime int64 `json:"modification_time"`

	// PrevPage / NextPage are populated after sorting; they are *Page to avoid
	// recursion when serializing.
	PrevPage *Page `json:"prev_page,omitempty"`
	NextPage *Page `json:"next_page,omitempty"`

	// Path is the absolute filesystem path (internal; not exposed to templates).
	Path string `json:"-"`
}

// AsMap returns the page as a map for template contexts. Both renderers
// (stick and html/template) handle maps uniformly, and Pico templates access
// fields by lowercased key (e.g. current_page.title).
func (p *Page) AsMap() map[string]any {
	if p == nil {
		return nil
	}
	m := map[string]any{
		"id":                p.ID,
		"url":               p.URL,
		"title":             p.Title,
		"description":       p.Description,
		"author":            p.Author,
		"date":              p.Date,
		"date_formatted":    p.DateFormatted,
		"time":              p.Time,
		"hidden":            p.Hidden,
		"raw_content":       p.RawContent,
		"content":           p.Content,
		"meta":              p.Meta,
		"modification_time": p.ModificationTime,
	}
	if p.PrevPage != nil {
		m["prev_page"] = p.PrevPage.shallowMap()
	}
	if p.NextPage != nil {
		m["next_page"] = p.NextPage.shallowMap()
	}
	return m
}

// shallowMap returns the page without prev/next links — used when a page is
// referenced as another page's prev/next to avoid infinite nesting.
func (p *Page) shallowMap() map[string]any {
	if p == nil {
		return nil
	}
	return map[string]any{
		"id":                p.ID,
		"url":               p.URL,
		"title":             p.Title,
		"description":       p.Description,
		"author":            p.Author,
		"date":              p.Date,
		"date_formatted":    p.DateFormatted,
		"time":              p.Time,
		"hidden":            p.Hidden,
		"meta":              p.Meta,
		"modification_time": p.ModificationTime,
	}
}
