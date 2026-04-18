// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

// Package plugin defines pigo's plugin interface and event dispatcher.
//
// Plugins are Go packages compiled into the binary. This replaces Pico's
// dynamic PHP plugin loading — users wanting plugins build their own pigo
// binary importing pigo and their plugins, calling pigo.RegisterPlugin().
package plugin

import "sync"

// Plugin is implemented by every pigo plugin.
type Plugin interface {
	// Name is the unique plugin identifier, used for dependency resolution
	// and for config keys like "MyPlugin.enabled".
	Name() string

	// DependsOn lists plugin names that must load before this one.
	DependsOn() []string

	// HandleEvent is invoked for every lifecycle event. Plugins typically
	// switch on `event` and type-assert params to mutate them (params are
	// passed by pointer where Pico uses pass-by-reference).
	HandleEvent(event string, params ...any) error
}

// EnabledChecker is an optional plugin interface. Plugins that implement it
// (typically by embedding Base) are skipped by the dispatcher when Enabled()
// returns false. Mirrors Pico's $this->setEnabled(false) pattern: a plugin
// disables itself in e.g. OnConfigLoaded or OnRequestURL so it receives no
// further events for this request.
type EnabledChecker interface {
	Enabled() bool
}

// Base is a tiny helper plugins can embed to gain thread-safe
// Enabled()/SetEnabled() behavior. Default state is enabled.
//
//	type MyPlugin struct{ plugin.Base }
//	func (p *MyPlugin) HandleEvent(event string, params ...any) error {
//	    if event == plugin.OnConfigLoaded {
//	        p.SetEnabled(false) // opt out of all further events
//	    }
//	    return nil
//	}
type Base struct {
	mu       sync.Mutex
	disabled bool
}

// Enabled reports whether the plugin currently participates in dispatch.
func (b *Base) Enabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.disabled
}

// SetEnabled toggles the plugin's dispatch participation. Safe for concurrent
// use; intended to be called from within HandleEvent.
func (b *Base) SetEnabled(v bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.disabled = !v
}
