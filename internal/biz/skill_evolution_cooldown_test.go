package biz

import (
	"context"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── F9 (P-evo-4): cooldown must only count active lifecycle states ───────────
//
// A rejected / expired / rolled_back suggestion must not suppress re-triggering
// for a week — rejecting a low-quality draft is not "using up" the skill's
// evolution opportunity. Only pending / approved / applied participate in the
// cooldown window.

func TestOrchestrator_CheckAndCreate_RejectedLatest_DoesNotCooldown(t *testing.T) {
	recent := &UnifiedEvolutionSuggestion{
		Status:    "rejected",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour), // well within 168h
	}
	check := &orchStubCheckReader{latestByAction: map[string]*UnifiedEvolutionSuggestion{
		string(EvolutionActionImprove): recent,
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
	orch.RegisterTrigger(tr)

	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("rejected suggestion must not trigger cooldown: created = %d, want 1", len(created))
	}
}

func TestOrchestrator_CheckAndCreate_ActiveLatest_StillCooldowns(t *testing.T) {
	for _, status := range []string{"pending", "approved", "applied"} {
		t.Run(status, func(t *testing.T) {
			recent := &UnifiedEvolutionSuggestion{
				Status:    status,
				CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
			}
			check := &orchStubCheckReader{latestByAction: map[string]*UnifiedEvolutionSuggestion{
				string(EvolutionActionImprove): recent,
			}}
			writer := &orchStubWriter{}
			orch := NewSkillEvolutionOrchestrator(check, writer, loggateway.NewNoop())
			tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{newTriggerSuggestion(EvolutionActionImprove)}}
			orch.RegisterTrigger(tr)

			created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
			if err != nil {
				t.Fatalf("CheckAndCreate: %v", err)
			}
			if len(created) != 0 {
				t.Fatalf("status %q within cooldown must still block: created = %d, want 0", status, len(created))
			}
		})
	}
}

func TestCheckEvolutionTriggers_RejectedLatest_DoesNotCooldown(t *testing.T) {
	agg := &mockSkillHealthAggregator{
		metrics: &SkillHealthMetrics{
			SkillID:         "skill-rej",
			InvocationCount: 10,
			SuccessCount:    3,
			SuccessRate:     0.3, // < 60% → fix_failure trigger fires
			AvgDurationMS:   5000,
		},
		tagCounts: []FailureTagCount{},
	}
	store := newRecordingUnifiedStore()
	store.latest = &UnifiedEvolutionSuggestion{
		Status:    "rejected",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour), // within cooldown window
	}
	lg := loggateway.NewNoop()
	scorer := NewSkillScoringUsecase(agg, lg)
	reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
	uc := NewSkillIntelligenceUsecase(scorer, reporter, store, agg, lg)

	suggestion, err := uc.CheckEvolutionTriggers(context.Background(), "skill-rej")
	if err != nil {
		t.Fatalf("CheckEvolutionTriggers: %v", err)
	}
	if suggestion == nil {
		t.Fatal("rejected suggestion must not trigger cooldown: expected a new suggestion")
	}
}

func TestCheckEvolutionTriggers_ActiveLatest_StillCooldowns(t *testing.T) {
	for _, status := range []string{"pending", "approved", "applied"} {
		t.Run(status, func(t *testing.T) {
			agg := &mockSkillHealthAggregator{
				metrics: &SkillHealthMetrics{
					SkillID:         "skill-act",
					InvocationCount: 10,
					SuccessCount:    3,
					SuccessRate:     0.3,
					AvgDurationMS:   5000,
				},
				tagCounts: []FailureTagCount{},
			}
			store := newRecordingUnifiedStore()
			store.latest = &UnifiedEvolutionSuggestion{
				Status:    status,
				CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
			}
			lg := loggateway.NewNoop()
			scorer := NewSkillScoringUsecase(agg, lg)
			reporter := NewSkillReportUsecase(nil, nil, nil, scorer, nil, lg)
			uc := NewSkillIntelligenceUsecase(scorer, reporter, store, agg, lg)

			suggestion, err := uc.CheckEvolutionTriggers(context.Background(), "skill-act")
			if err != nil {
				t.Fatalf("CheckEvolutionTriggers: %v", err)
			}
			if suggestion != nil {
				t.Fatalf("status %q within cooldown must still block: got suggestion type %q", status, suggestion.Type)
			}
		})
	}
}

// ── D8 自适应降频：trigger_source 冷却乘数 ──────────────────────────────────

