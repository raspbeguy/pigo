// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package fileprefixes

import "github.com/raspbeguy/pigo/plugin"

// Self-registration: any binary that imports plugins/fileprefixes (directly
// or via blank import) will have "PicoFilePrefixes" available in the
// plugin registry. Users enable the plugin per site via:
//
//	plugins:
//	  - PicoFilePrefixes
//
// in the site's config.yml.
func init() {
	plugin.Register("PicoFilePrefixes", func() plugin.Plugin { return &Plugin{} })
}
