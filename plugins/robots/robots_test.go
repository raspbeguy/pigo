// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package robots_test

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
	"github.com/raspbeguy/pigo/plugins/robots"
)

// writeSite writes a pigo-compatible site layout. extraYAML is appended to
// config.yml; caller passes the PicoRobots block.
func writeSite(t *testing.T, root, extraYAML string) {
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
	cfg := "site_title: Test\ntheme: t\nrewrite_url: true\n" + extraYAML
	must(filepath.Join(root, "config", "config.yml"), cfg)
	// The default theme has to exist; robots/sitemap templates come from the
	// plugin's embedded loader, but requests fall through pigo's render path
	// which needs a default "index.twig".
	must(filepath.Join(root, "themes", "t", "index.twig"), "TITLE:{{ current_page.title }}")
	// Ensure content dir exists even if no files.
	if err := os.MkdirAll(filepath.Join(root, "content"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRobotsServesRobotsTxt(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, `PicoRobots:
  robots:
    - user_agents: ["*"]
      disallow: ["/private"]
    - user_agents: ["BadBot"]
      disallow: ["/"]
`)

	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&robots.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("status %d\n%s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type=%q, want text/plain...", ct)
	}
	s := string(body)
	for _, want := range []string{
		"Sitemap: ", "/sitemap.xml",
		"User-agent: *",
		"Disallow: /private",
		"User-agent: BadBot",
		"Disallow: /",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("robots.txt missing %q\nbody:\n%s", want, s)
		}
	}
}

func TestRobotsServesSitemapXML(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, `PicoRobots:
  sitemap:
    - url: "%base_url%/external"
      changefreq: weekly
      priority: 0.5
`)
	// Pages: visible, noindex-excluded, hidden-excluded.
	must := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, "content", "index.md"), "---\nTitle: Home\n---\n")
	must(filepath.Join(root, "content", "about.md"), "---\nTitle: About\n---\n")
	must(filepath.Join(root, "content", "noindex.md"),
		"---\nTitle: Secret\nRobots: noindex\n---\n")
	must(filepath.Join(root, "content", "_drafts", "wip.md"),
		"---\nTitle: Draft\n---\n")
	must(filepath.Join(root, "content", "excluded.md"),
		"---\nTitle: Excluded\nSitemap: false\n---\n")

	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&robots.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/sitemap.xml")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("status %d\n%s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("Content-Type=%q, want application/xml...", ct)
	}
	s := string(body)
	// Must contain visible pages.
	if !strings.Contains(s, "/about") {
		t.Errorf("sitemap missing /about\n%s", s)
	}
	// Root index URL — pigo's PageURL collapses "index" → baseURL+"/".
	if !strings.Contains(s, "<loc>"+ts.URL+"/</loc>") {
		t.Errorf("sitemap missing home URL <loc>%s/</loc>\n%s", ts.URL, s)
	}
	// Must not contain excluded pages.
	for _, forbidden := range []string{"/noindex", "/_drafts", "/wip", "/excluded"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("sitemap should not include %q\n%s", forbidden, s)
		}
	}
	// Config-supplied extra URL with %base_url% substituted.
	if !strings.Contains(s, ts.URL+"/external") {
		t.Errorf("sitemap missing config-supplied URL\n%s", s)
	}
	// Changefreq + priority emitted for the config entry.
	if !strings.Contains(s, "<changefreq>weekly</changefreq>") {
		t.Errorf("sitemap missing <changefreq>weekly</changefreq>\n%s", s)
	}
	if !strings.Contains(s, "<priority>0.5</priority>") {
		t.Errorf("sitemap missing <priority>0.5</priority>\n%s", s)
	}
}

func TestRobotsThemeOverride(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, "")

	// Custom theme-level robots.twig overrides the embedded default.
	if err := os.WriteFile(filepath.Join(root, "themes", "t", "robots.twig"),
		[]byte("CUSTOM ROBOTS"), 0o644); err != nil {
		t.Fatal(err)
	}

	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&robots.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d\n%s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "CUSTOM ROBOTS") {
		t.Errorf("theme robots.twig should have won; got:\n%s", body)
	}
}

func TestRobotsNonPluginURLsStill404(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, "")
	site, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&robots.Plugin{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("unrelated URL should still 404, got %d", res.StatusCode)
	}
}
