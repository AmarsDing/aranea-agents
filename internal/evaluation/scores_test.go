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

// Y7: when both tool_call_accuracy and tool_trajectory are scored, the legacy
// ToolCallAccuracy column must deterministically hold tool_call_accuracy —
// regardless of the order the two metrics are merged in.
func TestY7RunToolColumnDeterministic(t *testing.T) {
	for _, order := range [][2]string{
		{MetricToolCallAccuracy, MetricToolTrajectory},
		{MetricToolTrajectory, MetricToolCallAccuracy},
	} {
		run := biz.EvalRun{ScoresJSON: "{}"}
		avgs := map[string]float32{MetricToolCallAccuracy: 0.25, MetricToolTrajectory: 0.75}
		mergeRunScores(&run, order[0], avgs[order[0]])
		mergeRunScores(&run, order[1], avgs[order[1]])
		if run.ToolCallAccuracy != 0.25 {
			t.Fatalf("order %v: column = %v, want tool_call_accuracy 0.25", order, run.ToolCallAccuracy)
		}
		scores := biz.ParseEvalScores(run.ScoresJSON)
		if scores[MetricToolTrajectory] != 0.75 {
			t.Fatalf("order %v: scores_json lost tool_trajectory: %v", order, scores)
		}
	}
}

// Y7: tool_trajectory alone still mirrors into the legacy column.
func TestY7RunToolTrajectoryMirrorWhenAccuracyAbsent(t *testing.T) {
	run := biz.EvalRun{ScoresJSON: "{}"}
	mergeRunScores(&run, MetricToolTrajectory, 0.6)
	if run.ToolCallAccuracy != 0.6 {
		t.Fatalf("mirror: %v, want 0.6", run.ToolCallAccuracy)
	}
}

// Y7 (case level): a legit 0.0 tool_call_accuracy score must not be
// overwritten by the trajectory mirror, whichever metric is applied last.
func TestY7CaseToolColumnZeroScoreNotOverwritten(t *testing.T) {
	for _, order := range [][2]string{
		{MetricToolCallAccuracy, MetricToolTrajectory},
		{MetricToolTrajectory, MetricToolCallAccuracy},
	} {
		res := biz.EvalCaseResult{ScoresJSON: "{}"}
		vals := map[string]float32{MetricToolCallAccuracy: 0, MetricToolTrajectory: 0.9}
		applyMetricResult(&res, order[0], vals[order[0]], 1.0)
		applyMetricResult(&res, order[1], vals[order[1]], 1.0)
		if res.ToolCallAccuracy != 0 {
			t.Fatalf("order %v: column = %v, want 0 (accuracy wins)", order, res.ToolCallAccuracy)
		}
	}
}
