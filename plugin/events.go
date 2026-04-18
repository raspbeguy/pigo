// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package plugin

// Event names mirror Pico's PHP event system exactly. See DummyPlugin.php for
// Pico's authoritative list. Plugins should switch on these constants rather
// than raw strings so renames surface at compile time.
//
// Each constant's doc comment describes the params tuple pigo dispatches.
// Params marked *T are mutable (pointer-to-T); plugins may reassign *ptr to
// influence downstream behavior. Params listed bare are pass-by-value.
const (
	// OnPluginsLoaded — params: ([]Plugin)
	// Fired once after all plugins have been topologically sorted.
	OnPluginsLoaded = "onPluginsLoaded"

	// OnPluginManuallyLoaded — NOT DISPATCHED by pigo.
	// Pico fires this when a plugin is dynamically loaded at runtime; pigo
	// plugins are compiled into the binary so this concept does not apply.
	// Kept for source-level compatibility with ports that reference it.
	OnPluginManuallyLoaded = "onPluginManuallyLoaded"

	// OnConfigLoaded — params: (*config.Config)
	// Fired once after all config/*.yml files are merged. Plugins typically
	// capture the pointer for later use and/or read their own section from
	// cfg.Custom.
	OnConfigLoaded = "onConfigLoaded"

	// OnThemeLoading — params: (*string theme)
	// Fired once before the theme dir is resolved. Plugins may reassign
	// *theme to force a different theme.
	OnThemeLoading = "onThemeLoading"

	// OnThemeLoaded — params: (string theme)
	// Fired once after the theme has been resolved.
	OnThemeLoaded = "onThemeLoaded"

	// OnRequestURL — params: (*string url)
	// Fired at the start of every request. Plugins may mutate *url to
	// redirect to a different content path.
	OnRequestURL = "onRequestUrl"

	// OnRequestFile — params: (*string filePath)
	// Fired after initial file resolution. Plugins may reassign *filePath
	// to point at an alternate file; the handler re-stats the path and
	// honors the change if the new target exists.
	OnRequestFile = "onRequestFile"

	// OnContentLoading — params: ()
	// Fired just before the current page's raw content is loaded.
	OnContentLoading = "onContentLoading"

	// On404ContentLoading — params: ()
	// Fired when the request resolved to no file and the 404 page is about
	// to be loaded.
	On404ContentLoading = "on404ContentLoading"

	// On404ContentLoaded — params: (*string rawContent)
	// Fired after 404 content has been read; rawContent is mutable.
	On404ContentLoaded = "on404ContentLoaded"

	// OnContentLoaded — params: (*string rawContent)
	// Fired after the current page's raw content has been read; rawContent
	// is mutable.
	OnContentLoaded = "onContentLoaded"

	// OnMetaParsing — params: (*string yamlText)
	// Fired before YAML front-matter parsing; plugins may mutate the raw
	// yaml text.
	OnMetaParsing = "onMetaParsing"

	// OnMetaParsed — params: (*map[string]any meta)
	// Fired after YAML front-matter parsing; plugins may mutate the parsed
	// meta map.
	OnMetaParsed = "onMetaParsed"

	// OnMetaHeaders — params: (*map[string]string headers)
	// Fired once during Site init. `headers` maps canonical front-matter
	// key → registered alias. Plugins can add their own aliases (e.g.
	// "Sitemap" → "sitemap"). Pigo stores any front-matter key in Meta
	// unchanged, so aliases primarily serve documentation and plugin
	// inter-operation.
	OnMetaHeaders = "onMetaHeaders"

	// OnContentParsing — params: (*string rawContent)
	// Fired before placeholder substitution and Markdown rendering.
	OnContentParsing = "onContentParsing"

	// OnContentPrepared — params: (*string content)
	// Fired after placeholder substitution, just before Markdown rendering.
	OnContentPrepared = "onContentPrepared"

	// OnContentParsed — params: (*string html)
	// Fired after Markdown rendering.
	OnContentParsed = "onContentParsed"

	// OnPagesLoading — params: ()
	// Fired once before the content dir is scanned.
	OnPagesLoading = "onPagesLoading"

	// OnSinglePageLoading — params: (*string id)
	// Fired per-page during scan, before the file is read. Setting *id to
	// "" cancels the load (page is omitted from results).
	OnSinglePageLoading = "onSinglePageLoading"

	// OnSinglePageContent — params: (string id, *string rawContent)
	// Fired per-page after the file is read, before front-matter splitting.
	OnSinglePageContent = "onSinglePageContent"

	// OnSinglePageLoaded — params: (*content.Page page)
	// Fired per-page after the page has been fully parsed. Plugins may
	// mutate any fields; the Page pointer is the final in-memory object.
	OnSinglePageLoaded = "onSinglePageLoaded"

	// OnPagesDiscovered — params: ([]*content.Page pages)
	// Fired once after scanning completes but before sorting.
	OnPagesDiscovered = "onPagesDiscovered"

	// OnPagesLoaded — params: ([]*content.Page pages)
	// Fired once after pages have been sorted and prev/next-linked.
	OnPagesLoaded = "onPagesLoaded"

	// OnCurrentPageDiscovered — params: (*content.Page current, *content.Page prev, *content.Page next)
	// Fired per-request once the canonical Page for this URL is identified.
	OnCurrentPageDiscovered = "onCurrentPageDiscovered"

	// OnPageTreeBuilt — params: (*tree.Node root)
	// Fired once after the page tree is materialized.
	OnPageTreeBuilt = "onPageTreeBuilt"

	// OnPageRendering — params: (*string templateName, *map[string]any ctx, http.Header, *int status)
	// Fired per-request just before template execution. Plugins may mutate
	// any of: the template name, the render context, the response headers
	// (e.g. Content-Type), and the HTTP status code.
	OnPageRendering = "onPageRendering"

	// OnPageRendered — params: (*[]byte body, http.Header, *int status)
	// Fired per-request after template execution, before the body is
	// written. Plugins may rewrite the body or adjust headers/status.
	OnPageRendered = "onPageRendered"

	// OnYAMLParserRegistered — params: () [pigo limitation: no parser handle]
	// Pigo uses yaml.Unmarshal directly; there is no long-lived parser
	// instance to hand out. The event still fires so ports of Pico plugins
	// that only depend on the timing can react.
	OnYAMLParserRegistered = "onYamlParserRegistered"

	// OnMarkdownRegistered — params: (*content.MarkdownRegistrar)
	// Fired once during Site init. Plugins may add goldmark extensions via
	// the registrar.
	OnMarkdownRegistered = "onParsedownRegistered" // kept PHP name for drop-in plugin port

	// OnTwigRegistered — params: (*render.TwigRegistrar)
	// Fired once during Site init if the active template engine is Twig.
	// Plugins may add template search paths, filters, or functions.
	OnTwigRegistered = "onTwigRegistered"
)
