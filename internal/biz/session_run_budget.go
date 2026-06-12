package biz

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// BudgetPhaseCallbacks fires when soft/hard budgets elapse during Interactive phase.
type BudgetPhaseCallbacks struct {
	OnSoftBudget func(phase string)
	OnHardBudget func(phase string)
}

// hardBudgetGracePeriod is the time allowed after hard budget fires for the
// caller to gracefully transition the run to a terminal phase. If the run is
// still non-terminal after this period, it is force-failed to prevent resource
// leaks (a run stuck in durable phase forever consumes runtime resources and
// blocks session concurrency).
const hardBudgetGracePeriod = 60 * time.Second

// StartBudgetWatcher monitors wall-clock budgets until cancel is called or hard budget fires.
// Returns cancel to stop watchers when the turn finishes.
func (u *SessionRunUsecase) StartBudgetWatcher(
	parent context.Context,
	runID string,
	budget SessionRunBudget,
	cb BudgetPhaseCallbacks,
) context.CancelFunc {
	if u == nil || u.repo == nil || strings.TrimSpace(runID) == "" {
		return func() {}
	}
	if budget.SoftBudgetSec <= 0 {
		budget = DefaultSessionRunBudget()
	}
	if budget.HardBudgetSec <= 0 {
		budget.HardBudgetSec = DefaultSessionRunBudget().HardBudgetSec
	}
	ctx, cancel := context.WithCancel(parent)
	var softFired, hardFired atomic.Bool

	fireSoft := func() {
		if !softFired.CompareAndSwap(false, true) {
			return
		}
		if err := u.MarkPhase(ctx, runID, SessionRunPhaseEscalating); err != nil {
			u.lg.Warn("mark soft budget phase failed", loggateway.Err(err), loggateway.Str("run_id", runID))
		}
		if cb.OnSoftBudget != nil {
			cb.OnSoftBudget(SessionRunPhaseEscalating)
		}
	}
	fireHard := func() {
		if !hardFired.CompareAndSwap(false, true) {
			return
		}
		// CC-R-OPT-03: checkpoint + MarkPhase(durable) happen only in escalateSessionRunToDurable.
		if cb.OnHardBudget != nil {
			cb.OnHardBudget(SessionRunPhaseDurable)
		}
		// Grace period: after hard budget fires, allow hardBudgetGracePeriod for the caller
		// to gracefully transition the run to a terminal phase. If the run is
		// still non-terminal after the grace period, force-fail it.
		// Note: this goroutine uses context.WithoutCancel to ensure the DB operations
		// (repo.Get, repo.MarkTerminal) complete even if the parent ctx is cancelled,
		// since the parent ctx may be cancelled when the hard budget watcher exits.
		safego.Go(ctx, "session-run-hard-budget-grace", func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(hardBudgetGracePeriod):
				// Use background context for DB operations to avoid cancellation
				// when the parent budget watcher ctx is cancelled.
				dbc := context.WithoutCancel(ctx)
				run, err := u.repo.Get(dbc, runID)
				if err != nil {
					u.lg.Warn("hard budget grace: failed to get run", loggateway.Err(err), loggateway.Str("run_id", runID))
					return
				}
				if !IsSessionRunPhaseTerminal(ParseSessionRunPhase(run.Phase)) {
					u.lg.Warn("hard budget grace: forcing run to failed", loggateway.Str("run_id", runID), loggateway.Str("phase", run.Phase))
					if err := u.repo.MarkTerminal(dbc, runID, string(PhaseFailed), "hard budget grace period exceeded"); err != nil {
						u.lg.Warn("hard budget grace: force-fail failed", loggateway.Err(err), loggateway.Str("run_id", runID))
					}
				}
			}
		})
	}

	safego.Go(ctx, "session-run-budget-watcher", func() {
		soft := time.NewTimer(time.Duration(budget.SoftBudgetSec) * time.Second)
		hard := time.NewTimer(time.Duration(budget.HardBudgetSec) * time.Second)
		defer soft.Stop()
		defer hard.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-soft.C:
				fireSoft()
			case <-hard.C:
				fireHard()
				return
			}
		}
	})
	return cancel
}
