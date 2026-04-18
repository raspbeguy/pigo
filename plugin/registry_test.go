// Copyright 2026 Guy Godfroy
// SPDX-License-Identifier: MIT

package plugin

import (
	"reflect"
	"testing"
)

type stubPlugin struct{ Base }

func (*stubPlugin) Name() string                                  { return "stub" }
func (*stubPlugin) DependsOn() []string                           { return nil }
func (*stubPlugin) HandleEvent(event string, _ ...any) error      { return nil }

func TestRegistryRegisterAndLookup(t *testing.T) {
	t.Cleanup(resetRegistry)
	resetRegistry()

	Register("A", func() Plugin { return &stubPlugin{} })
	f, ok := Lookup("A")
	if !ok {
		t.Fatalf("Lookup(A) = false")
	}
	if f == nil || f() == nil {
		t.Fatalf("factory returned nil")
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	t.Cleanup(resetRegistry)
	resetRegistry()
	Register("dup", func() Plugin { return &stubPlugin{} })

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on duplicate Register")
		}
	}()
	Register("dup", func() Plugin { return &stubPlugin{} })
}

func TestRegistryRegisteredIsSorted(t *testing.T) {
	t.Cleanup(resetRegistry)
	resetRegistry()

	Register("Charlie", func() Plugin { return &stubPlugin{} })
	Register("Alpha", func() Plugin { return &stubPlugin{} })
	Register("Bravo", func() Plugin { return &stubPlugin{} })

	got := Registered()
	want := []string{"Alpha", "Bravo", "Charlie"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Registered() = %v, want %v", got, want)
	}
}

func TestRegistryEmptyNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on empty name")
		}
	}()
	Register("", func() Plugin { return &stubPlugin{} })
}

func TestRegistryNilFactoryPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil factory")
		}
	}()
	Register("X", nil)
}
