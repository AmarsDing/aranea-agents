package evaluation

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
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
			sc := scoreLegacyCase(ctx, c, actual, want, r.llmJudge)
			res.ExactMatch = sc.ExactMatch
			res.ContainsMatch = sc.ContainsMatch
			res.LLMJudgeScore = sc.LLMJudgeScore
			res.ToolCallAccuracy = sc.ToolCallAccuracy
			agg.add(sc, want)
		}

		_ = r.uc.InsertCaseResult(ctx, res)
		run.CompletedCases++
		_ = r.uc.UpdateRun(ctx, run)
	}

	agg.finalize(&run)
	run.Status = "completed"
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return r.uc.UpdateRun(ctx, run)
}
