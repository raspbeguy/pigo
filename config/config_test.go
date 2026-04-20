// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Defaults()
	if c.SiteTitle != "Pico" {
		t.Errorf("SiteTitle default: got %q, want %q", c.SiteTitle, "Pico")
	}
	if c.ContentExt != ".md" {
		t.Errorf("ContentExt default: got %q, want .md", c.ContentExt)
	}
	if c.TemplateEngine != "twig" {
		t.Errorf("TemplateEngine default: got %q, want twig", c.TemplateEngine)
	}
}

func TestLoadMergeAlphabetical(t *testing.T) {
	dir := t.TempDir()
	// a.yml sets site_title; b.yml also sets it — a wins because it's first
	// alphabetically (matches Pico: first value wins).
	writeFile(t, filepath.Join(dir, "a.yml"), "site_title: First\ntheme: darkly\n")
	writeFile(t, filepath.Join(dir, "b.yml"), "site_title: Second\ndebug: true\n")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SiteTitle != "First" {
		t.Errorf("merge: got %q, want First", cfg.SiteTitle)
	}
	if cfg.Theme != "darkly" {
		t.Errorf("theme: got %q", cfg.Theme)
	}
	if !cfg.Debug {
		t.Errorf("debug: got false, want true")
	}
}

func TestLoadMissingDir(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	// Returns defaults.
	if cfg.SiteTitle != "Pico" {
		t.Errorf("expected defaults, got %+v", cfg)
	}
}

func TestCustomKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yml"), "my_custom_setting: hello\nDummyPlugin.enabled: false\n")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := cfg.Custom["my_custom_setting"]; !ok || v != "hello" {
		t.Errorf("custom: got %v, ok=%v", v, ok)
	}
	m := cfg.AsMap()
	if m["my_custom_setting"] != "hello" {
		t.Errorf("AsMap did not surface custom key")
	}
}

func TestServeRootStatic(t *testing.T) {
	// Default (key absent): AsMap surfaces true.
	cfg, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServeRootStatic != nil {
		t.Errorf("default: got %v, want nil pointer", cfg.ServeRootStatic)
	}
	if cfg.AsMap()["serve_root_static"] != true {
		t.Errorf("default AsMap: got %v, want true", cfg.AsMap()["serve_root_static"])
	}

	// First file wins (a.yml=false beats b.yml=true).
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yml"), "serve_root_static: false\n")
	writeFile(t, filepath.Join(dir, "b.yml"), "serve_root_static: true\n")
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServeRootStatic == nil || *cfg.ServeRootStatic != false {
		t.Errorf("merge: got %v, want *false", cfg.ServeRootStatic)
	}
	if cfg.AsMap()["serve_root_static"] != false {
		t.Errorf("merge AsMap: got %v, want false", cfg.AsMap()["serve_root_static"])
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
