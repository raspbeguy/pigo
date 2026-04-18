// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package fileprefixes_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raspbeguy/pigo"
	"github.com/raspbeguy/pigo/plugin"
	"github.com/raspbeguy/pigo/plugins/fileprefixes"
)

// writeSite sets up a minimal pigo site under root with a configurable
// PicoFilePrefixes section.
func writeSite(t *testing.T, root, picoFilePrefixesYAML string) {
	t.Helper()
	must := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := "site_title: Test\ntheme: t\nrewrite_url: true\n"
	if picoFilePrefixesYAML != "" {
		cfg += picoFilePrefixesYAML
	}
	must(filepath.Join(root, "config", "config.yml"), cfg)
	must(filepath.Join(root, "content", "index.md"), "---\nTitle: Home\n---\nHome body")
	must(filepath.Join(root, "themes", "t", "index.twig"),
		`TITLE:{{ current_page.title }}
URL:{{ current_page.url }}
{% for p in pages %}P:{{ p.id }}|{{ p.url }}
{% endfor %}`)
}

func TestFilePrefixesRewritesURLAndServes(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, "") // default config: recursiveDirs=[blog]

	blog := filepath.Join(root, "content", "blog")
	if err := os.MkdirAll(blog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blog, "20240101.hello.md"),
		[]byte("---\nTitle: Hello\n---\nPost body"), 0o644); err != nil {
		t.Fatal(err)
	}

	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&fileprefixes.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	// Request the clean URL; plugin should retarget the file and serve it.
	res, err := http.Get(ts.URL + "/blog/hello")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("/blog/hello status=%d\n%s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "TITLE:Hello") {
		t.Errorf("/blog/hello missing title:\n%s", body)
	}

	// The page listing should expose the rewritten URL, not the prefixed one.
	res, err = http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	s := string(body)
	if !strings.Contains(s, "P:blog/20240101.hello|") {
		t.Errorf("page list should still carry original id; got:\n%s", s)
	}
	// URL in the page list, for that page, should end in "/blog/hello".
	var line string
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(ln, "P:blog/20240101.hello|") {
			line = ln
			break
		}
	}
	if line == "" {
		t.Fatalf("could not find prefixed blog entry in pages list:\n%s", s)
	}
	urlPart := strings.TrimPrefix(line, "P:blog/20240101.hello|")
	if !strings.HasSuffix(urlPart, "/blog/hello") {
		t.Errorf("expected rewritten URL ending /blog/hello, got %q", urlPart)
	}
}

func TestFilePrefixesDisabledWhenConfigBlank(t *testing.T) {
	root := t.TempDir()
	// Explicit empty config → plugin disables itself.
	writeSite(t, root, "PicoFilePrefixes:\n  recursiveDirs: []\n  dirs: []\n")

	blog := filepath.Join(root, "content", "blog")
	if err := os.MkdirAll(blog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blog, "20240101.hello.md"),
		[]byte("---\nTitle: Hello\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&fileprefixes.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/blog/hello")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("plugin should be disabled; expected 404, got %d", res.StatusCode)
	}
}

func TestFilePrefixesCollisionLexHighestWins(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, "")

	blog := filepath.Join(root, "content", "blog")
	if err := os.MkdirAll(blog, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two files both producing clean id "blog/hello"; "20250101.hello"
	// lexicographically > "20240101.hello" so it should win.
	mustWrite := func(p, body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(blog, "20240101.hello.md"), "---\nTitle: Older\n---\n")
	mustWrite(filepath.Join(blog, "20250101.hello.md"), "---\nTitle: Newer\n---\n")

	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&fileprefixes.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/blog/hello")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d\n%s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "TITLE:Newer") {
		t.Errorf("expected lex-highest to win; got:\n%s", body)
	}
}

func TestFilePrefixesRecursiveDotMatchesAll(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, "PicoFilePrefixes:\n  recursiveDirs:\n    - .\n")

	// Prefix at top level, not under blog.
	if err := os.WriteFile(filepath.Join(root, "content", "20240101.top.md"),
		[]byte("---\nTitle: Top\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&fileprefixes.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/top")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "TITLE:Top") {
		t.Errorf("'.' in recursiveDirs should match top-level; got %d\n%s", res.StatusCode, body)
	}
}
