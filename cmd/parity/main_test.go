// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// fixtures embedded as plain strings — kept small so a future Pico/pigo
// rename shows up as a single-line diff in this file, not a whole-file churn.

const fixturePigoEvents = `
package plugin

const (
	OnPluginsLoaded = "onPluginsLoaded"

	// doc comment
	OnConfigLoaded = "onConfigLoaded"

	OnThemeLoading = "onThemeLoading"
	OnPageRendered = "onPageRendered"
)
`

const fixturePicoPhp = `
class Pico
{
    public function run()
    {
        $this->triggerEvent('onConfigLoaded', array(&$this->config));
        $this->triggerEvent('onThemeLoading', array(&$theme));
        $this->triggerEvent('onPageRendered', array(&$output));
        // duplicate should dedupe
        $this->triggerEvent('onConfigLoaded', array());
    }

    public function getMetaHeaders()
    {
        $this->metaHeaders = array(
            'Title' => 'title',
            'Description' => 'description',
            'Author' => 'author'
        );
    }

    protected function getTwigVariables()
    {
        return array(
            'config' => $this->getConfig(),
            'base_url' => rtrim($this->getBaseUrl(), '/'),
            'site_title' => $this->getConfig('site_title')
        );
    }

    public function getTwig()
    {
        $this->twig->addFilter(new Twig_SimpleFilter('content', function ($page) { }));
    }
}
`

const fixturePigoConfig = `
package config

type Config struct {
	SiteTitle  string ` + "`yaml:\"site_title\"`" + `
	BaseURL    string ` + "`yaml:\"base_url\"`" + `
	Plugins    []string ` + "`yaml:\"plugins\"`" + `
	Custom     map[string]any ` + "`yaml:\",inline\"`" + `
}
`

const fixturePicoConfigYml = `
##
# Basic
#
site_title: Pico                    # The title
base_url: ~
theme_config:
    widescreen: false               # nested, must be ignored
content_dir: ~
DummyPlugin.enabled: false          # plugin-namespaced, must be filtered
my_custom_setting: Hello World!
`

const fixturePigoContextGo = `
package render

func BuildContext() map[string]any {
	return map[string]any{
		"config":      cfg,
		"base_url":    baseURL,
		"pages":       pagesHash,
		"page_tree":   pageTree.AsMap(),
	}
}
`

const fixturePigoPigoGo = `
package pigo

func defaultMetaHeaders() map[string]string {
	return map[string]string{
		"Title":       "title",
		"Description": "description",
		"Author":      "author",
	}
}
`

const fixturePigoTwigGo = `
package render

func (r *TwigRenderer) registerPicoFilters() {
	r.env.Filters["markdown"] = mdFn
	r.env.Filters["url"] = urlFn
	r.env.Functions["url_param"] = urlParamFn
	r.env.Functions["pages"] = pagesFn
}
`

const fixturePicoExtension = `
class PicoTwigExtension
{
    public function getFilters()
    {
        return array(
            'markdown' => new Twig_SimpleFilter('markdown', $m),
            'map' => new Twig_SimpleFilter('map', $x),
            'url' => new Twig_SimpleFilter('url', $u)
        );
    }

    public function getFunctions()
    {
        return array(
            'url_param' => new Twig_SimpleFunction('url_param', $up),
            'pages' => new Twig_SimpleFunction('pages', $p)
        );
    }
}
`

const fixturePigoMain = `
package main

import "flag"

func main() {
	root := flag.String("root", ".", "usage")
	addr := flag.String("addr", ":8080", "usage")
	debug := flag.Bool("debug", false, "usage")
	_ = root; _ = addr; _ = debug
}
`

// assertEqualUnordered compares two slices ignoring order. Most extractors
// emit insertion order but the test shouldn't be brittle to that.
func assertEqualUnordered(t *testing.T, got, want []string) {
	t.Helper()
	g := append([]string(nil), got...)
	w := append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if !reflect.DeepEqual(g, w) {
		t.Errorf("got %v, want %v", g, w)
	}
}

func TestExtractPigoEvents(t *testing.T) {
	got := extractPigoEvents([]byte(fixturePigoEvents))
	assertEqualUnordered(t, got, []string{
		"onPluginsLoaded", "onConfigLoaded", "onThemeLoading", "onPageRendered",
	})
}

func TestExtractPicoEvents(t *testing.T) {
	got := extractPicoEvents([]byte(fixturePicoPhp))
	// Dedup check: the fixture has onConfigLoaded twice.
	assertEqualUnordered(t, got, []string{
		"onConfigLoaded", "onThemeLoading", "onPageRendered",
	})
}

