// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package pigo

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raspbeguy/pigo/content"
	"github.com/raspbeguy/pigo/plugin"
	"github.com/raspbeguy/pigo/render"
	"github.com/raspbeguy/pigo/tree"
)

// probePlugin records every event it sees, plus the type of each param.
// Used to verify pigo's hook dispatch surface.
type probePlugin struct {
	plugin.Base
	// events is an ordered log of (event, paramTypes...).
	events []probeEntry
	// onEvent, if set, can perform extra assertions or mutate state.
	onEvent func(p *probePlugin, event string, params []any) error
}

type probeEntry struct {
	event  string
	params []any
}

func (p *probePlugin) Name() string        { return "probe" }
func (p *probePlugin) DependsOn() []string { return nil }
func (p *probePlugin) HandleEvent(event string, params ...any) error {
	paramsCopy := make([]any, len(params))
	copy(paramsCopy, params)
	p.events = append(p.events, probeEntry{event: event, params: paramsCopy})
	if p.onEvent != nil {
		return p.onEvent(p, event, paramsCopy)
	}
	return nil
}

// eventNames returns just the ordered event names seen by the probe.
func (p *probePlugin) eventNames() []string {
	out := make([]string, len(p.events))
	for i, e := range p.events {
		out[i] = e.event
	}
	return out
}

// firstParams returns the param slice for the first occurrence of event.
func (p *probePlugin) firstParams(event string) []any {
	for _, e := range p.events {
		if e.event == event {
			return e.params
		}
	}
	return nil
}

