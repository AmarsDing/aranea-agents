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

// MemoryL1ArchiveWorker archives idle L1 tasks (creating L2 episodes) and
// cleans up expired L1 fields on a periodic schedule.
//
// On each tick it:
//  1. Deletes L1 fields whose expires_at has passed (DB cleanup).
//  2. Lists idle L1 tasks (active, not archived, updated_at < cutoff).
//  3. For each idle task: EndL1Task (set status=cancelled) then
//     ArchiveAndCreateEpisodeTx (set archived_at + create bare L2 episode).
//
// The worker can be disabled via MEMORY_L1_ARCHIVE_DISABLED env var.
type MemoryL1ArchiveWorker struct {
	interval time.Duration
	store    biz.L1IdleTaskReader
	writer   biz.L1TaskWriter
	cleaner  biz.L1ExpiredFieldCleaner
	agents   *biz.AgentUsecase
	lg       loggateway.Logger
}

// NewMemoryL1ArchiveWorker creates a worker that archives idle L1 tasks and
// cleans up expired fields. The store must implement L1IdleTaskReader,
// L1TaskWriter, and L1ExpiredFieldCleaner (SessionAdminStore satisfies all
// three).
func NewMemoryL1ArchiveWorker(interval time.Duration, store biz.SessionAdminStore, agents *biz.AgentUsecase, lg loggateway.Logger) *MemoryL1ArchiveWorker {
	if interval <= 0 {
		interval = memoryL1ArchiveDefaultInterval
	}
	return &MemoryL1ArchiveWorker{
		interval: interval,
		store:    store,
		writer:   store,
		cleaner:  store,
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
		w.cleanupExpiredFields(ctx)
		w.archiveIdleTasks(ctx)
	})
}

// RunOnceExposed runs a single archive cycle synchronously (for testing).
func (w *MemoryL1ArchiveWorker) RunOnceExposed(ctx context.Context) {
	w.cleanupExpiredFields(ctx)
	w.archiveIdleTasks(ctx)
}

// cleanupExpiredFields deletes L1 fields whose TTL has expired. These fields
// are already filtered from normal reads; this is pure DB cleanup.
func (w *MemoryL1ArchiveWorker) cleanupExpiredFields(ctx context.Context) {
	if w.cleaner == nil {
		return
	}
	deleted, err := w.cleaner.DeleteExpiredL1Fields(ctx)
	if err != nil {
		w.lg.Warn("L1 expired field cleanup failed", loggateway.Err(err))
		return
	}
	if deleted > 0 {
		w.lg.Info("L1 expired field cleanup completed",
			loggateway.Int("deleted", deleted))
	}
}

// archiveIdleTasks ends idle L1 tasks and creates L2 episodes for them.
func (w *MemoryL1ArchiveWorker) archiveIdleTasks(ctx context.Context) {
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
		agentID := jsonutil.IfaceStr(m, "agent_id")
		if taskID == "" || sessionID == "" {
			continue
		}
		// Step 1: End the task (set status=cancelled, ended_at=now).
		if _, err := w.writer.EndL1Task(ctx, sessionID, taskID, "cancelled"); err != nil {
			w.lg.Warn("L1 archive auto-end failed",
				loggateway.Str("task_id", taskID),
				loggateway.Err(err))
			continue
		}
		// Step 2: Archive the task and create a bare L2 episode.
		// The data layer builds the full snapshot (task + fields) inside
		// the transaction and creates the episode atomically.
		if _, err := w.writer.ArchiveAndCreateEpisodeTx(ctx, sessionID, taskID, biz.L1ArchiveEpisodeInsert{
			SessionID: sessionID,
			AgentID:   agentID,
			TaskID:    taskID,
		}); err != nil {
			w.lg.Warn("L1 archive episode creation failed",
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
}

func MemoryL1ArchiveDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L1_ARCHIVE_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
