// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package config loads and merges pigo configuration.
//
// Configuration files are YAML. All files matching config/*.yml are loaded in
// alphabetical order; the FIRST value for a given key wins (matches Pico's
// behavior — see Pico.php::loadConfig). Unknown keys are preserved under the
// Custom map so themes and plugins can reach them via {{ config.foo }}.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Config holds every setting pigo recognizes. Keys mirror Pico's
// config/config.yml.template one-for-one so existing Pico sites work unchanged.
type Config struct {
	SiteTitle      string         `yaml:"site_title"`
	BaseURL        string         `yaml:"base_url"`
	RewriteURL     *bool          `yaml:"rewrite_url"`
	Debug          bool           `yaml:"debug"`
	Timezone       string         `yaml:"timezone"`
	Locale         string         `yaml:"locale"`
	Theme          string         `yaml:"theme"`
	ThemesURL      string         `yaml:"themes_url"`
	ThemeConfig    map[string]any `yaml:"theme_config"`
	TwigConfig     map[string]any `yaml:"twig_config"`
	DateFormat     string         `yaml:"date_format"`
	PagesOrderBy   string         `yaml:"pages_order_by"`
	PagesOrderByM  string         `yaml:"pages_order_by_meta"`
	PagesOrder     string         `yaml:"pages_order"`
	ContentDir     string         `yaml:"content_dir"`
	ContentExt     string         `yaml:"content_ext"`
	ContentConfig  map[string]any `yaml:"content_config"`
	AssetsDir      string         `yaml:"assets_dir"`
	AssetsURL      string         `yaml:"assets_url"`
	PluginsURL     string         `yaml:"plugins_url"`
	TemplateEngine string         `yaml:"template_engine"` // "twig" or "go"; pigo extension

	// Plugins lists plugins to enable for this site by registered name. At
	// Site init, pigo resolves each name against the in-process plugin
	// registry (see plugin.Register). Example:
	//
	//   plugins:
	//     - PicoFilePrefixes
	//     - PicoRobots
	//
	// Each plugin's own config section stays top-level under the plugin's
	// name (Pico convention) and arrives in Custom.
	Plugins []string `yaml:"plugins"`

	// Custom holds any additional keys (including plugin-specific ones like
	// "DummyPlugin.enabled"). Accessible from templates via {{ config.foo }}.
	Custom map[string]any `yaml:",inline"`
}

// Defaults returns a Config pre-populated with Pico's defaults.
func Defaults() *Config {
	return &Config{
		SiteTitle:      "Pico",
		Theme:          "default",
		DateFormat:     "%D %T",
		PagesOrderBy:   "alpha",
		PagesOrder:     "asc",
		ContentExt:     ".md",
		AssetsDir:      "assets/",
		TemplateEngine: "twig",
		ThemeConfig:    map[string]any{},
		TwigConfig: map[string]any{
			"autoescape":       "html",
			"strict_variables": false,
			"charset":          "utf-8",
		},
		ContentConfig: map[string]any{
			"extra":     true,
			"breaks":    false,
			"escape":    false,
			"auto_urls": true,
		},
		Custom: map[string]any{},
	}
}

