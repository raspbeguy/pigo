// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package server exposes pigo as an http.Handler.
package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/raspbeguy/pigo/config"
	"github.com/raspbeguy/pigo/content"
	"github.com/raspbeguy/pigo/plugin"
	"github.com/raspbeguy/pigo/render"
	"github.com/raspbeguy/pigo/router"
	"github.com/raspbeguy/pigo/tree"
)

// Deps is the server's dependency bundle. All fields are required except
// TwigRegistrar (nil falls back to an empty registrar) and Logger (nil
// falls back to slog.Default()).
type Deps struct {
	Config        *config.Config
	ContentDir    string
	ThemesDir     string
	AssetsDir     string
	RootDir       string
	MountPath     string
	Pages         []*content.Page
	PageByID      map[string]*content.Page
	PageTree      *tree.Node
	Scanner       *content.Scanner
	Markdown      *content.Markdown
	ThemeDir      string
	Dispatcher    *plugin.Dispatcher
	TwigRegistrar *render.TwigRegistrar
	Version       string
	Logger        *slog.Logger
}

// New returns an http.Handler that serves the site.
func New(d *Deps) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	mux := http.NewServeMux()

	// Static asset serving: /assets/, /themes/, /plugins/
	assetsDir := d.Config.AssetsDir
	if assetsDir == "" {
		assetsDir = "assets/"
	}
	if !filepath.IsAbs(assetsDir) {
		assetsDir = filepath.Join(d.RootDir, assetsDir)
	}
	if d.AssetsDir != "" {
		assetsDir = d.AssetsDir
	}
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
	mux.Handle("/themes/", http.StripPrefix("/themes/", http.FileServer(http.Dir(d.ThemesDir))))
	if pluginsDir := filepath.Join(d.RootDir, "plugins"); dirExists(pluginsDir) {
		mux.Handle("/plugins/", http.StripPrefix("/plugins/", http.FileServer(http.Dir(pluginsDir))))
	}

	// Everything else goes through pigo's page handler.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handlePage(w, r, d, assetsDir)
	})

	return accessLog(d.Logger, mux)
}

