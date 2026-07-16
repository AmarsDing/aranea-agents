package biz

import (
	"context"
	"testing"

	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/loggateway"
)

type memCBRepo struct {
	store map[string]biztool.CircuitBreakerStateEntry
}

func newMemCBRepo() *memCBRepo {
	return &memCBRepo{store: map[string]biztool.CircuitBreakerStateEntry{}}
}

func (r *memCBRepo) SaveState(_ context.Context, key string, state biztool.CircuitBreakerStateEntry) error {
	r.store[key] = state
	return nil
}

func (r *memCBRepo) LoadState(_ context.Context, key string) (biztool.CircuitBreakerStateEntry, error) {
	if e, ok := r.store[key]; ok {
		return e, nil
	}
	return biztool.CircuitBreakerStateEntry{}, nil
}

func (r *memCBRepo) LoadAllStates(_ context.Context) (map[string]biztool.CircuitBreakerStateEntry, error) {
	out := make(map[string]biztool.CircuitBreakerStateEntry, len(r.store))
	for k, v := range r.store {
		out[k] = v
	}
	return out, nil
}

func TestProvideNodeCircuitBreakerRegistry_PersistsOpen(t *testing.T) {
	repo := newMemCBRepo()
	reg := ProvideNodeCircuitBreakerRegistry(repo, loggateway.NewNoop())
	pol := &CircuitBreakerPolicy{FailureThreshold: 1, ResetTimeoutSeconds: 60}
	cb := reg.ForNode("team:t1", "member-1", pol)
	if cb == nil {
		t.Fatal("expected breaker")
	}
	cb.RecordFailure()
	if cb.State() != biztool.CircuitOpen {
		t.Fatalf("want open, got %s", cb.State())
	}
	if len(repo.store) == 0 {
		t.Fatal("expected state persisted to repo")
	}

	// New registry restores via LoadState on ForNode/Get.
	reg2 := NewNodeCircuitBreakerRegistry(biztool.WithStateRepo(repo), biztool.WithLogger(loggateway.NewNoop()))
	cb2 := reg2.ForNode("team:t1", "member-1", pol)
	if cb2 == nil {
		t.Fatal("expected restored breaker")
	}
	if cb2.State() != biztool.CircuitOpen {
		t.Fatalf("restored want open, got %s (store=%v)", cb2.State(), repo.store)
	}
}
