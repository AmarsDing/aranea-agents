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
	// TeamIDs 是 workspace 维度的团队集合过滤（个人工作台告警 lane 无具体
	// team/session 上下文时，按调用方可见团队批量收敛）。非空时与 TeamID
	// 互斥使用（service 层只填其一）。
	TeamIDs []string
	Status  string
	Limit   int
}

// ShouldRecordTaskDeadLetter returns true when failure_policy.on_error=halt.
func ShouldRecordTaskDeadLetter(rawDefinitionJSON string) bool {
	spec, err := ParseOrchestrationSpec(rawDefinitionJSON)
	if err != nil || spec.FailurePolicy == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(spec.FailurePolicy.OnError), FailurePolicyHalt)
}
