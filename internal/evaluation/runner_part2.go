package evaluation

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

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
	if err := r.uc.UpdateRun(ctx, run); err != nil {

		r.lg.Error("evaluation persist failed state failed",
			loggateway.StepID("evaluation.run.persist_failed_fail"),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
		return err
	}

	r.publishEvalCompleted(ctx, run)
	return fmt.Errorf("%w: %s", errEvalRunFailed, msg)
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

		return fmt.Sprintf("er-%s-%d", time.Now().UTC().Format("20060102150405.000000000"), time.Now().UnixNano()%0xffffff)
	}
	return "er-" + time.Now().UTC().Format("20060102150405.000000000") + "-" + hex.EncodeToString(b[:])
}

func (r *Runner) publishEvalCompleted(ctx context.Context, run biz.EvalRun) {
	if r == nil || r.bus == nil {
		return
	}
	msg := fmt.Sprintf("评估运行 %s 已结束（%s）", run.ID, run.Status)
	meta := map[string]any{
		"event_type":  "eval.completed",
		"run_id":      run.ID,
		"dataset_id":  run.DatasetID,
		"agent_id":    run.AgentID,
		"status":      run.Status,
		"total_cases": run.TotalCases,
	}
	r.bus.Publish(context.WithoutCancel(ctx), biz.NewSystemNoticeEvent("", "eval.completed", msg, meta))
}

func (r *Runner) recordEvalFailures(ctx context.Context, run biz.EvalRun, results []biz.EvalCaseResult) {
	if r == nil || r.obs == nil {
		return
	}
	for _, res := range results {
		if !isEvalFailure(res) {
			continue
		}
		meta, _ := json.Marshal(map[string]string{
			"run_id":  run.ID,
			"case_id": res.CaseID,
		})
		content := strings.TrimSpace(res.ErrorMessage)
		if content == "" {
			content = "evaluation case failed"
		}
		if _, err := r.obs.RecordObservation(ctx, biz.Observation{
			AgentID:    run.AgentID,
			SessionID:  res.SessionID,
			Kind:       biz.ObservationKindEvalFailure,
			Content:    content,
			Metadata:   string(meta),
			ObservedAt: time.Now().UTC(),
		}); err != nil {
			r.lg.Warn("eval failure observation not recorded",
				loggateway.StepID("evaluation.run.obs_fail"),
				loggateway.Str("run_id", run.ID),
				loggateway.Str("case_id", res.CaseID),
				loggateway.Err(err))
		}
	}
}

func (r *Runner) startLeaseHeartbeat(ctx context.Context, runID string) func() {
	if r == nil || r.uc == nil || strings.TrimSpace(runID) == "" {
		return func() {}
	}
	_ = r.uc.TouchRunLease(ctx, runID)
	done := make(chan struct{})
	var once sync.Once
	safego.Go(ctx, "eval-run-lease", func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if err := r.uc.TouchRunLease(ctx, runID); err != nil {
					r.lg.Warn("eval lease touch failed",
						loggateway.StepID("evaluation.run.lease_touch_fail"),
						loggateway.Str("run_id", runID),
						loggateway.Err(err))
				}
			}
		}
	})
	return func() { once.Do(func() { close(done) }) }
}

func isEvalFailure(res biz.EvalCaseResult) bool {
	if strings.TrimSpace(res.ErrorMessage) != "" {
		return true
	}
	if v, ok := biz.ParseEvalScores(res.ScoresJSON)["exact_match"]; ok && v < 1 {
		return true
	}
	return false
}