func handlePage(w http.ResponseWriter, r *http.Request, d *Deps, assetsDir string) {
	reqPath := router.EvaluateRequestURL(r, d.MountPath)
	_ = d.Dispatcher.Dispatch(plugin.OnRequestURL, &reqPath)

	filePath, ok := router.ResolveFilePath(d.ContentDir, reqPath, d.Config.ContentExt)
	// Even when no file was found, give the plugin a candidate path derived
	// from reqPath so it can decide how to rewrite it (matches Pico's
	// Pico::discoverRequestFile behavior of always passing a path).
	if filePath == "" {
		if reqPath == "" {
			filePath = filepath.Join(d.ContentDir, "index"+d.Config.ContentExt)
		} else {
			filePath = filepath.Join(d.ContentDir, reqPath+d.Config.ContentExt)
		}
	}
	_ = d.Dispatcher.Dispatch(plugin.OnRequestFile, &filePath)

	// Honor plugin-mutated filePath: if the plugin pointed filePath at a
	// real file (typical for prefix-stripping or alias plugins), accept it.
	// Conversely, if the plugin blanked filePath, fall through to 404.
	if filePath == "" {
		ok = false
	} else if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		ok = true
	} else {
		ok = false
	}

	var page *content.Page
	status := http.StatusOK
	if ok {
		_ = d.Dispatcher.Dispatch(plugin.OnContentLoading)
		p, err := d.Scanner.Load(filePath, router.IDFromPath(d.ContentDir, filePath, d.Config.ContentExt))
		if err != nil {
			d.Logger.Error("content load failed", "path", r.URL.Path, "file", filePath, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			// Plugin cancelled the load via OnSinglePageLoading — treat as 404.
			ok = false
		} else {
			page = p
			// Replace with canonical entry from scanner output so prev/next chains link.
			if canonical, ok := d.PageByID[page.ID]; ok {
				canonical.RawContent = page.RawContent
				canonical.Meta = page.Meta
				canonical.ModificationTime = page.ModificationTime
				page = canonical
			}
			_ = d.Dispatcher.Dispatch(plugin.OnContentLoaded, &page.RawContent)
		}
	}
	if !ok {
		// Root-level static file fallback: Pico's convention is that
		// favicon.ico, robots.txt, google-verify tokens and similar live
		// at the site root and are served as plain files. Mirror Pico's
		// .htaccess rules: deny config/content/vendor/.git/etc. and
		// dotfile paths (except .well-known/), then serve whatever's
		// left.
		if reqPath != "" && !staticBlocked(reqPath) {
			if staticPath, safe := resolveRootStatic(d.RootDir, reqPath); safe {
				if info, err := os.Stat(staticPath); err == nil && !info.IsDir() {
					http.ServeFile(w, r, staticPath)
					return
				}
			}
		}
		_ = d.Dispatcher.Dispatch(plugin.On404ContentLoading)
		errPage, _ := d.Scanner.LoadErrorPage(reqPath)
		if errPage == nil {
			errPage = &content.Page{
				ID:         "404",
				Title:      "Error 404",
				Meta:       map[string]any{"title": "Error 404"},
				RawContent: "Woops. Looks like this page doesn't exist.",
			}
		}
		page = errPage
		status = http.StatusNotFound
		_ = d.Dispatcher.Dispatch(plugin.On404ContentLoaded, &page.RawContent)
	}

	// Build URL placeholders.
	baseURL := router.DetectBaseURL(r, d.Config.BaseURL, d.MountPath)
	rewrite := true
	if d.Config.RewriteURL != nil {
		rewrite = *d.Config.RewriteURL
	}
	baseURLQ := ""
	if !rewrite {
		baseURLQ = "?"
	}
	themesURL := firstNonEmpty(d.Config.ThemesURL, baseURL+"/themes")
	themeURL := themesURL + "/" + d.Config.Theme
	assetsURL := firstNonEmpty(d.Config.AssetsURL, baseURL+"/assets")
	pluginsURL := firstNonEmpty(d.Config.PluginsURL, baseURL+"/plugins")

	placeholders := router.PlaceholderMap{
		BaseURL:    baseURL,
		BaseURLQ:   baseURLQ,
		ThemeURL:   themeURL,
		ThemesURL:  themesURL,
		AssetsURL:  assetsURL,
		PluginsURL: pluginsURL,
		Version:    d.Version,
		Meta:       page.Meta,
		Config:     d.Config.AsMap(),
	}

	// Populate URLs on all pages for template consumption.
	for _, p := range d.Pages {
		p.URL = router.PageURL(baseURL, p.ID, rewrite)
	}
	page.URL = router.PageURL(baseURL, page.ID, rewrite)

	// OnCurrentPageDiscovered fires once the canonical page (including its
	// prev/next chain) is identified.
	_ = d.Dispatcher.Dispatch(plugin.OnCurrentPageDiscovered, page, page.PrevPage, page.NextPage)

	// OnContentParsing: raw content, before any substitution or markdown.
	_ = d.Dispatcher.Dispatch(plugin.OnContentParsing, &page.RawContent)

	// Pre-render content for the current page.
	substituted := placeholders.Substitute(page.RawContent)
	_ = d.Dispatcher.Dispatch(plugin.OnContentPrepared, &substituted)
	html, err := d.Markdown.Render(substituted)
	if err != nil {
		d.Logger.Error("markdown render failed", "path", r.URL.Path, "page", page.ID, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = d.Dispatcher.Dispatch(plugin.OnContentParsed, &html)
	page.Content = html

	// Build render filters and context.
	filters := &render.Filters{
		Pages:        d.Pages,
		Config:       d.Config.AsMap(),
		Meta:         page.Meta,
		BaseURL:      baseURL,
		Rewrite:      rewrite,
		Tree:         d.PageTree,
		Markdown:     d.Markdown,
		Placeholders: placeholders,
		Req:          r,
	}

	ctx := render.BuildContext(
		d.Config.AsMap(),
		d.Pages,
		page, page.PrevPage, page.NextPage,
		d.PageTree,
		baseURL, themeURL, themesURL, assetsURL, pluginsURL, d.Version, d.Config.SiteTitle,
		page.Meta, html,
	)
	// Expose the normalized request path to plugins/themes. Useful for
	// plugins serving virtual URLs (robots.txt, sitemap.xml) that need to
	// identify the request in OnPageRendering without racing on shared
	// plugin state.
	ctx["request_url"] = reqPath

	// Pick template engine.
	engine := d.Config.TemplateEngine
	if engine == "" {
		engine = "twig"
	}
	var renderer render.Renderer
	switch engine {
	case "go", "gotmpl", "html":
		r2, err := render.NewGoRenderer(d.ThemeDir, filters)
		if err != nil {
			d.Logger.Error("go-template setup failed", "path", r.URL.Path, "theme_dir", d.ThemeDir, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderer = r2
	default:
		renderer = render.NewTwigRenderer(d.ThemeDir, filters, d.TwigRegistrar)
	}

	// Determine which template to use (meta.template, else "index").
	tmplName := "index"
	if v, ok := page.Meta["template"].(string); ok && v != "" {
		tmplName = v
	}
	_ = d.Dispatcher.Dispatch(plugin.OnPageRendering, &tmplName, &ctx, w.Header(), &status)

	body, err := renderer.Render(tmplName, ctx)
	if err != nil {
		d.Logger.Error("template render failed", "path", r.URL.Path, "template", tmplName, "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = d.Dispatcher.Dispatch(plugin.OnPageRendered, &body, w.Header(), &status)

	// Default Content-Type only if no plugin already set one (so e.g. a
	// robots.txt plugin can emit text/plain).
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// staticBlockedDirs lists the directories pigo refuses to serve as static
// files, matching Pico's .htaccess:
//
//	RewriteRule ^(\.git|config|content-sample|content|lib|vendor)(/|$) - [R=404,L]
//
// plus `plugins` (Pico 2.x renamed lib → plugins; pigo's own plugins/ dir
// is already served under /plugins/ with its own file server, so blocking
// it at the root-static layer is correct).
var staticBlockedDirs = []string{
	".git",
	"config",
	"content",
	"content-sample",
	"lib",
	"plugins",
	"vendor",
}

// staticBlocked reports whether reqPath is a Pico-style denied path.
// Denies anything under the blocked dirs and any dotfile path — except
// .well-known/ (Let's Encrypt, security.txt).
func staticBlocked(reqPath string) bool {
	for _, d := range staticBlockedDirs {
		if reqPath == d || strings.HasPrefix(reqPath, d+"/") {
			return true
		}
	}
	// Dotfile paths: allow .well-known/..., deny the rest.
	if strings.HasPrefix(reqPath, ".well-known/") || reqPath == ".well-known" {
		return false
	}
	if strings.HasPrefix(reqPath, ".") {
		return true
	}
	// Also deny any segment starting with a dot (/foo/.secret).
	for _, seg := range strings.Split(reqPath, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// resolveRootStatic joins rootDir with the request path and verifies the
// result stays within rootDir (defends against "../" traversal). Returns
// the joined path and whether it's safe to serve.
func resolveRootStatic(rootDir, reqPath string) (string, bool) {
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", false
	}
	joined := filepath.Join(rootAbs, filepath.FromSlash(reqPath))
	resolved, err := filepath.Abs(joined)
	if err != nil {
		return "", false
	}
	rootWithSep := rootAbs + string(filepath.Separator)
	if resolved != rootAbs && !strings.HasPrefix(resolved, rootWithSep) {
		return "", false
	}
	return resolved, true
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
