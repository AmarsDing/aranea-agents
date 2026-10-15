package evaluation

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"testing"
	"time"
)

func TestCompareEvalRuns(t *testing.T) {
	t.Run("delta calculation between runs", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1", CreatedAt: "t1",
					ExactMatchScore: 0.5, ContainsMatchScore: 0.6, LLMJudgeScore: 0.7, ToolCallAccuracy: 0.8, PassAtK: 0.9, PassHatK: 0.4},
				{ID: "r2", AgentID: "a1", DatasetID: "d1", CreatedAt: "t2",
					ExactMatchScore: 0.7, ContainsMatchScore: 0.5, LLMJudgeScore: 0.9, ToolCallAccuracy: 0.6, PassAtK: 0.8, PassHatK: 0.5},
			},
		}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(comps) != 2 {
			t.Fatalf("expected 2 comparisons, got %d", len(comps))
		}

		c0 := comps[0]
		if c0.RunID != "r1" {
			t.Fatalf("expected run_id 'r1', got %q", c0.RunID)
		}
		if c0.DeltaExactMatch != 0.0 {
			t.Fatalf("baseline DeltaExactMatch should be 0, got %v", c0.DeltaExactMatch)
		}
		if c0.DeltaContainsMatch != 0.0 {
			t.Fatalf("baseline DeltaContainsMatch should be 0, got %v", c0.DeltaContainsMatch)
		}
		if c0.DeltaLLMJudge != 0.0 {
			t.Fatalf("baseline DeltaLLMJudge should be 0, got %v", c0.DeltaLLMJudge)
		}
		if c0.DeltaToolAccuracy != 0.0 {
			t.Fatalf("baseline DeltaToolAccuracy should be 0, got %v", c0.DeltaToolAccuracy)
		}

		c1 := comps[1]
		if c1.RunID != "r2" {
			t.Fatalf("expected run_id 'r2', got %q", c1.RunID)
		}
		if !approxEq(c1.ExactMatchScore, 0.7) {
			t.Fatalf("expected ExactMatchScore 0.7, got %v", c1.ExactMatchScore)
		}
		if !approxEq(c1.DeltaExactMatch, 0.2) {
			t.Fatalf("expected DeltaExactMatch 0.2, got %v", c1.DeltaExactMatch)
		}
		if !approxEq(c1.DeltaContainsMatch, -0.1) {
			t.Fatalf("expected DeltaContainsMatch -0.1, got %v", c1.DeltaContainsMatch)
		}
		if !approxEq(c1.DeltaLLMJudge, 0.2) {
			t.Fatalf("expected DeltaLLMJudge 0.2, got %v", c1.DeltaLLMJudge)
		}
		if !approxEq(c1.DeltaToolAccuracy, -0.2) {
			t.Fatalf("expected DeltaToolAccuracy -0.2, got %v", c1.DeltaToolAccuracy)
		}
	})

	t.Run("single run ID returns error", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), []string{"r1"})
		if err == nil {
			t.Fatal("expected error for single run ID, got nil")
		}
	})

	t.Run("empty run IDs returns error", func(t *testing.T) {
		repo := &mockRepo{}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for empty run IDs, got nil")
		}
	})

	t.Run("repo returns fewer than 2 runs", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1"},
			},
		}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r-missing"})
		if err == nil {
			t.Fatal("expected error when repo returns fewer than 2 runs, got nil")
		}
	})

	t.Run("repo returns error", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsErr: errors.New("db down"),
		}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		_, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err == nil {
			t.Fatal("expected error from repo, got nil")
		}
	})

	t.Run("three runs delta against baseline", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1", CreatedAt: "t1",
					ExactMatchScore: 0.4, ContainsMatchScore: 0.5, LLMJudgeScore: 0.6, ToolCallAccuracy: 0.7, PassAtK: 0.8, PassHatK: 0.3},
				{ID: "r2", AgentID: "a1", DatasetID: "d1", CreatedAt: "t2",
					ExactMatchScore: 0.6, ContainsMatchScore: 0.5, LLMJudgeScore: 0.8, ToolCallAccuracy: 0.7, PassAtK: 0.9, PassHatK: 0.4},
				{ID: "r3", AgentID: "a1", DatasetID: "d1", CreatedAt: "t3",
					ExactMatchScore: 0.3, ContainsMatchScore: 0.7, LLMJudgeScore: 0.5, ToolCallAccuracy: 0.9, PassAtK: 0.7, PassHatK: 0.6},
			},
		}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2", "r3"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(comps) != 3 {
			t.Fatalf("expected 3 comparisons, got %d", len(comps))
		}

		if comps[0].DeltaExactMatch != 0.0 {
			t.Fatalf("r1 is baseline, DeltaExactMatch should be 0, got %v", comps[0].DeltaExactMatch)
		}
		if !approxEq(comps[1].DeltaExactMatch, 0.2) {
			t.Fatalf("r2 DeltaExactMatch should be 0.2, got %v", comps[1].DeltaExactMatch)
		}
		if !approxEq(comps[2].DeltaExactMatch, -0.1) {
			t.Fatalf("r3 DeltaExactMatch should be -0.1, got %v", comps[2].DeltaExactMatch)
		}
		if !approxEq(comps[2].DeltaContainsMatch, 0.2) {
			t.Fatalf("r3 DeltaContainsMatch should be 0.2, got %v", comps[2].DeltaContainsMatch)
		}
		if !approxEq(comps[2].DeltaToolAccuracy, 0.2) {
			t.Fatalf("r3 DeltaToolAccuracy should be 0.2, got %v", comps[2].DeltaToolAccuracy)
		}
	})

	t.Run("comparison fields carry run data", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1", AgentID: "a1", DatasetID: "d1", CreatedAt: "t1",
					ExactMatchScore: 0.5, ContainsMatchScore: 0.6, LLMJudgeScore: 0.7, ToolCallAccuracy: 0.8, PassAtK: 0.9, PassHatK: 0.4},
				{ID: "r2", AgentID: "a2", DatasetID: "d2", CreatedAt: "t2",
					ExactMatchScore: 0.5, ContainsMatchScore: 0.6, LLMJudgeScore: 0.7, ToolCallAccuracy: 0.8, PassAtK: 0.9, PassHatK: 0.4},
			},
		}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		c1 := comps[1]
		if c1.AgentID != "a2" {
			t.Fatalf("expected AgentID 'a2', got %q", c1.AgentID)
		}
		if c1.DatasetID != "d2" {
			t.Fatalf("expected DatasetID 'd2', got %q", c1.DatasetID)
		}
		if c1.CreatedAt != "t2" {
			t.Fatalf("expected CreatedAt 't2', got %q", c1.CreatedAt)
		}
		if !approxEq(c1.PassAtK, 0.9) {
			t.Fatalf("expected PassAtK 0.9, got %v", c1.PassAtK)
		}
		if !approxEq(c1.PassHatK, 0.4) {
			t.Fatalf("expected PassHatK 0.4, got %v", c1.PassHatK)
		}
	})

	t.Run("zero scores delta is zero", func(t *testing.T) {
		repo := &mockRepo{
			getRunsByIDsRuns: []Run{
				{ID: "r1"}, {ID: "r2"},
			},
		}
		uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
		comps, err := uc.CompareEvalRuns(context.Background(), []string{"r1", "r2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if comps[1].DeltaExactMatch != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaExactMatch)
		}
		if comps[1].DeltaContainsMatch != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaContainsMatch)
		}
		if comps[1].DeltaLLMJudge != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaLLMJudge)
		}
		if comps[1].DeltaToolAccuracy != 0 {
			t.Fatalf("expected 0 delta, got %v", comps[1].DeltaToolAccuracy)
		}
	})
}