// Load reads every *.yml file in dir (alphabetically) and merges them onto the
// defaults. First non-zero value wins per Pico semantics.
func Load(dir string) (*Config, error) {
	cfg := Defaults()
	if dir == "" {
		return cfg, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		if e.Name() == "config.yml.template" {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		partial := &Config{Custom: map[string]any{}}
		if err := yaml.Unmarshal(data, partial); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		cfg.merge(partial)
	}

	return cfg, nil
}

// merge applies src on top of c; first value wins, so c's non-zero fields stay.
func (c *Config) merge(src *Config) {
	if c.SiteTitle == "" || c.SiteTitle == "Pico" {
		if src.SiteTitle != "" {
			c.SiteTitle = src.SiteTitle
		}
	}
	if c.BaseURL == "" {
		c.BaseURL = src.BaseURL
	}
	if c.RewriteURL == nil {
		c.RewriteURL = src.RewriteURL
	}
	if !c.Debug && src.Debug {
		c.Debug = src.Debug
	}
	if c.Timezone == "" {
		c.Timezone = src.Timezone
	}
	if c.Locale == "" {
		c.Locale = src.Locale
	}
	if c.Theme == "" || c.Theme == "default" {
		if src.Theme != "" {
			c.Theme = src.Theme
		}
	}
	if c.ThemesURL == "" {
		c.ThemesURL = src.ThemesURL
	}
	for k, v := range src.ThemeConfig {
		if _, exists := c.ThemeConfig[k]; !exists {
			c.ThemeConfig[k] = v
		}
	}
	for k, v := range src.TwigConfig {
		if _, exists := c.TwigConfig[k]; !exists {
			c.TwigConfig[k] = v
		}
	}
	if c.DateFormat == "" || c.DateFormat == "%D %T" {
		if src.DateFormat != "" {
			c.DateFormat = src.DateFormat
		}
	}
	if c.PagesOrderBy == "" || c.PagesOrderBy == "alpha" {
		if src.PagesOrderBy != "" {
			c.PagesOrderBy = src.PagesOrderBy
		}
	}
	if c.PagesOrderByM == "" {
		c.PagesOrderByM = src.PagesOrderByM
	}
	if c.PagesOrder == "" || c.PagesOrder == "asc" {
		if src.PagesOrder != "" {
			c.PagesOrder = src.PagesOrder
		}
	}
	if c.ContentDir == "" {
		c.ContentDir = src.ContentDir
	}
	if c.ContentExt == "" || c.ContentExt == ".md" {
		if src.ContentExt != "" {
			c.ContentExt = src.ContentExt
		}
	}
	for k, v := range src.ContentConfig {
		if _, exists := c.ContentConfig[k]; !exists {
			c.ContentConfig[k] = v
		}
	}
	if c.AssetsDir == "" || c.AssetsDir == "assets/" {
		if src.AssetsDir != "" {
			c.AssetsDir = src.AssetsDir
		}
	}
	if c.AssetsURL == "" {
		c.AssetsURL = src.AssetsURL
	}
	if c.PluginsURL == "" {
		c.PluginsURL = src.PluginsURL
	}
	if c.TemplateEngine == "" || c.TemplateEngine == "twig" {
		if src.TemplateEngine != "" {
			c.TemplateEngine = src.TemplateEngine
		}
	}
	// First file wins for the plugins list — subsequent files are ignored
	// rather than appended, matching the first-value-wins merge style used
	// throughout this function.
	if len(c.Plugins) == 0 {
		c.Plugins = src.Plugins
	}
	for k, v := range src.Custom {
		if _, exists := c.Custom[k]; !exists {
			c.Custom[k] = v
		}
	}
}

// Get returns a setting by flat key, checking structured fields first, then Custom.
// Used by filters like %config.foo% substitution.
func (c *Config) Get(key string) (any, bool) {
	switch key {
	case "site_title":
		return c.SiteTitle, true
	case "base_url":
		return c.BaseURL, true
	case "theme":
		return c.Theme, true
	case "content_dir":
		return c.ContentDir, true
	case "content_ext":
		return c.ContentExt, true
	case "assets_dir":
		return c.AssetsDir, true
	case "date_format":
		return c.DateFormat, true
	}
	v, ok := c.Custom[key]
	return v, ok
}

// AsMap returns a flat map suitable for template rendering ({{ config.X }}).
func (c *Config) AsMap() map[string]any {
	out := map[string]any{
		"site_title":          c.SiteTitle,
		"base_url":            c.BaseURL,
		"rewrite_url":         derefBool(c.RewriteURL),
		"debug":               c.Debug,
		"timezone":            c.Timezone,
		"locale":              c.Locale,
		"theme":               c.Theme,
		"themes_url":          c.ThemesURL,
		"theme_config":        c.ThemeConfig,
		"twig_config":         c.TwigConfig,
		"date_format":         c.DateFormat,
		"pages_order_by":      c.PagesOrderBy,
		"pages_order_by_meta": c.PagesOrderByM,
		"pages_order":         c.PagesOrder,
		"content_dir":         c.ContentDir,
		"content_ext":         c.ContentExt,
		"content_config":      c.ContentConfig,
		"assets_dir":          c.AssetsDir,
		"assets_url":          c.AssetsURL,
		"plugins_url":         c.PluginsURL,
		"template_engine":     c.TemplateEngine,
	}
	for k, v := range c.Custom {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}
