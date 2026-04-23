# pigo

A Go reimplementation of [Pico](https://github.com/picocms/Pico), aiming to be
a drop-in replacement. Drop an existing Pico `content/`, `themes/`, and
`config/` into pigo and serve it with a single static binary.

Pico itself has reached end of life; pigo exists so you can keep running Pico
sites on maintained software.

## Status

**Early development — breaking changes possible between minor releases.**
While pigo is pre-1.0, config keys, CLI flags, plugin lifecycle events,
exported Go APIs, the router's request-path semantics, and the template
context shape can still change. Pin a specific release tag if you're
building on pigo, and skim the release notes on each bump. See
[`docs/parity/SUMMARY.md`](docs/parity/SUMMARY.md) for the current diff
against Pico.

Core rendering works on both template engines, unit + integration tests
pass, CI enforces parity with upstream Pico. Plugins require a Go port —
PHP plugins cannot be loaded.

## Install / run

**Prebuilt binary** (Linux/macOS/Windows, amd64/arm64/armv7):
[GitHub Releases](https://github.com/raspbeguy/pigo/releases/latest). Each
archive ships the binary, `README.md`, `LICENSE`, and a `.sha256` checksum.

**From source** (Go 1.26+):

```sh
go install github.com/raspbeguy/pigo/cmd/pigo@latest
pigo --root /path/to/your/pico/site --addr :8080
```

**From a local checkout** (same tree, e.g. to add your own plugins):

```sh
git clone https://github.com/raspbeguy/pigo && cd pigo
go build -o pigo ./cmd/pigo
./pigo --root /path/to/your/pico/site --addr :8080
```

Flags:

- `--root` — site root containing `config/`, `content/`, `themes/`.
- `--config`, `--content`, `--themes`, `--assets` — override individual dirs.
- `--addr` — HTTP listen address (default `:8080`).
- `--list-plugins` — print the plugins this binary knows about and exit.
- `--log-level` — `debug` | `info` | `warn` | `error` (default `info`). Also
  settable via `PIGO_LOG_LEVEL` env or `log_level` in `config/*.yml`.
- `--log-format` — `text` (logfmt-style, default) or `json`. Also settable via
  `PIGO_LOG_FORMAT` env or `log_format` in `config/*.yml`.

Precedence: flag > env > config > default. One structured `request` line is
emitted per HTTP response at `info` level; 500s emit an additional `error`
line with the request path and cause.

## Template engines

Pick via `template_engine` in `config/*.yml`:

| value  | engine                         | file extension |
| ------ | ------------------------------ | -------------- |
| `twig` | [stick](https://github.com/tyler-sommer/stick) (Twig 1.x) — **default** | `.twig` |
| `go`   | Go `html/template`             | `.html`        |

Both engines receive the same context. Custom filters/functions are registered
under identical names:

| name          | kind      | purpose                                    |
| ------------- | --------- | ------------------------------------------ |
| `markdown`    | filter    | render Markdown, substitute `%meta.X%` etc |
| `url`         | filter    | resolve `%base_url%`, `%assets_url%`, …    |
| `link`        | filter    | page-id → public URL                       |
| `content`     | filter    | rendered HTML of another page              |
| `sort_by`     | filter    | sort array by dotted key path              |
| `map`         | filter    | extract values at dotted key path          |
| `url_param`   | function  | read a query-string parameter              |
| `form_param`  | function  | read a POST form parameter                 |
| `pages`       | function  | query the page tree (start/depth/offset)   |

## Template variables

Matches Pico: `site_title`, `base_url`, `theme_url`, `themes_url`, `assets_url`,
`plugins_url`, `version`, `config`, `meta`, `content`, `pages` (string-keyed
by page id, iterates in the configured sort order — `{% for p in pages %}` and
`pages[id]` both work, just like Pico's PHP `$pages`), `pages_by_id` (alias of
`pages`, retained for templates that reference it by name), `current_page`,
`previous_page`, `next_page`, `page_tree`.

## Meta headers

Supported YAML keys in content front-matter (either `--- … ---` or the
deprecated `/* … */` delimiter):

`Title`, `Description`, `Author`, `Date`, `Formatted Date`, `Time`, `Robots`,
`Template`, `Hidden`, plus any custom field (all lowercased into `meta.*`).

## Plugin API

Plugins are Go packages compiled into a pigo binary. Each plugin self-registers
with a process-wide registry at init time. Each site enables the plugins it
wants via its `config.yml`:

```yaml
plugins:
  - PicoFilePrefixes
  - PicoRobots

PicoFilePrefixes:
  recursiveDirs: [blog]
```

The stock `pigo` binary ships with `PicoFilePrefixes` and `PicoRobots`
available. One binary, many sites, different plugin sets — just point
`--root` at different site directories:

```sh
pigo --root /srv/site-a    # uses site-a/config/config.yml
pigo --root /srv/site-b    # different plugins: list, different behavior
pigo --list-plugins         # what this binary knows about
```

To add a **new** plugin, copy `cmd/pigo/main.go` into your own repo, add a
blank import for the plugin's package, `go build`. Operators then enable it
per site via YAML — no further code changes.

Event names (`plugin.On…`) mirror Pico's PHP events exactly. For a full
porting walkthrough — the event table, `$this->` helper translation, response
control, shipping Twig templates — see
[`docs/porting-pico-plugins.md`](docs/porting-pico-plugins.md). Two official
Pico plugins are shipped as reference ports: [`plugins/fileprefixes`](plugins/fileprefixes/)
and [`plugins/robots`](plugins/robots/).

Considering runtime-dynamic plugins (gRPC subprocesses, WASM, Yaegi
interpreter)? See [`docs/future-ideas.md`](docs/future-ideas.md) for the
research and why each was deferred.

## Migrating a Pico site

1. Copy `content/`, `themes/`, `config/` into a directory.
2. Point pigo at it: `pigo --root <dir>`.
3. If a Twig theme uses constructs stick doesn't support, adjust or switch to
   the Go template engine and author a theme there.
4. For any Pico PHP plugin, port to Go using the same event names.

Root-level files (`favicon.ico`, `robots.txt`, Google site-verification tokens,
etc.) placed directly in `--root/` are served as static files after content
lookup fails and before the 404 page. No separate webserver needed. Pico's
`.htaccess` deny rules are mirrored: `config/`, `content/`, `content-sample/`,
`lib/`, `plugins/`, `vendor/`, `.git/`, and any dotfile path (except
`.well-known/`) always 404 — raw markdown, site config, and dependency
sources are never exposed.

In production behind nginx/Apache, set `serve_root_static: false` in
`config/config.yml` so the webserver in front handles static files directly —
it's faster and keeps pigo focused on dynamic content.

## Known divergences from Pico

See [docs/parity/SUMMARY.md](docs/parity/SUMMARY.md) for the auto-generated
diff of pigo's public surface against Pico's, across seven categories
(events, config keys, template variables, meta headers, Twig filters/
functions, CLI flags). Regenerate with
`go run ./cmd/parity --pico-dir ../Pico`; CI re-runs it against the Pico
commit pinned in `docs/parity/pico.ref` and fails on drift.

Quick highlights not covered by the name-surface diff:

- Plugins must be Go, not PHP.
- Twig support via stick is ~Twig 1.x; a few advanced PHP-Twig features
  (e.g. some filter edge cases) may not be identical.
- Markdown is rendered by [goldmark](https://github.com/yuin/goldmark) instead
  of Parsedown Extra — output should be ~identical, but minor whitespace
  differences are possible.
- The root-static blocklist compares request paths as-is. On
  case-insensitive filesystems (macOS, most Windows), a request to
  `/Config/config.yml` can resolve to `--root/config/config.yml` and slip
  past the `config/` prefix match. Run pigo on case-sensitive storage
  (Linux, case-sensitive APFS) or front it with a webserver that handles
  static files.
- `resolveRootStatic` joins the request path with `--root` and confirms the
  result stays inside `--root`, but doesn't call `filepath.EvalSymlinks`.
  Symlinks under `--root` that point outside are followed. Operator-
  controlled; low real risk, but worth knowing.

## License

MIT, matching upstream Pico.
