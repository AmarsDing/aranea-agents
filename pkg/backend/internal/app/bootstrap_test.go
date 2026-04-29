package app

import (
	"context"
	"encoding/json"
	"testing"

	"arenea/backend/internal/kernel/contracts"
)

// TestP0_ModulesCompose proves that every Context module can be constructed,
// its ports bootstrapped, and its lifecycle driven without error. This is
// the executable contract for the P0 scaffold described in
// aranea/docs/migration-status.md.
func TestP0_ModulesCompose(t *testing.T) {
	c := &Container{}
	mods := InitModules(c)
	if got, want := len(mods), 6; got != want {
		t.Fatalf("module count: got %d, want %d", got, want)
	}

	wantNames := map[string]bool{
		"identity":     true,
		"catalog":      true,
		"capability":   true,
		"memory":       true,
		"conversation": true,
		"operations":   true,
	}
	seen := map[string]bool{}
	for _, m := range mods {
		if name := m.Name(); !wantNames[name] {
			t.Errorf("unexpected module name %q", name)
		} else {
			seen[name] = true
		}
		if v := m.Version(); v == "" {
			t.Errorf("module %q has empty Version()", m.Name())
		}
		spec, err := m.OpenAPISpec()
		if err != nil {
			t.Errorf("module %q OpenAPISpec error: %v", m.Name(), err)
			continue
		}
		if !json.Valid(spec) {
			t.Errorf("module %q OpenAPISpec is not valid JSON", m.Name())
		}
	}
	for name := range wantNames {
		if !seen[name] {
			t.Errorf("missing module %q", name)
		}
	}

	reg := contracts.NewRegistry()
	BootstrapPorts(mods, reg)

	ctx := context.Background()
	if err := StartModules(ctx, mods); err != nil {
		t.Fatalf("StartModules: %v", err)
	}
	if err := ShutdownModules(ctx, mods); err != nil {
		t.Fatalf("ShutdownModules: %v", err)
	}
}
