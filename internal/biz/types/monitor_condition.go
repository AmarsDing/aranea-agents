package types

import "time"

// SelfCheckResult captures the outcome of a single self-check cycle.
// This type is defined here as a cross-module shared type used by both
// the monitor self-check scheduler and the self-check-repair subsystem.
type SelfCheckResult struct {
	CheckID    string                  `json:"check_id"`
	Checker    string                  `json:"checker"`     // e.g., "session_integrity", "agent_health"
	Status     SelfCheckStatus         `json:"status"`      // "passed", "warning", "failed"
	Message    string                  `json:"message,omitempty"`
	Details    map[string]any          `json:"details,omitempty"`
	Conditions []SelfCheckStatusCondition `json:"conditions,omitempty"`
	CheckedAt  time.Time               `json:"checked_at"`
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
type SelfCheckStatusCondition struct {
	Checker    string `json:"checker"`
	Status     string `json:"status"`     // "warning" or "failed"
	Detail     string `json:"detail,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

// AutoHealedCondition is a condition type for RootCauseCondition oneof
// extension, used by the monitor-self-healing subsystem to indicate that
// a root cause was automatically healed.
type AutoHealedCondition struct {
	RuleID      string  `json:"rule_id"`
	FixAction   string  `json:"fix_action"`    // e.g., "retry", "reconnect", "fallback"
	Confidence  float64 `json:"confidence"`
	HealedAt    string  `json:"healed_at"`
}
