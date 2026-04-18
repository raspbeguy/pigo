// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package robots is a pigo port of the official Pico plugin PicoRobots
// (https://github.com/PhrozenByte/pico-robots).
//
// It serves two virtual URLs with proper Content-Type and status:
//
//	/robots.txt  — robots exclusion rules (text/plain)
//	/sitemap.xml — sitemap 0.9 XML (application/xml)
//
// Robots rules come from the config:
//
//	PicoRobots:
//	  robots:
//	    - user_agents: ["*"]
//	      disallow:   []
//	      allow:      []
//
// Sitemap entries are derived automatically from pigo's known pages
// (excluding hidden, "noindex", or explicit `Sitemap: false` pages) plus
// any extra records in the `PicoRobots.sitemap` config list.
//
// Per-page sitemap metadata via YAML front-matter:
//
//	Sitemap:
//	  lastmod:    2024-12-01
//	  changefreq: monthly
//	  priority:   0.9
//
// Templates for both files ship embedded in the plugin (via go:embed); a
// user theme can override either by shipping its own robots.twig /
// sitemap.twig in the theme dir.
package robots

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/raspbeguy/pigo/config"
	"github.com/raspbeguy/pigo/content"
	"github.com/raspbeguy/pigo/plugin"
	"github.com/raspbeguy/pigo/render"
)

// Plugin is the PicoRobots Go port.
type Plugin struct {
	plugin.Base

	cfg *config.Config

	// Precomputed state built at init.
	robots        []map[string]any // rules from config, shaped for the template
	pageSitemap   []map[string]any // entries derived from pages
	configSitemap []sitemapRecord  // raw entries from config (URL substitution deferred)
}

type sitemapRecord struct {
	URL        string // may contain %base_url% etc.
	Lastmod    string // ISO 8601 string, "" if none
	ChangeFreq string
	Priority   string // pre-formatted, "" if none
}

// Name/DependsOn implement plugin.Plugin. Name mirrors the PHP class so the
// config key ports 1:1.
func (*Plugin) Name() string        { return "PicoRobots" }
func (*Plugin) DependsOn() []string { return nil }

// HandleEvent dispatches on event name.
func (p *Plugin) HandleEvent(event string, params ...any) error {
	switch event {
	case plugin.OnConfigLoaded:
		return p.onConfigLoaded(params[0].(*config.Config))
	case plugin.OnMetaHeaders:
		return p.onMetaHeaders(params[0].(*map[string]string))
	case plugin.OnPagesLoaded:
		return p.onPagesLoaded(params[0].([]*content.Page))
	case plugin.OnTwigRegistered:
		return p.onTwigRegistered(params[0].(*render.TwigRegistrar))
	case plugin.OnPageRendering:
		return p.onPageRendering(
			params[0].(*string),
			params[1].(*map[string]any),
			params[2].(interface {
				Set(string, string)
				Get(string) string
			}), // http.Header implements these
			params[3].(*int),
		)
	}
	return nil
}

func (p *Plugin) onConfigLoaded(cfg *config.Config) error {
	p.cfg = cfg
	raw, _ := cfg.Custom["PicoRobots"].(map[string]any)
	// Parse robots rules.
	for _, entry := range listOfMaps(raw["robots"]) {
		uas := stringList(entry["user_agents"])
		dis := stringList(entry["disallow"])
		allow := stringList(entry["allow"])
		if len(uas) == 0 {
			uas = []string{"*"}
		}
		// Pico default: if neither disallow nor allow given, disallow everything.
		if len(dis) == 0 && len(allow) == 0 {
			dis = []string{"/"}
		}
		p.robots = append(p.robots, map[string]any{
			"user_agents": asAnySlice(uas),
			"disallow":    asAnySlice(dis),
			"allow":       asAnySlice(allow),
		})
	}
	// Parse extra sitemap records (URL substitution deferred to per-request).
	for _, entry := range listOfMaps(raw["sitemap"]) {
		url, _ := entry["url"].(string)
		if url == "" {
			continue
		}
		p.configSitemap = append(p.configSitemap, sitemapRecord{
			URL:        url,
			Lastmod:    toISOString(entry["lastmod"]),
			ChangeFreq: validChangefreq(toString(entry["changefreq"])),
			Priority:   formatPriority(entry["priority"]),
		})
	}
	return nil
}

