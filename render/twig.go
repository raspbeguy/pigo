// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package render

import (
	"bytes"
	"fmt"

	"github.com/tyler-sommer/stick"
	"github.com/tyler-sommer/stick/twig"
)

// TwigRenderer is the stick-backed Twig engine.
type TwigRenderer struct {
	env     *stick.Env
	filters *Filters
}

// NewTwigRenderer builds a Twig renderer loading templates from themeDir.
// If reg is non-nil, its accumulated plugin registrations (extra template
// paths, filters, functions) are applied to the env. Pass nil when you don't
// need plugin contributions.
func NewTwigRenderer(themeDir string, filters *Filters, reg *TwigRegistrar) *TwigRenderer {
	loader := &multiLoader{loaders: []stick.Loader{stick.NewFilesystemLoader(themeDir)}}
	env := twig.New(loader)
	r := &TwigRenderer{env: env, filters: filters}
	r.registerPicoFilters()
	reg.apply(env, loader)
	return r
}

func (r *TwigRenderer) registerPicoFilters() {
	f := r.filters
	r.env.Filters["markdown"] = func(ctx stick.Context, val stick.Value, args ...stick.Value) stick.Value {
		text := stick.CoerceString(val)
		var meta map[string]any
		singleLine := false
		if len(args) >= 1 {
			if m, ok := args[0].(map[string]any); ok {
				meta = m
			}
		}
		if len(args) >= 2 {
			singleLine = stick.CoerceBool(args[1])
		}
		return stick.NewSafeValue(f.MarkdownFilter(text, meta, singleLine), "html")
	}
	r.env.Filters["url"] = func(ctx stick.Context, val stick.Value, args ...stick.Value) stick.Value {
		return f.URLFilter(stick.CoerceString(val))
	}
	r.env.Filters["link"] = func(ctx stick.Context, val stick.Value, args ...stick.Value) stick.Value {
		return f.LinkFilter(stick.CoerceString(val))
	}
	r.env.Filters["content"] = func(ctx stick.Context, val stick.Value, args ...stick.Value) stick.Value {
		return stick.NewSafeValue(f.ContentFilter(stick.CoerceString(val)), "html")
	}
	r.env.Filters["sort_by"] = func(ctx stick.Context, val stick.Value, args ...stick.Value) stick.Value {
		items := toAnySlice(val)
		keyPath := ""
		fallback := "bottom"
		if len(args) >= 1 {
			keyPath = stick.CoerceString(args[0])
		}
		if len(args) >= 2 {
			fallback = stick.CoerceString(args[1])
		}
		return f.SortByFilter(items, keyPath, fallback)
	}
	r.env.Filters["map"] = func(ctx stick.Context, val stick.Value, args ...stick.Value) stick.Value {
		items := toAnySlice(val)
		keyPath := ""
		if len(args) >= 1 {
			keyPath = stick.CoerceString(args[0])
		}
		return f.MapFilter(items, keyPath)
	}

	r.env.Functions["url_param"] = func(ctx stick.Context, args ...stick.Value) stick.Value {
		name, filter, def := paramArgs(args)
		return f.URLParam(name, filter, def)
	}
	r.env.Functions["form_param"] = func(ctx stick.Context, args ...stick.Value) stick.Value {
		name, filter, def := paramArgs(args)
		return f.FormParam(name, filter, def)
	}
	r.env.Functions["pages"] = func(ctx stick.Context, args ...stick.Value) stick.Value {
		start := ""
		depth := 0
		depthOffset := 0
		offset := 1
		if len(args) >= 1 {
			start = stick.CoerceString(args[0])
		}
		if len(args) >= 2 {
			depth = int(stick.CoerceNumber(args[1]))
		}
		if len(args) >= 3 {
			depthOffset = int(stick.CoerceNumber(args[2]))
		}
		if len(args) >= 4 {
			offset = int(stick.CoerceNumber(args[3]))
		}
		return f.PagesQuery(start, depth, depthOffset, offset)
	}
}

// paramArgs unpacks (name, filter, default) from a url_param/form_param call.
// Mirrors Pico's signature: url_param(name, filter='', default=null).
func paramArgs(args []stick.Value) (name, filter string, def any) {
	if len(args) >= 1 {
		name = stick.CoerceString(args[0])
	}
	if len(args) >= 2 {
		filter = stick.CoerceString(args[1])
	}
	if len(args) >= 3 {
		def = args[2]
	}
	return
}

// Render executes the named template (without extension — ".twig" is appended).
func (r *TwigRenderer) Render(name string, ctx map[string]any) ([]byte, error) {
	if name == "" {
		name = "index"
	}
	tmpl := name
	if len(tmpl) < 5 || tmpl[len(tmpl)-5:] != ".twig" {
		tmpl = tmpl + ".twig"
	}
	var buf bytes.Buffer
	// stick's context type is map[string]stick.Value (stick.Value is interface{}).
	sctx := make(map[string]stick.Value, len(ctx))
	for k, v := range ctx {
		sctx[k] = v
	}
	// Mark the rendered HTML content as safe so stick doesn't escape the <tags>.
	if html, ok := ctx["content"].(string); ok {
		sctx["content"] = stick.NewSafeValue(html, "html")
	}
	if err := r.env.Execute(tmpl, &buf, sctx); err != nil {
		return nil, fmt.Errorf("twig render %s: %w", tmpl, err)
	}
	return buf.Bytes(), nil
}

// toAnySlice coerces a stick.Value to []any. Supports []any, []map[string]any,
// and map[string]any (values only).
func toAnySlice(v stick.Value) []any {
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
