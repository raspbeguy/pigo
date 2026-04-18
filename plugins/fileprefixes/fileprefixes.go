// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package fileprefixes is a pigo port of the official Pico plugin
// PicoFilePrefixes (https://github.com/PhrozenByte/pico-file-prefixes).
//
// It removes a "<prefix>." segment from the filename of pages in configured
// directories — so `blog/20240101.hello.md` serves at `/blog/hello` and
// appears in page lists with that clean URL — while the file on disk keeps
// its prefix (useful for sorting, organizing).
//
// Configuration (under `PicoFilePrefixes:` in the user's config):
//
//	PicoFilePrefixes:
//	  recursiveDirs: ["blog"]  # strip prefixes from blog/** (default)
//	  dirs: []                  # and/or specific non-recursive dirs
//
// If both lists are empty, the plugin disables itself.
package fileprefixes

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/raspbeguy/pigo/config"
	"github.com/raspbeguy/pigo/content"
	"github.com/raspbeguy/pigo/plugin"
)

// Plugin is the PicoFilePrefixes Go port.
type Plugin struct {
	plugin.Base

	cfg      *config.Config
	pathRe   *regexp.Regexp
	pages    []*content.Page  // captured in OnPagesLoaded for later URL rewriting
	rewrites map[string]string // originalID → cleanID
}

// Name identifies the plugin; matches the PHP class name so config keys port 1:1.
func (*Plugin) Name() string        { return "PicoFilePrefixes" }
func (*Plugin) DependsOn() []string { return nil }

// HandleEvent dispatches on event name.
func (p *Plugin) HandleEvent(event string, params ...any) error {
	switch event {
	case plugin.OnConfigLoaded:
		return p.onConfigLoaded(params[0].(*config.Config))
	case plugin.OnPagesLoaded:
		return p.onPagesLoaded(params[0].([]*content.Page))
	case plugin.OnCurrentPageDiscovered:
		return p.onCurrentPageDiscovered()
	case plugin.OnRequestFile:
		return p.onRequestFile(params[0].(*string))
	}
	return nil
}

// onConfigLoaded captures the config pointer and builds the dir-match regex.
// If the user didn't configure the plugin at all, sensible defaults apply
// (strip prefixes from "blog/"). If both lists are explicitly empty the
// plugin disables itself.
func (p *Plugin) onConfigLoaded(cfg *config.Config) error {
	p.cfg = cfg

	raw, _ := cfg.Custom["PicoFilePrefixes"].(map[string]any)
	recursive := stringList(raw["recursiveDirs"])
	dirs := stringList(raw["dirs"])

	// PHP behavior: no PicoFilePrefixes key at all → defaults to
	// recursiveDirs=["blog"]; explicit config with both lists empty → disable.
	if raw == nil {
		recursive = []string{"blog"}
	} else if len(recursive) == 0 && len(dirs) == 0 {
		p.SetEnabled(false)
		return nil
	}

	p.pathRe = buildPathRegex(recursive, dirs)
	return nil
}

// onPagesLoaded walks all pages once at init and builds a rewrite map
// from prefixed id → clean id. Collisions between two prefixed candidates
// for the same clean id are resolved the PHP way: highest lexicographic
// original id wins.
func (p *Plugin) onPagesLoaded(pages []*content.Page) error {
	p.pages = pages

	// Non-prefixed pages by clean id — these block prefix rewrites to the
	// same clean id (a real page exists at that URL already).
	existing := make(map[string]struct{}, len(pages))
	for _, pg := range pages {
		existing[pg.ID] = struct{}{}
	}

	// Iterate in sorted order for deterministic collision resolution.
	sorted := make([]*content.Page, len(pages))
	copy(sorted, pages)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	// prefixOwner[cleanID] = original prefixed id currently assigned to it.
	prefixOwner := map[string]string{}
	p.rewrites = map[string]string{}

	for _, pg := range sorted {
		dir := filepath.ToSlash(filepath.Dir(pg.ID))
		if dir == "" {
			dir = "."
		}
		if !p.pathRe.MatchString(dir) {
			continue
		}
		base := filepath.Base(pg.ID)
		dot := strings.IndexByte(base, '.')
		if dot < 0 {
			continue
		}
		cleanBase := base[dot+1:]
		cleanID := cleanBase
		if dir != "." {
			cleanID = dir + "/" + cleanBase
		}
		// A real page already owns this clean id.
		if _, taken := existing[cleanID]; taken {
			continue
		}
		if prior, clash := prefixOwner[cleanID]; clash {
			// PHP: if current <= prior, current stays prefixed. Since we
			// iterate sorted ascending, current > prior always — but port
			// the comparison explicitly for clarity.
			if pg.ID <= prior {
				continue
			}
			// Current wins: revoke prior's rewrite, claim the clean id.
			delete(p.rewrites, prior)
		}
		prefixOwner[cleanID] = pg.ID
		p.rewrites[pg.ID] = cleanID
	}
	return nil
}

