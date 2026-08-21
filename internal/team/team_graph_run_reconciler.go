package team

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// defaultStaleRunThreshold is the P0 终态一致性 aging window: a running
// session with no graph activity beyond it means the graph watch died
// (panic / dropped subscription) and nothing will ever close the run again —
// ReconcileStaleRuns force-fails it. Must exceed WatchTimeout (a live watch
// finalizes first) and stay below SessionMaxAge (CleanupStaleSessions would
// otherwise evict the session without finalizing the run).
const defaultStaleRunThreshold = 45 * time.Minute

// staleRunHandlerHolder is process-wide: the coordinator is a singleton
// (ProvideTeamGraphRunCoordinator), so a package-level slot keeps the wiring
// without touching the coordinator struct. Stores
// func(teamID, teamRunID, reason string).
var staleRunHandlerHolder atomic.Value

// SetStaleRunHandler wires the callback invoked when ReconcileStaleRuns
// force-fails a lost running team run (P0 终态一致性). The service layer
// forwards the failure to PlanExecutor.NotifyTeamCompletion so a blocked DAG
// step unblocks (cascade-fails) instead of waiting forever.
func (c *TeamGraphRunCoordinator) SetStaleRunHandler(h func(teamID, teamRunID, reason string)) {
	if c == nil || h == nil {
		return
	}
	staleRunHandlerHolder.Store(h)
}

// ReconcileStaleRuns force-fails "lost" running team runs: sessions whose
// in-memory status is running but that have seen no graph activity for longer
// than the stale threshold. waiting_human sessions belong to the HITL SLA
// path and are never touched. Runs already terminal in the DB only get their
// residual session evicted (no duplicate finalize, no handler call).
// Driven by the cleanup ticker in provider.go. Returns the number of runs
// force-failed this pass.
func (c *TeamGraphRunCoordinator) ReconcileStaleRuns(ctx context.Context, now time.Time, threshold time.Duration) int {
	if c == nil {
		return 0
	}
	if threshold <= 0 {
		threshold = defaultStaleRunThreshold
	}
	// Phase 1: collect candidates under read lock (finalize/evict take the
	// write lock — never hold the lock across them).
	var stale []*teamGraphRunSession
	c.mu.RLock()
	for _, sess := range c.sessions {
		if sess.status != biz.TeamRunStatusRunning {
			continue
		}
		last := sess.lastActivityAt
		if last.IsZero() {
			last = sess.registeredAt
		}
		if last.IsZero() || now.Sub(last) <= threshold {
			continue
		}
		stale = append(stale, sess)
	}
	c.mu.RUnlock()

	reconciled := 0
	for _, sess := range stale {
		last := sess.lastActivityAt
		if last.IsZero() {
			last = sess.registeredAt
		}
		reason := fmt.Sprintf("stale run reconciled: no graph activity for %s (threshold %s)", now.Sub(last).Round(time.Second), threshold)
		// The run may already be terminal (finalized elsewhere, e.g. service
		// gate) while the session leaked — evict the residue without firing a
		// duplicate failure notification.
		if c.teamRunReader != nil {
			if run, err := c.teamRunReader.GetTeamRunByID(ctx, sess.teamRunID); err == nil && biz.IsTeamRunTerminalStatus(run.Status) {
				c.evictSession(sess.execID)
				c.lg.Warn("ReconcileStaleRuns: evicted residual session of terminal run",
					loggateway.StepID("team.run.reconcile_residual"),
					loggateway.Str("exec_id", sess.execID),
					loggateway.Str("team_run_id", sess.teamRunID),
					loggateway.Str("run_status", run.Status))
				continue
			}
		}
		c.finalizeTeamRun(ctx, sess, true, reason)
		if c.session(sess.execID) != nil {
			// finalize rejected/exhausted (FSM or CAS) — session retained for
			// the next tick; escalated logging already emitted by finalize.
			c.lg.Error("ReconcileStaleRuns: finalize did not close run, will retry next tick",
				loggateway.StepID("team.run.reconcile_retry"),
				loggateway.Str("exec_id", sess.execID),
				loggateway.Str("team_run_id", sess.teamRunID))
			continue
		}
		reconciled++
		c.lg.Warn("ReconcileStaleRuns: force-failed lost running team run",
			loggateway.StepID("team.run.reconciled"),
			loggateway.Str("exec_id", sess.execID),
			loggateway.Str("team_run_id", sess.teamRunID),
			loggateway.Str("team_id", sess.teamID),
			loggateway.Str("reason", reason))
		if h, ok := staleRunHandlerHolder.Load().(func(teamID, teamRunID, reason string)); ok && h != nil {
			h(sess.teamID, sess.teamRunID, reason)
		}
	}
	return reconciled
}