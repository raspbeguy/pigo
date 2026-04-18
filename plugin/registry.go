// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package plugin

import (
	"fmt"
	"sort"
	"sync"
)

// Factory builds a fresh Plugin instance. Called once per pigo.New.
// Plugins self-register their factories from init(); a Site then resolves
// the config's `plugins:` list against the registry.
type Factory func() Plugin

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register adds a name → factory entry. Intended for use from init():
//
//	func init() { plugin.Register("MyPlugin", func() plugin.Plugin { return &MyPlugin{} }) }
//
// Panics on duplicate name so the collision surfaces at program startup
// rather than as a silent config mismatch at request time.
func Register(name string, f Factory) {
	if name == "" {
		panic("plugin.Register: empty name")
	}
	if f == nil {
		panic("plugin.Register: nil factory for " + name)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("plugin.Register: %q already registered", name))
	}
	registry[name] = f
}

// Lookup returns the factory for name, or (nil, false) when unknown.
func Lookup(name string) (Factory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[name]
	return f, ok
}

// Registered returns the sorted list of registered plugin names. Useful for
// `--list-plugins` output and error messages that suggest what IS available.
func Registered() []string {
	registryMu.RLock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	registryMu.RUnlock()
	sort.Strings(names)
	return names
}

// resetRegistry is test-only: wipe the registry so tests don't leak state
// into each other. Exported with a lowercase name so it stays package-private.
func resetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]Factory{}
}
