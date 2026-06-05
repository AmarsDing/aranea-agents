package monitor

import (
	"context"
	"time"

	"aranea-agents/internal/biz/types"
)

// SelfChecker is the interface for periodic self-check plugins.
// Each checker inspects one subsystem and returns a SelfCheckResult.
type SelfChecker interface {
	// Name returns the unique checker identifier (e.g. "db_health").
	Name() string
	// Check executes the self-check and returns the result.
	Check(ctx context.Context) types.SelfCheckResult
}

// SelfCheckReport is the aggregated report for one self-check cycle.
type SelfCheckReport struct {
	ID            string                  `json:"id"`
	CheckResults  []types.SelfCheckResult `json:"check_results"`
	OverallStatus types.SelfCheckStatus   `json:"overall_status"`
	RepairActions []RepairOutcome         `json:"repair_actions,omitempty"`
	StartedAt     time.Time               `json:"started_at"`
	FinishedAt    time.Time               `json:"finished_at"`
	DurationMs    int64                   `json:"duration_ms"`
}

// AggregateOverallStatus returns the worst status among all results.
// Priority: failed > warning > passed.
func AggregateOverallStatus(results []types.SelfCheckResult) types.SelfCheckStatus {
	worst := types.SelfCheckStatusPassed
	for _, r := range results {
		switch r.Status {
		case types.SelfCheckStatusFailed:
			return types.SelfCheckStatusFailed
		case types.SelfCheckStatusWarning:
			worst = types.SelfCheckStatusWarning
		}
	}
	return worst
}

// SelfCheckRepairer is the interface for auto-repair actions.
// Repairers are decoupled from checkers (SRP): check → diagnose → repair.
type SelfCheckRepairer interface {
	// CanRepair returns true if this repairer handles the given check name and status.
	CanRepair(checkName string, status types.SelfCheckStatus) bool
	// Repair attempts to fix the issue and returns the outcome.
	Repair(ctx context.Context, result types.SelfCheckResult) RepairOutcome
}

// RepairOutcome documents the result of a repair attempt.
type RepairOutcome struct {
	Success bool   `json:"success"`
	Action  string `json:"action"` // e.g. "restarted_worker", "reconnected_subscription"
	Message string `json:"message,omitempty"`
}
