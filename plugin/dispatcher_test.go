// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package plugin

import (
	"testing"
)

// recordingPlugin records every event it sees. Optionally embeds Base so it
// can self-disable like Pico's setEnabled(false).
type recordingPlugin struct {
	Base
	name   string
	events []string
	// onEvent fires for each event before it is recorded; use it to
	// SetEnabled(false) or otherwise react mid-dispatch.
	onEvent func(p *recordingPlugin, event string)
}

func (p *recordingPlugin) Name() string        { return p.name }
func (p *recordingPlugin) DependsOn() []string { return nil }
func (p *recordingPlugin) HandleEvent(event string, _ ...any) error {
	if p.onEvent != nil {
		p.onEvent(p, event)
	}
	p.events = append(p.events, event)
	return nil
}

func TestDispatcherSkipsDisabledPlugin(t *testing.T) {
	// Plugin A disables itself on OnConfigLoaded.
	a := &recordingPlugin{name: "A"}
	a.onEvent = func(p *recordingPlugin, event string) {
		if event == OnConfigLoaded {
			p.SetEnabled(false)
		}
	}
	// Plugin B stays enabled.
	b := &recordingPlugin{name: "B"}

	disp, err := NewDispatcher([]Plugin{a, b})
	if err != nil {
		t.Fatal(err)
	}

	for _, ev := range []string{OnConfigLoaded, OnThemeLoaded, OnRequestURL} {
		if err := disp.Dispatch(ev); err != nil {
			t.Fatalf("dispatch %s: %v", ev, err)
		}
	}

	// A was called for OnConfigLoaded (the disable fires inside the handler,
	// AFTER HandleEvent returns the call for the current event is already
	// recorded). For OnThemeLoaded onward A must be skipped.
	if len(a.events) != 1 || a.events[0] != OnConfigLoaded {
		t.Errorf("A events = %v, want [%s]", a.events, OnConfigLoaded)
	}
	wantB := []string{OnConfigLoaded, OnThemeLoaded, OnRequestURL}
	if !eq(b.events, wantB) {
		t.Errorf("B events = %v, want %v", b.events, wantB)
	}
}

func TestDispatcherReEnable(t *testing.T) {
	p := &recordingPlugin{name: "P"}
	p.SetEnabled(false)

	disp, err := NewDispatcher([]Plugin{p})
	if err != nil {
		t.Fatal(err)
	}
	_ = disp.Dispatch(OnConfigLoaded)
	if len(p.events) != 0 {
		t.Errorf("disabled plugin received events: %v", p.events)
	}

	p.SetEnabled(true)
	_ = disp.Dispatch(OnThemeLoaded)
	if !eq(p.events, []string{OnThemeLoaded}) {
		t.Errorf("re-enabled events = %v", p.events)
	}
}

func TestDispatcherTopoOrder(t *testing.T) {
	// B depends on A; expect A before B in dispatch order.
	a := &recordingPlugin{name: "A"}
	b := &bPlugin{recordingPlugin: recordingPlugin{name: "B"}}

	disp, err := NewDispatcher([]Plugin{b, a}) // reversed input
	if err != nil {
		t.Fatal(err)
	}
	_ = disp.Dispatch(OnConfigLoaded)

	order := []string{}
	for _, p := range disp.Plugins() {
		order = append(order, p.Name())
	}
	if !eq(order, []string{"A", "B"}) {
		t.Errorf("dispatch order = %v, want [A B]", order)
	}
}

type bPlugin struct {
	recordingPlugin
}

func (p *bPlugin) DependsOn() []string { return []string{"A"} }

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
