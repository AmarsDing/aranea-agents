package evaluation

import (
	"strings"
	"testing"
)

func TestParseMetrics(t *testing.T) {
	t.Parallel()
	all := parseMetrics("")
	if len(all) != 4 {
		t.Fatalf("expected 4 metrics, got %d", len(all))
	}
	subset := parseMetrics("exact_match, llm_as_judge")
	if !subset[MetricExactMatch] || !subset[MetricLLMAsJudge] || subset[MetricContainsMatch] {
		t.Fatalf("unexpected subset: %+v", subset)
	}
}

// P3-2 runtime fix: llm_as_judge must resolve to the framework's
// llm_rubric_response evaluator — llm_final_response's prompt has no rubric
// slot, so case-level scoring standards never reached the judge.
func TestLLMAsJudgeSpecUsesRubricResponseEvaluator(t *testing.T) {
	t.Parallel()
	specs := buildMetricSpecs(metricSet{MetricLLMAsJudge: true})
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].evaluatorName != "llm_rubric_response" {
		t.Fatalf("llm_as_judge evaluator = %q, want llm_rubric_response", specs[0].evaluatorName)
	}
}

// The judge runner's system instruction must not impose its own output format:
// the framework evaluator prompts each define their required format, and a
// conflicting instruction causes intermittent judge parse failures.
func TestJudgeInstructionDoesNotImposeOutputFormat(t *testing.T) {
	t.Parallel()
	if strings.Contains(judgeSystemInstruction, `"score"`) || strings.Contains(judgeSystemInstruction, "JSON object") {
		t.Fatalf("judge instruction must not force a JSON {score} format: %q", judgeSystemInstruction)
	}
}

func TestScoreToolCallAccuracy(t *testing.T) {
	t.Parallel()
	if got := scoreToolCallAccuracy("", "output"); got != 0 {
		t.Fatalf("empty meta: %v", got)
	}
	meta := `{"expected_tools":["get_weather","search"]}`
	got := scoreToolCallAccuracy(meta, "called get_weather tool")
	if got != 0.5 {
		t.Fatalf("expected 0.5, got %v", got)
	}
}

func TestBoolToFloat(t *testing.T) {
	t.Parallel()
	if boolToFloat(true) != 1 || boolToFloat(false) != 0 {
		t.Fatal("boolToFloat mismatch")
	}
}
