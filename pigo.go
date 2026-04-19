// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package pigo is a Go reimplementation of the Pico flat-file CMS.
// It aims to be a drop-in replacement: existing Pico content/, themes/, and
// config/ directories work unchanged. Two template engines are supported
// (Twig via stick, and Go's html/template), selectable via the
// template_engine config key.
package pigo

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/raspbeguy/pigo/config"
	"github.com/raspbeguy/pigo/content"
	"github.com/raspbeguy/pigo/plugin"
	"github.com/raspbeguy/pigo/render"
	"github.com/raspbeguy/pigo/router"
	"github.com/raspbeguy/pigo/server"
	"github.com/raspbeguy/pigo/tree"
)

// Version is the pigo release string. Exposed to templates as {{ version }}.
const Version = "0.1.0"

// Options configures a pigo site. RootDir is the only required field.
type Options struct {
	RootDir    string // Site root (contains config/, content/, themes/, plugins/).
	ConfigDir  string // Override config dir (default: RootDir/config).
	ContentDir string // Override content dir (default: config.content_dir or RootDir/content).
	ThemesDir  string // Override themes dir (default: RootDir/themes).
	AssetsDir  string // Override assets dir (default: RootDir/assets).
	Plugins    []plugin.Plugin
	MountPath  string // URL prefix (e.g. "/pico") when hosting under a sub-path.
	Logger     *slog.Logger // nil → slog.Default().
}

// Site is a live pigo instance, reloadable via Reload().
type Site struct {
	opts          Options
	cfg           *config.Config
	pages         []*content.Page
	pageByID      map[string]*content.Page
	pageTree      *tree.Node
	scanner       *content.Scanner
	markdown      *content.Markdown
	dispatcher    *plugin.Dispatcher
	twigRegistrar *render.TwigRegistrar
	contentDir    string
	themeDir      string
	logger        *slog.Logger
}

// Logger returns the site's structured logger.
func (s *Site) Logger() *slog.Logger { return s.logger }

// New builds a Site, loading config and scanning content.
func New(opts Options) (*Site, error) {
	if opts.RootDir == "" {
		return nil, fmt.Errorf("pigo: RootDir is required")
	}
	if opts.ConfigDir == "" {
		opts.ConfigDir = filepath.Join(opts.RootDir, "config")
	}
	if opts.ThemesDir == "" {
		opts.ThemesDir = filepath.Join(opts.RootDir, "themes")
	}

	cfg, err := config.Load(opts.ConfigDir)
	if err != nil {
		return nil, err
	}

	plugins, err := resolvePlugins(cfg.Plugins, opts.Plugins)
	if err != nil {
		return nil, err
	}

	dispatcher, err := plugin.NewDispatcher(plugins)
	if err != nil {
		return nil, err
	}
	if err := dispatcher.Dispatch(plugin.OnPluginsLoaded, dispatcher.Plugins()); err != nil {
		return nil, err
	}
	if err := dispatcher.Dispatch(plugin.OnConfigLoaded, cfg); err != nil {
		return nil, err
	}

	// OnThemeLoading fires before the theme dir is resolved so a plugin can
	// swap the theme.
	if err := dispatcher.Dispatch(plugin.OnThemeLoading, &cfg.Theme); err != nil {
		return nil, err
	}

	lg := opts.Logger
	if lg == nil {
		lg = slog.Default()
	}
	s := &Site{opts: opts, cfg: cfg, dispatcher: dispatcher, logger: lg}

	// Resolve content dir.
	s.contentDir = opts.ContentDir
	if s.contentDir == "" {
		if cfg.ContentDir != "" {
			s.contentDir = cfg.ContentDir
			if !filepath.IsAbs(s.contentDir) {
				s.contentDir = filepath.Join(opts.RootDir, s.contentDir)
			}
		} else {
			// Fall back to RootDir/content.
			s.contentDir = filepath.Join(opts.RootDir, "content")
		}
	}
	// Expose the resolved absolute path to plugins via cfg.ContentDir so
	// they don't need access to RootDir to locate content files.
	cfg.ContentDir = s.contentDir

	s.themeDir = filepath.Join(opts.ThemesDir, cfg.Theme)
	if err := dispatcher.Dispatch(plugin.OnThemeLoaded, cfg.Theme); err != nil {
		return nil, err
	}

	// Markdown: built, then exposed to plugins so they can register goldmark
	// extensions before any rendering happens.
	s.markdown = content.NewMarkdown(cfg.ContentConfig)
	mdReg := content.NewMarkdownRegistrar(s.markdown)
	if err := dispatcher.Dispatch(plugin.OnMarkdownRegistered, mdReg); err != nil {
		return nil, err
	}

	// YAML parser: pigo uses yaml.Unmarshal directly, so this event fires
	// with no parser handle. Plugins that only care about the lifecycle
	// timing still get the callback.
	if err := dispatcher.Dispatch(plugin.OnYAMLParserRegistered); err != nil {
		return nil, err
	}

	// Meta headers: fire once so plugins can register custom front-matter
	// aliases. Pigo keeps any YAML key in Meta unchanged, so the map is
	// informational; plugins mostly use the timing as "pre-scan" init.
	metaHeaders := defaultMetaHeaders()
	if err := dispatcher.Dispatch(plugin.OnMetaHeaders, &metaHeaders); err != nil {
		return nil, err
	}

	// Twig registrar: fire once and retain the registrar for per-request
	// renderer construction. Only fire when the active engine is twig; the
	// event has no meaning for Go html/template (which doesn't support
	// plugin template registration in this release).
	s.twigRegistrar = render.NewTwigRegistrar()
	engine := cfg.TemplateEngine
	if engine == "" {
		engine = "twig"
	}
	if engine == "twig" {
		if err := dispatcher.Dispatch(plugin.OnTwigRegistered, s.twigRegistrar); err != nil {
			return nil, err
		}
	}

	s.scanner = &content.Scanner{
		Dir:        s.contentDir,
		Ext:        cfg.ContentExt,
		DateFormat: cfg.DateFormat,
		Dispatcher: dispatcher,
	}

	if err := s.loadPages(); err != nil {
		return nil, err
	}
	return s, nil
}

