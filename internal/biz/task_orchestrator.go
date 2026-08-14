package biz

import (
	"context"
	"strings"
)

// CancelReason 标识编排/团队取消的原因类型（P2-6）。
// 用于事件 meta、日志和前端展示，区分用户主动取消、超时熔断、错误级联等场景。
type CancelReason string

const (
	// CancelReasonUser 是用户主动点击停止/调用 cancel_orchestration。
	CancelReasonUser CancelReason = "user_cancel"
	// CancelReasonTimeout 是执行超时（如 team timeout timer 触发）。
	CancelReasonTimeout CancelReason = "timeout"
	// CancelReasonError 是执行错误导致的取消（如子任务失败级联）。
	CancelReasonError CancelReason = "error"
	// CancelReasonDoomLoop 是 doom-loop 检测触发的早退取消。
	CancelReasonDoomLoop CancelReason = "doom_loop"
	// CancelReasonParent 是父级编排取消导致的级联取消。
	CancelReasonParent CancelReason = "parent_cancel"
	// CancelReasonUnknown 是未指定/默认原因。
	CancelReasonUnknown CancelReason = ""
)

// NormalizeCancelReason 归一化取消原因字符串，非法值回落到 Unknown。
func NormalizeCancelReason(s string) CancelReason {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(CancelReasonUser):
		return CancelReasonUser
	case string(CancelReasonTimeout):
		return CancelReasonTimeout
	case string(CancelReasonError):
		return CancelReasonError
	case string(CancelReasonDoomLoop):
		return CancelReasonDoomLoop
	case string(CancelReasonParent):
		return CancelReasonParent
	}
	return CancelReasonUnknown
}

// TaskOrchestratorPort is the port interface for the TaskOrchestrator (Phase 3 of Spirit orchestration).
// Single responsibility: build execution graph from TaskPlan + AllocationPlan and execute it.
type TaskOrchestratorPort interface {
	Orchestrate(ctx context.Context, taskPlan *TaskPlan, allocPlan *AllocationPlan) (*OrchestrationHandle, error)
	CheckProgress(ctx context.Context, orchestrationID string) ([]TaskProgress, error)
	Cancel(ctx context.Context, orchestrationID string, reason CancelReason) error
	Synthesize(ctx context.Context, orchestrationID string) (*SynthesisOutput, error)
	Recover(ctx context.Context, orchestrationID string) error
	RecoverAllInterrupted(ctx context.Context) error
}

// RecoveredPlanConsumer is optionally implemented by TaskOrchestratorPort
// to hand back Phase 1 (TaskPlan) / Phase 2 (AllocationPlan) rows restored
// after process interruption. Consume is once-per-session: the next
// plan_and_execute in that spirit session reuses the persisted plan instead
// of calling the planner LLM again.
//
// Stability: evolving
type RecoveredPlanConsumer interface {
	ConsumeRecoveredPlan(spiritSessionID, userMessage string) (plan *TaskPlan, alloc *AllocationPlan, ok bool)
}

// OrchestrationHandle represents a running orchestration.
type OrchestrationHandle struct {
	ID                  string
	TaskPlanID          string
	AllocationID        string
	SpiritSessionID     string
	TraceID             string
	Strategy            OrchestrationStrategy
	GraphExecutionID    string
	TeamIDs             []string
	AgentKeys           []string // Real agent keys from AllocationPlan, used for performance tracking
	Status              OrchestrationStatus
	CancelReason        CancelReason // P2-6：取消原因（仅 Status=Cancelled 时有值）
	CheckpointID        string
	SynthesisResultJSON string
	CreatedAt           string
	UpdatedAt           string
}

// OrchestrationStatus represents the status of an orchestration.
type OrchestrationStatus string

const (
	OrchestrationStatusPending     OrchestrationStatus = "pending"
	OrchestrationStatusRunning     OrchestrationStatus = "running"
	OrchestrationStatusCompleted   OrchestrationStatus = "completed"
	OrchestrationStatusFailed      OrchestrationStatus = "failed"
	OrchestrationStatusCancelled   OrchestrationStatus = "cancelled"
	OrchestrationStatusInterrupted OrchestrationStatus = "interrupted"
)

// TaskProgress represents the progress of a single task in the orchestration.
type TaskProgress struct {
	SubTaskID   string  `json:"sub_task_id"`
	SubTaskName string  `json:"sub_task_name"`
	AgentKey    string  `json:"agent_key"`
	Status      string  `json:"status"`   // pending/running/completed/failed
	Progress    float64 `json:"progress"` // 0.0-1.0
	Result      string  `json:"result,omitempty"`
}

// AssignedType represents whether a subtask is assigned to an agent or a team.
type AssignedType string

const (
	AssignedTypeAgent AssignedType = "agent"
	AssignedTypeTeam  AssignedType = "team"
)

// OrchestrationRepository is the repository interface for OrchestrationHandle persistence.
type OrchestrationRepository interface {
	Create(ctx context.Context, handle *OrchestrationHandle) (*OrchestrationHandle, error)
	GetByID(ctx context.Context, id string) (*OrchestrationHandle, error)
	Update(ctx context.Context, handle *OrchestrationHandle) (*OrchestrationHandle, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*OrchestrationHandle, error)
	ListByStatus(ctx context.Context, status OrchestrationStatus) ([]*OrchestrationHandle, error)
}
