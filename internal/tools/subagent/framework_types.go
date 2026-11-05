package subagent

import (
	"errors"
	"time"
)

// 本文件承载原 trpc-agent-go/openclaw/subagent 的控制面类型（框架库化迁移
// 2026-11-03：官方库未包含这些类型的项目定制字段 Kind/Workspace，故下沉为
// 应用自有类型）。仅保留应用实际使用的视图，不再依赖 openclaw 模块。

// ErrRunNotFound indicates that a subagent run does not exist.
var ErrRunNotFound = errors.New("subagent: run not found")

// Status describes the lifecycle state of a subagent run.
type Status string

const (
	// StatusQueued means the run was accepted but has not started yet.
	StatusQueued Status = "queued"
	// StatusRunning means the child agent is executing.
	StatusRunning Status = "running"
	// StatusFinalizing means the child agent exited and final metadata is
	// being attached.
	StatusFinalizing Status = "finalizing"
	// StatusCanceling means cancellation was requested and the child agent
	// has not exited yet.
	StatusCanceling Status = "canceling"
	// StatusCompleted means the child agent completed successfully.
	StatusCompleted Status = "completed"
	// StatusFailed means the child agent failed.
	StatusFailed Status = "failed"
	// StatusCanceled means the child agent exited after cancellation.
	StatusCanceled Status = "canceled"
)

// IsTerminal reports whether the status will no longer change under normal
// execution.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}

// Workspace describes the workspace lease attached to a subagent run.
type Workspace struct {
	Isolation string `json:"isolation,omitempty"`
	Path      string `json:"path,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Cleanup   string `json:"cleanup,omitempty"`
}

// Run is the product-facing view of one subagent run.
type Run struct {
	ID              string     `json:"id,omitempty"`
	ParentSessionID string     `json:"parent_session_id,omitempty"`
	ChildSessionID  string     `json:"child_session_id,omitempty"`
	Task            string     `json:"task,omitempty"`
	Kind            string     `json:"kind,omitempty"`
	Status          Status     `json:"status,omitempty"`
	Summary         string     `json:"summary,omitempty"`
	Result          string     `json:"result,omitempty"`
	Error           string     `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	Workspace       *Workspace `json:"workspace,omitempty"`
}

// ListFilter limits the subagent runs returned by ListForUser.
type ListFilter struct {
	ParentSessionID string
	Status          Status
}
