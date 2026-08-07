package evaluation

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ISSUE-004: the comparison baseline must be the earliest run by CreatedAt,
// regardless of the order run IDs are passed in (the frontend table is sorted
// newest-first, so input order would otherwise pick the newest as baseline).
func TestCompareEvalRunsBaselineIsEarliestByCreatedAt(t *testing.T) {
	repo := &mockRepo{getRunsByIDsRuns: []Run{
		{ID: "new", CreatedAt: "2026-08-07T10:00:00Z", ExactMatchScore: 0.5, LLMJudgeScore: 0.6},
		{ID: "old", CreatedAt: "2026-08-06T10:00:00Z", ExactMatchScore: 0.8, LLMJudgeScore: 0.4},
	}}
	uc := NewUsecase(repo, loggateway.NewNoop())

	out, err := uc.CompareEvalRuns(context.Background(), []string{"new", "old"})
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 comparisons, got %d", len(out))
	}
	byID := map[string]RunComparison{}
	for _, c := range out {
		byID[c.RunID] = c
	}
	oldC := byID["old"]
	if oldC.DeltaExactMatch != 0 || oldC.DeltaLLMJudge != 0 {
		t.Fatalf("earliest run must be the baseline (delta=0), got exact=%v llm=%v", oldC.DeltaExactMatch, oldC.DeltaLLMJudge)
	}
	newC := byID["new"]
	if !approxEq(newC.DeltaExactMatch, -0.3) {
		t.Fatalf("expected new delta exact=-0.3 vs earliest baseline, got %v", newC.DeltaExactMatch)
	}
	if !approxEq(newC.DeltaLLMJudge, 0.2) {
		t.Fatalf("expected new delta llm=+0.2 vs earliest baseline, got %v", newC.DeltaLLMJudge)
	}
}

// Single-run input must still be rejected.
func TestCompareEvalRunsRequiresTwoRuns(t *testing.T) {
	uc := NewUsecase(&mockRepo{}, loggateway.NewNoop())
	if _, err := uc.CompareEvalRuns(context.Background(), []string{"only"}); err == nil {
		t.Fatal("expected error for fewer than two run ids")
	}
}
