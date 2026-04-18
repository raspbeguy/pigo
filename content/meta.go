// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package content

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lestrrat-go/strftime"
	"gopkg.in/yaml.v3"
)

// metaPattern matches a YAML front-matter block delimited by "---" lines.
// Matches at the very start of the file (optionally after a UTF-8 BOM).
// Also accepts the deprecated /* ... */ delimiter for backwards compat.
var (
	yamlBlock   = regexp.MustCompile(`(?s)\A(?:\x{FEFF})?[ \t]*(?:---)[ \t]*\r?\n(.*?)(?:\r?\n)(?:---)[ \t]*\r?\n?`)
	cStyleBlock = regexp.MustCompile(`(?s)\A(?:\x{FEFF})?[ \t]*/\*[ \t]*\r?\n?(.*?)\r?\n?\*/[ \t]*\r?\n?`)
)

// SplitFrontMatter separates the YAML header (if any) from the Markdown body.
// Returns the header text (without delimiters) and the body. If no front-matter
// block is present, yamlText is empty and body is the input unchanged.
func SplitFrontMatter(raw string) (yamlText, body string) {
	if m := yamlBlock.FindStringSubmatchIndex(raw); m != nil {
		return raw[m[2]:m[3]], raw[m[1]:]
	}
	if m := cStyleBlock.FindStringSubmatchIndex(raw); m != nil {
		return raw[m[2]:m[3]], raw[m[1]:]
	}
	return "", raw
}

// ParseMeta extracts structured meta from a YAML front-matter text. Keys are
// lowercased to match Pico's lowerFileMeta behavior (Pico.php:1419).
// dateFormat is applied via strftime for DateFormatted; if empty, DateFormatted
// is left empty.
func ParseMeta(yamlText, dateFormat string) (map[string]any, error) {
	out := map[string]any{
		"title":          "",
		"description":    "",
		"author":         "",
		"date":           "",
		"date_formatted": "",
		"time":           int64(0),
		"robots":         "",
		"template":       "",
		"hidden":         false,
	}
	if strings.TrimSpace(yamlText) == "" {
		return out, nil
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(yamlText), &raw); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}

	for k, v := range raw {
		out[strings.ToLower(k)] = v
	}

	// yaml.v3 auto-parses ISO timestamps to time.Time; normalize "date" to the
	// original string form and still derive Unix time / formatted date from it.
	var parsedDate time.Time
	var dateGiven bool
	switch v := out["date"].(type) {
	case time.Time:
		parsedDate = v
		dateGiven = true
		out["date"] = v.Format("2006-01-02")
	case string:
		if v != "" {
			if t, err := parseFlexibleDate(v); err == nil {
				parsedDate = t
				dateGiven = true
			}
		}
	}

	if dateGiven {
		if _, given := raw["Time"]; !given {
			if _, givenLower := raw["time"]; !givenLower {
				out["time"] = parsedDate.Unix()
			}
		}
		if _, given := raw["Formatted Date"]; !given {
			if _, givenLower := raw["formatted date"]; !givenLower {
				if dateFormat != "" {
					if f, err := strftime.Format(dateFormat, parsedDate); err == nil {
						out["date_formatted"] = f
					}
				}
			}
		}
	}

	// Normalize hidden to bool.
	if h, ok := out["hidden"]; ok {
		out["hidden"] = asBool(h)
	}

	return out, nil
}

func parseFlexibleDate(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"01/02/2006",
		time.RFC1123,
		time.RFC1123Z,
		time.RFC822,
		time.RFC822Z,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date: %q", s)
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s == "true" || s == "yes" || s == "1"
	case int:
		return x != 0
	case int64:
		return x != 0
	}
	return false
}
