# pigo

A Go reimplementation of [Pico](https://github.com/picocms/Pico), aiming to be
a drop-in replacement. Drop an existing Pico `content/`, `themes/`, and
`config/` into pigo and serve it with a single static binary.

Pico itself has reached end of life; pigo exists so you can keep running Pico
sites on maintained software.

## Status

Early. Core rendering works (both engines), unit + integration tests pass.
Plugins require a Go port — PHP plugins cannot be loaded.

## Install / run

```sh
cd pigo
go build -o pigo ./cmd/pigo
./pigo --root /path/to/your/pico/site --addr :8080
```

Flags:

- `--root` — site root containing `config/`, `content/`, `themes/`.
- `--config`, `--content`, `--themes`, `--assets` — override individual dirs.
- `--addr` — HTTP listen address (default `:8080`).

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
`plugins_url`, `version`, `config`, `meta`, `content`, `pages` (ordered slice),
`pages_by_id` (map lookup), `current_page`, `previous_page`, `next_page`,
`page_tree`.

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

## Known divergences from Pico

- Plugins must be Go, not PHP.
- Twig support via stick is ~Twig 1.x; a few advanced PHP-Twig features
  (e.g. some filter edge cases) may not be identical.
- `pages` is exposed as an ordered slice rather than an ordered associative
  array; use `pages_by_id` for direct id lookup.
- Markdown is rendered by [goldmark](https://github.com/yuin/goldmark) instead
  of Parsedown Extra — output should be ~identical, but minor whitespace
  differences are possible.

## License

MIT, matching upstream Pico.
