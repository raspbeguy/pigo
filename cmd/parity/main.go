// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Command parity compares pigo's public surface against Pico's and emits a
// machine-readable diff plus a human-readable summary.
//
// For each of seven categories (events, config keys, template variables, meta
// headers, Twig filters, Twig functions, CLI flags) the tool extracts the
// item list from a canonical source file on each side with a regex, and
// classifies every name as pico-only, pigo-only, or shared.
//
// Outputs:
//   - docs/parity/data.json   — full structured diff
//   - docs/parity/SUMMARY.md  — rendered from data.json + exemptions.yaml
//
// Modes:
//
//	go run ./cmd/parity --pico-dir ../Pico            # regenerate outputs
//	go run ./cmd/parity --pico-dir ../Pico --check    # exit 1 on drift
//
// The tool intentionally compares name surfaces only. Behavioural divergence
// (rendering, routing, 404 fallback, pagination) is called out in the
// rendered summary from exemptions.yaml but not mechanically checked.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Category is one comparison bucket (events, config_keys, …).
type Category struct {
	Key         string     `json:"-"`
	Title       string     `json:"-"`
	SourcePico  string     `json:"source_pico"`
	SourcePigo  string     `json:"source_pigo"`
	Pico        []string   `json:"pico"`
	Pigo        []string   `json:"pigo"`
	Both        []pairItem `json:"both"`
	PicoOnly    []string   `json:"pico_only"`
	PigoOnly    []string   `json:"pigo_only"`
	Note        string     `json:"note,omitempty"`
}

// pairItem holds the original names on each side for an item that is
// considered shared after normalisation. Pico's onFoo and pigo's OnFoo
// map to the same entry; the struct keeps both spellings so the rendered
// summary shows what each side calls it.
type pairItem struct {
	Pico string `json:"pico"`
	Pigo string `json:"pigo"`
}

// Exemptions is the hand-curated divergence annotations loaded from
// docs/parity/exemptions.yaml. Each category's pico_only and pigo_only
// lists may carry a free-form reason string.
type Exemptions struct {
	Categories map[string]struct {
		PicoOnly []exemptionItem `yaml:"pico_only"`
		PigoOnly []exemptionItem `yaml:"pigo_only"`
	} `yaml:",inline"`
	Behavioural []struct {
		Name   string `yaml:"name"`
		Reason string `yaml:"reason"`
	} `yaml:"behavioural"`
}

type exemptionItem struct {
	Name   string `yaml:"name"`
	Reason string `yaml:"reason"`
}