func TestFailStaleRunsUsesGrace(t *testing.T) {
	repo := &mockRepo{}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	before := time.Now().Add(-StaleRunGrace)
	n, err := uc.FailStaleRuns(context.Background(), 0)
	after := time.Now().Add(-StaleRunGrace)
	if err != nil || n != 0 {
		t.Fatalf("grace sweep: n=%d err=%v", n, err)
	}
	if repo.staleCutoff.Before(before.Add(-2*time.Second)) || repo.staleCutoff.After(after.Add(2*time.Second)) {
		t.Fatalf("cutoff %v not around now-StaleRunGrace", repo.staleCutoff)
	}
}

func TestUpdateRunRejectsIllegalTransition(t *testing.T) {
	repo := &mockRepo{runs: []Run{{ID: "r1", Status: RunStatusCompleted}}}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	if err := uc.UpdateRun(context.Background(), Run{ID: "r1", Status: RunStatusRunning}); err == nil {
		t.Fatal("completed → running must be rejected")
	}
}

func TestUpdateCase(t *testing.T) {
	repo := &mockRepo{cases: []Case{{ID: "c1", DatasetID: "ds-1", Input: "old"}}}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	got, err := uc.UpdateCase(context.Background(), Case{ID: "c1", DatasetID: "ds-1", Input: "new", ExpectedOutput: "out"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Input != "new" || got.ExpectedOutput != "out" {
		t.Fatalf("got %+v", got)
	}
	if _, err := uc.UpdateCase(context.Background(), Case{ID: "c1", DatasetID: "ds-1"}); err == nil {
		t.Fatal("empty input must fail")
	}
}

func TestDeleteCase(t *testing.T) {
	repo := &mockRepo{cases: []Case{{ID: "c1", DatasetID: "ds-1", Input: "q"}}}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	if err := uc.DeleteCase(context.Background(), "ds-1", "c1"); err != nil {
		t.Fatal(err)
	}
	if len(repo.cases) != 0 {
		t.Fatalf("expected empty cases, got %d", len(repo.cases))
	}
	if err := uc.DeleteCase(context.Background(), "", "c1"); err == nil {
		t.Fatal("empty dataset_id must fail")
	}
}

func TestFindInFlightRun(t *testing.T) {
	repo := &mockRepo{runs: []Run{
		{ID: "done", DatasetID: "ds-1", AgentID: "a1", Status: RunStatusCompleted},
		{ID: "live", DatasetID: "ds-1", AgentID: "a1", Status: RunStatusRunning},
	}}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	run, ok, err := uc.FindInFlightRun(context.Background(), "ds-1", "a1")
	if err != nil || !ok || run.ID != "live" {
		t.Fatalf("got run=%+v ok=%v err=%v", run, ok, err)
	}
	if _, ok, err := uc.FindInFlightRun(context.Background(), "ds-1", "other"); err != nil || ok {
		t.Fatal("other agent must have no in-flight run")
	}
}

func TestFindInFlightExperimentCell(t *testing.T) {
	repo := &mockRepo{runs: []Run{
		{ID: "gpt4", DatasetID: "ds-1", AgentID: "a1", Status: RunStatusRunning, VariantLabel: "a1/gpt-4o"},
		{ID: "mini", DatasetID: "ds-1", AgentID: "a1", Status: RunStatusPending, VariantLabel: "a1/mini"},
	}}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	got, ok, err := uc.FindInFlightExperimentCell(context.Background(), "ds-1", "a1", "a1/mini")
	if err != nil || !ok || got.ID != "mini" {
		t.Fatalf("cell lock: %+v ok=%v err=%v", got, ok, err)
	}
	if _, ok, err := uc.FindInFlightExperimentCell(context.Background(), "ds-1", "a1", "a1/other"); err != nil || ok {
		t.Fatal("unknown cell must not lock")
	}
}

func TestCancelRun(t *testing.T) {
	repo := &mockRepo{runs: []Run{{ID: "r1", Status: RunStatusRunning}}}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	got, err := uc.CancelRun(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != RunStatusCancelled {
		t.Fatalf("status = %q", got.Status)
	}
	if _, err := uc.CancelRun(context.Background(), "r1"); err == nil {
		t.Fatal("second cancel must conflict")
	}
}

func TestUpdateRunRefreshesInFlightLease(t *testing.T) {
	repo := &mockRepo{runs: []Run{{
		ID:         "r1",
		Status:     RunStatusRunning,
		LeaseUntil: "2000-01-01T00:00:00Z",
	}}}
	uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
	if err := uc.UpdateRun(context.Background(), repo.runs[0]); err != nil {
		t.Fatal(err)
	}
	if repo.runs[0].LeaseUntil == "2000-01-01T00:00:00Z" || repo.runs[0].LeaseUntil == "" {
		t.Fatalf("in-flight UpdateRun must refresh lease, got %q", repo.runs[0].LeaseUntil)
	}
}
