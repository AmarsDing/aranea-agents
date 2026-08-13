package evaluation

import (
	"strings"

	"aranea-agents/internal/biz"
)

// applyMetricResult maps framework metric results onto biz case result fields + scores JSON.
func applyMetricResult(res *biz.EvalCaseResult, name string, score float32, threshold float64) {
	scores := biz.ParseEvalScores(res.ScoresJSON)
	scores[name] = score
	res.ScoresJSON = biz.MarshalEvalScores(scores)

	switch name {
	case MetricExactMatch:
		res.ExactMatch = score >= float32(threshold)
	case MetricContainsMatch:
		res.ContainsMatch = score >= float32(threshold)
	case MetricToolCallAccuracy:
		res.ToolCallAccuracy = score
	case MetricLLMAsJudge:
		res.LLMJudgeScore = score
	case MetricToolTrajectory:
		// Full trajectory score lives in scores_json; mirror into the legacy
		// ToolCallAccuracy column only when tool_call_accuracy was not scored
		// for this case. The old `== 0` guard conflated "unset" with a legit
		// 0.0 score and let slice order decide the column (Y7): whichever of
		// the two metrics was applied last won. Presence in the scores map is
		// the authoritative "scored" signal and is order-independent —
		// tool_call_accuracy always wins the column when both are requested.
		if _, has := scores[MetricToolCallAccuracy]; !has {
			res.ToolCallAccuracy = score
		}
	}
}

func mergeRunScores(run *biz.EvalRun, name string, avg float32) {
	scores := biz.ParseEvalScores(run.ScoresJSON)
	scores[name] = avg
	run.ScoresJSON = biz.MarshalEvalScores(scores)
	switch name {
	case MetricExactMatch:
		run.ExactMatchScore = avg
	case MetricContainsMatch:
		run.ContainsMatchScore = avg
	case MetricLLMAsJudge:
		run.LLMJudgeScore = avg
	case MetricToolCallAccuracy:
		run.ToolCallAccuracy = avg
	case MetricToolTrajectory:
		// Same Y7 priority rule as applyMetricResult: the legacy column
		// belongs to tool_call_accuracy; tool_trajectory only mirrors when
		// accuracy is absent from this run's score set. The caller merges in
		// sorted name order ("tool_call_accuracy" < "tool_trajectory"), so
		// the accuracy entry is already visible here when both were scored.
		if _, has := scores[MetricToolCallAccuracy]; !has {
			run.ToolCallAccuracy = avg
		}
	}
}

func normalizeScoresJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	return raw
}
