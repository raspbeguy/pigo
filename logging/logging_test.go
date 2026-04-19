// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewDefault(t *testing.T) {
	var buf bytes.Buffer
	lg, err := New(Options{Writer: &buf})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	lg.Info("hello", "k", "v")
	if !strings.Contains(buf.String(), `msg=hello`) || !strings.Contains(buf.String(), `k=v`) {
		t.Errorf("default (text) format missing expected fields: %q", buf.String())
	}
}

func TestNewJSON(t *testing.T) {
	var buf bytes.Buffer
	lg, err := New(Options{Format: "json", Writer: &buf})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	lg.Info("hello", "k", "v")
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if out["msg"] != "hello" || out["k"] != "v" {
		t.Errorf("json fields mismatch: %v", out)
	}
}

func TestLevelFilter(t *testing.T) {
	var buf bytes.Buffer
	lg, err := New(Options{Level: "warn", Writer: &buf})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	lg.Info("should be filtered")
	lg.Warn("should pass")
	out := buf.String()
	if strings.Contains(out, "filtered") {
		t.Errorf("info emitted under warn threshold: %q", out)
	}
	if !strings.Contains(out, "should pass") {
		t.Errorf("warn suppressed under warn threshold: %q", out)
	}
}

func TestInvalidLevel(t *testing.T) {
	_, err := New(Options{Level: "loud"})
	if err == nil {
		t.Errorf("expected error on invalid level, got nil")
	}
}

func TestInvalidFormat(t *testing.T) {
	_, err := New(Options{Format: "csv"})
	if err == nil {
		t.Errorf("expected error on invalid format, got nil")
	}
}

func TestResolvePrecedence(t *testing.T) {
	t.Setenv("PIGO_LOG_LEVEL", "warn")
	t.Setenv("PIGO_LOG_FORMAT", "json")
	cfg := map[string]any{"log_level": "debug", "log_format": "text"}

	// No flag → env wins over config.
	opts := Resolve(nil, nil, cfg)
	if opts.Level != "warn" || opts.Format != "json" {
		t.Errorf("env layer: got %+v, want {Level:warn Format:json}", opts)
	}

	// Flag wins over env.
	flagLevel := "error"
	flagFormat := "text"
	opts = Resolve(&flagLevel, &flagFormat, cfg)
	if opts.Level != "error" || opts.Format != "text" {
		t.Errorf("flag layer: got %+v, want {Level:error Format:text}", opts)
	}

	// Config seen only when flag and env are absent.
	t.Setenv("PIGO_LOG_LEVEL", "")
	t.Setenv("PIGO_LOG_FORMAT", "")
	opts = Resolve(nil, nil, cfg)
	if opts.Level != "debug" || opts.Format != "text" {
		t.Errorf("config layer: got %+v, want {Level:debug Format:text}", opts)
	}

	// Fallback to defaults when everything is empty.
	opts = Resolve(nil, nil, nil)
	if opts.Level != "" || opts.Format != "" {
		t.Errorf("defaults: got %+v, want zero-value opts", opts)
	}
}

func TestResolveEmptyFlagPointerFallsThrough(t *testing.T) {
	// A flag pointer with empty string value should still fall through to
	// env / config — the flag package reports an unset flag as its default
	// (we default to ""), which must not shadow lower layers.
	t.Setenv("PIGO_LOG_LEVEL", "info")
	t.Setenv("PIGO_LOG_FORMAT", "text")
	empty := ""
	opts := Resolve(&empty, &empty, nil)
	if opts.Level != "info" || opts.Format != "text" {
		t.Errorf("empty flag should fall through: got %+v", opts)
	}
}
