package jobs

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	memoryL1ArchiveDefaultInterval = 5 * time.Minute
	memoryL1ArchiveDefaultIdleMin  = 60

	// memoryL1ArchiveRetryCutoffMin is the ended_at cutoff for the
	// ended-but-unarchived retry branch (P1-2). Tasks ended more recently are
	// skipped so the worker never races the synchronous end+archive path in
	// MemoryAdminUsecase.EndL1Task.
	memoryL1ArchiveRetryCutoffMin = 2

	// memoryL1ArchiveAlarmThreshold is the number of consecutive archive
	// failures for the same task that triggers a flow-log dead-letter alarm
	// (step system.memory_l1_archive.failed). The task stays in the scan set
	// and keeps retrying — the alarm exists so persistent failures are never
	// silent.
	memoryL1ArchiveAlarmThreshold = 3
	// memoryL1ArchiveAlarmEvery re-fires the alarm every N additional
	// consecutive failures after the threshold (frequency cap).
	memoryL1ArchiveAlarmEvery = 10
)

// MemoryL1ArchiveWorker archives idle L1 tasks (creating L2 episodes) and
// cleans up expired L1 fields on a periodic schedule.
//
// On each tick it:
//  1. Deletes L1 fields whose expires_at has passed (DB cleanup).
//  2. Lists tasks pending archive in two branches (P1-2):
//     - idle active tasks (updated_at < idle cutoff): EndL1Task(cancelled)
//     then ArchiveAndCreateEpisodeTx (archived_at + bare L2 episode,
//     atomically).
//     - ended-but-unarchived tasks (archived_at=”, ended_at < retry cutoff):
//     a previous archive attempt failed — retry the archive tx only, never
//     re-end. This keeps failures inside the scan set instead of silently
//     dropping them.
//  3. Raises a flow-log dead-letter alarm when a task's consecutive archive
//     failures cross the threshold; the task stays in the scan set and keeps
//     retrying.
//
// The worker can be disabled via MEMORY_L1_ARCHIVE_DISABLED env var.
type MemoryL1ArchiveWorker struct {
	interval time.Duration
	store    biz.L1IdleTaskReader
	writer   biz.L1TaskWriter
	cleaner  biz.L1ExpiredFieldCleaner
	agents   *biz.AgentUsecase
	flowLog  biz.FlowLogWriter // optional: dead-letter alarm channel (nil-safe)
	lg       loggateway.Logger

	// failCounts tracks consecutive archive failures per task ID for the
	// dead-letter alarm. Entries are removed on the first success.
	failMu     sync.Mutex
	failCounts map[string]int
}

// NewMemoryL1ArchiveWorker creates a worker that archives idle L1 tasks and
// cleans up expired fields. The store must implement L1IdleTaskReader,
// L1TaskWriter, and L1ExpiredFieldCleaner (SessionAdminStore satisfies all
// three). flowLog is the optional dead-letter alarm channel (nil-safe).
func NewMemoryL1ArchiveWorker(interval time.Duration, store biz.SessionAdminStore, agents *biz.AgentUsecase, lg loggateway.Logger, flowLog biz.FlowLogWriter) *MemoryL1ArchiveWorker {
	if interval <= 0 {
		interval = memoryL1ArchiveDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MemoryL1ArchiveWorker{
		interval: interval,
		store:    store,
		writer:   store,
		cleaner:  store,
		agents:   agents,
		flowLog:  flowLog,
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

// archiveIdleTasks ends idle L1 tasks and creates L2 episodes for them, and
// retries the archive for ended-but-unarchived tasks (P1-2).
func (w *MemoryL1ArchiveWorker) archiveIdleTasks(ctx context.Context) {
	idleMin := memoryL1ArchiveDefaultIdleMin
	now := time.Now().UTC()
	idleCutoff := now.Add(-time.Duration(idleMin) * time.Minute).Format(time.RFC3339Nano)
	retryCutoff := now.Add(-memoryL1ArchiveRetryCutoffMin * time.Minute).Format(time.RFC3339Nano)
	tasks, err := w.store.ListIdleL1Tasks(ctx, idleCutoff, retryCutoff)
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
		status := jsonutil.IfaceStr(m, "status")
		if taskID == "" || sessionID == "" {
			continue
		}
		// Active branch (idle-expired safety net): end the task first.
		// Already-ended tasks skip straight to the archive retry.
		if status == "" || status == "active" {
			if _, err := w.writer.EndL1Task(ctx, sessionID, taskID, "cancelled"); err != nil {
				w.lg.Warn("L1 archive auto-end failed",
					loggateway.Str("task_id", taskID),
					loggateway.Err(err))
				w.noteArchiveFailure(sessionID, taskID, "end", err)
				continue
			}
		}
		// Archive the task and create a bare L2 episode atomically. The data
		// layer builds the full snapshot (task + fields) inside the
		// transaction; on failure the archive is rolled back and the task
		// stays in the scan set for the next tick.
		if _, err := w.writer.ArchiveAndCreateEpisodeTx(ctx, sessionID, taskID, biz.L1ArchiveEpisodeInsert{
			SessionID: sessionID,
			AgentID:   agentID,
			TaskID:    taskID,
		}); err != nil {
			w.lg.Warn("L1 archive episode creation failed",
				loggateway.Str("task_id", taskID),
				loggateway.Err(err))
			w.noteArchiveFailure(sessionID, taskID, "archive", err)
			continue
		}
		w.resetArchiveFailure(taskID)
		archived++
	}
	if archived > 0 {
		w.lg.Info("memory l1 archive completed",
			loggateway.Int("archived", archived),
			loggateway.Int("idle_min", idleMin))
	}
}

// noteArchiveFailure increments the consecutive-failure counter for a task and
// fires a flow-log dead-letter alarm at the threshold (then every N additional
// failures). The task remains in the scan set and keeps retrying.
func (w *MemoryL1ArchiveWorker) noteArchiveFailure(sessionID, taskID, stage string, cause error) {
	w.failMu.Lock()
	if w.failCounts == nil {
		w.failCounts = make(map[string]int)
	}
	w.failCounts[taskID]++
	n := w.failCounts[taskID]
	w.failMu.Unlock()

	if n < memoryL1ArchiveAlarmThreshold {
		return
	}
	if n > memoryL1ArchiveAlarmThreshold && (n-memoryL1ArchiveAlarmThreshold)%memoryL1ArchiveAlarmEvery != 0 {
		return
	}
	w.lg.Error("memory l1 archive consecutive failures (dead-letter alarm)",
		loggateway.StepID("system.memory_l1_archive.failed"),
		loggateway.Str("task_id", taskID),
		loggateway.Str("stage", stage),
		loggateway.Int("consecutive_failures", n),
		loggateway.Err(cause))
	if w.flowLog != nil {
		w.flowLog.LogFlowError(context.Background(), sessionID,
			"system.memory_l1_archive.failed",
			fmt.Sprintf("L1 任务 %s 归档连续失败 %d 次（%s 阶段），任务保留在重试集合中：%v", taskID, n, stage, cause),
			biz.LogPair{Key: "task_id", Value: taskID},
			biz.LogPair{Key: "stage", Value: stage},
			biz.LogPair{Key: "consecutive_failures", Value: strconv.Itoa(n)})
	}
}

// resetArchiveFailure clears the consecutive-failure counter after a
// successful archive.
func (w *MemoryL1ArchiveWorker) resetArchiveFailure(taskID string) {
	w.failMu.Lock()
	delete(w.failCounts, taskID)
	w.failMu.Unlock()
}

func MemoryL1ArchiveDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_L1_ARCHIVE_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
