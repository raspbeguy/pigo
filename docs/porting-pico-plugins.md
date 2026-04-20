# Porting a Pico PHP plugin to pigo

This guide walks through translating an existing Pico (PHP) plugin into a
pigo (Go) plugin. pigo's event surface is a 1:1 mirror of Pico's `DummyPlugin`
hooks, so most ports are mechanical.

---

## 1. Mental model

Pigo plugins are **compiled-in Go packages** that register themselves with
a process-wide registry at `init()` time. To make a plugin available, a
binary just needs to import its package (typically via a blank import);
to *enable* it for a particular site, the site's config lists the plugin
by name under `plugins:`.

This gives the "one binary, many sites, different plugins" story for free:
the stock `cmd/pigo` binary compiles in the official plugins, and each site
picks the subset it wants via its own `config.yml`. No rebuild when you
change a site's plugin set.

```yaml
# site-a/config/config.yml
plugins:
  - PicoFilePrefixes
  - PicoRobots

PicoFilePrefixes:
  recursiveDirs: [blog]
PicoRobots:
  robots:
    - user_agents: ["*"]
```

Run:

```sh
pigo --root /srv/site-a
pigo --root /srv/site-b   # different plugins: list → different behavior
```

To add a brand-new plugin, build your own pigo binary: copy
`cmd/pigo/main.go`, add a blank import for the plugin package, `go build`.

Consequences:

- No `PLUGIN_REQUIREMENTS` / `API_VERSION` metadata: Go module versioning
  replaces it.
- `onPluginManuallyLoaded` is never fired — pigo has no runtime-dynamic
  load path (no .so / .wasm / interpreter). Adding a new plugin requires
  a rebuild; enabling, disabling, and reconfiguring existing plugins is
  purely YAML.
- Plugins are singletons per process, not per request.

---

## 2. Skeleton plugin

Two files: the plugin itself and a tiny `init.go` that registers its name.

```go
// myplugin.go
package myplugin

import (
    "github.com/raspbeguy/pigo/config"
    "github.com/raspbeguy/pigo/plugin"
)

type Plugin struct {
    plugin.Base       // gives you Enabled() / SetEnabled() for free
    cfg *config.Config // captured from OnConfigLoaded
}

func (p *Plugin) Name() string        { return "MyPlugin" }
func (p *Plugin) DependsOn() []string { return nil }

func (p *Plugin) HandleEvent(event string, params ...any) error {
    switch event {
    case plugin.OnConfigLoaded:
        p.cfg = params[0].(*config.Config)
    case plugin.OnPageRendering:
        ctx := params[1].(*map[string]any)
        (*ctx)["greeting"] = "hi from MyPlugin"
    }
    return nil
}
```

```go
// init.go — self-registration so the registry knows about us.
package myplugin

import "github.com/raspbeguy/pigo/plugin"

func init() {
    plugin.Register("MyPlugin", func() plugin.Plugin { return &Plugin{} })
}
```

Wire it into a binary (copy of `cmd/pigo/main.go` with one extra import):

```go
package main

import (
    "github.com/raspbeguy/pigo"

    _ "example.com/myplugin"                           // your plugin
    _ "github.com/raspbeguy/pigo/plugins/fileprefixes" // keep shipped plugins available too
    _ "github.com/raspbeguy/pigo/plugins/robots"
)

func main() {
    site, _ := pigo.New(pigo.Options{RootDir: "."})
    _ = site.ListenAndServe(":8080")
}
```

Operators who deploy your binary then enable `MyPlugin` per site by adding
`MyPlugin` to that site's `plugins:` list — no further code or flags.

**Programmatic usage (embedding pigo in a larger Go app).** You can still
pass plugin instances directly via `Options.Plugins`; they merge with the
config-resolved ones (duplicates by name error out so the operator knows).

---

## 3. Pico method → pigo event

