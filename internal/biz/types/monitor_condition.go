package types

import "time"

// SelfCheckResult captures the outcome of a single self-check cycle.
// This type is defined here as a cross-module shared type used by both
// the monitor self-check scheduler and the self-check-repair subsystem.
//
// SelfCheckResult is a value object; direct construction via &SelfCheckResult{} is acceptable.
type SelfCheckResult struct {
	CheckID string          `json:"check_id"`
	Checker string          `json:"checker"` // e.g., "session_integrity", "agent_health"
	Status  SelfCheckStatus `json:"status"`  // "passed", "warning", "failed"
	Message string          `json:"message,omitempty"`
	// Details contains checker-specific information.
	// Common keys: "component" (string), "duration_ms" (int64), "error" (string).
	Details    map[string]any             `json:"details,omitempty"`
	Conditions []SelfCheckStatusCondition `json:"conditions,omitempty"`
	CheckedAt  time.Time                  `json:"checked_at"`
}

// SelfCheckStatus represents the outcome of a self-check.
type SelfCheckStatus string

const (
	SelfCheckStatusPassed  SelfCheckStatus = "passed"
	SelfCheckStatusWarning SelfCheckStatus = "warning"
	SelfCheckStatusFailed  SelfCheckStatus = "failed"
)

// SelfCheckStatusCondition is a condition type for RootCauseCondition oneof
// extension, used by the monitor-selfcheck-repair subsystem.
//
// SelfCheckStatusCondition is a value object; direct construction via &SelfCheckStatusCondition{} is acceptable.
type SelfCheckStatusCondition struct {
	Checker string `json:"checker"`
	// Status is one of SelfCheckStatus constants: StatusPassed, StatusWarning, StatusFailed.
	Status     string `json:"status"` // "warning" or "failed"
	Detail     string `json:"detail,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

// AutoHealedCondition is a condition type for RootCauseCondition oneof
// extension, used by the monitor-self-healing subsystem to indicate that
// a root cause was automatically healed.
//
// AutoHealedCondition is a value object; direct construction via &AutoHealedCondition{} is acceptable.
type AutoHealedCondition struct {
	RuleID     string  `json:"rule_id"`
	FixAction  string  `json:"fix_action"` // e.g., "retry", "reconnect", "fallback"
	Confidence float64 `json:"confidence"`
	HealedAt   string  `json:"healed_at"`
}
