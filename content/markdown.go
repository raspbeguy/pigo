// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package content

import (
	"bytes"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
)

// Markdown wraps a goldmark instance configured to approximate Parsedown Extra,
// which is Pico's default. Extensions enabled: GFM (tables, strikethrough,
// linkified URLs, task lists), footnotes, definition lists.
//
// Plugins can register additional goldmark extensions via a MarkdownRegistrar
// during the OnMarkdownRegistered event; the internal goldmark instance is
// rebuilt on every registration so subsequent Render calls pick up the change.
type Markdown struct {
	mu         sync.RWMutex
	md         goldmark.Markdown
	exts       []goldmark.Extender
	parserOpts []parser.Option
	htmlOpts   []renderer.Option
}

// NewMarkdown builds a Markdown renderer honoring Pico's content_config keys:
//
//	extra: bool (enable GFM/extras)
//	breaks: bool (treat single newline as <br>)
//	escape: bool (escape raw HTML)
//	auto_urls: bool (linkify bare URLs)
func NewMarkdown(opts map[string]any) *Markdown {
	extra := truthy(opts, "extra", true)
	breaks := truthy(opts, "breaks", false)
	escape := truthy(opts, "escape", false)
	autoURLs := truthy(opts, "auto_urls", true)

	var exts []goldmark.Extender
	if extra {
		exts = append(exts,
			extension.Table,
			extension.Strikethrough,
			extension.TaskList,
			extension.Footnote,
			extension.DefinitionList,
		)
	}
	if autoURLs {
		exts = append(exts, extension.Linkify)
	}

	parserOpts := []parser.Option{parser.WithAutoHeadingID()}

	var htmlOpts []renderer.Option
	if breaks {
		htmlOpts = append(htmlOpts, html.WithHardWraps())
	}
	if !escape {
		htmlOpts = append(htmlOpts, html.WithUnsafe())
	}

	m := &Markdown{
		exts:       exts,
		parserOpts: parserOpts,
		htmlOpts:   htmlOpts,
	}
	m.rebuild()
	return m
}

// AddExtension registers a goldmark extension and rebuilds the internal
// goldmark instance. Intended for use during OnMarkdownRegistered.
func (m *Markdown) AddExtension(ext goldmark.Extender) {
	if ext == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exts = append(m.exts, ext)
	m.rebuildLocked()
}

func (m *Markdown) rebuild() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebuildLocked()
}

func (m *Markdown) rebuildLocked() {
	m.md = goldmark.New(
		goldmark.WithExtensions(m.exts...),
		goldmark.WithParserOptions(m.parserOpts...),
		goldmark.WithRendererOptions(m.htmlOpts...),
	)
}

// Render converts Markdown source to HTML.
func (m *Markdown) Render(src string) (string, error) {
	m.mu.RLock()
	md := m.md
	m.mu.RUnlock()
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderLine renders inline Markdown, stripping the outer <p>..</p> wrapper.
// Used by the {{ text|markdown(singleLine=true) }} filter variant.
func (m *Markdown) RenderLine(src string) (string, error) {
	s, err := m.Render(src)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<p>") && strings.HasSuffix(s, "</p>") {
		return s[len("<p>") : len(s)-len("</p>")], nil
	}
	return s, nil
}

// MarkdownRegistrar is the handle passed to plugins via OnMarkdownRegistered.
// Exposes a narrow surface so plugin code doesn't couple to internals.
type MarkdownRegistrar struct {
	md *Markdown
}

// NewMarkdownRegistrar wraps a *Markdown for plugin registration.
func NewMarkdownRegistrar(md *Markdown) *MarkdownRegistrar {
	return &MarkdownRegistrar{md: md}
}

// AddExtension adds a goldmark extension. Safe to call during
// OnMarkdownRegistered.
func (r *MarkdownRegistrar) AddExtension(ext goldmark.Extender) {
	r.md.AddExtension(ext)
}

func truthy(m map[string]any, key string, def bool) bool {
	v, ok := m[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "yes" || x == "1"
	case int:
		return x != 0
	case int64:
		return x != 0
	}
	return def
}
