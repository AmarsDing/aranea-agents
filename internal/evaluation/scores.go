package evaluation

import (
	"encoding/json"
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
	case MetricToolTrajectory:
		// Full trajectory score stored in scores_json; mirror into ToolCallAccuracy when legacy unset.
		if res.ToolCallAccuracy == 0 {
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
	case MetricToolCallAccuracy, MetricToolTrajectory:
		run.ToolCallAccuracy = avg
	}
}

// scoresFromJSON extracts a float map for analytics export.
func scoresFromJSON(raw string) map[string]float32 {
	return biz.ParseEvalScores(raw)
}

func marshalScoresMap(m map[string]float32) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func normalizeScoresJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	return raw
}
