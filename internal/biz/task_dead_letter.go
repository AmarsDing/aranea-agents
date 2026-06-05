package biz

import "strings"

// TaskDeadLetter is a halted orchestration job awaiting operator review (FP-04).
type TaskDeadLetter struct {
	ID               string
	SourceType       string
	SourceID         string
	TeamID           string
	TeamRunID        string
	SessionID        string
	GraphExecutionID string
	ErrorMessage     string
	PayloadJSON      string
	Status           string
	CreatedAt        string
	ResolvedAt       string
}

const (
	TaskDeadLetterStatusPending  = "pending"
	TaskDeadLetterStatusResolved = "resolved"
	TaskDeadLetterSourceTeamRun  = "team_run"
)

// TaskDeadLetterListFilter scopes admin list queries (FP-04).
type TaskDeadLetterListFilter struct {
	SessionID string
	TeamID    string
	Status    string
	Limit     int
}

// ShouldRecordTaskDeadLetter returns true when failure_policy.on_error=halt.
func ShouldRecordTaskDeadLetter(rawDefinitionJSON string) bool {
	spec, err := ParseOrchestrationSpec(rawDefinitionJSON)
	if err != nil || spec.FailurePolicy == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(spec.FailurePolicy.OnError), FailurePolicyHalt)
}
