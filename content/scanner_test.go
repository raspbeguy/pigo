// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanAllAndSort(t *testing.T) {
	dir := t.TempDir()
	writeF(t, filepath.Join(dir, "index.md"), "---\nTitle: Home\n---\nHi")
	writeF(t, filepath.Join(dir, "about.md"), "---\nTitle: About\n---\nHi")
	writeF(t, filepath.Join(dir, "sub/page.md"), "---\nTitle: Sub\n---\nHi")
	writeF(t, filepath.Join(dir, "_hidden.md"), "---\nTitle: Hidden\n---\n")
	writeF(t, filepath.Join(dir, "404.md"), "---\nTitle: 404\n---\n")

	s := &Scanner{Dir: dir, Ext: ".md", DateFormat: "%D %T"}
	pages, err := s.ScanAll()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]*Page{}
	for _, p := range pages {
		ids[p.ID] = p
	}
	if _, ok := ids["404"]; ok {
		t.Errorf("404 must be excluded from scan")
	}
	if _, ok := ids["_hidden"]; ok {
		t.Errorf("_hidden must be excluded")
	}
	for _, id := range []string{"index", "about", "sub/page"} {
		if _, ok := ids[id]; !ok {
			t.Errorf("missing %q", id)
		}
	}

	SortPages(pages, "alpha", "asc", "")
	if pages[0].ID != "about" || pages[len(pages)-1].ID != "sub/page" {
		t.Errorf("sort order wrong: first=%s last=%s", pages[0].ID, pages[len(pages)-1].ID)
	}

	SortPages(pages, "alpha", "desc", "")
	if pages[0].ID != "sub/page" {
		t.Errorf("desc sort: first=%s", pages[0].ID)
	}
}

func TestSortByMeta(t *testing.T) {
	pages := []*Page{
		{ID: "a", Meta: map[string]any{"weight": "3"}},
		{ID: "b", Meta: map[string]any{"weight": "1"}},
		{ID: "c", Meta: map[string]any{"weight": "2"}},
	}
	SortPages(pages, "meta", "asc", "weight")
	got := []string{pages[0].ID, pages[1].ID, pages[2].ID}
	want := []string{"b", "c", "a"}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("pos %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

func TestLinkPrevNext(t *testing.T) {
	pages := []*Page{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	LinkPrevNext(pages)
	if pages[1].PrevPage.ID != "a" || pages[1].NextPage.ID != "c" {
		t.Errorf("middle links wrong")
	}
	if pages[0].PrevPage != nil {
		t.Errorf("first should have no prev")
	}
	if pages[2].NextPage != nil {
		t.Errorf("last should have no next")
	}
}

func TestLoadErrorPageWalksUp(t *testing.T) {
	dir := t.TempDir()
	writeF(t, filepath.Join(dir, "404.md"), "---\nTitle: Not Found\n---\nMissing.")
	s := &Scanner{Dir: dir, Ext: ".md"}
	p, err := s.LoadErrorPage("sub/deep/missing")
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.Title != "Not Found" {
		t.Errorf("expected 404 page, got %+v", p)
	}
}

func writeF(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
