package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	memoryDeadLetterReplayDefaultInterval = 30 * time.Minute
	memoryDeadLetterReplayBatchSize       = 50
	memoryDeadLetterMaxAttempts           = 3
)

type DeadLetterEnqueueFunc func(sessionID, appName, userID, feedbackMsgID string, priority biz.MemoryJobPriority)

type MemoryDeadLetterReplayer struct {
	interval    time.Duration
	repo        biz.MemoryDeadLetterAdminRepo
	enqueueFunc DeadLetterEnqueueFunc
	lg          loggateway.Logger
}

func NewMemoryDeadLetterReplayer(interval time.Duration, repo biz.MemoryDeadLetterAdminRepo, enqueueFunc DeadLetterEnqueueFunc, lg loggateway.Logger) *MemoryDeadLetterReplayer {
	if interval <= 0 {
		interval = memoryDeadLetterReplayDefaultInterval
	}
	return &MemoryDeadLetterReplayer{
		interval:    interval,
		repo:        repo,
		enqueueFunc: enqueueFunc,
		lg:          lg,
	}
}

func (w *MemoryDeadLetterReplayer) Start(ctx context.Context) {
	if w == nil || w.repo == nil || w.enqueueFunc == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *MemoryDeadLetterReplayer) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.dead_letter_replay", func() {
		entries, err := w.repo.ListDeadLetters(ctx, "pending", memoryDeadLetterReplayBatchSize)
		if err != nil {
			w.lg.Warn("list pending dead letters failed", loggateway.Err(err))
			return
		}
		if len(entries) == 0 {
			return
		}
		var replayed, failed, abandoned int
		for _, e := range entries {
			if e.Attempts >= memoryDeadLetterMaxAttempts {
				if abandonErr := w.repo.MarkDeadLetterAbandoned(ctx, e.ID, "max_attempts_exceeded"); abandonErr == nil {
					abandoned++
					w.lg.Warn("abandoned after max attempts",
						loggateway.Any("id", e.ID), loggateway.Int("attempts", e.Attempts))
				}
				continue
			}
			if err := w.repo.ReplayDeadLetterIntoQueue(ctx, e.ID, w.enqueueFunc); err != nil {
				failed++
				w.lg.Warn("replay failed",
					loggateway.Any("id", e.ID), loggateway.Err(err))
				continue
			}
			replayed++
		}
		if replayed > 0 || failed > 0 || abandoned > 0 {
			w.lg.Info("dead letter replay summary",
				loggateway.Int("replayed", replayed),
				loggateway.Int("failed", failed),
				loggateway.Int("abandoned", abandoned),
				loggateway.Int("total", len(entries)))
		}
	})
}

func MemoryDeadLetterReplayDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_DEAD_LETTER_REPLAY_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