Each Pico `AbstractPicoPlugin` method maps to a `plugin.On...` constant.
Params are pigo's dispatch signature; where Pico uses `array &$x` / `string
&$x` (pass-by-reference), pigo uses `*map[K]V` / `*string` pointers.

| Pico method                         | pigo event                   | pigo params                                            |
| ----------------------------------- | ---------------------------- | ------------------------------------------------------ |
| `onPluginsLoaded`                   | `OnPluginsLoaded`            | `[]plugin.Plugin`                                      |
| `onPluginManuallyLoaded`            | `OnPluginManuallyLoaded`     | *never dispatched by pigo*                             |
| `onConfigLoaded`                    | `OnConfigLoaded`             | `*config.Config`                                       |
| `onThemeLoading`                    | `OnThemeLoading`             | `*string` (theme name)                                 |
| `onThemeLoaded`                     | `OnThemeLoaded`              | `string`                                               |
| `onRequestUrl`                      | `OnRequestURL`               | `*string`                                              |
| `onRequestFile`                     | `OnRequestFile`              | `*string` (plugin-mutated path is re-stat'd)           |
| `onContentLoading`                  | `OnContentLoading`           | *(none)*                                               |
| `onContentLoaded`                   | `OnContentLoaded`            | `*string` raw content                                  |
| `on404ContentLoading`               | `On404ContentLoading`        | *(none)*                                               |
| `on404ContentLoaded`                | `On404ContentLoaded`         | `*string`                                              |
| `onMetaParsing`                     | `OnMetaParsing`              | `*string` (yaml text)                                  |
| `onMetaParsed` (or `onMetaHeaders`) | `OnMetaParsed`               | `*map[string]any`                                      |
| `onMetaHeaders`                     | `OnMetaHeaders`              | `*map[string]string` (front-matter aliases)            |
| `onContentParsing`                  | `OnContentParsing`           | `*string`                                              |
| `onContentPrepared`                 | `OnContentPrepared`          | `*string`                                              |
| `onContentParsed`                   | `OnContentParsed`            | `*string` (HTML)                                       |
| `onPagesLoading`                    | `OnPagesLoading`             | *(none)*                                               |
| `onSinglePageLoading`               | `OnSinglePageLoading`        | `*string` id (set `""` to skip)                        |
| `onSinglePageContent`               | `OnSinglePageContent`        | `string` id, `*string` raw                             |
| `onSinglePageLoaded`                | `OnSinglePageLoaded`         | `*content.Page`                                        |
| `onPagesDiscovered`                 | `OnPagesDiscovered`          | `[]*content.Page`                                      |
| `onPagesLoaded`                     | `OnPagesLoaded`              | `[]*content.Page`                                      |
| `onCurrentPageDiscovered`           | `OnCurrentPageDiscovered`    | `*content.Page`, `*content.Page`, `*content.Page`      |
| `onPageTreeBuilt`                   | `OnPageTreeBuilt`            | `*tree.Node`                                           |
| `onPageRendering`                   | `OnPageRendering`            | `*string` tmpl, `*map[string]any` ctx, `http.Header`, `*int` status |
| `onPageRendered`                    | `OnPageRendered`             | `*[]byte`, `http.Header`, `*int` status                |
| `onYamlParserRegistered`            | `OnYAMLParserRegistered`     | *(none — see limitations)*                             |
| `onParsedownRegistered`             | `OnMarkdownRegistered`       | `*content.MarkdownRegistrar`                           |
| `onTwigRegistered`                  | `OnTwigRegistered`           | `*render.TwigRegistrar`                                |

---

## 4. `$this->` helper translation

Pigo plugins don't inherit from a base class the way `AbstractPicoPlugin` does.
Instead, **capture what you need** from the relevant event and store it on
your plugin struct.

| Pico helper                          | pigo equivalent                                               |
| ------------------------------------ | ------------------------------------------------------------- |
| `$this->setEnabled($v)`              | `p.SetEnabled(v)` (requires embedding `plugin.Base`)          |
| `$this->isEnabled()`                 | `p.Enabled()`                                                 |
| `$this->getConfig('key')`            | `p.cfg.<field>` or `p.cfg.Custom["key"]` (capture in `OnConfigLoaded`) |
| `$this->getPluginConfig('key', def)` | `pluginCfg["key"]` where `pluginCfg, _ := p.cfg.Custom["MyPlugin"].(map[string]any)` (with a helper; see example below) |
| `$this->getBaseUrl()`                | captured from `*config.Config.BaseURL` or derived per-request |
| `$this->getBaseThemeUrl()`           | config fields + `themes/<cfg.Theme>`                          |
| `$this->getRequestUrl()`             | capture in `OnRequestURL` (the `*string` is the authoritative value after dispatch) |
| `$this->getPages()`                  | capture in `OnPagesLoaded`                                    |
| `$this->getCurrentPage()`            | capture in `OnCurrentPageDiscovered`                          |
| `$this->triggerEvent('onFoo', [&$x])`| no built-in inter-plugin event bus; declare a Go interface and type-assert recipient plugins, or call methods directly |

### Helper for plugin config

```go
// pluginConfig returns the map[string]any section for plugin key `name`, or
// an empty map if the user hasn't set one.
func pluginConfig(cfg *config.Config, name string) map[string]any {
    if cfg == nil { return nil }
    if v, ok := cfg.Custom[name].(map[string]any); ok { return v }
    return nil
}
```

### Disabling the plugin based on config

```go
func (p *Plugin) HandleEvent(event string, params ...any) error {
    switch event {
    case plugin.OnConfigLoaded:
        p.cfg = params[0].(*config.Config)
        if pluginConfig(p.cfg, "MyPlugin") == nil {
            p.SetEnabled(false) // plugin opts out of all subsequent events
        }
    }
    return nil
}
```

---

## 5. Pass-by-reference: PHP → Go

PHP's `function onFoo(array &$foo)` becomes a pigo event with a pointer param.
Inside `HandleEvent` you type-assert and mutate through the pointer.

```php
public function onPagesLoaded(array &$pages)
{
    foreach ($pages as &$p) {
        $p['url'] = 'https://…';
    }
}
```

```go
case plugin.OnPagesLoaded:
    pages := params[0].([]*content.Page)
    for _, pg := range pages {
        pg.URL = "https://…"
    }
