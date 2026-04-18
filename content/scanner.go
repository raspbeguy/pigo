// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package content

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/raspbeguy/pigo/plugin"
)

// Scanner discovers content files under a directory.
type Scanner struct {
	Dir        string // content directory
	Ext        string // file extension (e.g. ".md")
	DateFormat string // strftime for meta date_formatted

	// Dispatcher, when non-nil, receives per-page events during Load:
	// OnSinglePageLoading, OnSinglePageContent, OnMetaParsing, OnMetaParsed,
	// OnSinglePageLoaded. Tests and standalone callers may leave it nil.
	Dispatcher *plugin.Dispatcher
}

// ScanAll walks the content dir and returns pages (excluding 404.md and files
// prefixed "_"). The 404 page (if any) is returned separately via LoadErrorPage.
func (s *Scanner) ScanAll() ([]*Page, error) {
	if s.Dir == "" {
		return nil, nil
	}
	info, err := os.Stat(s.Dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("content dir %q: %w", s.Dir, err)
	}

	var pages []*Page
	err = filepath.WalkDir(s.Dir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), s.Ext) {
			return nil
		}
		// Compute id relative to content dir without extension.
		rel, err := filepath.Rel(s.Dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		id := strings.TrimSuffix(rel, s.Ext)

		// Skip 404.md anywhere; it's loaded on demand.
		if filepath.Base(id) == "404" {
			return nil
		}
		// Skip pages whose id starts with underscore (hidden from scan).
		base := filepath.Base(id)
		if strings.HasPrefix(base, "_") {
			return nil
		}

		p, err := s.Load(path, id)
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		if p == nil {
			// A plugin cancelled the load via OnSinglePageLoading.
			return nil
		}
		pages = append(pages, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Stable default order by id; final sort done by caller based on config.
	sort.Slice(pages, func(i, j int) bool { return pages[i].ID < pages[j].ID })
	return pages, nil
}

// Load reads a single content file and parses its front-matter. Returns
// (nil, nil) if a plugin cancelled the load by blanking *id in
// OnSinglePageLoading.
func (s *Scanner) Load(path, id string) (*Page, error) {
	// Let plugins veto the load or rewrite the id before we touch the disk.
	if err := s.dispatch(plugin.OnSinglePageLoading, &id); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var mtime int64
	if info, err := os.Stat(path); err == nil {
		mtime = info.ModTime().Unix()
	}

	rawContent := string(data)
	if err := s.dispatch(plugin.OnSinglePageContent, id, &rawContent); err != nil {
		return nil, err
	}

	return s.loadFromBytes([]byte(rawContent), path, id, mtime)
}

func (s *Scanner) loadFromBytes(data []byte, path, id string, mtime int64) (*Page, error) {
	yamlText, body := SplitFrontMatter(string(data))

	if err := s.dispatch(plugin.OnMetaParsing, &yamlText); err != nil {
		return nil, err
	}
	meta, err := ParseMeta(yamlText, s.DateFormat)
	if err != nil {
		return nil, err
	}
	if err := s.dispatch(plugin.OnMetaParsed, &meta); err != nil {
		return nil, err
	}

	p := &Page{
		ID:               id,
		Path:             path,
		RawContent:       body,
		Meta:             meta,
		ModificationTime: mtime,
	}
	if v, ok := meta["title"].(string); ok {
		p.Title = v
	}
	if v, ok := meta["description"].(string); ok {
		p.Description = v
	}
	if v, ok := meta["author"].(string); ok {
		p.Author = v
	}
	if v, ok := meta["date"].(string); ok {
		p.Date = v
	}
	if v, ok := meta["date_formatted"].(string); ok {
		p.DateFormatted = v
	}
	if v, ok := meta["time"].(int64); ok {
		p.Time = v
	}
	if v, ok := meta["hidden"].(bool); ok {
		p.Hidden = v
	}
	// A page whose id starts with underscore or is "index" inside a dir
	// starting with underscore is also considered hidden.
	if isHiddenByID(id) {
		p.Hidden = true
	}

	if err := s.dispatch(plugin.OnSinglePageLoaded, p); err != nil {
		return nil, err
	}
	return p, nil
}

// dispatch is a nil-safe wrapper: Scanner used outside a full Site (e.g. in
// unit tests that construct it directly) has no Dispatcher and all per-page
// events become no-ops.
func (s *Scanner) dispatch(event string, params ...any) error {
	if s.Dispatcher == nil {
		return nil
	}
	return s.Dispatcher.Dispatch(event, params...)
}

func isHiddenByID(id string) bool {
	for _, seg := range strings.Split(id, "/") {
		if strings.HasPrefix(seg, "_") {
			return true
		}
	}
	return false
}

// LoadErrorPage walks up from reqPath looking for a 404.md; returns nil if none.
// reqPath is relative to the content dir (e.g. "sub/missing").
func (s *Scanner) LoadErrorPage(reqPath string) (*Page, error) {
	dir := filepath.Dir(reqPath)
	for {
		candidate := filepath.Join(s.Dir, dir, "404"+s.Ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			id := strings.TrimSuffix(filepath.ToSlash(filepath.Join(dir, "404")), "/")
			if id == "" {
				id = "404"
			}
			return s.Load(candidate, id)
		}
		if dir == "." || dir == "/" || dir == "" {
			break
		}
		dir = filepath.Dir(dir)
	}
	return nil, nil
}

// SortPages sorts in-place per Pico semantics.
//
//	orderBy: "alpha" | "date" | "meta"
//	order:   "asc" | "desc"
//	metaKey: used when orderBy == "meta"
func SortPages(pages []*Page, orderBy, order, metaKey string) {
	desc := order == "desc"
	sort.SliceStable(pages, func(i, j int) bool {
		switch orderBy {
		case "date":
			a, b := pages[i].Time, pages[j].Time
			if desc {
				return a > b
			}
			return a < b
		case "meta":
			av, _ := pages[i].Meta[metaKey].(string)
			bv, _ := pages[j].Meta[metaKey].(string)
			if desc {
				return av > bv
			}
			return av < bv
		default: // alpha
			if desc {
				return pages[i].ID > pages[j].ID
			}
			return pages[i].ID < pages[j].ID
		}
	})
}

// LinkPrevNext populates PrevPage/NextPage for each non-hidden page based on
// current slice ordering. Hidden pages are skipped in the chain.
func LinkPrevNext(pages []*Page) {
	visible := make([]*Page, 0, len(pages))
	for _, p := range pages {
		if !p.Hidden {
			visible = append(visible, p)
		}
	}
	for i, p := range visible {
		if i > 0 {
			p.PrevPage = visible[i-1]
		}
		if i < len(visible)-1 {
			p.NextPage = visible[i+1]
		}
	}
}
