// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package render

import (
	"os"

	"github.com/tyler-sommer/stick"
)

// multiLoader is a stick.Loader that tries each configured sub-loader in
// order and returns the first successful load.
type multiLoader struct {
	loaders []stick.Loader
}

// Load implements stick.Loader.
func (l *multiLoader) Load(name string) (stick.Template, error) {
	var lastErr error
	for _, sub := range l.loaders {
		t, err := sub.Load(name)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, lastErr
}

// TwigRegistrar is the handle passed to plugins via OnTwigRegistered. It lets
// a plugin register additional template sources (directories or arbitrary
// stick.Loaders), Twig filters, and Twig functions without exposing stick
// internals directly.
//
// The registrar is a recipe: NewTwigRenderer applies the accumulated
// registrations every time a renderer is built, so plugin contributions made
// once at Site init take effect on every request.
//
// Lookup order for templates: the user's theme dir is always searched first,
// so theme files override plugin-shipped defaults by name.
type TwigRegistrar struct {
	loaders   []stick.Loader // plugin-added loaders; consulted after theme dir
	filters   map[string]stick.Filter
	functions map[string]stick.Func
	mutators  []func(env *stick.Env) // advanced escape hatch
}

// NewTwigRegistrar returns an empty registrar. Pigo creates one at Site init
// and passes it to plugins via OnTwigRegistered.
func NewTwigRegistrar() *TwigRegistrar {
	return &TwigRegistrar{
		filters:   map[string]stick.Filter{},
		functions: map[string]stick.Func{},
	}
}

// AddPath registers an additional template search directory. Convenience for
// AddLoader(stick.NewFilesystemLoader(dir)). Plugin-added sources are
// consulted after the theme dir.
func (r *TwigRegistrar) AddPath(dir string) {
	if dir == "" {
		return
	}
	r.loaders = append(r.loaders, stick.NewFilesystemLoader(dir))
}

// AddLoader registers an arbitrary stick.Loader. Useful for in-memory or
// embed.FS-backed templates via stick.MemoryLoader or a custom loader.
func (r *TwigRegistrar) AddLoader(l stick.Loader) {
	if l == nil {
		return
	}
	r.loaders = append(r.loaders, l)
}

// AddFilter registers (or overrides) a Twig filter.
func (r *TwigRegistrar) AddFilter(name string, fn stick.Filter) {
	r.filters[name] = fn
}

// AddFunction registers (or overrides) a Twig function.
func (r *TwigRegistrar) AddFunction(name string, fn stick.Func) {
	r.functions[name] = fn
}

// Mutate registers a callback that receives the raw stick.Env on each
// renderer build. Escape hatch for advanced plugins needing to register
// tests, tags, or other Twig extensions not covered by the narrow surface.
func (r *TwigRegistrar) Mutate(fn func(env *stick.Env)) {
	if fn != nil {
		r.mutators = append(r.mutators, fn)
	}
}

// apply replays the registrar's accumulated registrations onto a newly
// constructed env/loader pair. Called by NewTwigRenderer.
func (r *TwigRegistrar) apply(env *stick.Env, loader *multiLoader) {
	if r == nil {
		return
	}
	loader.loaders = append(loader.loaders, r.loaders...)
	for name, fn := range r.filters {
		env.Filters[name] = fn
	}
	for name, fn := range r.functions {
		env.Functions[name] = fn
	}
	for _, m := range r.mutators {
		m(env)
	}
}
