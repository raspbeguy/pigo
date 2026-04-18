// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package pigo_test

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

	// Blank imports populate the plugin registry at test-binary init, so
	// pigo.New can resolve "PicoRobots" / "PicoFilePrefixes" from the
	// config's plugins: list without any programmatic Options.Plugins.
	_ "github.com/raspbeguy/pigo/plugins/fileprefixes"
	_ "github.com/raspbeguy/pigo/plugins/robots"
)

// writeSite writes a minimal pigo-compatible site layout into root. cfg is
// the full config.yml body (caller composes the plugins: block).
func writeSite(t *testing.T, root, cfg string) {
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
	must(filepath.Join(root, "config", "config.yml"), cfg)
	must(filepath.Join(root, "themes", "t", "index.twig"), "TITLE:{{ current_page.title }}")
	if err := os.MkdirAll(filepath.Join(root, "content"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestRegistryEnablesPluginFromConfig verifies the core "one binary, many
// sites" story: a site whose config lists PicoRobots (and nothing is passed
// via Options.Plugins) has a working /robots.txt.
func TestRegistryEnablesPluginFromConfig(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, `site_title: Test
theme: t
plugins:
  - PicoRobots
PicoRobots:
  robots:
    - user_agents: ["*"]
      disallow: ["/private"]
`)

	site, err := pigo.New(pigo.Options{RootDir: root})
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
		t.Fatalf("/robots.txt status=%d\n%s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type=%q, want text/plain...", ct)
	}
	if !strings.Contains(string(body), "Disallow: /private") {
		t.Errorf("expected plugin-rendered rule; got:\n%s", body)
	}
}

// TestRegistrySeparateSitesDifferentPlugins simulates the "same binary used
// for two sites" scenario: two pigo.New calls with different config produce
// different plugin sets.
func TestRegistrySeparateSitesDifferentPlugins(t *testing.T) {
	// Site A: robots only, no file-prefixes.
	rootA := t.TempDir()
	writeSite(t, rootA, `site_title: A
theme: t
plugins:
  - PicoRobots
`)
	// Site B: file-prefixes only, no robots.
	rootB := t.TempDir()
	writeSite(t, rootB, `site_title: B
theme: t
plugins:
  - PicoFilePrefixes
`)
	// Give B a prefixed blog post so its plugin has something to rewrite.
	if err := os.MkdirAll(filepath.Join(rootB, "content", "blog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(rootB, "content", "blog", "20240101.hi.md"),
		[]byte("---\nTitle: Hi\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	siteA, err := pigo.New(pigo.Options{RootDir: rootA})
	if err != nil {
		t.Fatal(err)
	}
	siteB, err := pigo.New(pigo.Options{RootDir: rootB})
	if err != nil {
		t.Fatal(err)
	}

	tsA := httptest.NewServer(siteA.Handler())
	defer tsA.Close()
	tsB := httptest.NewServer(siteB.Handler())
	defer tsB.Close()

	// A serves robots.txt (robots plugin active) but has no prefix rewriting.
	resA, _ := http.Get(tsA.URL + "/robots.txt")
	if resA.StatusCode != 200 {
		t.Errorf("site A /robots.txt expected 200, got %d", resA.StatusCode)
	}
	resA.Body.Close()

	// A should 404 for /blog/hi (no file-prefixes plugin to rewrite).
	resA2, _ := http.Get(tsA.URL + "/blog/hi")
	if resA2.StatusCode != 404 {
		t.Errorf("site A /blog/hi expected 404 (no fileprefixes), got %d", resA2.StatusCode)
	}
	resA2.Body.Close()

	// B resolves /blog/hi via file-prefixes plugin.
	resB, err := http.Get(tsB.URL + "/blog/hi")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resB.Body)
	resB.Body.Close()
	if resB.StatusCode != 200 || !strings.Contains(string(body), "TITLE:Hi") {
		t.Errorf("site B /blog/hi expected 200 with Hi; got %d\n%s", resB.StatusCode, body)
	}

	// B should 404 for /robots.txt (no robots plugin active here).
	resB2, _ := http.Get(tsB.URL + "/robots.txt")
	if resB2.StatusCode != 404 {
		t.Errorf("site B /robots.txt expected 404 (no robots plugin), got %d", resB2.StatusCode)
	}
	resB2.Body.Close()
}

func TestRegistryUnknownPluginNameErrors(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, `site_title: T
theme: t
plugins:
  - DoesNotExist
`)
	_, err := pigo.New(pigo.Options{RootDir: root})
	if err == nil {
		t.Fatal("expected error for unknown plugin name")
	}
	if !strings.Contains(err.Error(), "DoesNotExist") {
		t.Errorf("error should name the unknown plugin: %v", err)
	}
	// Error should list what IS registered so the operator can fix the typo.
	for _, known := range plugin.Registered() {
		if !strings.Contains(err.Error(), known) {
			t.Errorf("error should hint at registered plugins (missing %q): %v", known, err)
		}
	}
}

// probe is a minimal plugin used only to test dup-detection.
type probe struct {
	plugin.Base
	name string
}

func (p *probe) Name() string                                 { return p.name }
func (p *probe) DependsOn() []string                          { return nil }
func (p *probe) HandleEvent(event string, _ ...any) error     { return nil }

func TestRegistryDuplicateConfigAndOptionsErrors(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root, `site_title: T
theme: t
plugins:
  - PicoRobots
`)
	// Also pass a PicoRobots instance programmatically → duplicate.
	_, err := pigo.New(pigo.Options{
		RootDir: root,
		Plugins: []plugin.Plugin{&probe{name: "PicoRobots"}},
	})
	if err == nil {
		t.Fatal("expected duplicate-plugin error")
	}
	if !strings.Contains(err.Error(), "PicoRobots") {
		t.Errorf("duplicate error should name the plugin: %v", err)
	}
}
