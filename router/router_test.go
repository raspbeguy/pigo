// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package router

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"/", ""},
		{"foo/bar", "foo/bar"},
		{"/foo/bar/", "foo/bar"},
		{"foo/../bar", "bar"},
		{"../../etc/passwd", "etc/passwd"},
		{"./a", "a"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveFilePath(t *testing.T) {
	dir := t.TempDir()
	writeF(t, filepath.Join(dir, "index.md"), "# home")
	writeF(t, filepath.Join(dir, "about.md"), "# about")
	writeF(t, filepath.Join(dir, "sub/index.md"), "# sub home")
	writeF(t, filepath.Join(dir, "sub/page.md"), "# sub page")
	writeF(t, filepath.Join(dir, "_hidden.md"), "# hidden")

	cases := []struct {
		req, expectBase string
		ok              bool
	}{
		{"", "index.md", true},
		{"about", "about.md", true},
		{"sub", "sub/index.md", true},
		{"sub/page", "sub/page.md", true},
		{"missing", "", false},
		{"_hidden", "", false},
	}
	for _, c := range cases {
		p, ok := ResolveFilePath(dir, c.req, ".md")
		if ok != c.ok {
			t.Errorf("ResolveFilePath(%q): ok=%v, want %v", c.req, ok, c.ok)
			continue
		}
		if ok && filepath.Base(p) != filepath.Base(c.expectBase) {
			t.Errorf("ResolveFilePath(%q): got %q, want ...%q", c.req, p, c.expectBase)
		}
	}
}

func TestEvaluateRequestURLQueryString(t *testing.T) {
	r := httptest.NewRequest("GET", "/?sub/page", nil)
	got := EvaluateRequestURL(r, "")
	if got != "sub/page" {
		t.Errorf("got %q, want sub/page", got)
	}
}

func TestEvaluateRequestURLPath(t *testing.T) {
	r := httptest.NewRequest("GET", "/sub/page", nil)
	got := EvaluateRequestURL(r, "")
	if got != "sub/page" {
		t.Errorf("got %q", got)
	}
}

func TestPageURL(t *testing.T) {
	if got := PageURL("https://example.com", "about", true); got != "https://example.com/about" {
		t.Errorf("pretty: got %q", got)
	}
	if got := PageURL("https://example.com", "about", false); got != "https://example.com/?about" {
		t.Errorf("query-string: got %q", got)
	}
	if got := PageURL("https://example.com", "", true); got != "https://example.com/" {
		t.Errorf("root: got %q", got)
	}
	if got := PageURL("https://example.com", "sub/index", true); got != "https://example.com/sub" {
		t.Errorf("strip /index: got %q", got)
	}
}

func TestPlaceholderSubstitute(t *testing.T) {
	pm := PlaceholderMap{
		BaseURL:   "https://example.com",
		BaseURLQ:  "?",
		ThemeURL:  "https://example.com/themes/default",
		AssetsURL: "https://example.com/assets",
		Version:   "0.1.0",
		Meta:      map[string]any{"title": "Hi"},
		Config:    map[string]any{"site_title": "Site"},
	}
	got := pm.Substitute("Check %base_url% and %assets_url%/img.png and %meta.title% and %config.site_title% %version%")
	want := "Check https://example.com and https://example.com/assets/img.png and Hi and Site 0.1.0"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	// base_url%? variant
	got = pm.Substitute("%base_url%?foo")
	if got != "https://example.com?foo" {
		t.Errorf("baseQ: got %q", got)
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
