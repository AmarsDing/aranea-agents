package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── fakes ────────────────────────────────────────────────────────────────────

// fakeUnifiedStore implements UnifiedEvolutionStore with only the methods the
// attribution path uses; everything else panics to surface unexpected calls.
type fakeUnifiedStore struct {
	applied        []UnifiedEvolutionSuggestion
	listErr        error
	metaUpdates    map[string]string // key → last written value
	metaUpdateErr  error
}

func (f *fakeUnifiedStore) ListByTarget(_ context.Context, targetType, targetID, status string, limit, offset int) ([]UnifiedEvolutionSuggestion, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if status != "applied" {
		return nil, fmt.Errorf("unexpected status filter %q", status)
	}
	return f.applied, nil
}

func (f *fakeUnifiedStore) UpdateMetadataKey(_ context.Context, id, key, value string) error {
	if f.metaUpdateErr != nil {
		return f.metaUpdateErr
	}
	if f.metaUpdates == nil {
		f.metaUpdates = map[string]string{}
	}
	f.metaUpdates[key] = value
	return nil
}

// Unused interface methods — panic to surface unexpected calls.
func (f *fakeUnifiedStore) HasPendingForTarget(context.Context, string, string) (bool, error) {
	panic("unused")
}
func (f *fakeUnifiedStore) GetLatestByTarget(context.Context, string, string) (*UnifiedEvolutionSuggestion, error) {
	panic("unused")
}
func (f *fakeUnifiedStore) GetLatestByTargetAndAction(context.Context, string, string, string) (*UnifiedEvolutionSuggestion, error) {
	panic("unused")
}
func (f *fakeUnifiedStore) GetByID(context.Context, string) (*UnifiedEvolutionSuggestion, error) {
	panic("unused")
}
func (f *fakeUnifiedStore) CountByTarget(context.Context, string, string, string) (int, error) {
	panic("unused")
}
func (f *fakeUnifiedStore) ListByTargetAndAction(context.Context, string, string, string, string, int, int) ([]UnifiedEvolutionSuggestion, error) {
	panic("unused")
}
func (f *fakeUnifiedStore) CountByTargetAndAction(context.Context, string, string, string, string) (int, error) {
	panic("unused")
}
func (f *fakeUnifiedStore) Create(context.Context, UnifiedEvolutionSuggestion) error { panic("unused") }
func (f *fakeUnifiedStore) UpdateStatus(context.Context, string, string, string, string) error {
	panic("unused")
}
func (f *fakeUnifiedStore) UpdateDraftBody(context.Context, string, string) error { panic("unused") }
func (f *fakeUnifiedStore) UpdateLifecycleStatus(context.Context, string, string) error {
	panic("unused")
}
func (f *fakeUnifiedStore) UpdateSandboxResult(context.Context, string, bool, json.RawMessage) error {
	panic("unused")
}
func (f *fakeUnifiedStore) ExpireOlderThan(context.Context, time.Time) (int, error) { panic("unused") }

// fakeAggregator implements SkillHealthAggregator.
type fakeAggregator struct {
	metrics *SkillHealthMetrics
	err     error
	called  bool
}