func main() {
	var (
		picoDir   = flag.String("pico-dir", "", "path to a Pico checkout (required)")
		outDir    = flag.String("out", "docs/parity", "output directory")
		check     = flag.Bool("check", false, "exit non-zero if outputs would differ from the committed versions")
		picoRef   = flag.String("pico-ref-file", "docs/parity/pico.ref", "file containing the expected Pico commit SHA (for informational header only)")
	)
	flag.Parse()
	if *picoDir == "" {
		fmt.Fprintln(os.Stderr, "parity: --pico-dir is required")
		os.Exit(2)
	}

	// pigoRoot is the working directory: every extractor resolves files
	// relative to it. Running the tool from anywhere else is the caller's
	// problem to fix.
	pigoRoot, err := os.Getwd()
	if err != nil {
		fatal(err)
	}

	ref, _ := os.ReadFile(filepath.Join(pigoRoot, *picoRef))
	pinnedRef := strings.TrimSpace(string(ref))

	categories, err := extractAll(pigoRoot, *picoDir)
	if err != nil {
		fatal(err)
	}

	exemptions, err := loadExemptions(filepath.Join(pigoRoot, *outDir, "exemptions.yaml"))
	if err != nil {
		fatal(err)
	}

	dataJSON, err := renderJSON(categories)
	if err != nil {
		fatal(err)
	}
	summaryMD, err := renderMarkdown(categories, exemptions, pinnedRef)
	if err != nil {
		fatal(err)
	}

	if *check {
		if err := diffFile(filepath.Join(pigoRoot, *outDir, "data.json"), dataJSON); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := diffFile(filepath.Join(pigoRoot, *outDir, "SUMMARY.md"), summaryMD); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	jsonPath := filepath.Join(pigoRoot, *outDir, "data.json")
	mdPath := filepath.Join(pigoRoot, *outDir, "SUMMARY.md")
	if err := os.WriteFile(jsonPath, dataJSON, 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(mdPath, summaryMD, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d categories)\nwrote %s\n", jsonPath, len(categories), mdPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "parity:", err)
	os.Exit(1)
}

// extractAll runs every extractor and returns the categories in a stable order.
func extractAll(pigoRoot, picoDir string) ([]Category, error) {
	readPigo := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(pigoRoot, rel)) }
	readPico := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(picoDir, rel)) }

	var cats []Category

	// Events — pigo's go const block vs Pico's triggerEvent() calls.
	pigoEvents, err := readPigo("plugin/events.go")
	if err != nil {
		return nil, fmt.Errorf("read pigo events: %w", err)
	}
	picoEvents, err := readPico("lib/Pico.php")
	if err != nil {
		return nil, fmt.Errorf("read pico events: %w", err)
	}
	cats = append(cats, classify(Category{
		Key:        "events",
		Title:      "Plugin events",
		SourcePigo: "plugin/events.go",
		SourcePico: "lib/Pico.php",
		Pigo:       extractPigoEvents(pigoEvents),
		Pico:       extractPicoEvents(picoEvents),
	}))

	// Config keys — pigo's struct yaml tags vs Pico's config.yml.template.
	pigoCfg, err := readPigo("config/config.go")
	if err != nil {
		return nil, err
	}
	picoCfg, err := readPico("config/config.yml.template")
	if err != nil {
		return nil, err
	}
	cats = append(cats, classify(Category{
		Key:        "config_keys",
		Title:      "Config keys",
		SourcePigo: "config/config.go",
		SourcePico: "config/config.yml.template",
		Pigo:       extractPigoConfigKeys(pigoCfg),
		Pico:       extractPicoConfigKeys(picoCfg),
	}))

	// Template variables — pigo's BuildContext map vs Pico's getTwigVariables.
	pigoCtx, err := readPigo("render/context.go")
	if err != nil {
		return nil, err
	}
	cats = append(cats, classify(Category{
		Key:        "template_vars",
		Title:      "Template variables",
		SourcePigo: "render/context.go",
		SourcePico: "lib/Pico.php",
		Pigo:       extractPigoContextKeys(pigoCtx),
		Pico:       extractPicoTwigVariables(picoEvents),
	}))

	// Meta headers — pigo's defaultMetaHeaders vs Pico's getMetaHeaders.
	pigoPigo, err := readPigo("pigo.go")
	if err != nil {
		return nil, err
	}
	cats = append(cats, classify(Category{
		Key:        "meta_headers",
		Title:      "Meta header keys",
		SourcePigo: "pigo.go",
		SourcePico: "lib/Pico.php",
		Pigo:       extractPigoMetaHeaders(pigoPigo),
		Pico:       extractPicoMetaHeaders(picoEvents),
	}))

	// Twig filters & functions — pigo's registerPicoFilters vs Pico's
	// PicoTwigExtension.getFilters/getFunctions (plus the `content` filter
	// registered directly in Pico.php::getTwig).
	pigoTwig, err := readPigo("render/twig.go")
	if err != nil {
		return nil, err
	}
	picoExt, err := readPico("lib/PicoTwigExtension.php")
	if err != nil {
		return nil, err
	}
	picoFilters, picoFunctions := extractPicoTwigFiltersFunctions(picoExt, picoEvents)

	cats = append(cats, classify(Category{
		Key:        "twig_filters",
		Title:      "Twig filters (Pico-specific)",
		SourcePigo: "render/twig.go",
		SourcePico: "lib/PicoTwigExtension.php",
		Pigo:       extractPigoTwigNames(pigoTwig, "Filters"),
		Pico:       picoFilters,
		Note:       "Only Pico-specific filters are diffed. Standard PHP-Twig built-ins (upper, lower, date, …) are provided by stick on the pigo side; their coverage is tracked via stick's own tests.",
	}))
	cats = append(cats, classify(Category{
		Key:        "twig_functions",
		Title:      "Twig functions (Pico-specific)",
		SourcePigo: "render/twig.go",
		SourcePico: "lib/PicoTwigExtension.php",
		Pigo:       extractPigoTwigNames(pigoTwig, "Functions"),
		Pico:       picoFunctions,
		Note:       "Only Pico-specific functions are diffed. Standard PHP-Twig built-ins live in stick.",
	}))

	// CLI flags — pigo-only by construction (Pico has no CLI).
	pigoMain, err := readPigo("cmd/pigo/main.go")
	if err != nil {
		return nil, err
	}
	cats = append(cats, classify(Category{
		Key:        "cli_flags",
		Title:      "CLI flags",
		SourcePigo: "cmd/pigo/main.go",
		SourcePico: "(n/a — Pico is web-only)",
		Pigo:       extractPigoFlags(pigoMain),
		Pico:       nil,
		Note:       "Pico is a PHP web app with no CLI. Every entry here is a pigo-only operational flag by design.",
	}))

	return cats, nil
}

