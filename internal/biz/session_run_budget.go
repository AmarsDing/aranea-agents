package biz

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/safego"
)

// BudgetPhaseCallbacks fires when soft/hard budgets elapse during Interactive phase.
type BudgetPhaseCallbacks struct {
	OnSoftBudget func(phase string)
	OnHardBudget func(phase string)
}

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
		_ = u.MarkPhase(ctx, runID, SessionRunPhaseEscalating)
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
