package modelsync

import (
	"testing"

	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"
)

type stubBackend struct {
	modelregistry.ApplyBackend
}

func TestBuildPhases_ReturnsAllPhases(t *testing.T) {
	backend := &stubBackend{}
	phases := BuildPhases(backend, loggateway.NewNoop())
	if phases == nil {
		t.Fatal("expected non-nil phases")
	}
	if phases.fetchPhase == nil {
		t.Error("expected fetchPhase")
	}
	if phases.migratePhase == nil {
		t.Error("expected migratePhase")
	}
	if phases.applyPhase == nil {
		t.Error("expected applyPhase")
	}
	if phases.logoPhase == nil {
		t.Error("expected logoPhase")
	}
}

func TestPhases_List_ReturnsThreePhases(t *testing.T) {
	backend := &stubBackend{}
	phases := BuildPhases(backend, loggateway.NewNoop())
	list := phases.List()
	if len(list) != 3 {
		t.Errorf("expected 3 phases in List, got %d", len(list))
	}
}

func TestPhases_LogoPhase(t *testing.T) {
	backend := &stubBackend{}
	phases := BuildPhases(backend, loggateway.NewNoop())
	logo := phases.LogoPhase()
	if logo == nil {
		t.Fatal("expected logo phase")
	}
	if logo.Name() != "logos" {
		t.Errorf("expected logo phase name 'logos', got %q", logo.Name())
	}
}

func TestRegisterAll_ReturnsFourTools(t *testing.T) {
	backend := &stubBackend{}
	phases := BuildPhases(backend, loggateway.NewNoop())
	deps := Deps{
		Phases:        phases,
		StoreProvider: nil,
		Backend:       backend,
	}
	tools := RegisterAll(deps)
	if len(tools) != 4 {
		t.Errorf("expected 4 tools, got %d", len(tools))
	}
}

func TestPhaseNames(t *testing.T) {
	backend := &stubBackend{}
	phases := BuildPhases(backend, loggateway.NewNoop())

	if phases.fetchPhase.Name() != "fetch" {
		t.Errorf("expected fetch phase name 'fetch', got %q", phases.fetchPhase.Name())
	}
	if phases.migratePhase.Name() != "migrate" {
		t.Errorf("expected migrate phase name 'migrate', got %q", phases.migratePhase.Name())
	}
	if phases.applyPhase.Name() != "apply" {
		t.Errorf("expected apply phase name 'apply', got %q", phases.applyPhase.Name())
	}
	if phases.logoPhase.Name() != "logos" {
		t.Errorf("expected logo phase name 'logos', got %q", phases.logoPhase.Name())
	}
}

func TestPhaseTimeouts(t *testing.T) {
	backend := &stubBackend{}
	phases := BuildPhases(backend, loggateway.NewNoop())

	if phases.fetchPhase.Timeout() <= 0 {
		t.Error("fetch phase should have positive timeout")
	}
	if phases.migratePhase.Timeout() <= 0 {
		t.Error("migrate phase should have positive timeout")
	}
	if phases.applyPhase.Timeout() <= 0 {
		t.Error("apply phase should have positive timeout")
	}
	if phases.logoPhase.Timeout() <= 0 {
		t.Error("logo phase should have positive timeout")
	}
}