// onMetaHeaders registers the Sitemap alias. Pigo keeps arbitrary YAML keys
// in Meta verbatim, so this is purely informational for other plugins.
func (p *Plugin) onMetaHeaders(headers *map[string]string) error {
	(*headers)["Sitemap"] = "sitemap"
	return nil
}

// onPagesLoaded precomputes per-page sitemap eligibility + metadata. Runs
// once at Site init, so serving /sitemap.xml later is just a formatting pass.
func (p *Plugin) onPagesLoaded(pages []*content.Page) error {
	p.pageSitemap = nil
	for _, pg := range pages {
		if !sitemapEligible(pg) {
			continue
		}
		rec := map[string]any{
			"url_id":     pg.ID, // resolved to an absolute URL per-request
			"lastmod":    pageLastmod(pg),
			"changefreq": validChangefreq(metaString(pg.Meta, "sitemap", "changefreq")),
			"priority":   formatPriority(metaGet(pg.Meta, "sitemap", "priority")),
		}
		p.pageSitemap = append(p.pageSitemap, rec)
	}
	return nil
}

// sitemapEligible applies Pico's exclusion heuristics.
func sitemapEligible(pg *content.Page) bool {
	// Explicit Sitemap: false in front-matter wins.
	if sv, ok := pg.Meta["sitemap"]; ok {
		if b, isBool := sv.(bool); isBool && !b {
			return false
		}
	}
	// Any path segment starting with "_" → exclude.
	for _, seg := range strings.Split(pg.ID, "/") {
		if strings.HasPrefix(seg, "_") {
			return false
		}
	}
	// meta.robots contains "noindex" (case-insensitive) → exclude.
	if r, ok := pg.Meta["robots"].(string); ok && r != "" {
		for _, tok := range strings.Split(r, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "noindex") {
				return false
			}
		}
	}
	return true
}

// pageLastmod returns ISO-formatted lastmod for a page:
//   - meta.sitemap.lastmod if set (string or date-parseable)
//   - otherwise page.ModificationTime (fs mtime) if >0
//   - else ""
func pageLastmod(pg *content.Page) string {
	if v := metaGet(pg.Meta, "sitemap", "lastmod"); v != nil {
		if s := toISOString(v); s != "" {
			return s
		}
	}
	if pg.ModificationTime > 0 {
		return time.Unix(pg.ModificationTime, 0).UTC().Format(time.RFC3339)
	}
	return ""
}

// onTwigRegistered plugs in the embedded template loader. Theme dir remains
// first in the composite loader so user overrides take precedence.
func (p *Plugin) onTwigRegistered(reg *render.TwigRegistrar) error {
	loader, err := loadEmbeddedTemplates()
	if err != nil {
		return fmt.Errorf("robots: load embedded templates: %w", err)
	}
	reg.AddLoader(loader)
	return nil
}

// onPageRendering intercepts the two virtual URLs and replaces the
// template + context. request_url is exposed in ctx by the pigo server.
func (p *Plugin) onPageRendering(tmpl *string, ctx *map[string]any, headers headerSetter, status *int) error {
	reqURL, _ := (*ctx)["request_url"].(string)
	baseURL, _ := (*ctx)["base_url"].(string)

	switch reqURL {
	case "robots.txt":
		*tmpl = "robots"
		(*ctx)["robots"] = asAnySlice(p.robots)
		headers.Set("Content-Type", "text/plain; charset=utf-8")
		*status = 200
	case "sitemap.xml":
		*tmpl = "sitemap"
		(*ctx)["sitemap"] = p.buildSitemapEntries(baseURL, (*ctx)["rewrite_url"])
		headers.Set("Content-Type", "application/xml; charset=utf-8")
		*status = 200
	}
	return nil
}