// TestPluginHookCoverage exercises every event pigo dispatches and asserts
// the param types match what plugins (and the docs) expect. Fires one 200
// request and one 404 request so both content paths are covered.
func TestPluginHookCoverage(t *testing.T) {
	probe := &probePlugin{}

	site, err := New(Options{
		RootDir: "testdata/site",
		Plugins: []plugin.Plugin{probe},
	})
	if err != nil {
		t.Fatal(err)
	}

	// After Site.New, init-time events should all have fired.
	initWant := []string{
		plugin.OnPluginsLoaded,
		plugin.OnConfigLoaded,
		plugin.OnThemeLoading,
		plugin.OnThemeLoaded,
		plugin.OnMarkdownRegistered,
		plugin.OnYAMLParserRegistered,
		plugin.OnMetaHeaders,
		plugin.OnTwigRegistered,
		plugin.OnPagesLoading,
		// per-page events interleaved here: OnSinglePageLoading,
		// OnSinglePageContent, OnMetaParsing, OnMetaParsed,
		// OnSinglePageLoaded — one cycle per page under testdata/site.
		plugin.OnPagesDiscovered,
		plugin.OnPagesLoaded,
		plugin.OnPageTreeBuilt,
	}
	assertContainsInOrder(t, "init", probe.eventNames(), initWant)

	// Check per-page sub-events fired at least once during scan.
	for _, ev := range []string{
		plugin.OnSinglePageLoading,
		plugin.OnSinglePageContent,
		plugin.OnMetaParsing,
		plugin.OnMetaParsed,
		plugin.OnSinglePageLoaded,
	} {
		if !contains(probe.eventNames(), ev) {
			t.Errorf("init: %s was never fired (scan produced no page events)", ev)
		}
	}

	// Param-type assertions for init events.
	mustType[[]plugin.Plugin](t, plugin.OnPluginsLoaded, probe.firstParams(plugin.OnPluginsLoaded), 0)
	mustType[*render.TwigRegistrar](t, plugin.OnTwigRegistered, probe.firstParams(plugin.OnTwigRegistered), 0)
	mustType[*content.MarkdownRegistrar](t, plugin.OnMarkdownRegistered, probe.firstParams(plugin.OnMarkdownRegistered), 0)
	mustType[*map[string]string](t, plugin.OnMetaHeaders, probe.firstParams(plugin.OnMetaHeaders), 0)
	mustType[*string](t, plugin.OnThemeLoading, probe.firstParams(plugin.OnThemeLoading), 0)
	mustType[string](t, plugin.OnThemeLoaded, probe.firstParams(plugin.OnThemeLoaded), 0)
	mustType[[]*content.Page](t, plugin.OnPagesDiscovered, probe.firstParams(plugin.OnPagesDiscovered), 0)
	mustType[[]*content.Page](t, plugin.OnPagesLoaded, probe.firstParams(plugin.OnPagesLoaded), 0)
	mustType[*tree.Node](t, plugin.OnPageTreeBuilt, probe.firstParams(plugin.OnPageTreeBuilt), 0)

	// Param-type assertions for per-page events.
	mustType[*string](t, plugin.OnSinglePageLoading, probe.firstParams(plugin.OnSinglePageLoading), 0)
	perPage := probe.firstParams(plugin.OnSinglePageContent)
	mustType[string](t, plugin.OnSinglePageContent, perPage, 0)
	mustType[*string](t, plugin.OnSinglePageContent, perPage, 1)
	mustType[*string](t, plugin.OnMetaParsing, probe.firstParams(plugin.OnMetaParsing), 0)
	mustType[*map[string]any](t, plugin.OnMetaParsed, probe.firstParams(plugin.OnMetaParsed), 0)
	mustType[*content.Page](t, plugin.OnSinglePageLoaded, probe.firstParams(plugin.OnSinglePageLoaded), 0)

	// Now drive a real 200 request and verify per-request events.
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	beforeReq := len(probe.events)
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()

	got200 := eventNamesFrom(probe.events, beforeReq)
	want200 := []string{
		plugin.OnRequestURL,
		plugin.OnRequestFile,
		plugin.OnContentLoading,
		// per-page events fire again because Scanner.Load is called per-request too
		plugin.OnContentLoaded,
		plugin.OnCurrentPageDiscovered,
		plugin.OnContentParsing,
		plugin.OnContentPrepared,
		plugin.OnContentParsed,
		plugin.OnPageRendering,
		plugin.OnPageRendered,
	}
	assertContainsInOrder(t, "200 req", got200, want200)

	// Param-type assertions for per-request events.
	mustType[*string](t, plugin.OnRequestURL, firstParamsFrom(probe.events, beforeReq, plugin.OnRequestURL), 0)
	mustType[*string](t, plugin.OnRequestFile, firstParamsFrom(probe.events, beforeReq, plugin.OnRequestFile), 0)
	mustType[*string](t, plugin.OnContentLoaded, firstParamsFrom(probe.events, beforeReq, plugin.OnContentLoaded), 0)
	cur := firstParamsFrom(probe.events, beforeReq, plugin.OnCurrentPageDiscovered)
	mustType[*content.Page](t, plugin.OnCurrentPageDiscovered, cur, 0)
	// prev/next may be nil but the slot exists
	if len(cur) < 3 {
		t.Errorf("%s: expected 3 params, got %d", plugin.OnCurrentPageDiscovered, len(cur))
	}
	mustType[*string](t, plugin.OnContentParsing, firstParamsFrom(probe.events, beforeReq, plugin.OnContentParsing), 0)
	mustType[*string](t, plugin.OnContentPrepared, firstParamsFrom(probe.events, beforeReq, plugin.OnContentPrepared), 0)
	mustType[*string](t, plugin.OnContentParsed, firstParamsFrom(probe.events, beforeReq, plugin.OnContentParsed), 0)

	rndParams := firstParamsFrom(probe.events, beforeReq, plugin.OnPageRendering)
	mustType[*string](t, plugin.OnPageRendering, rndParams, 0)
	mustType[*map[string]any](t, plugin.OnPageRendering, rndParams, 1)
	mustType[http.Header](t, plugin.OnPageRendering, rndParams, 2)
	mustType[*int](t, plugin.OnPageRendering, rndParams, 3)

	rndDone := firstParamsFrom(probe.events, beforeReq, plugin.OnPageRendered)
	mustType[*[]byte](t, plugin.OnPageRendered, rndDone, 0)
	mustType[http.Header](t, plugin.OnPageRendered, rndDone, 1)
	mustType[*int](t, plugin.OnPageRendered, rndDone, 2)

	// 404 request: should fire On404ContentLoading and On404ContentLoaded.
	before404 := len(probe.events)
	res, err = http.Get(ts.URL + "/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Errorf("404 path returned status %d", res.StatusCode)
	}

	got404 := eventNamesFrom(probe.events, before404)
	want404 := []string{
		plugin.OnRequestURL,
		plugin.OnRequestFile,
		plugin.On404ContentLoading,
		plugin.On404ContentLoaded,
		plugin.OnCurrentPageDiscovered,
		plugin.OnPageRendering,
		plugin.OnPageRendered,
	}
	assertContainsInOrder(t, "404 req", got404, want404)

	mustType[*string](t, plugin.On404ContentLoaded, firstParamsFrom(probe.events, before404, plugin.On404ContentLoaded), 0)
}

