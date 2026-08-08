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