// onCurrentPageDiscovered fires per-request after pigo has populated p.URL
// for every page using the request-derived base URL. We apply the rewrite
// map by swapping the page-id suffix of each affected URL for its clean id.
// BuildContext runs AFTER this event, so the template context reflects the
// rewritten URLs.
func (p *Plugin) onCurrentPageDiscovered() error {
	if len(p.rewrites) == 0 {
		return nil
	}
	for _, pg := range p.pages {
		if clean, ok := p.rewrites[pg.ID]; ok {
			pg.URL = swapURLID(pg.URL, pg.ID, clean)
		}
	}
	return nil
}

// onRequestFile handles the inverse: a request arriving for the clean URL
// has no content file. If the requested file lives under a configured dir
// and is missing, glob for "<dir>/*.<basename>" and pick the last match
// (matching PHP's end(glob(...))).
func (p *Plugin) onRequestFile(filePath *string) error {
	if p.cfg == nil || p.pathRe == nil || *filePath == "" {
		return nil
	}
	if _, err := os.Stat(*filePath); err == nil {
		return nil
	}
	contentDir := p.contentDir()
	if contentDir == "" {
		return nil
	}
	rel, err := filepath.Rel(contentDir, *filePath)
	if err != nil {
		return nil
	}
	rel = filepath.ToSlash(rel)
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "" {
		dir = "."
	}
	if !p.pathRe.MatchString(dir) {
		return nil
	}
	base := filepath.Base(rel)
	var pattern string
	if dir == "." {
		pattern = filepath.Join(contentDir, "*."+base)
	} else {
		pattern = filepath.Join(contentDir, dir, "*."+base)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	*filePath = matches[len(matches)-1]
	return nil
}

// contentDir returns the absolute content directory that pigo resolved
// during Site.New and wrote back into cfg.ContentDir.
func (p *Plugin) contentDir() string {
	if p.cfg == nil {
		return ""
	}
	return p.cfg.ContentDir
}

// buildPathRegex mirrors PicoFilePrefixes.php regex construction.
// Special case: "." in recursiveDirs matches every directory.
func buildPathRegex(recursive, dirs []string) *regexp.Regexp {
	for _, d := range recursive {
		if d == "." {
			return regexp.MustCompile(`^.+$`)
		}
	}
	parts := []string{}
	if len(recursive) > 0 {
		q := make([]string, len(recursive))
		for i, d := range recursive {
			q[i] = regexp.QuoteMeta(d)
		}
		parts = append(parts, "(?:"+strings.Join(q, "|")+")(?:/.+)?")
	}
	if len(dirs) > 0 {
		q := make([]string, len(dirs))
		for i, d := range dirs {
			q[i] = regexp.QuoteMeta(d)
		}
		parts = append(parts, strings.Join(q, "|"))
	}
	if len(parts) == 0 {
		return regexp.MustCompile(`$^`) // matches nothing
	}
	// PHP builds `^(recursive-branch)|(dirs-branch)$` — ambiguous by
	// precedence but we replicate it verbatim for behavior parity.
	return regexp.MustCompile("^" + strings.Join(parts, "|") + "$")
}

// swapURLID swaps the trailing page-id segment of url with newID, honoring
// both router.PageURL encodings (/<id> and ?<id>) and the "/index" trim
// that PageURL applies.
func swapURLID(url, oldID, newID string) string {
	oldPath := strings.TrimSuffix(oldID, "/index")
	if oldPath == "index" {
		oldPath = ""
	}
	newPath := strings.TrimSuffix(newID, "/index")
	if newPath == "index" {
		newPath = ""
	}
	if oldPath == newPath {
		return url
	}
	if oldPath == "" {
		if strings.HasSuffix(url, "/") {
			return url + newPath
		}
		return url + "/" + newPath
	}
	if strings.HasSuffix(url, "/"+oldPath) {
		return strings.TrimSuffix(url, oldPath) + newPath
	}
	if strings.HasSuffix(url, "?"+oldPath) {
		return strings.TrimSuffix(url, oldPath) + newPath
	}
	return url
}

// stringList coerces a YAML value that may be a single string, []any, or
// []string into []string. Matches PHP's loose typing around config lists.
func stringList(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	}
	return nil
}
