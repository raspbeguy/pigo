// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package robots

import "github.com/raspbeguy/pigo/plugin"

// Self-registration: importing plugins/robots (directly or via blank
// import) makes "PicoRobots" resolvable by name in the plugin registry.
// Enable per site with:
//
//	plugins:
//	  - PicoRobots
//
// in config.yml.
func init() {
	plugin.Register("PicoRobots", func() plugin.Plugin { return &Plugin{} })
}
