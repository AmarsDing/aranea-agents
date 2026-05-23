package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
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
