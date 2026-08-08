// Package evaluation provides an async evaluation runner with 4 built-in metrics.
package evaluation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
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

// Runner executes an evaluation run asynchronously.
type Runner struct {
	uc          *biz.EvalUsecase
	agent       AgentRunner
	framework   *FrameworkBridge
	monitorBus  contract.MonitorBus
	dropAlerter *ScoreDropAlerter
	lg          loggateway.Logger
}

func NewRunner(uc *biz.EvalUsecase, agent AgentRunner, framework *FrameworkBridge, lg loggateway.Logger) *Runner {
	return &Runner{uc: uc, agent: agent, framework: framework, lg: lg}
}

// WithMonitorBus wires the typed monitor bus for flow-log emission (chainable).
// When nil, flow-log emission is skipped.
func (r *Runner) WithMonitorBus(bus contract.MonitorBus) *Runner {
	if r != nil && bus != nil {
		r.monitorBus = bus
	}
	return r
}

// WithDropAlerter wires the online score-drop alerter (chainable, P2-2).
// When nil, drop detection is skipped.
func (r *Runner) WithDropAlerter(a *ScoreDropAlerter) *Runner {
	if r != nil && a != nil {
		r.dropAlerter = a
	}
	return r
}

// flowEmitter builds a run-scoped flow-log emitter for evaluation lifecycle events.
// Returns nil when the monitor bus is not wired (emission skipped).
func (r *Runner) flowEmitter(ctx context.Context, runID string) *event.TraceEmitter {
	if r == nil || r.monitorBus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		RunID:  runID,
		Domain: event.TraceDomainSystem,
		LG:     r.lg,
		Infra:  event.NewInfraFromBus(r.monitorBus),
	})
}

