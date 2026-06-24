package turn

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
)

// Runner executes the turn body after admission checks pass.
type Runner interface {
	RunWithOutcome(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error)
}

// Executor is the L3 TurnExecutor skeleton. It classifies outcomes and delegates
// execution to a Runner implementation (ChatOrchestrator in production).
type Executor struct {
	runner Runner
}

// NewExecutor wires a Runner into the turn executor.
func NewExecutor(runner Runner) *Executor {
	return &Executor{runner: runner}
}

// Execute runs a single turn and returns a classified TurnResult.
func (e *Executor) Execute(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error) {
	if e == nil || e.runner == nil {
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, fmt.Errorf("turn executor: runner not configured")
	}
	return e.runner.RunWithOutcome(ctx, input)
}
