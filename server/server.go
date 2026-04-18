// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package server exposes pigo as an http.Handler.
package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/raspbeguy/pigo/config"
	"github.com/raspbeguy/pigo/content"
	"github.com/raspbeguy/pigo/plugin"
	"github.com/raspbeguy/pigo/render"
	"github.com/raspbeguy/pigo/router"
	"github.com/raspbeguy/pigo/tree"
)

// Deps is the server's dependency bundle. All fields are required except
// TwigRegistrar (nil falls back to an empty registrar).
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
}

// New returns an http.Handler that serves the site.
func New(d *Deps) http.Handler {
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

	return mux
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

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
