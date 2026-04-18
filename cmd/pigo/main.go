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
	"log"
	"os"

	"github.com/raspbeguy/pigo"
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

	site, err := pigo.New(pigo.Options{
		RootDir:    *root,
		ConfigDir:  *cfgDir,
		ContentDir: *contentD,
		ThemesDir:  *themes,
		AssetsDir:  *assets,
	})
	if err != nil {
		log.Fatalf("pigo: %v", err)
	}
	log.Printf("pigo %s listening on %s", pigo.Version, *addr)
	if err := site.ListenAndServe(*addr); err != nil {
		log.Fatalf("pigo: %v", err)
	}
}