func TestExtractPigoConfigKeys(t *testing.T) {
	got := extractPigoConfigKeys([]byte(fixturePigoConfig))
	// `yaml:",inline"` on Custom must not appear.
	assertEqualUnordered(t, got, []string{"site_title", "base_url", "plugins"})
}

func TestExtractPicoConfigKeys(t *testing.T) {
	got := extractPicoConfigKeys([]byte(fixturePicoConfigYml))
	// Nested keys (widescreen) and plugin-namespaced keys (DummyPlugin.enabled)
	// must be filtered out; everything top-level must come through.
	assertEqualUnordered(t, got, []string{
		"site_title", "base_url", "theme_config", "content_dir", "my_custom_setting",
	})
}

func TestExtractPigoContextKeys(t *testing.T) {
	got := extractPigoContextKeys([]byte(fixturePigoContextGo))
	assertEqualUnordered(t, got, []string{"config", "base_url", "pages", "page_tree"})
}

func TestExtractPicoTwigVariables(t *testing.T) {
	got := extractPicoTwigVariables([]byte(fixturePicoPhp))
	assertEqualUnordered(t, got, []string{"config", "base_url", "site_title"})
}

func TestExtractPigoMetaHeaders(t *testing.T) {
	got := extractPigoMetaHeaders([]byte(fixturePigoPigoGo))
	assertEqualUnordered(t, got, []string{"title", "description", "author"})
}

func TestExtractPicoMetaHeaders(t *testing.T) {
	got := extractPicoMetaHeaders([]byte(fixturePicoPhp))
	assertEqualUnordered(t, got, []string{"title", "description", "author"})
}

func TestExtractPigoTwigNames(t *testing.T) {
	filters := extractPigoTwigNames([]byte(fixturePigoTwigGo), "Filters")
	assertEqualUnordered(t, filters, []string{"markdown", "url"})
	functions := extractPigoTwigNames([]byte(fixturePigoTwigGo), "Functions")
	assertEqualUnordered(t, functions, []string{"url_param", "pages"})
}

func TestExtractPicoTwigFiltersFunctions(t *testing.T) {
	// Pico registers the `content` filter inline in Pico.php (the
	// fixturePicoPhp), not in the extension; verify both sources merge.
	filters, functions := extractPicoTwigFiltersFunctions([]byte(fixturePicoExtension), []byte(fixturePicoPhp))
	assertEqualUnordered(t, filters, []string{"markdown", "map", "url", "content"})
	assertEqualUnordered(t, functions, []string{"url_param", "pages"})
}

func TestExtractPigoFlags(t *testing.T) {
	got := extractPigoFlags([]byte(fixturePigoMain))
	assertEqualUnordered(t, got, []string{"root", "addr", "debug"})
}

func TestClassifyNormalisesCase(t *testing.T) {
	// Pico says onFoo, pigo says OnFoo — classify must treat them as matched.
	c := classify(Category{
		Pico: []string{"onFoo", "onBar"},
		Pigo: []string{"OnFoo", "OnBaz"},
	})
	if len(c.Both) != 1 || c.Both[0].Pico != "onFoo" || c.Both[0].Pigo != "OnFoo" {
		t.Errorf("Both mismatch: %+v", c.Both)
	}
	assertEqualUnordered(t, c.PicoOnly, []string{"onBar"})
	assertEqualUnordered(t, c.PigoOnly, []string{"OnBaz"})
}

func TestClassifyEmptySide(t *testing.T) {
	// CLI flags category: no Pico side. Every pigo item must go to PigoOnly.
	c := classify(Category{
		Pico: nil,
		Pigo: []string{"root", "addr"},
	})
	if len(c.Both) != 0 {
		t.Errorf("expected no shared items, got %+v", c.Both)
	}
	assertEqualUnordered(t, c.PigoOnly, []string{"root", "addr"})
}

func TestAnchorize(t *testing.T) {
	cases := map[string]string{
		"Plugin events":                "plugin-events",
		"Config keys":                  "config-keys",
		"Twig filters (Pico-specific)": "twig-filters-pico-specific",
	}
	for in, want := range cases {
		if got := anchorize(in); got != want {
			t.Errorf("anchorize(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestRenderJSONStableOrder(t *testing.T) {
	// Category order in data.json is part of the output contract: --check
	// diffs byte-for-byte.
	out, err := renderJSON([]Category{
		{Key: "events"},
		{Key: "config_keys"},
		{Key: "cli_flags"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	iEv := strings.Index(s, `"events"`)
	iCfg := strings.Index(s, `"config_keys"`)
	iCli := strings.Index(s, `"cli_flags"`)
	if !(iEv < iCfg && iCfg < iCli) {
		t.Errorf("category order not preserved in JSON output: events=%d config=%d cli=%d", iEv, iCfg, iCli)
	}
}
