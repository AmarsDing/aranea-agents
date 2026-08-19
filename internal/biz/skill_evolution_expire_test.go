package biz

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// expireTargetStubStore 同时实现 checkReader/writer 及 per-target 过期的两个可选接口。
type expireTargetStubStore struct {
	targets      []UnifiedEvolutionPendingTarget
	expiredFor   map[string]time.Time // key: targetType/targetID → 实际使用的 cutoff
	globalCalls  int
	perTargetSum int
}

func (s *expireTargetStubStore) HasPendingForTarget(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (s *expireTargetStubStore) GetLatestByTarget(_ context.Context, _, _ string) (*UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *expireTargetStubStore) GetLatestByTargetAndAction(_ context.Context, _, _, _ string) (*UnifiedEvolutionSuggestion, error) {
	return nil, nil
}
func (s *expireTargetStubStore) Create(_ context.Context, _ UnifiedEvolutionSuggestion) error {
	return nil
}
func (s *expireTargetStubStore) UpdateStatus(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (s *expireTargetStubStore) UpdateStatusCAS(_ context.Context, _ string, _ []string, _, _, _ string) (bool, error) {
	return true, nil
}
func (s *expireTargetStubStore) UpdateDraftBody(_ context.Context, _, _ string) error { return nil }
func (s *expireTargetStubStore) UpdateLifecycleStatus(_ context.Context, _, _ string) error {
	return nil
}
func (s *expireTargetStubStore) UpdateSandboxResult(_ context.Context, _ string, _ bool, _ json.RawMessage) error {
	return nil
}
func (s *expireTargetStubStore) UpdateMetadataKey(_ context.Context, _, _, _ string) error {
	return nil
}
func (s *expireTargetStubStore) ExpireOlderThan(_ context.Context, _ time.Time) (int, error) {
	s.globalCalls++
	return 0, nil
}
func (s *expireTargetStubStore) ListPendingTargets(_ context.Context) ([]UnifiedEvolutionPendingTarget, error) {
	return s.targets, nil
}
func (s *expireTargetStubStore) ExpireOlderThanForTarget(_ context.Context, targetType, targetID string, cutoff time.Time) (int, error) {
	if s.expiredFor == nil {
		s.expiredFor = map[string]time.Time{}
	}
	s.expiredFor[targetType+"/"+targetID] = cutoff
	s.perTargetSum++
	return 1, nil
}

// 接线 resolver 后：agent target 用其独立 TTL，resolver 返回 <=0 的 target 回退全局默认。
func TestOrchestrator_ExpirePending_PerTargetTTL(t *testing.T) {
	store := &expireTargetStubStore{
		targets: []UnifiedEvolutionPendingTarget{
			{TargetType: string(EvolutionTargetAgent), TargetID: "agent-a"},
			{TargetType: string(EvolutionTargetSkill), TargetID: "skill-1"},
		},
	}
	orch := NewSkillEvolutionOrchestrator(store, store, loggateway.NewNoop())
	orch.SetExpirationResolver(func(_ context.Context, targetType, targetID string) time.Duration {
		if targetType == string(EvolutionTargetAgent) && targetID == "agent-a" {
			return 30 * 24 * time.Hour
		}
		return 0
	})

	total, err := orch.ExpirePending(context.Background())
	if err != nil {
		t.Fatalf("ExpirePending: %v", err)
	}
	if total != 2 {
		t.Fatalf("total=%d, want 2", total)
	}
	if store.globalCalls != 0 {
		t.Fatalf("global ExpireOlderThan called %d times, want 0", store.globalCalls)
	}
	before := time.Now().UTC()
	agentCutoff, ok := store.expiredFor["agent/agent-a"]
	if !ok {
		t.Fatalf("agent-a not expired per-target: %#v", store.expiredFor)
	}
	if d := before.Sub(agentCutoff); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("agent-a cutoff delta=%v, want ~30d", d)
	}
	skillCutoff, ok := store.expiredFor["skill/skill-1"]
	if !ok {
		t.Fatalf("skill-1 not expired per-target: %#v", store.expiredFor)
	}
	wantFallback := time.Duration(EvoExpirationDays) * 24 * time.Hour
	if d := before.Sub(skillCutoff); d < wantFallback-time.Hour || d > wantFallback+time.Hour {
		t.Fatalf("skill-1 cutoff delta=%v, want ~%v (global fallback)", d, wantFallback)
	}
}

// 未接线 resolver 时保持全局统一过期（兼容路径）。
func TestOrchestrator_ExpirePending_NoResolver_FallsBackGlobal(t *testing.T) {
	store := &expireTargetStubStore{
		targets: []UnifiedEvolutionPendingTarget{
			{TargetType: string(EvolutionTargetAgent), TargetID: "agent-a"},
		},
	}
	orch := NewSkillEvolutionOrchestrator(store, store, loggateway.NewNoop())
	if _, err := orch.ExpirePending(context.Background()); err != nil {
		t.Fatalf("ExpirePending: %v", err)
	}
	if store.globalCalls != 1 {
		t.Fatalf("global ExpireOlderThan called %d times, want 1", store.globalCalls)
	}
	if store.perTargetSum != 0 {
		t.Fatalf("per-target expire called %d times, want 0", store.perTargetSum)
	}
}
