// Package evaluation provides an async evaluation runner with 4 built-in metrics.
package evaluation

import (
	"context"
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
	framework *FrameworkBridge
}

// NewRunner creates an evaluation Runner.
func NewRunner(uc *biz.EvalUsecase, agent AgentRunner, judge LLMJudge, framework *FrameworkBridge) *Runner {
	return &Runner{uc: uc, agent: agent, llmJudge: judge, framework: framework}
}

// Start launches an async goroutine to execute the run and immediately returns.
func (r *Runner) Start(ctx context.Context, run biz.EvalRun, metrics string, numRuns int, useUserSimulation bool) {
	evalRunsTotal.WithLabelValues("started").Inc()
	safego.Go(context.Background(), "eval-runner", func() {
		if err := r.execute(ctx, run, metrics, numRuns, useUserSimulation); err != nil {
			evalRunsTotal.WithLabelValues("error").Inc()
		} else {
			evalRunsTotal.WithLabelValues("completed").Inc()
		}
	})
}

func (r *Runner) execute(ctx context.Context, run biz.EvalRun, metrics string, numRuns int, useUserSimulation bool) error {
	cases, err := r.uc.ListCases(ctx, run.DatasetID)
	if err != nil {
		return r.failRun(ctx, run, "failed to load cases: "+err.Error())
	}

	ds, dsErr := r.uc.GetDataset(ctx, run.DatasetID)
	if dsErr != nil {
		return r.failRun(ctx, run, "failed to load dataset: "+dsErr.Error())
	}

	run.TotalCases = len(cases)
	run.Status = "running"
	run.StartedAt = time.Now().UTC().Format(time.RFC3339)
	if err := r.uc.UpdateRun(ctx, run); err != nil {
		return err
	}

	wantMetrics := parseMetrics(metrics)

	if r.framework != nil && r.agent != nil {
		return r.executeFramework(ctx, run, ds, cases, wantMetrics, numRuns, useUserSimulation)
	}

	return r.executeLegacy(ctx, run, cases, wantMetrics)
}

func (r *Runner) executeFramework(
	ctx context.Context,
	run biz.EvalRun,
	ds biz.EvalDataset,
	cases []biz.EvalCase,
	wantMetrics map[string]bool,
	numRuns int,
	useUserSimulation bool,
) error {
	results, scores, passAtK, passHatK, err := r.framework.Execute(ctx, ds, cases, RunConfig{
		AgentID:           run.AgentID,
		NumRuns:           numRuns,
		Metrics:           wantMetrics,
		UseUserSimulation: useUserSimulation,
	})
	if err != nil {
		return r.failRun(ctx, run, err.Error())
	}
	for i := range results {
		results[i].RunID = run.ID
		_ = r.uc.InsertCaseResult(ctx, results[i])
		run.CompletedCases++
		_ = r.uc.UpdateRun(ctx, run)
	}
	run.ScoresJSON = normalizeScoresJSON(run.ScoresJSON)
	for name, avg := range scores {
		mergeRunScores(&run, name, avg)
	}
	run.PassAtK = passAtK
	run.PassHatK = passHatK
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

func newEvalResultID() string {
	// reuse the biz ID generator via a simple time+rand approach
	return "er-" + time.Now().UTC().Format("20060102150405.000000000")
}
