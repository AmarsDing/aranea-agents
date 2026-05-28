package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

const sessionRunWorkerPollInterval = 5 * time.Second

// SessionRunDurableWorker resumes agent turns from durable checkpoints (CC-R-03).
type SessionRunDurableWorker struct {
	runs *biz.SessionRunUsecase
	chat *ChatService
}

func NewSessionRunDurableWorker(runs *biz.SessionRunUsecase, chat *ChatService) *SessionRunDurableWorker {
	if runs == nil || chat == nil {
		return nil
	}
	return &SessionRunDurableWorker{runs: runs, chat: chat}
}

func (w *SessionRunDurableWorker) Start(ctx context.Context) {
	if w == nil {
		return
	}
	// Clean up zombie runs from a previous process crash/restart.
	if n, err := w.runs.CleanupOrphanedRuns(context.Background()); err != nil {
		event.SysLogWarn("session.durable_worker", "orphan cleanup failed", event.P("error", err.Error()))
	} else if n > 0 {
		event.SysLogInfo("session.durable_worker", "orphaned runs cleaned up", event.P("count", n))
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
		if w.chat.orch.HasActiveRun(run.SessionID) {
			continue
		}
		_ = w.chat.ResumeDurableSessionRun(ctx, run.ID)
	}
}
