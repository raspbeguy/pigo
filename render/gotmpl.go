// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package render

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// GoRenderer uses Go's html/template package. Selectable via
// template_engine: go in config.
type GoRenderer struct {
	tmpl    *template.Template
	filters *Filters
	themeDir string
}

// NewGoRenderer walks themeDir, parsing all .html files into one template set.
func NewGoRenderer(themeDir string, filters *Filters) (*GoRenderer, error) {
	r := &GoRenderer{filters: filters, themeDir: themeDir}
	funcs := template.FuncMap{
		"markdown": func(text string, args ...any) template.HTML {
			var meta map[string]any
			singleLine := false
			if len(args) >= 1 {
				if m, ok := args[0].(map[string]any); ok {
					meta = m
				}
			}
			if len(args) >= 2 {
				if b, ok := args[1].(bool); ok {
					singleLine = b
				}
			}
			return template.HTML(filters.MarkdownFilter(text, meta, singleLine))
		},
		"url":  func(s string) string { return filters.URLFilter(s) },
		"link": func(id string) string { return filters.LinkFilter(id) },
		"content": func(id string) template.HTML {
			return template.HTML(filters.ContentFilter(id))
		},
		"sort_by": func(items any, keyPath string, fallback ...string) []any {
			fb := "bottom"
			if len(fallback) > 0 {
				fb = fallback[0]
			}
			return filters.SortByFilter(coerceSlice(items), keyPath, fb)
		},
		"map": func(items any, keyPath string) []any {
			return filters.MapFilter(coerceSlice(items), keyPath)
		},
		"url_param":  filters.URLParam,
		"form_param": filters.FormParam,
		"pages": func(args ...any) []map[string]any {
			start, depth, depthOffset, offset := "", 0, 0, 1
			if len(args) >= 1 {
				if s, ok := args[0].(string); ok {
					start = s
				}
			}
			if len(args) >= 2 {
				depth = toInt(args[1])
			}
			if len(args) >= 3 {
				depthOffset = toInt(args[2])
			}
			if len(args) >= 4 {
				offset = toInt(args[3])
			}
			return filters.PagesQuery(start, depth, depthOffset, offset)
		},
		// Safe marks a string as pre-escaped HTML — convenient shorthand.
		"safe": func(s string) template.HTML { return template.HTML(s) },
	}
	t := template.New("").Funcs(funcs)
	err := filepath.WalkDir(themeDir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".html") {
			return nil
		}
		rel, err := filepath.Rel(themeDir, path)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".html")
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := t.New(name).Parse(string(data)); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.tmpl = t
	return r, nil
}

func (r *GoRenderer) Render(name string, ctx map[string]any) ([]byte, error) {
	if name == "" {
		name = "index"
	}
	// html/template auto-escapes everything; caller uses `safe` or we pre-mark
	// content as template.HTML.
	ctx2 := make(map[string]any, len(ctx))
	for k, v := range ctx {
		ctx2[k] = v
	}
	if html, ok := ctx["content"].(string); ok {
		ctx2["content"] = template.HTML(html)
	}
	tmpl := r.tmpl.Lookup(name)
	if tmpl == nil {
		return nil, fmt.Errorf("template %q not found in %s", name, r.themeDir)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx2); err != nil {
		return nil, fmt.Errorf("go-template render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

func coerceSlice(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = e
		}
		return out
	case map[string]any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			out = append(out, e)
		}
		return out
	}
	return nil
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}