// 乘数 ×2 后，原冷却窗外、加倍窗口内的建议仍被抑制。
func TestOrchestrator_CooldownMultiplier_ExtendsWindow(t *testing.T) {
	sug := newTriggerSuggestion(EvolutionActionImprove)
	sug.TriggerSource = TriggerSourceErrorCluster
	// 原 168h 窗口外（+1h）、加倍 336h 窗口内。
	old := &UnifiedEvolutionSuggestion{
		Status:    "pending",
		CreatedAt: time.Now().UTC().Add(-(EvoTriggerCooldownHours + 1) * time.Hour),
	}
	check := &orchStubCheckReader{latestByAction: map[string]*UnifiedEvolutionSuggestion{
		string(EvolutionActionImprove): old,
	}}
	writer := &orchStubWriter{}
	orch := NewSkillEvolutionOrchestrator(check, writer, loggateway.NewNoop())
	tr := &stubTrigger{targetType: EvolutionTargetSkill, suggestions: []UnifiedEvolutionSuggestion{sug}}
	orch.RegisterTrigger(tr)

	// 无乘数：窗口已过 → 创建。
	created, err := orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("无乘数时窗口已过应创建, 实际 %d", len(created))
	}

	// 乘数 ×2：加倍窗口内 → 抑制。
	orch.SetTriggerCooldownMultiplier(TriggerSourceErrorCluster, 2)
	writer.created = nil
	created, err = orch.CheckAndCreate(context.Background(), EvolutionTargetSkill, "skill-1")
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("×2 后加倍窗口内应抑制, 实际创建 %d", len(created))
	}
}

// 乘数叠加上限 8×。
func TestOrchestrator_CooldownMultiplier_Capped(t *testing.T) {
	orch := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubWriter{}, loggateway.NewNoop())
	for i := 0; i < 5; i++ { // 2^5=32 → 应封顶 8
		orch.SetTriggerCooldownMultiplier(TriggerSourceErrorCluster, 2)
	}
	if m := orch.triggerCooldownMultiplier(TriggerSourceErrorCluster); m != siMaxTriggerCooldownMultiplier {
		t.Fatalf("乘数应封顶 %.0f, 实际 %.1f", siMaxTriggerCooldownMultiplier, m)
	}
	// 未设置的 source 默认 1；factor<=1 为 no-op。
	if m := orch.triggerCooldownMultiplier("other"); m != 1 {
		t.Fatalf("未设置 source 应为 1, 实际 %.1f", m)
	}
	orch.SetTriggerCooldownMultiplier("other", 1)
	if m := orch.triggerCooldownMultiplier("other"); m != 1 {
		t.Fatalf("factor<=1 不应生效, 实际 %.1f", m)
	}
}

type memCooldownStore struct {
	m map[string]float64
}

func (s *memCooldownStore) LoadTriggerCooldownMultipliers(_ context.Context) (map[string]float64, error) {
	out := make(map[string]float64, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out, nil
}

func (s *memCooldownStore) SaveTriggerCooldownMultipliers(_ context.Context, multipliers map[string]float64) error {
	s.m = make(map[string]float64, len(multipliers))
	for k, v := range multipliers {
		s.m[k] = v
	}
	return nil
}

// 冷却写入后新编排器实例（模拟重启）仍读到同一倍率。
func TestOrchestrator_CooldownMultiplier_PersistsAcrossRestart(t *testing.T) {
	store := &memCooldownStore{}
	orch1 := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubWriter{}, loggateway.NewNoop())
	orch1.AttachCooldownStore(store)
	orch1.SetTriggerCooldownMultiplier(TriggerSourceErrorCluster, 2)
	orch1.SetTriggerCooldownMultiplier(TriggerSourceErrorCluster, 2) // 4×
	if m := orch1.triggerCooldownMultiplier(TriggerSourceErrorCluster); m != 4 {
		t.Fatalf("before restart want 4, got %.1f", m)
	}

	orch2 := NewSkillEvolutionOrchestrator(&orchStubCheckReader{}, &orchStubWriter{}, loggateway.NewNoop())
	orch2.AttachCooldownStore(store)
	if err := orch2.HydrateTriggerCooldowns(context.Background()); err != nil {
		t.Fatalf("HydrateTriggerCooldowns: %v", err)
	}
	if m := orch2.triggerCooldownMultiplier(TriggerSourceErrorCluster); m != 4 {
		t.Fatalf("after restart want 4, got %.1f", m)
	}
	if m := orch2.triggerCooldownMultiplier("other"); m != 1 {
		t.Fatalf("unset source after restart want 1, got %.1f", m)
	}
}
