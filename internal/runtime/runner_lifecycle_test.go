package runtime

import (
	"testing"
)

func TestNewPersistenceSetEmptyMemory(t *testing.T) {
	p := NewPersistenceSet(nil, nil, nil, nil)
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
	ms := MemorySet{Admin: newSessionAdminStoreAdapter(nil)}
	if ms.Available() {
		t.Fatal("TRPC nil should not be available")
	}
}
