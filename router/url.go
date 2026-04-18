// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package router

import (
	"net/http"
	"net/url"
	"strings"
)

// DetectBaseURL reconstructs the site's base URL from an HTTP request, matching
// Pico::getBaseUrl. The returned URL has no trailing slash (normalized later by
// callers that need one).
//
// If configured is non-empty, it wins.
func DetectBaseURL(r *http.Request, configured, mountPath string) string {
	if configured != "" {
		return strings.TrimRight(configured, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if x := r.Header.Get("X-Forwarded-Proto"); x != "" {
		scheme = x
	}
	host := r.Host
	if x := r.Header.Get("X-Forwarded-Host"); x != "" {
		host = x
	}
	base := scheme + "://" + host
	if mountPath != "" && mountPath != "/" {
		base += strings.TrimRight(mountPath, "/")
	}
	return base
}

// PageURL builds the public URL for a given page id. If rewriteURL is true,
// links use pretty paths; otherwise they use ?id format. Mirrors
// Pico::getPageUrl — strips a trailing "/index" so the root page URL stays
// base-only.
func PageURL(baseURL, id string, rewriteURL bool) string {
	id = strings.TrimSuffix(id, "/index")
	if id == "index" {
		id = ""
	}
	if id == "" {
		if rewriteURL {
			return baseURL + "/"
		}
		return baseURL + "/"
	}
	if rewriteURL {
		return baseURL + "/" + id
	}
	return baseURL + "/?" + id
}

// PlaceholderMap contains the URL placeholders Pico substitutes into content
// and template strings.
type PlaceholderMap struct {
	BaseURL    string
	BaseURLQ   string // "?" if rewriting disabled, else ""
	ThemeURL   string
	ThemesURL  string
	AssetsURL  string
	PluginsURL string
	Version    string
	Meta       map[string]any
	Config     map[string]any
}

// Substitute replaces Pico-style %foo% placeholders in a string.
// Supported: %base_url%, %base_url%?, %theme_url%, %themes_url%, %assets_url%,
// %plugins_url%, %version%, %meta.X%, %config.X%.
func (pm PlaceholderMap) Substitute(s string) string {
	replacements := []string{
		"%base_url%?", pm.BaseURL + pm.BaseURLQ,
		"%base_url%", pm.BaseURL,
		"%theme_url%", pm.ThemeURL,
		"%themes_url%", pm.ThemesURL,
		"%assets_url%", pm.AssetsURL,
		"%plugins_url%", pm.PluginsURL,
		"%version%", pm.Version,
	}
	s = strings.NewReplacer(replacements...).Replace(s)
	s = substituteDotted(s, "%meta.", pm.Meta)
	s = substituteDotted(s, "%config.", pm.Config)
	return s
}

func substituteDotted(s, prefix string, src map[string]any) string {
	for {
		i := strings.Index(s, prefix)
		if i < 0 {
			return s
		}
		end := strings.Index(s[i+len(prefix):], "%")
		if end < 0 {
			return s
		}
		key := s[i+len(prefix) : i+len(prefix)+end]
		full := s[i : i+len(prefix)+end+1]
		var val string
		if v, ok := src[key]; ok {
			val = toString(v)
		}
		s = strings.Replace(s, full, val, 1)
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	}
	// Best-effort fallback.
	return ""
}

// JoinURL joins a base URL with a path segment, ensuring exactly one slash
// between them. Used for theme_url, assets_url, etc.
func JoinURL(base, seg string) string {
	if seg == "" {
		return base
	}
	if base == "" {
		return seg
	}
	u, err := url.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(seg, "/")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(seg, "/")
	return u.String()
}
