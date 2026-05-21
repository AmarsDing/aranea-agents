package evaluation

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

// metricSet holds which metrics to compute for a run.
type metricSet map[string]bool

// allMetrics returns the default metric set (four built-in metrics).
func allMetrics() metricSet {
	return parseMetrics("")
}

// ExtendedMetricNames lists opt-in framework metrics beyond the default four.
func ExtendedMetricNames() []string {
	return []string{MetricJSONMatch, MetricXMLMatch, MetricRougeL, MetricToolTrajectory}
}
// An empty string means all four built-in metrics.
func parseMetrics(raw string) metricSet {
	all := metricSet{
		MetricExactMatch:       true,
		MetricContainsMatch:    true,
		MetricLLMAsJudge:       true,
		MetricToolCallAccuracy: true,
	}
	if strings.TrimSpace(raw) == "" {
		return all
	}
	result := make(metricSet)
	for _, m := range strings.Split(raw, ",") {
		key := strings.TrimSpace(m)
		if key != "" {
			result[key] = true
		}
	}
	return result
}

// legacyCaseScores holds per-case metric outcomes for the legacy runner path.
type legacyCaseScores struct {
	ExactMatch       bool
	ContainsMatch    bool
	LLMJudgeScore    float32
	LLMJudgeScored   bool
	ToolCallAccuracy float32
}

// scoreLegacyCase computes selected metrics for one case (legacy runner, no framework).
func scoreLegacyCase(
	ctx context.Context,
	c biz.EvalCase,
	actual string,
	want metricSet,
	llmJudge LLMJudge,
) legacyCaseScores {
	var out legacyCaseScores
	if want[MetricExactMatch] {
		out.ExactMatch = strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(c.ExpectedOutput))
	}
	if want[MetricContainsMatch] {
		out.ContainsMatch = strings.Contains(strings.ToLower(actual), strings.ToLower(c.ExpectedOutput))
	}
	if want[MetricLLMAsJudge] && llmJudge != nil {
		score, err := llmJudge(ctx, c.Input, c.ExpectedOutput, actual)
		if err == nil {
			out.LLMJudgeScore = score
			out.LLMJudgeScored = true
		}
	}
	if want[MetricToolCallAccuracy] {
		out.ToolCallAccuracy = scoreToolCallAccuracy(c.MetadataJSON, actual)
	}
	return out
}

// legacyAgg accumulates run-level metric averages for the legacy path.
type legacyAgg struct {
	exact    aggBucket
	contains aggBucket
	llm      aggBucket
	tool     aggBucket
}

type aggBucket struct {
	sum   float32
	count float32
}

func (a *legacyAgg) add(sc legacyCaseScores, want metricSet) {
	if want[MetricExactMatch] {
		a.exact.sum += boolToFloat(sc.ExactMatch)
		a.exact.count++
	}
	if want[MetricContainsMatch] {
		a.contains.sum += boolToFloat(sc.ContainsMatch)
		a.contains.count++
	}
	if want[MetricLLMAsJudge] && sc.LLMJudgeScored {
		a.llm.sum += sc.LLMJudgeScore
		a.llm.count++
	}
	if want[MetricToolCallAccuracy] {
		a.tool.sum += sc.ToolCallAccuracy
		a.tool.count++
	}
}

func (a *legacyAgg) finalize(run *biz.EvalRun) {
	if a.exact.count > 0 {
		run.ExactMatchScore = a.exact.sum / a.exact.count
	}
	if a.contains.count > 0 {
		run.ContainsMatchScore = a.contains.sum / a.contains.count
	}
	if a.llm.count > 0 {
		run.LLMJudgeScore = a.llm.sum / a.llm.count
	}
	if a.tool.count > 0 {
		run.ToolCallAccuracy = a.tool.sum / a.tool.count
	}
}

// scoreToolCallAccuracy checks if expected tools (metadata_json.expected_tools) appear in output.
func scoreToolCallAccuracy(metadataJSON, actual string) float32 {
	if metadataJSON == "" {
		return 0
	}
	var meta struct {
		ExpectedTools []string `json:"expected_tools"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil || len(meta.ExpectedTools) == 0 {
		return 0
	}
	hit := 0
	for _, tool := range meta.ExpectedTools {
		if strings.Contains(actual, tool) {
			hit++
		}
	}
	return float32(hit) / float32(len(meta.ExpectedTools))
}

func boolToFloat(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
