package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	memoryL1ArchiveDefaultInterval = 5 * time.Minute
	memoryL1ArchiveDefaultIdleMin  = 60
)

// MemoryL1ArchiveWorker archives idle L1 tasks and creates L2 episodes.
type MemoryL1ArchiveWorker struct {
	interval time.Duration
	store    biz.L1IdleTaskReader
	writer   biz.L1TaskWriter
	agents   *biz.AgentUsecase
	lg       loggateway.Logger
}

func NewMemoryL1ArchiveWorker(interval time.Duration, store biz.SessionAdminStore, agents *biz.AgentUsecase, lg loggateway.Logger) *MemoryL1ArchiveWorker {
	if interval <= 0 {
		interval = memoryL1ArchiveDefaultInterval
	}
	return &MemoryL1ArchiveWorker{
		interval: interval,
		store:    store,
		writer:   store,
		agents:   agents,
		lg:       lg,
	}
}

func (w *MemoryL1ArchiveWorker) Start(ctx context.Context) {
	if w == nil || w.store == nil {
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

func (w *MemoryL1ArchiveWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.l1_archive", func() {
		idleMin := memoryL1ArchiveDefaultIdleMin
		cutoff := time.Now().UTC().Add(-time.Duration(idleMin) * time.Minute).Format(time.RFC3339Nano)
		tasks, err := w.store.ListIdleL1Tasks(ctx, cutoff)
		if err != nil {
			w.lg.Warn("L1 archive idle task scan failed", loggateway.Err(err))
			return
		}
		var archived int
		for _, raw := range tasks {
			m, _ := jsonutil.ParseMap(raw)
			taskID := jsonutil.IfaceStr(m, "id")
			sessionID := jsonutil.IfaceStr(m, "session_id")
			if taskID == "" || sessionID == "" {
				continue
			}
			if _, err := w.writer.EndL1Task(ctx, sessionID, taskID, "cancelled"); err != nil {
				w.lg.Warn("L1 archive auto-end failed",
					loggateway.Str("task_id", taskID),
					loggateway.Err(err))
				continue
			}
			archived++
		}
		if archived > 0 {
			w.lg.Info("memory l1 archive completed",
				loggateway.Int("archived", archived),
				loggateway.Int("idle_min", idleMin))
		}
	})
}

func MemoryL1ArchiveDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L1_ARCHIVE_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
