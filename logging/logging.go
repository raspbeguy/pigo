// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package logging builds the pigo *slog.Logger from config / env / flag
// inputs. Uses stdlib log/slog — JSONHandler for machine-parseable output
// and TextHandler for human-friendly logfmt-style output. Level filtering
// is provided by slog's standard LevelVar.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options configures the logger. Zero values are valid: empty strings pick
// the defaults (info / text), nil Writer picks os.Stderr.
type Options struct {
	Level  string    // debug|info|warn|error (case-insensitive); "" → info.
	Format string    // text|json; "" → text (logfmt-style).
	Writer io.Writer // nil → os.Stderr.
}

// New builds a *slog.Logger honoring Options. Invalid level or format
// returns an error.
func New(opts Options) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	hopts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(opts.Format)) {
	case "", "text", "logfmt":
		h = slog.NewTextHandler(w, hopts)
	case "json":
		h = slog.NewJSONHandler(w, hopts)
	default:
		return nil, fmt.Errorf("logging: invalid format %q (want text or json)", opts.Format)
	}
	return slog.New(h), nil
}

// Resolve picks effective Options from a three-layer lookup: explicit
// argument (flag) → process environment (PIGO_LOG_LEVEL / PIGO_LOG_FORMAT)
// → config map (log_level / log_format). Empty strings at higher layers
// fall through to lower ones. The returned Options are still subject to
// validation by New.
//
// Pass nil for flagLevel / flagFormat to skip the flag layer. Pass a nil
// cfg to skip the config layer.
func Resolve(flagLevel, flagFormat *string, cfg map[string]any) Options {
	level := first(
		derefString(flagLevel),
		os.Getenv("PIGO_LOG_LEVEL"),
		stringFromMap(cfg, "log_level"),
	)
	format := first(
		derefString(flagFormat),
		os.Getenv("PIGO_LOG_FORMAT"),
		stringFromMap(cfg, "log_format"),
	)
	return Options{Level: level, Format: format}
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error", "err":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("logging: invalid level %q (want debug, info, warn, or error)", s)
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func stringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func first(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
