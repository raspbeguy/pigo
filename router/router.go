// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package router resolves HTTP requests to content files.
package router

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// EvaluateRequestURL reduces an incoming HTTP request to a "request path"
// (e.g. "sub/page"). It honors both URL-rewriting mode (path-based) and
// query-string mode (?sub/page). Mirrors Pico::evaluateRequestUrl.
//
// basePath is the URL path where pigo is mounted (usually "" or "/").
func EvaluateRequestURL(r *http.Request, basePath string) string {
	// Query string takes precedence, matching Pico behavior (Pico.php:1248).
	raw := r.URL.RawQuery
	if raw != "" && !strings.Contains(raw, "=") {
		return Normalize(raw)
	}
	urlPath := r.URL.Path
	// Strip base path prefix.
	if basePath != "" && strings.HasPrefix(urlPath, basePath) {
		urlPath = urlPath[len(basePath):]
	}
	urlPath = strings.TrimPrefix(urlPath, "/")
	return Normalize(urlPath)
}

// Normalize cleans a request path: removes leading/trailing slashes, ".."
// segments and "." segments. Prevents directory traversal.
func Normalize(reqPath string) string {
	if reqPath == "" {
		return ""
	}
	// path.Clean handles ".." and "." collapsing.
	cleaned := path.Clean("/" + reqPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

// ResolveFilePath maps a request path to an on-disk content file.
//
// Lookup order (mirrors Pico::resolveFilePath):
//  1. reqPath == "" → contentDir/index<ext>
//  2. contentDir/reqPath<ext>
//  3. contentDir/reqPath/index<ext>
//
// Returns ("", false) if no match. Files whose basename starts with "_" are
// hidden from direct access and reported as missing.
func ResolveFilePath(contentDir, reqPath, ext string) (string, bool) {
	if reqPath == "" {
		p := filepath.Join(contentDir, "index"+ext)
		if isFile(p) {
			return p, true
		}
		return "", false
	}

	// Reject hidden segments.
	for _, seg := range strings.Split(reqPath, "/") {
		if strings.HasPrefix(seg, "_") {
			return "", false
		}
	}

	// Try as file.
	p := filepath.Join(contentDir, reqPath+ext)
	if isFile(p) {
		return p, true
	}
	// Try as directory index.
	p = filepath.Join(contentDir, reqPath, "index"+ext)
	if isFile(p) {
		return p, true
	}
	return "", false
}

// IDFromPath computes a page id from an absolute content file path, matching
// the id scheme used by the scanner.
func IDFromPath(contentDir, filePath, ext string) string {
	rel, err := filepath.Rel(contentDir, filePath)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	return strings.TrimSuffix(rel, ext)
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
