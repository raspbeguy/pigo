// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package robots

import (
	"embed"
	"path"

	"github.com/tyler-sommer/stick"
)

//go:embed theme/*.twig
var themeFS embed.FS

// loadEmbeddedTemplates reads the plugin's bundled twig files into a
// stick.MemoryLoader so they are resolvable by name without touching disk.
// Filenames: "robots.twig", "sitemap.twig" (sans directory prefix).
func loadEmbeddedTemplates() (*stick.MemoryLoader, error) {
	entries, err := themeFS.ReadDir("theme")
	if err != nil {
		return nil, err
	}
	tmpls := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := themeFS.ReadFile(path.Join("theme", e.Name()))
		if err != nil {
			return nil, err
		}
		tmpls[e.Name()] = string(data)
	}
	return &stick.MemoryLoader{Templates: tmpls}, nil
}
