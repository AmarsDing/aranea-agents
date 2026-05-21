package evaluation

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestApplyMetricResultExtended(t *testing.T) {
	res := biz.EvalCaseResult{ScoresJSON: "{}"}
	applyMetricResult(&res, MetricRougeL, 0.72, 0.5)
	scores := biz.ParseEvalScores(res.ScoresJSON)
	if scores[MetricRougeL] != 0.72 {
		t.Fatalf("rouge_l score: %v", scores[MetricRougeL])
	}
}

func TestMergeRunScores(t *testing.T) {
	run := biz.EvalRun{ScoresJSON: "{}"}
	mergeRunScores(&run, MetricJSONMatch, 0.9)
	if run.ScoresJSON == "{}" {
		t.Fatal("expected scores_json populated")
	}
	scores := biz.ParseEvalScores(run.ScoresJSON)
	if scores[MetricJSONMatch] != 0.9 {
		t.Fatalf("json_match: %v", scores[MetricJSONMatch])
	}
}