// buildSitemapEntries composes page-derived entries and config entries,
// resolving URL placeholders / ids against the per-request baseURL.
func (p *Plugin) buildSitemapEntries(baseURL string, rewriteAny any) []any {
	rewrite := true
	if p.cfg != nil && p.cfg.RewriteURL != nil {
		rewrite = *p.cfg.RewriteURL
	}
	out := make([]any, 0, len(p.pageSitemap)+len(p.configSitemap))
	for _, pg := range p.pageSitemap {
		id, _ := pg["url_id"].(string)
		rec := map[string]any{
			"url":        pageURL(baseURL, id, rewrite),
			"lastmod":    pg["lastmod"],
			"changefreq": pg["changefreq"],
			"priority":   pg["priority"],
		}
		out = append(out, rec)
	}
	for _, c := range p.configSitemap {
		url := substituteBaseURL(c.URL, baseURL, rewrite)
		out = append(out, map[string]any{
			"url":        url,
			"lastmod":    c.Lastmod,
			"changefreq": c.ChangeFreq,
			"priority":   c.Priority,
		})
	}
	_ = rewriteAny
	return out
}

// pageURL matches router.PageURL's rules without importing it (avoiding a
// test-awkward dependency cycle between plugins and the router package
// isn't a concern in practice, but this keeps the plugin leaner).
func pageURL(baseURL, id string, rewrite bool) string {
	id = strings.TrimSuffix(id, "/index")
	if id == "index" {
		id = ""
	}
	if id == "" {
		return baseURL + "/"
	}
	if rewrite {
		return baseURL + "/" + id
	}
	return baseURL + "/?" + id
}

// substituteBaseURL replaces Pico's URL placeholders in a config-supplied URL.
// Supports %base_url% and %base_url%? (trailing "?" when rewrites disabled).
func substituteBaseURL(s, baseURL string, rewrite bool) string {
	q := ""
	if !rewrite {
		q = "?"
	}
	s = strings.ReplaceAll(s, "%base_url%?", strings.TrimRight(baseURL, "/")+"/"+q)
	s = strings.ReplaceAll(s, "%base_url%", strings.TrimRight(baseURL, "/"))
	return s
}

// --- config helpers ---

// listOfMaps coerces a YAML value that may be a list of dicts into
// []map[string]any. A single-dict value is wrapped into a one-element list
// (matches PHP's array_key_exists + foreach leniency).
func listOfMaps(v any) []map[string]any {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{x}
	}
	return nil
}

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
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	}
	return nil
}

func asAnySlice[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

// toISOString renders a variety of date-ish input shapes as ISO 8601. Matches
// PHP strtotime flexibility plus yaml.v3's native time.Time parsing.
func toISOString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	case string:
		if x == "" {
			return ""
		}
		layouts := []string{
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, l := range layouts {
			if t, err := time.Parse(l, x); err == nil {
				return t.UTC().Format(time.RFC3339)
			}
		}
		return "" // unrecognized; PHP strtotime returns false → "" here
	case int:
		return time.Unix(int64(x), 0).UTC().Format(time.RFC3339)
	case int64:
		return time.Unix(x, 0).UTC().Format(time.RFC3339)
	}
	return ""
}

// validChangefreq returns the input only if it is one of the standard values.
func validChangefreq(s string) string {
	switch strings.ToLower(s) {
	case "always", "hourly", "daily", "weekly", "monthly", "yearly", "never":
		return strings.ToLower(s)
	}
	return ""
}

// formatPriority renders a priority to a compact decimal in [0, 1], rounded
// to one place (matches PHP `round($priority, 1)` + clamping). Empty string
// when absent or zero (sitemap protocol treats omitted as default 0.5).
func formatPriority(v any) string {
	var f float64
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		f = x
	case int:
		f = float64(x)
	case int64:
		f = float64(x)
	case string:
		if x == "" {
			return ""
		}
		parsed, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return ""
		}
		f = parsed
	default:
		return ""
	}
	f = math.Round(f*10) / 10
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// metaGet reads a nested meta map value, e.g. metaGet(meta, "sitemap", "lastmod").
func metaGet(meta map[string]any, keys ...string) any {
	var cur any = meta
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func metaString(meta map[string]any, keys ...string) string {
	s, _ := metaGet(meta, keys...).(string)
	return s
}

// headerSetter mirrors the subset of http.Header the plugin uses. Declared
// as an interface so the plugin doesn't have to import net/http just to
// type-assert a param.
type headerSetter interface {
	Set(string, string)
	Get(string) string
}
