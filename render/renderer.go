// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package render provides pigo's templating: the Renderer interface with Twig
// (stick) and Go (html/template) implementations, plus the shared context
// builder and filter functions used by both.
package render

// Renderer is the common surface for pigo's template engines.
type Renderer interface {
	// Render renders a template by name (e.g. "index"). The template is
	// looked up in the theme dir with the engine's conventional extension
	// (.twig or .html).
	Render(template string, ctx map[string]any) ([]byte, error)
}