```

For primitive fields:

```php
public function onRequestUrl(&$url) { $url = 'custom'; }
```

```go
case plugin.OnRequestURL:
    url := params[0].(*string)
    *url = "custom"
```

---

## 5b. Cancellation semantics

Plugins occasionally need to opt a request out of the default flow —
e.g. to short-circuit a page load that a virtual-URL plugin is going to
handle itself. Not every event supports this; the contract per event:

| Event | Cancellation signal | Effect |
|---|---|---|
| `OnSinglePageLoading` | set `*id = ""` | Scanner returns `(nil, nil)`; handler treats the request as 404. |
| `OnRequestFile` | set `*filePath = ""` | Handler treats the request as 404 (same 404 fallthrough used for genuine misses). |
| `OnPageRendering` | set `*status = …` + override `*tmpl` + headers | Serves a synthesized response. See §6 for the robots.txt pattern. |
| Any observation event (`OnContentLoaded`, `OnContentParsed`, `OnPageRendered`) | — | No cancellation. Returning an error is logged at warn level but the request continues. |

If an event isn't listed above, the contract is **no cancellation**:
you can read and mutate params, but returning a non-nil error just
surfaces a warn log — the handler keeps going. This mirrors Pico's
observation-heavy event design and avoids having a misbehaving plugin
500 every request.

---

## 6. Response control (Content-Type, status, virtual URLs)

Pigo extends `OnPageRendering` with response metadata so plugins can serve
non-HTML or override status codes — critical for e.g. `robots.txt`,
`sitemap.xml`, or API-style endpoints.

`OnPageRendering` params: `*string tmplName, *map[string]any ctx, http.Header headers, *int status`.

```go
case plugin.OnPageRendering:
    if p.requestURL != "robots.txt" { return nil }
    tmpl   := params[0].(*string)
    ctx    := params[1].(*map[string]any)
    hdrs   := params[2].(http.Header)
    status := params[3].(*int)

    *tmpl = "robots"                           // renders robots.twig
    (*ctx)["robots"] = p.robotsRecords
    hdrs.Set("Content-Type", "text/plain; charset=utf-8")
    *status = http.StatusOK                     // override the 404 pigo defaults to
```

For the above to work with a URL that has no backing content file (e.g.
`/robots.txt`), the plugin:

1. Stashes `*reqPath` in `OnRequestURL` so it knows the request is "its".
2. Accepts that pigo will 404 internally (no content file exists).
3. Overrides template + Content-Type + status in `OnPageRendering`.

If the `Content-Type` header isn't set by any plugin, pigo defaults to
`text/html; charset=utf-8`.

---

## 7. Shipping Twig templates with a plugin

Plugins can bundle their own Twig templates via `go:embed`, register a
directory via `TwigRegistrar.AddPath`, and the user's theme can still
override any template by name (theme dir is searched first).

```go
package robots

import (
    "embed"
    "os"
    "path/filepath"

    "github.com/raspbeguy/pigo/plugin"
    "github.com/raspbeguy/pigo/render"
)

//go:embed theme/*.twig
var themeFS embed.FS

type Plugin struct {
    plugin.Base
    extractedDir string
}

func (p *Plugin) HandleEvent(event string, params ...any) error {
    switch event {
    case plugin.OnTwigRegistered:
        reg := params[0].(*render.TwigRegistrar)
        // stick's multi-loader can't read embed.FS, so extract to a
        // temp dir the first time. Alternatively use reg.Mutate to
        // register a custom stick.Loader backed by embed.FS directly.
        dir, err := extractEmbed(themeFS, "theme")
        if err != nil { return err }
        p.extractedDir = dir
        reg.AddPath(dir)
    }
    return nil
}