// -------- extractors: pigo side --------

var (
	pigoEventConstRE = regexp.MustCompile(`(?m)^\s*On\w+\s*=\s*"([^"]+)"`)
	pigoYamlTagRE    = regexp.MustCompile("yaml:\"([a-zA-Z0-9_]+)")
	pigoMapKeyRE     = regexp.MustCompile(`(?m)^\s*"([a-zA-Z0-9_]+)"\s*:`)
	pigoFlagRE       = regexp.MustCompile(`flag\.(?:String|Bool|Int|Int64|Float64|Duration|Var)\(\s*(?:&\w+\s*,\s*)?"([a-zA-Z0-9_-]+)"`)
)

// extractPigoEvents pulls the event names from plugin/events.go's const block.
func extractPigoEvents(src []byte) []string {
	var out []string
	for _, m := range pigoEventConstRE.FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// extractPigoConfigKeys pulls `yaml:"foo"` tags out of config/config.go, keeping
// only struct-field-level tags (excludes `yaml:",inline"` on Custom).
func extractPigoConfigKeys(src []byte) []string {
	var out []string
	for _, m := range pigoYamlTagRE.FindAllSubmatch(src, -1) {
		name := string(m[1])
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return dedupe(out)
}

// extractPigoContextKeys extracts the map keys returned by BuildContext.
// The function's return map is a bare map[string]any{...} literal; we scan
// the file for "key": entries inside that specific function to avoid false
// positives from other maps in the file.
func extractPigoContextKeys(src []byte) []string {
	start := bytes.Index(src, []byte("func BuildContext("))
	if start < 0 {
		return nil
	}
	// Find the return statement — the first `return map[string]any{` after the
	// function header.
	retIdx := bytes.Index(src[start:], []byte("return map[string]any{"))
	if retIdx < 0 {
		return nil
	}
	blockStart := start + retIdx
	// Find the matching closing brace by counting.
	depth := 0
	end := blockStart
	for i := blockStart; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				goto done
			}
		}
	}
done:
	block := src[blockStart:end]
	var out []string
	for _, m := range pigoMapKeyRE.FindAllSubmatch(block, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// extractPigoMetaHeaders extracts the defaultMetaHeaders map values from
// pigo.go. The map is of canonical form "Title": "title", ...; we want the
// VALUES (the normalised YAML-key form Pico stores).
func extractPigoMetaHeaders(src []byte) []string {
	fnStart := bytes.Index(src, []byte("func defaultMetaHeaders()"))
	if fnStart < 0 {
		return nil
	}
	retIdx := bytes.Index(src[fnStart:], []byte("return map[string]string{"))
	if retIdx < 0 {
		return nil
	}
	blockStart := fnStart + retIdx
	depth := 0
	end := blockStart
	for i := blockStart; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				goto done
			}
		}
	}
