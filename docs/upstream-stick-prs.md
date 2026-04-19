# Upstream stick PRs surfaced by pigo dogfooding

While porting the `hashtagueule` Pico site (PHP-Twig 1.x theme) onto pigo, seven independent gaps in [`tyler-sommer/stick`](https://github.com/tyler-sommer/stick) were identified, patched, and contributed upstream. All seven are now merged into `main` and shipped as part of stick **v1.0.9**. Pigo's `go.mod` tracks that release; no local `replace` directive is needed anymore.

Kept as a record of the contribution and for the per-PR summaries.

| # | Branch | Summary |
|---|---|---|
| A | `feature/include-array-ignore-missing` | `{% include [a, b, c] %}` array form + `ignore missing` modifier. Resolved the TODO at `parse/parse_tag.go:376`. New `IncludeNode.IgnoreMissing` field + `NewIncludeNodeWithOptions` constructor (`NewIncludeNode` retained as shim for back-compat). Executor iterates candidates via `env.Loader.Load`, picks the first that resolves; `ignore missing` swallows the load error. |
| B | `feature/endblock-name` | `{% endblock NAME %}` — accepts the optional closing block name and validates it matches the opening (Twig 3.x parity). Resolved the TODO at `parse/parse_tag.go:135`. |
| C | `feature/twig-builtin-tests` | New `twig/tests` package mirroring the `twig/filter` layout, registers Twig 3.x's built-in tests (`defined`, `divisible by`, `empty`, `even`, `iterable`, `null`/`none`, `odd`, `same as`) on `twig.New`. Includes a faithful `is defined` interceptor in `exec.go` that doesn't evaluate the left operand (so `{% set x = null %}; x is defined` correctly reports `true`). Plus a small parser fix to accept `null`/`none` keywords as test names after `is`. |
| D | `feature/twig-builtin-filters` | Implements six TODO'd filter stubs in `twig/filter/filter.go` to PHP-Twig 3.x semantics: `split`, `sort`, `slice`, `striptags`, `nl2br`, `number_format`. Stub-state was "return val unchanged", which broke any template using `\|split` (silent runtime errors) or any of the others (silent wrong output). |
| E | `feature/if-nil-else-no-panic` | Fixes a nil-pointer panic on `{% for x in xs if cond %}` when no item matches the inline `if`. Two-line fix in two places: `parseFor` constructs the per-iteration filter with an empty Else body (not nil), and the executor's IfNode walk gains a defensive `if node.Else == nil { return nil }` so any caller that builds an IfNode without an Else gets a clean no-op. |
| F | `feature/filter-merge-mixed` | `{}\|merge([x])` (hash + list) used to silently drop the list and return the empty hash. Restructure `filterMerge` so the hash-only path returns only when the right side is also a hash; otherwise fall through to the existing sequence-building path. Real-world impact: the tagblog theme's `{% set tbpages = tbpages\|merge([page]) %}` accumulator was a no-op every iteration, so `All Articles (0)` rendered instead of `All Articles (107)`. |
| G | `feature/spaceless-tag` | Implements the Twig 1.x `{% spaceless %}…{% endspaceless %}` tag (whitespace stripping between HTML tags via `>\s+<` → `><`, matching PHP `preg_replace`). Deprecated in Twig 2.x, removed in Twig 3.x in favor of the `\|spaceless` filter, but still common in older themes. Mirrors the existing `{% filter %}` shape exactly. |

## Provenance

Issue and PR body drafts live in the scratch stick clone at `/home/alpine/repo/stick/` (untracked; kept for reference).

| # | Issue body draft | PR body draft |
|---|---|---|
| A | `ISSUE_DRAFT.md` | `PR_DESCRIPTION_include-array-ignore-missing.md` |
| B | `ISSUE_DRAFT_endblock.md` | `PR_DESCRIPTION_endblock-name.md` |
| C | `ISSUE_DRAFT_tests.md` | `PR_DESCRIPTION_twig-builtin-tests.md` |
| D | `ISSUE_DRAFT_filters.md` | `PR_DESCRIPTION_twig-builtin-filters.md` |
| E | `ISSUE_DRAFT_nilelse.md` | `PR_DESCRIPTION_if-nil-else-no-panic.md` |
| F | `ISSUE_DRAFT_merge.md` | `PR_DESCRIPTION_filter-merge-mixed.md` |
| G | `ISSUE_DRAFT_spaceless.md` | `PR_DESCRIPTION_spaceless-tag.md` |
| (umbrella) | `ISSUE_COMMENT_followup.md` | — |
