package runtime

import (
	"testing"
)

func TestNewEmptyPersistenceSet(t *testing.T) {
	p := NewEmptyPersistenceSet(nil, nil, nil)
	if p.Memory.Available() {
		t.Fatal("expected empty memory set")
	}
}

func TestRunnerManagerNil(t *testing.T) {
	var m *RunnerManager
	if _, err := m.NewTurnRunner(nil, TurnRunnerSpec{}); err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestMemorySetWithAdminOnly(t *testing.T) {
	ms := MemorySet{Admin: nil}
	if ms.Available() {
		t.Fatal("TRPC nil should not be available")
	}
}
