// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestStaticBlocked covers the Pico-mirrored deny rules. Each row is an
// independent decision, no shared state.
func TestStaticBlocked(t *testing.T) {
	cases := []struct {
		path    string
		blocked bool
	}{
		// Blocked: config / content / vendor / .git and friends.
		{"config/config.yml", true},
		{"config", true},
		{"content/index.md", true},
		{"content", true},
		{"content-sample/foo.md", true},
		{"lib/thing.php", true},
		{"plugins/whatever.go", true},
		{"vendor/autoload.php", true},
		{".git/config", true},
		{".git", true},
		// Blocked: dotfiles anywhere in the path.
		{".htaccess", true},
		{".env", true},
		{"foo/.secret", true},
		// Allowed: well-known path exception.
		{".well-known/security.txt", false},
		{".well-known", false},
		// Allowed: plain root files the migration doc promises.
		{"favicon.ico", false},
		{"robots.txt", false},
		{"google1234567890.html", false},
		{"LICENSE", false},
		// Edge cases.
		{"", false}, // empty path is handled by the caller; blocked set has no empty dir.
	}
	for _, tc := range cases {
		if got := staticBlocked(tc.path); got != tc.blocked {
			t.Errorf("staticBlocked(%q) = %v, want %v", tc.path, got, tc.blocked)
		}
	}
}

// TestResolveRootStatic checks traversal refusal and happy path. Uses a
// tempdir with a sentinel file to confirm the resolved path is usable.
func TestResolveRootStatic(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		req  string
		safe bool
	}{
		{"favicon.ico", true},
		{"sub/file.txt", true},
		{"../outside", false},
		{"../../etc/passwd", false},
		{"sub/../ok.txt", true}, // resolves inside root
	}
	for _, tc := range cases {
		got, safe := resolveRootStatic(dir, tc.req)
		if safe != tc.safe {
			t.Errorf("resolveRootStatic(%q) safe=%v, want %v (got=%q)", tc.req, safe, tc.safe, got)
			continue
		}
		if safe {
			abs, _ := filepath.Abs(dir)
			if !strings.HasPrefix(got, abs) {
				t.Errorf("resolveRootStatic(%q) path %q escaped %q", tc.req, got, abs)
			}
		}
	}
}

// TestRemoteIP covers IPv4, IPv6 brackets, and the fallback when
// RemoteAddr has no port.
func TestRemoteIP(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"127.0.0.1:8080", "127.0.0.1"},
		{"[::1]:8080", "::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"192.168.1.1", "192.168.1.1"}, // malformed (no port) → passthrough
		{"", ""},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tc.in
		if got := remoteIP(r); got != tc.out {
			t.Errorf("remoteIP(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

// TestStatusRecorderCountsBytes exercises the access-log recorder's
// Write path (implicit 200) and an explicit WriteHeader(404) to make
// sure the status captures the first call only.
func TestStatusRecorderCountsBytes(t *testing.T) {
	w := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	n, err := rec.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if rec.status != http.StatusOK || rec.bytes != 5 {
		t.Errorf("after Write: status=%d bytes=%d, want 200 / 5", rec.status, rec.bytes)
	}
	// A WriteHeader after Write should NOT re-record the status — the
	// first write implicitly froze it at 200.
	rec.WriteHeader(http.StatusInternalServerError)
	if rec.status != http.StatusOK {
		t.Errorf("WriteHeader after Write changed status: got %d, want 200", rec.status)
	}
}
