package evaluation

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// P1-3: judge-vs-human divergence statistics (judge calibration, T6).
// The judge "ran" for a result iff scores_json carries the llm_as_judge key —
// the legacy runner never computes it and the column default 0 must not be
// mistaken for a real judge score.

func judgedResult(id string, judgeScore float32, humanPass bool) JudgeAnnotatedResult {
	return JudgeAnnotatedResult{
		CaseResult: CaseResult{
			ID:            id,
			RunID:         "run-1",
			CaseID:        "case-" + id,
			LLMJudgeScore: judgeScore,
			ScoresJSON:    MarshalScores(Scores{LLMAsJudgeScoresKey: judgeScore}),
			HumanPass:     &humanPass,
			CreatedAt:     "2026-08-08T00:00:00Z",
		},
		Input:          "q-" + id,
		ExpectedOutput: "exp-" + id,
	}
}

func TestGetJudgeDivergence(t *testing.T) {
	t.Run("empty dataset id rejected", func(t *testing.T) {
		uc := NewUsecase(&mockRepo{}, loggateway.NewNoop())
		if _, err := uc.GetJudgeDivergence(context.Background(), "  ", "", 0, 0); err == nil {
			t.Fatal("expected error for blank dataset id")
		}
	})

	t.Run("repo error propagates", func(t *testing.T) {
		repo := &mockRepo{judgeAnnotatedErr: errors.New("db down")}
		uc := NewUsecase(repo, loggateway.NewNoop())
		if _, err := uc.GetJudgeDivergence(context.Background(), "ds-1", "", 0, 0); err == nil {
			t.Fatal("expected repo error to propagate")
		}
	})

	t.Run("results without judge score are excluded", func(t *testing.T) {
		humanPass := true
		repo := &mockRepo{judgeAnnotated: []JudgeAnnotatedResult{
			// legacy-path row: column default 0, no llm_as_judge key — must not
			// be counted as a judge/human disagreement.
			{CaseResult: CaseResult{ID: "legacy", HumanPass: &humanPass, ScoresJSON: "{}"}},
			judgedResult("judged", 0.9, true),
		}}
		uc := NewUsecase(repo, loggateway.NewNoop())
		out, err := uc.GetJudgeDivergence(context.Background(), "ds-1", "", 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.AnnotatedTotal != 1 {
			t.Fatalf("expected only the judged row counted, got total=%d", out.AnnotatedTotal)
		}
		if out.AgreeCount != 1 || out.DivergeCount != 0 {
			t.Fatalf("expected 1 agree 0 diverge, got %d/%d", out.AgreeCount, out.DivergeCount)
		}
	})

	t.Run("agreement matrix and rate", func(t *testing.T) {
		repo := &mockRepo{judgeAnnotated: []JudgeAnnotatedResult{
			judgedResult("agree-pass", 0.9, true),   // agree
			judgedResult("agree-fail", 0.1, false),  // agree
			judgedResult("lenient", 0.8, false),     // false_pass: judge pass, human fail
			judgedResult("strict", 0.2, true),       // false_fail: judge fail, human pass
			judgedResult("edge-exact", 0.5, true),   // score == threshold counts as pass → agree
		}}
		uc := NewUsecase(repo, loggateway.NewNoop())
		out, err := uc.GetJudgeDivergence(context.Background(), "ds-1", "", 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Threshold != DefaultJudgePassThreshold {
			t.Fatalf("expected default threshold %v, got %v", DefaultJudgePassThreshold, out.Threshold)
		}
		if out.AnnotatedTotal != 5 {
			t.Fatalf("total=%d, want 5", out.AnnotatedTotal)
		}
		if out.AgreeCount != 3 || out.DivergeCount != 2 {
			t.Fatalf("agree/diverge=%d/%d, want 3/2", out.AgreeCount, out.DivergeCount)
		}
		if out.FalsePassCount != 1 || out.FalseFailCount != 1 {
			t.Fatalf("false_pass/false_fail=%d/%d, want 1/1", out.FalsePassCount, out.FalseFailCount)
		}
		if !approxEq(out.AgreementRate, 0.6) {
			t.Fatalf("agreement rate=%v, want 0.6", out.AgreementRate)
		}
		if len(out.Cases) != 2 {
			t.Fatalf("expected 2 divergent cases, got %d", len(out.Cases))
		}
		byID := map[string]JudgeDivergenceCase{}
		for _, c := range out.Cases {
			byID[c.ResultID] = c
		}
		if c := byID["lenient"]; c.Kind != DivergenceFalsePass || !c.HumanPass && c.LLMJudgeScore < out.Threshold {
			t.Fatalf("lenient case wrong: %+v", c)
		}
		if c := byID["strict"]; c.Kind != DivergenceFalseFail {
			t.Fatalf("strict case wrong: %+v", c)
		}
		if c := byID["lenient"]; c.Input != "q-lenient" || c.ExpectedOutput != "exp-lenient" {
			t.Fatalf("case text must be carried for display: %+v", c)
		}
	})

	t.Run("custom threshold honored", func(t *testing.T) {
		repo := &mockRepo{judgeAnnotated: []JudgeAnnotatedResult{
			judgedResult("mid", 0.6, false), // pass at 0.5, fail at 0.7
		}}
		uc := NewUsecase(repo, loggateway.NewNoop())
		out, err := uc.GetJudgeDivergence(context.Background(), "ds-1", "", 0.7, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Threshold != 0.7 {
			t.Fatalf("threshold=%v, want 0.7", out.Threshold)
		}
		// judge 0.6 < 0.7 → fail; human fail → agree.
		if out.AgreeCount != 1 || out.DivergeCount != 0 {
			t.Fatalf("agree/diverge=%d/%d, want 1/0", out.AgreeCount, out.DivergeCount)
		}
	})

	t.Run("limit caps case list but not the full count", func(t *testing.T) {
		repo := &mockRepo{judgeAnnotated: []JudgeAnnotatedResult{
			judgedResult("d1", 0.9, false),
			judgedResult("d2", 0.8, false),
			judgedResult("d3", 0.7, false),
		}}
		uc := NewUsecase(repo, loggateway.NewNoop())
		out, err := uc.GetJudgeDivergence(context.Background(), "ds-1", "", 0, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.DivergeCount != 3 {
			t.Fatalf("diverge count must cover the full set, got %d", out.DivergeCount)
		}
		if len(out.Cases) != 2 {
			t.Fatalf("case list must be capped at limit=2, got %d", len(out.Cases))
		}
	})

	t.Run("no annotated rows yields zeroed summary", func(t *testing.T) {
		uc := NewUsecase(&mockRepo{}, loggateway.NewNoop())
		out, err := uc.GetJudgeDivergence(context.Background(), "ds-1", "", 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.AnnotatedTotal != 0 || out.AgreementRate != 0 || out.DivergeCount != 0 {
			t.Fatalf("expected zeroed summary, got %+v", out)
		}
		if out.Cases == nil {
			t.Fatal("cases must be non-nil for clean JSON serialization")
		}
	})
}
