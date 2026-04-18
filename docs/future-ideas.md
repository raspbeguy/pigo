# Future ideas

Loose roadmap notes — things we've evaluated and deliberately deferred.
Each section captures the research so a future reader doesn't have to
rediscover the tradeoffs.

---

## Runtime-installable plugins (no rebuild)

Today the plugin registry is static: adding a brand-new plugin to a pigo
deployment requires building a binary that imports the plugin package.
Enabling/disabling/reconfiguring existing plugins is already purely YAML,
but "operator drops a new plugin artifact and restarts" is not supported.

If that ever becomes a real ask, three routes are plausible. The ranking
below reflects how well each preserves pigo's current API (pointer
mutation, chatty per-request dispatch).

### A. Yaegi — Go interpreter in-process (most promising for pigo)

Like Traefik's plugin system. A plugin is a `.go` source file placed in a
configured directory; at startup pigo interprets it via
[yaegi](https://github.com/traefik/yaegi) and registers it alongside
compiled-in plugins.

- **Per-call cost:** single-digit μs (interpreter runs in-process, sees
  real Go types). Acceptable for pigo's ~10-events-per-request dispatch.
- **API preservation:** plugins still see `*content.Page`, `http.Header`,
  `*config.Config` exactly as today — pointer mutation works. No API
  redesign needed.
- **Limitations:** yaegi implements a subset of Go; some reflection paths,
  `unsafe`, and a handful of stdlib/third-party packages don't work.
  Debugging is harder than compiled code.
- **Security:** runs in-process, so a malicious plugin can reach anything
  the pigo process can. Needs operator trust in the plugin source.

**Verdict:** closest match to pigo's design. Best candidate if runtime
installability becomes important.

### B. WebAssembly — wazero + a codegen SDK

Like [knqyf263/go-plugin](https://github.com/knqyf263/go-plugin). Plugins
compile to `.wasm`, loaded by [wazero](https://wazero.io/) at startup.
Sandboxed, truly language-agnostic, hot-swappable without restart.

- **Per-call cost:** sub-μs for function invocation; real cost is
  marshalling Go types across the wasm boundary. `map[string]any`,
  `http.Header`, `*content.Page` all need explicit serialization — no
  pointer-mutation semantics. Expect 10–50 μs per event depending on
  payload size.
- **API preservation:** requires redesigning every event's param signature
  to be wasm-friendly (flat, copyable types; explicit return-of-diff for
  mutations).
- **Security:** strong sandbox by default (no filesystem/network unless
  host explicitly imports capabilities).
- **Operational story:** plugin artifacts are architecture-independent
  `.wasm` files, cleanly distributable.

**Verdict:** attractive security and distribution story; awkward fit for
pigo's pointer-heavy plugin API.

### C. gRPC subprocess plugins — HashiCorp go-plugin / Grafana model

Each plugin is its own Go binary; pigo launches it as a subprocess and
talks over gRPC. Used by Terraform, Vault, Nomad, Grafana backends.

- **Per-call cost:** ~30–50 μs with HashiCorp go-plugin, ~100 μs with
  vanilla gRPC over Unix socket. Dominated by context switches, not
  serialization (protobuf marshal of small messages is ~100–400 ns).
- **Per-request impact at pigo scale:** 10 events × N plugins × ~30 μs =
  **~0.5–1 ms per request** with 2 plugins. A typical pigo request is
  ~1–5 ms end-to-end, so plugin dispatch alone would add a 10–30 % tax
  even for plugins that do nothing but observe events.
- **API preservation:** zero. Every event's params need to be redesigned:
  no `*content.Page`, no `http.Header`, no `*config.Config` shared — only
  serializable copies + explicit diffs to return. `map[string]any` is
  especially painful (not protobuf-native). This is the dealbreaker, not
  the latency.
- **What it does buy:** crash isolation (a plugin panic doesn't take the
  host down), strong process sandboxing, and runtime install without
  recompile or restart of the host.
- **Why HashiCorp / Grafana accept the cost:** their plugin APIs are
  coarse-grained — one gRPC call per user operation (Terraform
  ReadResource, Grafana QueryData), not 10 per HTTP request. The 30 μs is
  rounding error next to database queries or HTTP backends.

**Verdict:** wrong shape for pigo. Good fit for coarse-grained plugin
APIs where crash isolation matters more than per-request latency.

### Summary

| Option | Per-event cost | API redesign needed | Security | Install story |
|---|---|---|---|---|
| **Yaegi** | μs | none | in-process | drop `.go`, restart |
| **WASM (wazero)** | 10–50 μs | yes (flatten types) | sandboxed | drop `.wasm`, hot-swap |
| **gRPC (go-plugin)** | 30–100 μs | wholesale | subprocess isolation | drop binary, restart |
| *Current: in-process registry* | ~10 ns | — | trust compiled-in code | rebuild binary |

### Research references
- [HashiCorp go-plugin README](https://github.com/hashicorp/go-plugin) — subprocess+gRPC, ~30–50 μs overhead per call.
- [Using gRPC for local IPC (F. Werner)](https://www.mpi-hd.mpg.de/personalhomes/fwerner/research/2021/09/grpc-for-ipc/) — ~100 μs unary-call latency over Unix sockets.
- [Grafana plugin SDK (Go)](https://github.com/grafana/grafana-plugin-sdk-go) — production example of the gRPC model.
- [Yaegi (Traefik)](https://github.com/traefik/yaegi) — in-process Go interpreter for plugins.
- [wazero](https://wazero.io/) and [knqyf263/go-plugin](https://github.com/knqyf263/go-plugin) — WASM-based plugin systems for Go.
- [Caddy modules](https://caddyserver.com/docs/architecture) and [xcaddy](https://github.com/caddyserver/xcaddy) — the compile-in model pigo currently uses.

---

## Multi-tenant in-process hosting

Serving many sites from a single pigo process, routed by HTTP `Host`
header or URL path prefix. The plugin registry + per-site config landed
in the current design already isolates per-site plugin state, so this is
purely a routing layer on top of `server.Handler`.

Deferred because the operational model ("same binary, one process per
site, different `--root`") is simpler and already covers the stated need.
Revisit when someone wants to collapse many small sites into a single
server to save memory.

---

## Custom inter-plugin events

Pico PHP exposes `triggerEvent('onFoo', [&$arg])` so plugins can fire
their own events for other plugins to extend. PicoRobots uses this for
`onRobots` and `onSitemap` to let downstream plugins modify the rendered
output.

Pigo's current dispatcher is name-based and one-directional (pigo → all
plugins). Plugins wanting to collaborate today have to define a Go
interface and type-assert on each other's instances — which works but
couples compile-time.

A small addition worth considering if the plugin ecosystem grows: expose
`dispatcher.Dispatch(event, params...)` to plugins themselves (via a
handle passed through a registration event) so they can fire arbitrary
named events and other plugins can subscribe via `HandleEvent`. Preserves
the existing API shape; purely additive.

---

## Go `html/template` registrar

Only the Twig engine currently lets plugins register their own template
search paths (via `TwigRegistrar.AddLoader` / `AddPath`). The Go
`html/template` engine has no equivalent — plugins shipping templates
(like PicoRobots) won't work there. Adding a `GoTmplRegistrar` is
mechanically straightforward; deferred because the Twig engine is the
Pico default and no user has asked yet.
