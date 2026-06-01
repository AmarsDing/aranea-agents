package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const sessionRunWorkerPollInterval = 5 * time.Second

// SessionRunDurableWorker resumes agent turns from durable checkpoints (CC-R-03).
type SessionRunDurableWorker struct {
	runs     *biz.SessionRunUsecase
	runCtrl  biz.TurnRunControlGateway
	resumer  biz.DurableResumeGateway
	lg       loggateway.Logger
}

func NewSessionRunDurableWorker(runs *biz.SessionRunUsecase, runCtrl biz.TurnRunControlGateway, resumer biz.DurableResumeGateway, lg loggateway.Logger) *SessionRunDurableWorker {
	if runs == nil || runCtrl == nil || resumer == nil {
		return nil
	}
	return &SessionRunDurableWorker{runs: runs, runCtrl: runCtrl, resumer: resumer, lg: lg}
}

func (w *SessionRunDurableWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	// Clean up zombie runs from a previous process crash/restart.
	if n, err := w.runs.CleanupOrphanedRuns(context.Background()); err != nil {
		w.lg.Warn("orphan cleanup failed",
			loggateway.StepID("session.durable_worker"),
			loggateway.Err(err),
		)
	} else if n > 0 {
		w.lg.Info("orphaned runs cleaned up",
			loggateway.StepID("session.durable_worker"),
			loggateway.Int("count", n),
		)
	}
	safego.Go(ctx, "session-run-durable-worker", func() {
		ticker := time.NewTicker(sessionRunWorkerPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.processOnce(context.Background())
			}
		}
	})
}

func (w *SessionRunDurableWorker) processOnce(ctx context.Context) {
	runs, err := w.runs.ListDurablePending(ctx, 8)
	if err != nil || len(runs) == 0 {
		return
	}
	for _, run := range runs {
		if strings.TrimSpace(run.CheckpointID) == "" {
			continue
		}
		if w.runCtrl.HasActiveRun(run.SessionID) {
			continue
		}
		if err := w.resumer.ResumeDurableSessionRun(ctx, run.ID); err != nil {
			w.lg.Warn("resume failed",
				loggateway.StepID("session.durable_worker"),
				loggateway.Str("run_id", run.ID),
				loggateway.Err(err),
			)
		}
	}
}
