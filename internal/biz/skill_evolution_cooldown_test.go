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
	orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
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
			orch := NewSkillEvolutionOrchestrator(check, &orchStubQueryReader{}, writer, loggateway.NewNoop())
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