done:
	block := src[blockStart:end]
	// Lines look like:   "Title":          "title",
	re := regexp.MustCompile(`"[^"]+"\s*:\s*"([a-zA-Z0-9_]+)"`)
	var out []string
	for _, m := range re.FindAllSubmatch(block, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// extractPigoTwigNames reads either r.env.Filters["name"] or r.env.Functions["name"]
// assignments out of render/twig.go.
func extractPigoTwigNames(src []byte, kind string) []string {
	re := regexp.MustCompile(`r\.env\.` + kind + `\["([a-zA-Z0-9_]+)"\]`)
	var out []string
	for _, m := range re.FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// extractPigoFlags finds every flag.<T>(…, "name", …) call in cmd/pigo/main.go.
func extractPigoFlags(src []byte) []string {
	var out []string
	for _, m := range pigoFlagRE.FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// -------- extractors: Pico side --------

var (
	picoEventRE       = regexp.MustCompile(`triggerEvent\('([a-zA-Z0-9_]+)'`)
	picoConfigKeyRE   = regexp.MustCompile(`(?m)^([a-zA-Z_][a-zA-Z0-9_]*)\s*:`)
	picoMetaHeaderRE  = regexp.MustCompile(`'[^']+'\s*=>\s*'([a-zA-Z0-9_]+)'`)
	picoTwigVarRE     = regexp.MustCompile(`'([a-zA-Z0-9_]+)'\s*=>`)
	picoSimpleFilterRE = regexp.MustCompile(`new Twig_SimpleFilter\(\s*'([a-zA-Z0-9_]+)'`)
	picoSimpleFuncRE   = regexp.MustCompile(`new Twig_SimpleFunction\(\s*'([a-zA-Z0-9_]+)'`)
)

// extractPicoEvents pulls every triggerEvent('onXxx'…) call from Pico.php.
func extractPicoEvents(src []byte) []string {
	var out []string
	for _, m := range picoEventRE.FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// extractPicoConfigKeys parses the commented YAML template. Top-level keys are
// "key: value" at column 0. Inline array keys (theme_config, twig_config,
// content_config) are captured as parents; their sub-keys are not compared
// (pigo holds them as freeform maps too).
func extractPicoConfigKeys(src []byte) []string {
	var out []string
	// Strip comments so regex doesn't see "# The path to…" tails as trouble —
	// not strictly needed here, but future-proofs the regex.
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" || strings.HasPrefix(strings.TrimLeft(line, " "), "#") {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue // nested (sub-map values)
		}
		m := picoConfigKeyRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		// Pico ships DummyPlugin.enabled as an example of a plugin-namespaced
		// key. Skip plugin-specific keys — they aren't part of the core
		// surface. Currently the template has none at top level that aren't
		// core; keep this filter as a safety net.
		if strings.Contains(name, ".") {
			continue
		}
		out = append(out, name)
	}
	return dedupe(out)
}

// extractPicoTwigVariables extracts the keys of the array returned by
// getTwigVariables() in Pico.php. The block runs from "function getTwigVariables"
// to the closing ");".
func extractPicoTwigVariables(src []byte) []string {
	fnIdx := bytes.Index(src, []byte("function getTwigVariables()"))
	if fnIdx < 0 {
		return nil
	}
	retIdx := bytes.Index(src[fnIdx:], []byte("return array("))
	if retIdx < 0 {
		return nil
	}
	blockStart := fnIdx + retIdx
	endIdx := bytes.Index(src[blockStart:], []byte(");"))
	if endIdx < 0 {
		return nil
	}
	block := src[blockStart : blockStart+endIdx]
	var out []string
	for _, m := range picoTwigVarRE.FindAllSubmatch(block, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// extractPicoMetaHeaders extracts the values of the metaHeaders array in
// getMetaHeaders(). Pico's line shape is 'Canonical' => 'normalised'; we want
// the normalised forms since those are the keys plugins and themes read.
func extractPicoMetaHeaders(src []byte) []string {
	fnIdx := bytes.Index(src, []byte("function getMetaHeaders()"))
	if fnIdx < 0 {
		return nil
	}
	assignIdx := bytes.Index(src[fnIdx:], []byte("$this->metaHeaders = array("))
	if assignIdx < 0 {
		return nil
	}
	blockStart := fnIdx + assignIdx
	endIdx := bytes.Index(src[blockStart:], []byte(");"))
	if endIdx < 0 {
		return nil
	}
	block := src[blockStart : blockStart+endIdx]
	var out []string
	for _, m := range picoMetaHeaderRE.FindAllSubmatch(block, -1) {
		out = append(out, string(m[1]))
	}
	return dedupe(out)
}

// extractPicoTwigFiltersFunctions returns (filters, functions) by scanning the
// PicoTwigExtension file plus Pico.php (which registers the `content` filter
// inline in getTwig()).
func extractPicoTwigFiltersFunctions(ext, core []byte) (filters, functions []string) {
	for _, m := range picoSimpleFilterRE.FindAllSubmatch(ext, -1) {
		filters = append(filters, string(m[1]))
	}
	for _, m := range picoSimpleFuncRE.FindAllSubmatch(ext, -1) {
		functions = append(functions, string(m[1]))
	}
	// The `content` filter is registered directly in Pico.php (not in the
	// extension class), because it needs a live reference to &$pages. Pick it
	// up separately.
	for _, m := range picoSimpleFilterRE.FindAllSubmatch(core, -1) {
		filters = append(filters, string(m[1]))
	}
	for _, m := range picoSimpleFuncRE.FindAllSubmatch(core, -1) {
		functions = append(functions, string(m[1]))
	}
	return dedupe(filters), dedupe(functions)
}

// -------- classification --------

// classify populates Both / PicoOnly / PigoOnly on c from the Pico and Pigo
// slices. Matching is by lowerCamel normalisation: pigo's "OnConfigLoaded"
// matches pico's "onConfigLoaded".
func classify(c Category) Category {
	picoIdx := map[string]string{}
	for _, n := range c.Pico {
		picoIdx[normalise(n)] = n
	}
	pigoIdx := map[string]string{}
	for _, n := range c.Pigo {
		pigoIdx[normalise(n)] = n
	}

	// Both: emit in Pico order for a stable, PHP-authoritative ordering.
	for _, n := range c.Pico {
		key := normalise(n)
		if pigoName, ok := pigoIdx[key]; ok {
			c.Both = append(c.Both, pairItem{Pico: n, Pigo: pigoName})
		}
	}

	for _, n := range c.Pico {
		if _, ok := pigoIdx[normalise(n)]; !ok {
			c.PicoOnly = append(c.PicoOnly, n)
		}
	}
	for _, n := range c.Pigo {
		if _, ok := picoIdx[normalise(n)]; !ok {
			c.PigoOnly = append(c.PigoOnly, n)
		}
	}
	sort.Strings(c.PicoOnly)
	sort.Strings(c.PigoOnly)
	return c
}

// normalise lowercases the first letter so casing differences (Pico's camelCase
// event names vs pigo's Go-exported PascalCase constants) don't spuriously
// separate matching items.
func normalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// -------- rendering --------

// renderJSON emits the category list as an ordered JSON object.
func renderJSON(cats []Category) ([]byte, error) {
	// Preserve category order by building the top-level as an ordered list of
	// {key, value} pairs, then hand-writing the JSON to avoid Go's map key
	// sorting.
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, c := range cats {
		b, err := json.MarshalIndent(c, "  ", "  ")
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&buf, "  %q: %s", c.Key, b)
		if i < len(cats)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

func loadExemptions(path string) (*Exemptions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Exemptions{Categories: map[string]struct {
				PicoOnly []exemptionItem `yaml:"pico_only"`
				PigoOnly []exemptionItem `yaml:"pigo_only"`
			}{}}, nil
		}
		return nil, err
	}
	var e Exemptions
	if err := yaml.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	if e.Categories == nil {
		e.Categories = map[string]struct {
			PicoOnly []exemptionItem `yaml:"pico_only"`
			PigoOnly []exemptionItem `yaml:"pigo_only"`
		}{}
	}
	// Folded-scalar reasons always end in "\n"; strip that so template output
	// doesn't produce a blank line between list items.
	for k, c := range e.Categories {
		for i := range c.PicoOnly {
			c.PicoOnly[i].Reason = strings.TrimSpace(c.PicoOnly[i].Reason)
		}
		for i := range c.PigoOnly {
			c.PigoOnly[i].Reason = strings.TrimSpace(c.PigoOnly[i].Reason)
		}
		e.Categories[k] = c
	}
	for i := range e.Behavioural {
		e.Behavioural[i].Reason = strings.TrimSpace(e.Behavioural[i].Reason)
	}
	return &e, nil
}

// annotate returns the reason string for `name` in the given category/side, or
// "" if there isn't one. Side is "pico_only" or "pigo_only".
func annotate(e *Exemptions, category, side, name string) string {
	c, ok := e.Categories[category]
	if !ok {
		return ""
	}
	var list []exemptionItem
	switch side {
	case "pico_only":
		list = c.PicoOnly
	case "pigo_only":
		list = c.PigoOnly
	}
	for _, it := range list {
		if it.Name == name {
			return it.Reason
		}
	}
	return ""
}

const summaryTemplate = `# Pigo ↔ Pico parity

<!-- Generated by cmd/parity. Do not edit by hand. Run ` + "`go run ./cmd/parity --pico-dir …/Pico`" + ` to regenerate. -->

Mechanically-generated diff of pigo's public surface vs Pico's, across seven
categories. Pico reference: {{ .PicoRef }}.

Name surfaces only — behavioural divergence is tracked at the bottom of this
file from hand-curated notes in ` + "`docs/parity/exemptions.yaml`" + `.

## Summary

| Category | Shared | Pico-only | Pigo-only |
|---|---:|---:|---:|
{{- range .Categories }}
| [{{ .Title }}](#{{ .Anchor }}) | {{ len .Both }} | {{ len .PicoOnly }} | {{ len .PigoOnly }} |
{{- end }}

{{ range .Categories }}
## {{ .Title }}

- Pigo source: ` + "`{{ .SourcePigo }}`" + `
- Pico source: ` + "`{{ .SourcePico }}`" + `
{{- if .Note }}

> {{ .Note }}
{{- end }}

**Shared ({{ len .Both }}):**
{{ if .Both -}}
{{ range .Both }}- ` + "`{{ .Pico }}`" + `{{ if ne .Pico .Pigo }} / pigo: ` + "`{{ .Pigo }}`" + `{{ end }}
{{ end -}}
{{ else }}_(none)_
{{ end }}
**Pico-only ({{ len .PicoOnly }}):**
{{ if .PicoOnly -}}
{{ range .PicoOnly }}- ` + "`{{ .Name }}`" + `{{ if .Reason }} — {{ .Reason }}{{ end }}
{{ end -}}
{{ else }}_(none)_
{{ end }}
**Pigo-only ({{ len .PigoOnly }}):**
{{ if .PigoOnly -}}
{{ range .PigoOnly }}- ` + "`{{ .Name }}`" + `{{ if .Reason }} — {{ .Reason }}{{ end }}
{{ end -}}
{{ else }}_(none)_
{{ end }}
{{ end }}
{{- if .Behavioural }}
## Behavioural divergences

These are not mechanically extracted; they're hand-curated notes on areas
where the two implementations behave differently despite having the same
names.

{{ range .Behavioural }}- **{{ .Name }}** — {{ .Reason }}
{{ end }}
{{- end }}
`

// annotatedItem is the view-model used by the Markdown template: wraps a raw
// name with the (possibly empty) exemption reason from exemptions.yaml.
type annotatedItem struct {
	Name   string
	Reason string
}

type summaryCategory struct {
	Category
	Anchor      string
	BothView    []pairItem
	PicoOnly    []annotatedItem
	PigoOnly    []annotatedItem
}

type summaryData struct {
	PicoRef     string
	Categories  []summaryCategory
	Behavioural []struct {
		Name   string
		Reason string
	}
}

func renderMarkdown(cats []Category, e *Exemptions, picoRef string) ([]byte, error) {
	data := summaryData{PicoRef: picoRefDisplay(picoRef)}
	for _, c := range cats {
		sc := summaryCategory{
			Category: c,
			Anchor:   anchorize(c.Title),
			BothView: c.Both,
		}
		for _, n := range c.PicoOnly {
			sc.PicoOnly = append(sc.PicoOnly, annotatedItem{Name: n, Reason: annotate(e, c.Key, "pico_only", n)})
		}
		for _, n := range c.PigoOnly {
			sc.PigoOnly = append(sc.PigoOnly, annotatedItem{Name: n, Reason: annotate(e, c.Key, "pigo_only", n)})
		}
		data.Categories = append(data.Categories, sc)
	}
	for _, b := range e.Behavioural {
		data.Behavioural = append(data.Behavioural, struct {
			Name   string
			Reason string
		}{b.Name, b.Reason})
	}

	t, err := template.New("summary").Parse(summaryTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func picoRefDisplay(ref string) string {
	if ref == "" {
		return "(unpinned)"
	}
	return ref
}

// anchorize produces the GitHub Markdown anchor slug for a heading.
func anchorize(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")
	// Drop parens and slashes; other punctuation in our titles is minimal.
	s = strings.NewReplacer("(", "", ")", "", "/", "", ",", "").Replace(s)
	return s
}

// diffFile compares want against the contents of path. Returns a descriptive
// error when they differ (or path is missing), nil when identical.
func diffFile(path string, want []byte) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s missing — run `go run ./cmd/parity --pico-dir …/Pico` and commit", path)
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s out of date — run `go run ./cmd/parity --pico-dir …/Pico` and commit the result", path)
	}
	return nil
}
