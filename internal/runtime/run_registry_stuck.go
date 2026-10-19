package runtime

import (
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// DefaultStuckRunTimeout bounds how long an active run may go without
// producing a single framework event before the stuck-run sweep declares it
// wedged and cancels it (TD-T1 heartbeat detection, 2026-10-19). It is far
// above every legitimate silence source: the turn timeout reminder fires at
// 10min and LLM streams/tools emit events continuously while healthy; runs
// parked for user input (awaiting_user) are excluded by status, not timing.
const DefaultStuckRunTimeout = 30 * time.Minute

// StuckRun describes one run cancelled by SweepStuckRuns.
type StuckRun struct {
	SessionID string
	RunID     string
	SilentFor time.Duration
}

// SweepStuckRuns cancels active runs whose runner has produced no framework
// events for longer than threshold (TD-T1: server-side stuck detection —
// previously a wedged runner held its session slot forever, blocking new
// turns until process restart).
//
// Safety invariants:
//   - threshold <= 0 disables the sweep entirely.
//   - Placeholder / runner-less / already-cancelling entries are skipped:
//     their lifecycle is owned by StorePlaceholder/StoreRunner and the M3
//     cancel-intent plumbing, not by activity timing.
//   - Runs in awaiting_user are never touched — clarification cards and
//     confirms may legitimately wait days with zero events. Terminal-status
//     entries are likewise skipped (they are being drained already).
//   - A run with no readable heartbeat signal (runner declines run-status
//     reporting AND no registry status entry) is never killed blind.
//
// Cancellation reuses the regular Cancel path (M2/M3 status recording,
// ManagedRunner.Cancel with Close fallback, cancelling-guard), so the turn
// settles through the same machinery as a user-initiated stop. The returned
// slice lists every run actually cancelled; between Range and Cancel a run
// may finish — Cancel re-checks the slot and simply returns false.
func (r *RunRegistry) SweepStuckRuns(now time.Time, threshold time.Duration) []StuckRun {
	if r == nil || threshold <= 0 {
		return nil
	}
	type candidate struct {
		sessionID string
		runID     string
		silentFor time.Duration
	}
	var candidates []candidate
	r.activeRuns.Range(func(sessionID string, run activeRun) bool {
		if run.placeholder || run.cancelling || run.runner == nil {
			return true
		}
		st, hasStatus := r.GetStatus(sessionID)
		if hasStatus {
			switch biz.RunState(st.Status) {
			case biz.RunStateAwaitingUser, biz.RunStateCompleted, biz.RunStateFailed, biz.RunStateCancelled:
				return true
			}
		}
		// Heartbeat: the framework runner's LastEventAt is authoritative.
		// A runner declining run-status reports falls back to the last
		// registry status-transition timestamp.
		var lastAt time.Time
		if h, ok := FrameworkRunStatusFromRunner(run.runner, sessionID); ok {
			lastAt = h.LastEventAt
		} else if hasStatus {
			lastAt = st.UpdatedAt
		}
		if lastAt.IsZero() {
			return true // no signal — never kill blind
		}
		if silent := now.Sub(lastAt); silent > threshold {
			candidates = append(candidates, candidate{sessionID, run.runID, silent})
		}
		return true
	})
	var stuck []StuckRun
	for _, c := range candidates {
		// Cancel outside the Range callback so the shard lock is not held
		// while runner-cancellation machinery runs.
		reason := fmt.Sprintf("stuck run: no events for %s (threshold %s)", c.silentFor.Round(time.Second), threshold)
		if ok, _ := r.Cancel(c.sessionID, reason); ok {
			stuck = append(stuck, StuckRun{SessionID: c.sessionID, RunID: c.runID, SilentFor: c.silentFor})
			if r.lg != nil {
				r.lg.Warn("stuck run cancelled by heartbeat sweep",
					loggateway.SessionID(c.sessionID),
					loggateway.Str("run_id", c.runID),
					loggateway.Str("silent_for", c.silentFor.Round(time.Second).String()),
				)
			}
		}
	}
	return stuck
}