func (f *fakeAggregator) GetHealthMetrics(context.Context, string, time.Time) (*SkillHealthMetrics, error) {
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return f.metrics, nil
}
func (f *fakeAggregator) GetFailureStats(context.Context, string, time.Time) (*SkillFailureStats, error) {
	panic("unused")
}
func (f *fakeAggregator) GetFailureTagCounts(context.Context, string, time.Time) ([]FailureTagCount, error) {
	panic("unused")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func appliedSuggestionWithMeta(t *testing.T, meta map[string]any) UnifiedEvolutionSuggestion {
	t.Helper()
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	return UnifiedEvolutionSuggestion{
		ID:         "sug-1",
		TargetType: EvolutionTargetSkill,
		TargetID:   "sk-1",
		Status:     "applied",
		Metadata:   raw,
		CreatedAt:  time.Now().UTC().Add(-time.Hour),
	}
}

func newAttributionUsecase(store UnifiedEvolutionStore, agg SkillHealthAggregator) *SkillIntelligenceUsecase {
	return &SkillIntelligenceUsecase{
		unifiedStore: store,
		aggregator:   agg,
		lg:           loggateway.NewNoop(),
	}
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestAttributeLastEvolution_VerdictQuadrants(t *testing.T) {
	cases := []struct {
		name        string
		baseline    float64
		current     float64
		invocations int
		want        string
	}{
		{"helpful", 0.50, 0.56, 10, EvoEffectivenessHelpful},
		{"harmful", 0.60, 0.54, 10, EvoEffectivenessHarmful},
		{"neutral", 0.50, 0.52, 10, EvoEffectivenessNeutral},
		{"insufficient_data", 0.50, 0.90, attributionMinInvocations - 1, EvoEffectivenessInsufficientData},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeUnifiedStore{
				applied: []UnifiedEvolutionSuggestion{
					appliedSuggestionWithMeta(t, map[string]any{EvoMetaBaselineSuccessRate: tc.baseline}),
				},
			}
			agg := &fakeAggregator{metrics: &SkillHealthMetrics{
				SuccessRate:     tc.current,
				InvocationCount: tc.invocations,
			}}
			uc := newAttributionUsecase(store, agg)

			attr := uc.AttributeLastEvolution(context.Background(), "sk-1")
			if attr == nil {
				t.Fatal("expected attribution, got nil")
			}
			if attr.Verdict != tc.want {
				t.Fatalf("verdict = %q, want %q", attr.Verdict, tc.want)
			}
			if attr.BaselineSuccessRate != tc.baseline || attr.CurrentSuccessRate != tc.current {
				t.Fatalf("rates = %v/%v", attr.BaselineSuccessRate, attr.CurrentSuccessRate)
			}
			// 裁决回写
			if got := store.metaUpdates[EvoMetaEffectiveness]; got != tc.want {
				t.Fatalf("effectiveness write-back = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAttributeLastEvolution_NoAppliedSuggestion(t *testing.T) {
	uc := newAttributionUsecase(&fakeUnifiedStore{}, &fakeAggregator{})
	if attr := uc.AttributeLastEvolution(context.Background(), "sk-1"); attr != nil {
		t.Fatalf("expected nil for first evolution cycle, got %+v", attr)
	}
}

func TestAttributeLastEvolution_MissingBaseline(t *testing.T) {
	store := &fakeUnifiedStore{
		applied: []UnifiedEvolutionSuggestion{appliedSuggestionWithMeta(t, map[string]any{})},
	}
	uc := newAttributionUsecase(store, &fakeAggregator{})
	if attr := uc.AttributeLastEvolution(context.Background(), "sk-1"); attr != nil {
		t.Fatalf("expected nil without baseline, got %+v", attr)
	}
}

func TestAttributeLastEvolution_IdempotentReuse(t *testing.T) {
	// 已裁决过的建议：直接复用存储的裁决，不重算、不回写、不调 aggregator。
	store := &fakeUnifiedStore{
		applied: []UnifiedEvolutionSuggestion{
			appliedSuggestionWithMeta(t, map[string]any{
				EvoMetaBaselineSuccessRate: 0.5,
				EvoMetaEffectiveness:       EvoEffectivenessHarmful,
			}),
		},
	}
	agg := &fakeAggregator{metrics: &SkillHealthMetrics{SuccessRate: 0.9, InvocationCount: 100}}
	uc := newAttributionUsecase(store, agg)

	attr := uc.AttributeLastEvolution(context.Background(), "sk-1")
	if attr == nil || attr.Verdict != EvoEffectivenessHarmful {
		t.Fatalf("expected stored verdict reused, got %+v", attr)
	}
	if agg.called {
		t.Fatal("aggregator must not be called for an adjudicated suggestion")
	}
	if len(store.metaUpdates) != 0 {
		t.Fatalf("no write-back expected, got %v", store.metaUpdates)
	}
}

func TestAttributeLastEvolution_AffectedRuleIDs(t *testing.T) {
	// delta_ops 经 UpdateMetadataKey 落库为 JSON 字符串（双重编码）。
	opsJSON, _ := json.Marshal([]DeltaOp{
		{Op: DeltaOpModify, RuleID: "timeout-retry", Content: "x"},
		{Op: DeltaOpAdd, RuleID: "rate-limit", Content: "y"},
		{Op: DeltaOpModify, RuleID: "timeout-retry", Content: "x2"}, // 重复 ID 去重
	})
	store := &fakeUnifiedStore{
		applied: []UnifiedEvolutionSuggestion{
			appliedSuggestionWithMeta(t, map[string]any{
				EvoMetaBaselineSuccessRate: 0.5,
				EvoMetaDeltaOps:            string(opsJSON),
			}),
		},
	}
	agg := &fakeAggregator{metrics: &SkillHealthMetrics{SuccessRate: 0.6, InvocationCount: 10}}
	uc := newAttributionUsecase(store, agg)

	attr := uc.AttributeLastEvolution(context.Background(), "sk-1")
	if attr == nil {
		t.Fatal("expected attribution")
	}
	if len(attr.AffectedRuleIDs) != 2 || attr.AffectedRuleIDs[0] != "timeout-retry" || attr.AffectedRuleIDs[1] != "rate-limit" {
		t.Fatalf("AffectedRuleIDs = %v", attr.AffectedRuleIDs)
	}
}

func TestAttributeLastEvolution_NilSafe(t *testing.T) {
	// store/aggregator 均为 nil → nil（首次进化或无依赖环境）
	uc := &SkillIntelligenceUsecase{lg: loggateway.NewNoop()}
	if attr := uc.AttributeLastEvolution(context.Background(), "sk-1"); attr != nil {
		t.Fatalf("expected nil with nil deps, got %+v", attr)
	}
}
