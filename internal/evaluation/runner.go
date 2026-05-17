// Package evaluation provides an async evaluation runner with 4 built-in metrics.
package evaluation

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/safego"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	evalRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_eval_runs_total",
		Help: "Total number of evaluation runs started.",
	}, []string{"status"})

	evalCaseDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_eval_case_duration_seconds",
		Help:    "Time to evaluate one case.",
		Buckets: prometheus.DefBuckets,
	})
)

// AgentRunner is the function signature used to run one evaluation case.
// It returns the agent's output for a given input string.
type AgentRunner func(ctx context.Context, agentID, input string) (string, error)

// LLMJudge is an optional function that scores an answer vs expected on [0, 1].
type LLMJudge func(ctx context.Context, input, expected, actual string) (float32, error)

// Runner executes an evaluation run asynchronously.
type Runner struct {
	uc        *biz.EvalUsecase
	agent     AgentRunner
	llmJudge  LLMJudge
}

// NewRunner creates an evaluation Runner.
func NewRunner(uc *biz.EvalUsecase, agent AgentRunner, judge LLMJudge) *Runner {
	return &Runner{uc: uc, agent: agent, llmJudge: judge}
}

// Start launches an async goroutine to execute the run and immediately returns.
// Progress is persisted via the biz.EvalRepo.
func (r *Runner) Start(ctx context.Context, run biz.EvalRun, metrics string) {
	evalRunsTotal.WithLabelValues("started").Inc()
	safego.Go(context.Background(), "eval-runner", func() {
		if err := r.execute(ctx, run, metrics); err != nil {
			evalRunsTotal.WithLabelValues("error").Inc()
		} else {
			evalRunsTotal.WithLabelValues("completed").Inc()
		}
	})
}

func (r *Runner) execute(ctx context.Context, run biz.EvalRun, metrics string) error {
	cases, err := r.uc.ListCases(ctx, run.DatasetID)
	if err != nil {
		return r.failRun(ctx, run, "failed to load cases: "+err.Error())
	}

	run.TotalCases = len(cases)
	run.Status = "running"
	run.StartedAt = time.Now().UTC().Format(time.RFC3339)
	if err := r.uc.UpdateRun(ctx, run); err != nil {
		return err
	}

	wantMetrics := parseMetrics(metrics)
	var (
		exactTotal, exactHit     float32
		containsTotal, containsHit float32
		llmTotal, llmSum         float32
		toolTotal, toolSum       float32
	)

	if r.agent == nil {
		return r.failRun(ctx, run, "agent runner not configured; inject an AgentRunner via evaluation.NewRunner")
	}

	for _, c := range cases {
		start := time.Now()
		actual, runErr := r.agent(ctx, run.AgentID, c.Input)
		evalCaseDuration.Observe(time.Since(start).Seconds())

		res := biz.EvalCaseResult{
			ID:     newEvalResultID(),
			RunID:  run.ID,
			CaseID: c.ID,
		}

		if runErr != nil {
			res.ErrorMessage = runErr.Error()
		} else {
			res.ActualOutput = actual

			if wantMetrics["exact_match"] {
				res.ExactMatch = strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(c.ExpectedOutput))
				exactHit += boolToFloat(res.ExactMatch)
				exactTotal++
			}

			if wantMetrics["contains_match"] {
				res.ContainsMatch = strings.Contains(strings.ToLower(actual), strings.ToLower(c.ExpectedOutput))
				containsHit += boolToFloat(res.ContainsMatch)
				containsTotal++
			}

			if wantMetrics["llm_as_judge"] && r.llmJudge != nil {
				score, err := r.llmJudge(ctx, c.Input, c.ExpectedOutput, actual)
				if err == nil {
					res.LLMJudgeScore = score
					llmSum += score
					llmTotal++
				}
			}

			if wantMetrics["tool_call_accuracy"] {
				res.ToolCallAccuracy = scoreToolCallAccuracy(c.MetadataJSON, actual)
				toolSum += res.ToolCallAccuracy
				toolTotal++
			}
		}

		_ = r.uc.InsertCaseResult(ctx, res)
		run.CompletedCases++
		_ = r.uc.UpdateRun(ctx, run)
	}

	// Aggregate scores.
	if exactTotal > 0 {
		run.ExactMatchScore = exactHit / exactTotal
	}
	if containsTotal > 0 {
		run.ContainsMatchScore = containsHit / containsTotal
	}
	if llmTotal > 0 {
		run.LLMJudgeScore = llmSum / llmTotal
	}
	if toolTotal > 0 {
		run.ToolCallAccuracy = toolSum / toolTotal
	}

	run.Status = "completed"
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return r.uc.UpdateRun(ctx, run)
}

func (r *Runner) failRun(ctx context.Context, run biz.EvalRun, msg string) error {
	run.Status = "failed"
	run.ErrorMessage = msg
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return r.uc.UpdateRun(ctx, run)
}

// parseMetrics converts a comma-separated metric list to a set.
// An empty string means "all metrics".
func parseMetrics(raw string) map[string]bool {
	all := map[string]bool{
		"exact_match":        true,
		"contains_match":     true,
		"llm_as_judge":       true,
		"tool_call_accuracy": true,
	}
	if strings.TrimSpace(raw) == "" {
		return all
	}
	result := make(map[string]bool)
	for _, m := range strings.Split(raw, ",") {
		result[strings.TrimSpace(m)] = true
	}
	return result
}

// scoreToolCallAccuracy checks if the expected tool calls (listed in metadata_json) appear in the output.
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

func newEvalResultID() string {
	// reuse the biz ID generator via a simple time+rand approach
	return "er-" + time.Now().UTC().Format("20060102150405.000000000")
}
