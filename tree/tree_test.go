// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package tree

import (
	"testing"

	"github.com/raspbeguy/pigo/content"
)

func mk(id string) *content.Page { return &content.Page{ID: id, Meta: map[string]any{}} }

func TestBuild(t *testing.T) {
	pages := []*content.Page{mk("index"), mk("about"), mk("sub/index"), mk("sub/page")}
	root := Build(pages)
	if root.Children["about"].Page == nil {
		t.Errorf("about missing")
	}
	if root.Children["sub"].Page == nil {
		t.Errorf("sub/index not attached to sub")
	}
	if root.Children["sub"].Children["page"].Page == nil {
		t.Errorf("sub/page missing")
	}
}

func TestQueryRoot(t *testing.T) {
	pages := []*content.Page{mk("index"), mk("about"), mk("sub/index"), mk("sub/page")}
	root := Build(pages)
	// pages() with offset=1 excludes the start page (root → index). So we get
	// about, sub/index (folder page), and sub/page.
	got := root.Query("", 0, 0, 1)
	if len(got) != 3 {
		t.Errorf("default query: got %d pages, want 3", len(got))
	}
	// With offset=0 the start page is included too.
	got = root.Query("", 0, 0, 0)
	if len(got) != 4 {
		t.Errorf("offset=0 query: got %d pages, want 4", len(got))
	}
}

func TestQueryStartFiltersSubtree(t *testing.T) {
	pages := []*content.Page{mk("index"), mk("sub/index"), mk("sub/page"), mk("sub/deep/leaf")}
	root := Build(pages)
	got := root.Query("sub", 0, 0, 1)
	ids := map[string]bool{}
	for _, p := range got {
		ids[p.ID] = true
	}
	if !ids["sub/page"] || !ids["sub/deep/leaf"] {
		t.Errorf("subtree missing pages: %+v", ids)
	}
	if ids["about"] || ids["index"] {
		t.Errorf("subtree leaked out: %+v", ids)
	}
}
