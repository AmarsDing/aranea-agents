package tool

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type fakeCBStateRepo struct {
	store map[string]CircuitBreakerStateEntry
}

func newFakeCBStateRepo() *fakeCBStateRepo {
	return &fakeCBStateRepo{store: make(map[string]CircuitBreakerStateEntry)}
}

func (f *fakeCBStateRepo) SaveState(_ context.Context, key string, e CircuitBreakerStateEntry) error {
	f.store[key] = e
	return nil
}

func (f *fakeCBStateRepo) LoadState(_ context.Context, key string) (CircuitBreakerStateEntry, error) {
	return f.store[key], nil
}

func (f *fakeCBStateRepo) LoadAllStates(context.Context) (map[string]CircuitBreakerStateEntry, error) {
	out := make(map[string]CircuitBreakerStateEntry, len(f.store))
	for k, v := range f.store {
		out[k] = v
	}
	return out, nil
}

// ADR-84：共享（tool:）熔断器 Open 状态必须落表；Agent 专属（tool_agent:）保持易失。
func TestProvideToolCircuitBreakerRegistry_PersistFilterNamespaces(t *testing.T) {
	repo := newFakeCBStateRepo()
	reg := ProvideToolCircuitBreakerRegistry(repo, loggateway.NewNoop())

	shared := ToolBreakerNameShared("ws1", "httpfetch")
	agentScoped := ToolBreakerNameAgent("agent-1", "httpfetch")
	cfg := CircuitBreakerConfig{FailureThreshold: 1, RecoveryTimeoutSec: 60, HalfOpenMaxProbe: 1}
	reg.SetOverride(shared, cfg)
	reg.SetOverride(agentScoped, cfg)

	reg.Get(shared, "").RecordFailure()
	reg.Get(agentScoped, "").RecordFailure()

	if _, ok := repo.store[shared]; !ok {
		t.Fatalf("shared breaker state must persist, store=%v", repo.store)
	}
	if entry := repo.store[shared]; entry.State != string(CircuitOpen) {
		t.Fatalf("shared breaker should persist open, got %q", entry.State)
	}
	if _, ok := repo.store[agentScoped]; ok {
		t.Fatalf("agent-scoped breaker must stay volatile, store=%v", repo.store)
	}
}

// ADR-84：重启恢复——新注册表 Get 时仅 tool: 前缀命中 StateRepo 恢复。
func TestProvideToolCircuitBreakerRegistry_RestoresSharedState(t *testing.T) {
	repo := newFakeCBStateRepo()
	shared := ToolBreakerNameShared("ws1", "smtp")
	repo.store[shared] = CircuitBreakerStateEntry{State: string(CircuitOpen), FailureCount: 3}
	// Node 熔断 / Agent 易失前缀不得被工具注册表认领（入口过滤双保险）。
	nodeKey := "graph_node:scope:node:n1"
	repo.store[nodeKey] = CircuitBreakerStateEntry{State: string(CircuitOpen), FailureCount: 3}

	reg := ProvideToolCircuitBreakerRegistry(repo, loggateway.NewNoop())

	cb := reg.Get(shared, "")
	if cb.State() != CircuitOpen {
		t.Fatalf("shared breaker should restore open, got %s", cb.State())
	}
	other := reg.Get(nodeKey, "")
	if other.State() != CircuitClosed {
		t.Fatalf("non-tool namespace key must not restore via tool registry, got %s", other.State())
	}
}

// nil repo 降级为全易失，不 panic、可正常熔断（TPM-P2-02 降级传统）。
func TestProvideToolCircuitBreakerRegistry_NilRepoDegrades(t *testing.T) {
	reg := ProvideToolCircuitBreakerRegistry(nil, loggateway.NewNoop())
	name := ToolBreakerNameShared("ws1", "x")
	reg.SetOverride(name, CircuitBreakerConfig{FailureThreshold: 1, RecoveryTimeoutSec: 60, HalfOpenMaxProbe: 1})
	cb := reg.Get(name, "")
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("volatile registry should still open, got %s", cb.State())
	}
}