// defaultMetaHeaders enumerates the front-matter keys pigo recognizes natively.
// Passed mutably to OnMetaHeaders so plugins can register their own aliases.
func defaultMetaHeaders() map[string]string {
	return map[string]string{
		"Title":          "title",
		"Description":    "description",
		"Author":         "author",
		"Date":           "date",
		"Date Formatted": "date_formatted",
		"Time":           "time",
		"Robots":         "robots",
		"Template":       "template",
		"Hidden":         "hidden",
	}
}

func (s *Site) loadPages() error {
	if err := s.dispatcher.Dispatch(plugin.OnPagesLoading); err != nil {
		return err
	}
	pages, err := s.scanner.ScanAll()
	if err != nil {
		if os.IsNotExist(err) {
			pages = nil
		} else {
			return err
		}
	}
	if err := s.dispatcher.Dispatch(plugin.OnPagesDiscovered, pages); err != nil {
		return err
	}
	content.SortPages(pages, s.cfg.PagesOrderBy, s.cfg.PagesOrder, s.cfg.PagesOrderByM)
	content.LinkPrevNext(pages)
	s.pages = pages
	s.pageByID = map[string]*content.Page{}
	for _, p := range pages {
		s.pageByID[p.ID] = p
	}
	s.pageTree = tree.Build(pages)
	if err := s.dispatcher.Dispatch(plugin.OnPagesLoaded, pages); err != nil {
		return err
	}
	if err := s.dispatcher.Dispatch(plugin.OnPageTreeBuilt, s.pageTree); err != nil {
		return err
	}
	return nil
}

// Reload re-scans content and re-parses config. Useful for long-running
// servers on sites that change without a restart.
func (s *Site) Reload() error {
	cfg, err := config.Load(s.opts.ConfigDir)
	if err != nil {
		return err
	}
	s.cfg = cfg
	s.markdown = content.NewMarkdown(cfg.ContentConfig)
	s.scanner.Ext = cfg.ContentExt
	s.scanner.DateFormat = cfg.DateFormat
	return s.loadPages()
}

// Config exposes the current parsed config.
func (s *Site) Config() *config.Config { return s.cfg }

// Handler returns an http.Handler serving the site.
func (s *Site) Handler() http.Handler {
	return server.New(&server.Deps{
		Config:        s.cfg,
		ContentDir:    s.contentDir,
		ThemesDir:     s.opts.ThemesDir,
		AssetsDir:     s.opts.AssetsDir,
		RootDir:       s.opts.RootDir,
		MountPath:     s.opts.MountPath,
		Pages:         s.pages,
		PageByID:      s.pageByID,
		PageTree:      s.pageTree,
		Scanner:       s.scanner,
		Markdown:      s.markdown,
		ThemeDir:      s.themeDir,
		Dispatcher:    s.dispatcher,
		TwigRegistrar: s.twigRegistrar,
		Version:       Version,
		Logger:        s.logger,
	})
}

// ListenAndServe is a convenience wrapper for a blocking HTTP server.
func (s *Site) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.Handler())
}

// URL constructs a public URL for a page id given the request context.
// Handy for plugins that need to resolve links outside of template rendering.
func URL(baseURL, id string, rewrite bool) string {
	return router.PageURL(baseURL, id, rewrite)
}

// resolvePlugins merges the config-declared `plugins:` list (resolved
// against the global plugin registry) with any programmatically supplied
// Options.Plugins, deduplicating by Plugin.Name(). The config-resolved
// plugins come first, preserving the order the operator wrote; programmatic
// additions come after. Unknown names produce an error that lists what IS
// registered, to make diagnosis obvious.
func resolvePlugins(names []string, explicit []plugin.Plugin) ([]plugin.Plugin, error) {
	out := make([]plugin.Plugin, 0, len(names)+len(explicit))
	seen := map[string]bool{}
	for _, name := range names {
		factory, ok := plugin.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("pigo: config references unknown plugin %q (registered: %v)",
				name, plugin.Registered())
		}
		p := factory()
		pname := p.Name()
		if seen[pname] {
			return nil, fmt.Errorf("pigo: plugin %q listed twice in config", pname)
		}
		seen[pname] = true
		out = append(out, p)
	}
	for _, p := range explicit {
		pname := p.Name()
		if seen[pname] {
			return nil, fmt.Errorf("pigo: plugin %q provided both via config and Options.Plugins", pname)
		}
		seen[pname] = true
		out = append(out, p)
	}
	return out, nil
}
