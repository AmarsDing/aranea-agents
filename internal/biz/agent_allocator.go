package biz

import "context"

// AgentAllocatorPort is the port interface for the AgentAllocator (Phase 2 of Spirit orchestration).
// Single responsibility: match each subtask in a TaskPlan to the best Agent or Team.
type AgentAllocatorPort interface {
	Allocate(ctx context.Context, taskPlan *TaskPlan) (*AllocationPlan, error)
	GetAllocation(ctx context.Context, allocationID string) (*AllocationPlan, error)
}

// AllocationPlan is the output of AgentAllocator.Allocate
type AllocationPlan struct {
	ID              string
	TaskPlanID      string
	SpiritSessionID string
	TraceID         string
	Allocations     []TaskAllocation
	Status          AllocationStatus
	CreatedAt       string
	UpdatedAt       string
}

// AllocationStatus represents the status of an AllocationPlan
type AllocationStatus string

const (
	AllocationStatusDraft     AllocationStatus = "draft"
	AllocationStatusConfirmed AllocationStatus = "confirmed"
	AllocationStatusExecuting AllocationStatus = "executing"
)

// TaskAllocation represents the allocation of a subtask to an agent or team
type TaskAllocation struct {
	SubTaskID      string   `json:"sub_task_id"`
	SubTaskName    string   `json:"sub_task_name"`
	AssignedType   string   `json:"assigned_type"` // "agent" or "team"
	AssignedKey    string   `json:"assigned_key"`  // agent_key or team definition key
	AssignedName   string   `json:"assigned_name"`
	MatchScore     float64  `json:"match_score"`
	MatchLayer     string   `json:"match_layer"` // "exact", "semantic", "llm_cold_start"
	MatchReason    string   `json:"match_reason"`
	FallbackKey    string   `json:"fallback_key,omitempty"`
	FallbackScore  float64  `json:"fallback_score,omitempty"`
	TeamMode       string   `json:"team_mode,omitempty"` // coordinator/sequential/parallel
	TeamMemberKeys []string `json:"team_member_keys,omitempty"`
}

// AllocationPlanRepository is the repository interface for AllocationPlan persistence
type AllocationPlanRepository interface {
	Create(ctx context.Context, plan *AllocationPlan) (*AllocationPlan, error)
	GetByID(ctx context.Context, id string) (*AllocationPlan, error)
	Update(ctx context.Context, plan *AllocationPlan) (*AllocationPlan, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*AllocationPlan, error)
}
