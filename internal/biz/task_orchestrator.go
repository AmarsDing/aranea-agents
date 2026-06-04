package biz

import "context"

// TaskOrchestratorPort is the port interface for the TaskOrchestrator (Phase 3 of Spirit orchestration).
// Single responsibility: build execution graph from TaskPlan + AllocationPlan and execute it.
type TaskOrchestratorPort interface {
	Orchestrate(ctx context.Context, taskPlan *TaskPlan, allocPlan *AllocationPlan) (*OrchestrationHandle, error)
	CheckProgress(ctx context.Context, orchestrationID string) ([]TaskProgress, error)
	Cancel(ctx context.Context, orchestrationID string) error
	Synthesize(ctx context.Context, orchestrationID string) (*SynthesisOutput, error)
	Recover(ctx context.Context, orchestrationID string) error
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
	Status              OrchestrationStatus
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
	Status      string  `json:"status"` // pending/running/completed/failed
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