// TestPluginOnRequestFileRetarget verifies that a plugin mutating *filePath
// to point at a different real file causes that file to be rendered instead
// of the original. Required for prefix-stripping plugins.
func TestPluginOnRequestFileRetarget(t *testing.T) {
	root := t.TempDir()
	writeSite(t, root)

	// Add a prefixed content file that isn't directly reachable by URL.
	blogDir := filepath.Join(root, "content", "blog")
	if err := os.MkdirAll(blogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blogDir, "20240101.hello.md"),
		[]byte("---\nTitle: Hello from blog\n---\nBody"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plugin rewrites filePath for /blog/hello to the prefixed file.
	p := &retargetPlugin{
		root: root,
	}

	site, err := New(Options{
		RootDir: root,
		Plugins: []plugin.Plugin{p},
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
		t.Fatalf("status %d (want 200)\nbody:\n%s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "TITLE:Hello from blog") {
		t.Errorf("body missing retargeted page title; body:\n%s", body)
	}
}

type retargetPlugin struct {
	plugin.Base
	root string
}

func (p *retargetPlugin) Name() string        { return "retarget" }
func (p *retargetPlugin) DependsOn() []string { return nil }
func (p *retargetPlugin) HandleEvent(event string, params ...any) error {
	if event != plugin.OnRequestFile {
		return nil
	}
	fp := params[0].(*string)
	// If the current target doesn't exist and the URL path corresponds to
	// /blog/hello, redirect to the prefixed file.
	if _, err := os.Stat(*fp); err == nil {
		return nil
	}
	if strings.HasSuffix(*fp, "/blog/hello.md") || strings.HasSuffix(*fp, "/blog/hello/index.md") {
		*fp = filepath.Join(p.root, "content", "blog", "20240101.hello.md")
	}
	return nil
}

// TestPluginResponseOverride verifies that a plugin can set Content-Type and
// status via OnPageRendering, matching the mechanism PicoRobots needs for
// /robots.txt etc.
func TestPluginResponseOverride(t *testing.T) {
	p := &responsePlugin{}
	site, err := New(Options{
		RootDir: "testdata/site",
		Plugins: []plugin.Plugin{p},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	defer ts.Close()

	// Hit the 404 path so the plugin overrides both status and Content-Type.
	res, err := http.Get(ts.URL + "/robots.txt")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Errorf("status = %d, want 200 (plugin should have forced it)", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
}

type responsePlugin struct {
	plugin.Base
}

func (p *responsePlugin) Name() string        { return "response" }
func (p *responsePlugin) DependsOn() []string { return nil }
func (p *responsePlugin) HandleEvent(event string, params ...any) error {
	if event != plugin.OnPageRendering {
		return nil
	}
	// params: [0]=*string tmpl, [1]=*map[string]any ctx, [2]=http.Header, [3]=*int status
	headers := params[2].(http.Header)
	status := params[3].(*int)
	headers.Set("Content-Type", "text/plain; charset=utf-8")
	*status = 200
	return nil
}

// writeSite writes a minimal pigo-compatible site layout into root.
func writeSite(t *testing.T, root string) {
	t.Helper()
	must := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(root, "config", "config.yml"), "site_title: Test\ntheme: t\n")
	must(filepath.Join(root, "content", "index.md"), "---\nTitle: Home\n---\nHi")
	must(filepath.Join(root, "themes", "t", "index.twig"), "TITLE:{{ current_page.title }}\nBODY:{{ content|raw }}")
}

// --- helpers ---

func contains(s []string, e string) bool {
	for _, x := range s {
		if x == e {
			return true
		}
	}
	return false
}

// assertContainsInOrder asserts that want appears as an in-order (but not
// necessarily contiguous) subsequence of got.
func assertContainsInOrder(t *testing.T, ctx string, got, want []string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i != len(want) {
		missing := want[i]
		t.Errorf("%s: event %q not found in order. got=%v want(in order)=%v", ctx, missing, got, want)
	}
}

func eventNamesFrom(events []probeEntry, start int) []string {
	out := []string{}
	for _, e := range events[start:] {
		out = append(out, e.event)
	}
	return out
}

func firstParamsFrom(events []probeEntry, start int, name string) []any {
	for _, e := range events[start:] {
		if e.event == name {
			return e.params
		}
	}
	return nil
}

// mustType asserts that params[idx] has type T, failing the test otherwise.
func mustType[T any](t *testing.T, event string, params []any, idx int) {
	t.Helper()
	if idx >= len(params) {
		t.Errorf("%s: want param[%d] of type %T, but only %d params", event, idx, *new(T), len(params))
		return
	}
	if _, ok := params[idx].(T); !ok {
		t.Errorf("%s: param[%d] = %T (%v), want %T",
			event, idx, params[idx], safeString(params[idx]), *new(T))
	}
}

func safeString(v any) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return s
}
