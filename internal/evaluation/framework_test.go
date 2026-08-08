package evaluation

import (
	"testing"

	trpceval "trpc.group/trpc-go/trpc-agent-go/evaluation"
	"trpc.group/trpc-go/trpc-agent-go/evaluation/evalresult"
)

// The framework's aggregateCaseRuns averages metric scores across runs into
// EvaluationCaseResult.MetricResults but drops Details, so the judge reason
// only survives on each run's OverallEvalMetricResults. judgeReasonFromRuns
// must read it from there.
func TestJudgeReasonFromRunsReadsPerRunDetails(t *testing.T) {
	t.Parallel()
	cr := &trpceval.EvaluationCaseResult{
		EvalCaseID: "c1",
		// Aggregated metric results carry no Details (framework drops them).
		MetricResults: []*evalresult.EvalMetricResult{
			{MetricName: MetricLLMAsJudge, Score: 1},
		},
		EvalCaseResults: []*evalresult.EvalCaseResult{
			{
				OverallEvalMetricResults: []*evalresult.EvalMetricResult{
					{
						MetricName: MetricLLMAsJudge,
						Score:      1,
						Details:    &evalresult.EvalMetricResultDetails{Reason: "rubric ok"},
					},
				},
			},
		},
	}
	if got := judgeReasonFromRuns(cr); got != "rubric ok" {
		t.Fatalf("judgeReasonFromRuns = %q, want %q", got, "rubric ok")
	}
}

func TestJudgeReasonFromRunsSkipsOtherMetricsAndEmpty(t *testing.T) {
	t.Parallel()
	cr := &trpceval.EvaluationCaseResult{
		EvalCaseResults: []*evalresult.EvalCaseResult{
			nil,
			{
				OverallEvalMetricResults: []*evalresult.EvalMetricResult{
					nil,
					{MetricName: MetricExactMatch, Details: &evalresult.EvalMetricResultDetails{Reason: "not judge"}},
					{MetricName: MetricLLMAsJudge},
					{MetricName: MetricLLMAsJudge, Details: &evalresult.EvalMetricResultDetails{Reason: "  "}},
				},
			},
		},
	}
	if got := judgeReasonFromRuns(cr); got != "" {
		t.Fatalf("judgeReasonFromRuns = %q, want empty", got)
	}
	if got := judgeReasonFromRuns(nil); got != "" {
		t.Fatalf("nil case result must yield empty, got %q", got)
	}
}

// P2-3: the framework wraps case inference errors with per-case unique IDs
// (evalCaseID/sessionID). Persisted error_message must drop that prefix so
// SQL failure grouping (GROUP BY error_message) actually clusters.
func TestNormalizeCaseErrorMessageStripsFrameworkPrefix(t *testing.T) {
	t.Parallel()
	raw := "inference eval case (evalCaseID=6775ca575ddc79922016, sessionID=1fb487d0-6674-48f9-864f-13debca12201): event: [CHAT/INTERNAL] eval: create session failed"
	want := "event: [CHAT/INTERNAL] eval: create session failed"
	if got := normalizeCaseErrorMessage(raw); got != want {
		t.Fatalf("normalizeCaseErrorMessage = %q, want %q", got, want)
	}
}

func TestNormalizeCaseErrorMessageKeepsPlainMessages(t *testing.T) {
	t.Parallel()
	for _, msg := range []string{"", "agent timeout", "inference eval case: unexpected shape"} {
		if got := normalizeCaseErrorMessage(msg); got != msg {
			t.Fatalf("normalizeCaseErrorMessage(%q) = %q, want unchanged", msg, got)
		}
	}
}
