// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// pigo is a drop-in Go replacement for the Pico flat-file CMS.
//
// This stock binary ships with two official plugins available in the plugin
// registry: PicoFilePrefixes and PicoRobots. Enable them per site via the
// config's `plugins:` list — the binary does not hard-code enablement, so a
// single pigo binary can serve many sites each with a different plugin set,
// just by pointing --root at a different site directory.
//
// To add more plugins, copy this file into your own repo, add blank imports
// for the plugin packages you want, and `go build`. The plugins register
// themselves via their init(); pigo.New picks them up from config.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/raspbeguy/pigo"
	"github.com/raspbeguy/pigo/config"
	"github.com/raspbeguy/pigo/logging"
	"github.com/raspbeguy/pigo/plugin"

	// Blank imports register the shipped plugins in the global registry.
	_ "github.com/raspbeguy/pigo/plugins/fileprefixes"
	_ "github.com/raspbeguy/pigo/plugins/robots"
)

func main() {
	var (
		root        = flag.String("root", ".", "site root (containing config/, content/, themes/)")
		cfgDir      = flag.String("config", "", "config dir (default: <root>/config)")
		contentD    = flag.String("content", "", "content dir (default: config.content_dir or <root>/content)")
		themes      = flag.String("themes", "", "themes dir (default: <root>/themes)")
		assets      = flag.String("assets", "", "assets dir (default: <root>/assets)")
		addr        = flag.String("addr", ":8080", "HTTP listen address")
		listPlugins = flag.Bool("list-plugins", false, "print registered plugin names and exit")
		logLevel    = flag.String("log-level", "", "log level: debug|info|warn|error (default info; env PIGO_LOG_LEVEL, config log_level)")
		logFormat   = flag.String("log-format", "", "log format: text|json (default text; env PIGO_LOG_FORMAT, config log_format)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pigo %s — Go port of Pico\n\nUsage: %s [flags]\n\n", pigo.Version, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if *listPlugins {
		for _, name := range plugin.Registered() {
			fmt.Println(name)
		}
		return
	}

	// Load the site config once up front so log_level / log_format can feed
	// into the logger. pigo.New will reload it internally; that's fine —
	// config.Load is deterministic and cheap.
	configDir := *cfgDir
	if configDir == "" {
		configDir = filepath.Join(*root, "config")
	}
	var logCfg map[string]any
	if cfg, err := config.Load(configDir); err == nil {
		logCfg = cfg.AsMap()
	}
	logger, err := logging.New(logging.Resolve(logLevel, logFormat, logCfg))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	slog.SetDefault(logger)

	site, err := pigo.New(pigo.Options{
		RootDir:    *root,
		ConfigDir:  *cfgDir,
		ContentDir: *contentD,
		ThemesDir:  *themes,
		AssetsDir:  *assets,
		Logger:     logger,
	})
	if err != nil {
		logger.Error("site init failed", "err", err)
		os.Exit(1)
	}
	logger.Info("pigo listening", "version", pigo.Version, "addr", *addr)
	if err := site.ListenAndServe(*addr); err != nil {
		logger.Error("listen failed", "addr", *addr, "err", err)
		os.Exit(1)
	}
}