func (r *Runner) Start(ctx context.Context, run biz.EvalRun, metrics string, numRuns int, useUserSimulation bool) {
	r.lg.Info("evaluation run started",
		loggateway.StepID("evaluation.run.start"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("dataset_id", run.DatasetID),
	)
	if flow := r.flowEmitter(ctx, run.ID); flow != nil {
		flow.LogStart("evaluation.run", "评测集运行",
			event.P("run_id", run.ID),
			event.P("dataset_id", run.DatasetID),
			event.P("agent_id", run.AgentID),
			event.P("num_runs", numRuns))
	}
	evalRunsTotal.WithLabelValues("started").Inc()
	// ISSUE-002: detach the async execution from the HTTP request ctx — Kratos
	// cancels it as soon as the handler returns, which would otherwise abort
	// all DB writes and leave the run stuck in "pending" forever. Values
	// (trace IDs etc.) are preserved; cancellation follows appctx only.
	execCtx := context.WithoutCancel(ctx)
	safego.Go(appctx.Ctx(), "eval-runner", func() {
		if err := r.execute(execCtx, run, metrics, numRuns, useUserSimulation); err != nil {
			evalRunsTotal.WithLabelValues("error").Inc()
		} else {
			evalRunsTotal.WithLabelValues("completed").Inc()
		}
	})
}

// RunSync executes a run inline and returns the final persisted state (P2-1
// publish gate). Unlike Start it does not fork a goroutine — the caller
// awaits the outcome. execute() persists the failed status before returning
// an error, so on error the final row is still read back for the message.
func (r *Runner) RunSync(ctx context.Context, run biz.EvalRun, metrics string, numRuns int) (biz.EvalRun, error) {
	if numRuns <= 0 {
		numRuns = 1
	}
	err := r.execute(ctx, run, metrics, numRuns, false)
	final, gerr := r.uc.GetRun(ctx, run.ID)
	if gerr != nil {
		return run, err
	}
	return final, err
}

func (r *Runner) execute(ctx context.Context, run biz.EvalRun, metrics string, numRuns int, useUserSimulation bool) error {
	cases, err := r.uc.ListCases(ctx, run.DatasetID)
	if err != nil {
		r.lg.Error("evaluation load cases failed",
			loggateway.StepID("evaluation.run.load_cases_fail"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("dataset_id", run.DatasetID),
			loggateway.Err(err))
		return r.failRun(ctx, run, "failed to load cases: "+err.Error())
	}

	ds, dsErr := r.uc.GetDataset(ctx, run.DatasetID)
	if dsErr != nil {
		r.lg.Error("evaluation load dataset failed",
			loggateway.StepID("evaluation.run.load_dataset_fail"),
			loggateway.Str("run_id", run.ID),
			loggateway.Str("dataset_id", run.DatasetID),
			loggateway.Err(dsErr))
		return r.failRun(ctx, run, "failed to load dataset: "+dsErr.Error())
	}

	run.TotalCases = len(cases)
	run.Status = "running"
	run.StartedAt = time.Now().UTC().Format(time.RFC3339)
	// P3-5: snapshot the dataset content hash so trend/compare can warn when
	// the dataset changed between runs (scores not directly comparable).
	run.DatasetHash = hashEvalCases(cases)
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
	caseErrs := make([]string, 0)
	for i := range results {
		results[i].RunID = run.ID
		if msg := strings.TrimSpace(results[i].ErrorMessage); msg != "" {
			caseErrs = append(caseErrs, fmt.Sprintf("case %s: %s", results[i].CaseID, msg))
		}
		if err := r.uc.InsertCaseResult(ctx, results[i]); err != nil {
			r.lg.Warn("failed to insert evaluation case result", loggateway.Err(err), loggateway.Str("run_id", run.ID))
			// Persistence failure means the case row is silently missing from
			// the database — count it as a case error so the run fails instead
			// of reporting "completed" with dropped results.
			caseErrs = append(caseErrs, fmt.Sprintf("case %s: persist result: %v", results[i].CaseID, err))
		} else {
			// Only count cases whose results were actually persisted; otherwise
			// CompletedCases would diverge from the persisted CaseResult rows
			// and the run would report "completed" with an inflated count.
			run.CompletedCases++
		}
		if err := r.uc.UpdateRun(ctx, run); err != nil {
			r.lg.Warn("failed to update evaluation run", loggateway.Err(err), loggateway.Str("run_id", run.ID))
		}
	}
	run.ScoresJSON = normalizeScoresJSON(run.ScoresJSON)
	for name, avg := range scores {
		mergeRunScores(&run, name, avg)
	}
	run.PassAtK = passAtK
	run.PassHatK = passHatK
	// P3-4: red-team attack success rate — computed only when the dataset
	// carries adversarial cases (metadata_json.redteam_category).
	mergeAttackSuccessRate(&run, cases, results)
	// ISSUE-006: partial case failure must fail the run — the framework returns
	// no global error when individual cases fail, so a silent "completed" would
	// report a broken evaluation as healthy.
	if len(results) < len(cases) {
		caseErrs = append(caseErrs, fmt.Sprintf("%d case(s) missing from framework result", len(cases)-len(results)))
	}
	if len(caseErrs) > 0 {
		return r.failRun(ctx, run, fmt.Sprintf("%d case(s) failed: %s", len(caseErrs), strings.Join(caseErrs, "; ")))
	}
	run.Status = "completed"
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if flow := r.flowEmitter(ctx, run.ID); flow != nil {
		flow.LogDone("evaluation.run", "评测集运行完成",
			event.P("run_id", run.ID),
			event.P("dataset_id", run.DatasetID),
			event.P("total_cases", run.TotalCases),
			event.P("completed_cases", run.CompletedCases),
			event.P("avg_score", meanScore(scores)),
			event.P("pass_at_k", passAtK),
			event.P("pass_hat_k", passHatK))
	}
	if err := r.uc.UpdateRun(ctx, run); err != nil {
		return err
	}
	// P2-2: online quality watch — check the consecutive-drop condition after
	// the run is durably completed so the trend query sees this run's score.
	if r.dropAlerter != nil {
		r.dropAlerter.CheckAfterRun(ctx, run)
	}
	return nil
}

func (r *Runner) failRun(ctx context.Context, run biz.EvalRun, msg string) error {
	r.lg.Warn("evaluation run failed",
		loggateway.StepID("evaluation.run.fail"),
		loggateway.Str("run_id", run.ID),
		loggateway.Str("error", msg),
	)
	if flow := r.flowEmitter(ctx, run.ID); flow != nil {
		flow.LogError("evaluation.run", "评测集运行失败",
			event.P("run_id", run.ID),
			event.P("dataset_id", run.DatasetID),
			event.P("error", msg))
	}
	run.Status = "failed"
	run.ErrorMessage = msg
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return r.uc.UpdateRun(ctx, run)
}

// meanScore averages per-metric mean scores into a single run-level mean.
func meanScore(scores map[string]float32) float32 {
	if len(scores) == 0 {
		return 0
	}
	var sum float32
	for _, v := range scores {
		sum += v
	}
	return sum / float32(len(scores))
}

func newEvalResultID() string {
	// Time prefix for sortability + random suffix for uniqueness: a pure
	// nanosecond timestamp collides within one clock tick (Windows timer
	// granularity ~0.5ms), which dropped case rows on eval_case_results_pkey.
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is vanishingly rare; fall back to a counter-free
		// best effort that still differs from a pure timestamp across calls.
		return fmt.Sprintf("er-%s-%d", time.Now().UTC().Format("20060102150405.000000000"), time.Now().UnixNano()%0xffffff)
	}
	return "er-" + time.Now().UTC().Format("20060102150405.000000000") + "-" + hex.EncodeToString(b[:])
}
