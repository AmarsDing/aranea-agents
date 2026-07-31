package evaluation

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func (r *Runner) executeLegacy(ctx context.Context, run biz.EvalRun, cases []biz.EvalCase, want metricSet) error {
	if r.agent == nil {
		return r.failRun(ctx, run, "agent runner not configured; inject an AgentRunner via evaluation.NewRunner")
	}

	var agg legacyAgg
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
			sc := scoreLegacyCase(ctx, c, actual, want)
			res.ExactMatch = sc.ExactMatch
			res.ContainsMatch = sc.ContainsMatch
			res.ToolCallAccuracy = sc.ToolCallAccuracy
			agg.add(sc, want)
		}

		if err := r.uc.InsertCaseResult(ctx, res); err != nil {
			r.lg.Warn("failed to insert evaluation case result", loggateway.Err(err), loggateway.Str("run_id", run.ID))
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

	agg.finalize(&run)
	run.Status = "completed"
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if flow := r.flowEmitter(ctx, run.ID); flow != nil {
		flow.LogDone("evaluation.run", "评测集运行完成",
			event.P("run_id", run.ID),
			event.P("dataset_id", run.DatasetID),
			event.P("total_cases", run.TotalCases),
			event.P("completed_cases", run.CompletedCases),
			event.P("avg_score", agg.avg()))
	}
	return r.uc.UpdateRun(ctx, run)
}
