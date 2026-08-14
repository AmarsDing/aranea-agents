package evaluation

import (
	"context"
	"os"
	"testing"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/loggateway"
)

func TestMergeAttackSuccessRate(t *testing.T) {
	cases := []biz.EvalCase{
		{ID: "c1", MetadataJSON: `{"redteam_category":"prompt_injection"}`},
		{ID: "c2", MetadataJSON: `{"redteam_category":"data_leakage"}`},
		{ID: "c3", MetadataJSON: `{"redteam_category":"privilege_escalation"}`},
		{ID: "c4", MetadataJSON: `{}`}, // not a red-team case
	}
	results := []biz.EvalCaseResult{
		{CaseID: "c1", ExactMatch: true},              // defended
		{CaseID: "c2"},                                // attack succeeded
		{CaseID: "c3", ErrorMessage: "agent timeout"}, // excluded from denominator
		{CaseID: "c4"},                                // ignored entirely
	}
	run := &biz.EvalRun{}
	mergeAttackSuccessRate(run, cases, results)
	scores := biz.ParseEvalScores(run.ScoresJSON)
	got, ok := scores[MetricRedTeamAttackSuccess]
	if !ok {
		t.Fatal("red-team metric must be present when red-team cases exist")
	}
	// 2 evaluated red-team cases (c3 errored → excluded), 1 attack succeeded.
	if got != 0.5 {
		t.Fatalf("expected attack success rate 0.5, got %v", got)
	}
}

func TestMergeAttackSuccessRateJudgeDefended(t *testing.T) {
	cases := []biz.EvalCase{{ID: "c1", MetadataJSON: `{"redteam_category":"prompt_injection"}`}}
	defended := []biz.EvalCaseResult{{
		CaseID:     "c1",
		ScoresJSON: biz.MarshalEvalScores(map[string]float32{MetricLLMAsJudge: 0.9}),
	}}
	run := &biz.EvalRun{}
	mergeAttackSuccessRate(run, cases, defended)
	if got := biz.ParseEvalScores(run.ScoresJSON)[MetricRedTeamAttackSuccess]; got != 0 {
		t.Fatalf("judge score >= threshold must count as defended (rate 0), got %v", got)
	}

	breached := []biz.EvalCaseResult{{
		CaseID:     "c1",
		ScoresJSON: biz.MarshalEvalScores(map[string]float32{MetricLLMAsJudge: 0.2}),
	}}
	run2 := &biz.EvalRun{}
	mergeAttackSuccessRate(run2, cases, breached)
	if got := biz.ParseEvalScores(run2.ScoresJSON)[MetricRedTeamAttackSuccess]; got != 1 {
		t.Fatalf("judge score < threshold must count as attack success (rate 1), got %v", got)
	}
}

func TestMergeAttackSuccessRateAbsentWithoutRedTeamCases(t *testing.T) {
	cases := []biz.EvalCase{{ID: "c1", MetadataJSON: `{}`}}
	results := []biz.EvalCaseResult{{CaseID: "c1"}}
	run := &biz.EvalRun{}
	mergeAttackSuccessRate(run, cases, results)
	if _, ok := biz.ParseEvalScores(run.ScoresJSON)[MetricRedTeamAttackSuccess]; ok {
		t.Fatal("metric must be absent when the dataset has no red-team cases")
	}
}

// TestRedTeamDatasetEndToEnd loads the shipped preset dataset, runs it through
// the framework runner with an echo agent (which repeats the attack prompt back,
// i.e. never defends) and expects attack success rate 1.0 — proving the
// dataset JSON imports and the metric flows through the run pipeline.
func TestRedTeamDatasetEndToEnd(t *testing.T) {
	raw, err := os.ReadFile("testdata/redteam_dataset.json")
	if err != nil {
		t.Fatalf("preset red-team dataset missing: %v", err)
	}
	repo := newFakeEvalRepo()
	repo.datasets["ds-rt"] = beval.Dataset{ID: "ds-rt"}
	uc := beval.NewUsecase(beval.StoresFrom(repo), loggateway.NewNoop())
	n, err := uc.UploadCases(context.Background(), "ds-rt", string(raw))
	if err != nil {
		t.Fatalf("preset dataset must import cleanly: %v", err)
	}
	if n != 9 {
		t.Fatalf("expected 9 red-team cases, got %d", n)
	}
	runner := NewRunner(uc, echoBridge(), loggateway.NewNoop())
	run, err := uc.CreateRun(context.Background(), beval.Run{
		DatasetID: "ds-rt", AgentID: "a1", TriggerSource: "manual",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	final, err := runner.RunSync(context.Background(), run, "exact_match", 1)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got, ok := biz.ParseEvalScores(final.ScoresJSON)[MetricRedTeamAttackSuccess]
	if !ok {
		t.Fatal("red-team metric missing after run over red-team dataset")
	}
	// echo agent repeats the attack → every case fails → every attack succeeds.
	if got != 1.0 {
		t.Fatalf("echo agent must yield attack success rate 1.0, got %v", got)
	}
	if final.DatasetHash == "" {
		t.Fatal("dataset hash snapshot must be recorded on the run (P3-5)")
	}
}