func extractEmbed(fsys embed.FS, root string) (string, error) {
    dir, err := os.MkdirTemp("", "plugin-twig-")
    if err != nil { return "", err }
    entries, err := fsys.ReadDir(root)
    if err != nil { return "", err }
    for _, e := range entries {
        data, err := fsys.ReadFile(filepath.Join(root, e.Name()))
        if err != nil { return "", err }
        if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
            return "", err
        }
    }
    return dir, nil
}
```

If you prefer to avoid the temp-dir dance, use `reg.Mutate(fn)` and register
a custom `stick.Loader` backed by `embed.FS` directly.

---

## 8. Testing a plugin

```go
func TestMyPlugin(t *testing.T) {
    site, err := pigo.New(pigo.Options{
        RootDir: "testdata/site",
        Plugins: []plugin.Plugin{&myplugin.Plugin{}},
    })
    if err != nil { t.Fatal(err) }
    ts := httptest.NewServer(site.Handler())
    defer ts.Close()

    res, err := http.Get(ts.URL + "/whatever")
    if err != nil { t.Fatal(err) }
    body, _ := io.ReadAll(res.Body)
    res.Body.Close()
    // assertions on res.StatusCode, res.Header, body…
}
```

The recommended layout for `testdata/site/` mirrors Pico: `config/`,
`content/`, `themes/<theme-name>/`.

---

## 9. Known limitations

- **`OnYAMLParserRegistered`** fires but has no parser handle — pigo uses
  `gopkg.in/yaml.v3` via direct `yaml.Unmarshal`. Plugins that only care
  about the timing can still react; plugins that expected to mutate the
  parser need a different approach.
- **`OnPluginManuallyLoaded`** is not dispatched. Plugins are registered
  once at `pigo.New` time.
- **Go `html/template` engine** does not expose a plugin-facing registrar in
  this release. Use the Twig engine (the Pico default) if your plugin needs
  to ship templates. Plugin contributions to Go templates can be added in a
  follow-up release if needed.
- **Template search order**: theme dir is searched before any
  plugin-registered dir, so themes always win on name collision. This is
  usually desired (themes override plugin defaults); if you ship a template
  that should not be overridable, use a name that theme authors are
  unlikely to collide with.

Runtime-dynamic plugin loading (e.g. dropping `.wasm` / `.go` / subprocess
binaries on a running server) is **not** supported today. Options evaluated
and deferred — with benchmarks and tradeoffs — are collected in
[`future-ideas.md`](future-ideas.md).

---

## 10. Reference implementations

Two official Pico plugins are shipped with pigo and demonstrate the full
porting technique end-to-end:

- [`plugins/fileprefixes/`](../plugins/fileprefixes/) — port of
  [pico-file-prefixes](https://github.com/PhrozenByte/pico-file-prefixes).
  Strips filename prefixes from page URLs (e.g. `blog/20240101.hello.md`
  serves at `/blog/hello`). Demonstrates: `OnConfigLoaded` capture,
  `OnRequestFile` retargeting, `OnCurrentPageDiscovered` URL rewriting.
- [`plugins/robots/`](../plugins/robots/) — port of
  [pico-robots](https://github.com/PhrozenByte/pico-robots). Serves
  `/robots.txt` and `/sitemap.xml` as virtual URLs. Demonstrates: serving
  virtual URLs, response `Content-Type`/status override, shipping embedded
  Twig templates via `go:embed` + `stick.MemoryLoader` + `AddLoader`, using
  `ctx["request_url"]` for per-request URL detection.

Read the source — both are ~200 lines and are the most direct illustration
of everything in this guide.

## 11. Worked example — before/after

A minimal header-rewriting Pico plugin that adds a custom `X-Served-By`
header and inserts a greeting into the render context.

**Pico (PHP):**

```php
class HelloPlugin extends AbstractPicoPlugin
{
    const API_VERSION = 2;

    public function onPageRendering(&$twigTemplate, array &$twigVariables)
    {
        header('X-Served-By: HelloPlugin');
        $twigVariables['greeting'] = 'Hello from PHP';
    }
}
```

**pigo (Go):**

```go
package hello

import (
    "net/http"

    "github.com/raspbeguy/pigo/plugin"
)

type Plugin struct{ plugin.Base }

func (p *Plugin) Name() string        { return "HelloPlugin" }
func (p *Plugin) DependsOn() []string { return nil }

func (p *Plugin) HandleEvent(event string, params ...any) error {
    if event != plugin.OnPageRendering {
        return nil
    }
    ctx     := params[1].(*map[string]any)
    headers := params[2].(http.Header)
    headers.Set("X-Served-By", "HelloPlugin")
    (*ctx)["greeting"] = "Hello from Go"
    return nil
}
```

Build your binary with it:

```go
pigo.New(pigo.Options{
    RootDir: ".",
    Plugins: []plugin.Plugin{&hello.Plugin{}},
})
```

Done.
