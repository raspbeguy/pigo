// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package plugin

import "fmt"

// Dispatcher owns the ordered plugin list and broadcasts events.
type Dispatcher struct {
	plugins []Plugin
}

// NewDispatcher returns a dispatcher with its plugins topologically ordered by
// their declared dependencies.
func NewDispatcher(plugins []Plugin) (*Dispatcher, error) {
	ordered, err := topoSort(plugins)
	if err != nil {
		return nil, err
	}
	return &Dispatcher{plugins: ordered}, nil
}

// Plugins returns the ordered plugin list (useful for OnPluginsLoaded).
func (d *Dispatcher) Plugins() []Plugin { return d.plugins }

// Dispatch invokes event on every plugin in order. Plugins that implement
// EnabledChecker and return false are skipped. The first error stops dispatch
// and is returned.
func (d *Dispatcher) Dispatch(event string, params ...any) error {
	for _, p := range d.plugins {
		if ec, ok := p.(EnabledChecker); ok && !ec.Enabled() {
			continue
		}
		if err := p.HandleEvent(event, params...); err != nil {
			return fmt.Errorf("plugin %s on %s: %w", p.Name(), event, err)
		}
	}
	return nil
}

func topoSort(plugins []Plugin) ([]Plugin, error) {
	byName := map[string]Plugin{}
	for _, p := range plugins {
		if _, dup := byName[p.Name()]; dup {
			return nil, fmt.Errorf("duplicate plugin name %q", p.Name())
		}
		byName[p.Name()] = p
	}

	var out []Plugin
	visited := map[string]int{} // 0 unvisited, 1 in-progress, 2 done
	var visit func(p Plugin) error
	visit = func(p Plugin) error {
		switch visited[p.Name()] {
		case 2:
			return nil
		case 1:
			return fmt.Errorf("plugin dependency cycle involving %q", p.Name())
		}
		visited[p.Name()] = 1
		for _, depName := range p.DependsOn() {
			dep, ok := byName[depName]
			if !ok {
				return fmt.Errorf("plugin %q depends on missing %q", p.Name(), depName)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[p.Name()] = 2
		out = append(out, p)
		return nil
	}
	for _, p := range plugins {
		if err := visit(p); err != nil {
			return nil, err
		}
	}
	return out, nil
}
