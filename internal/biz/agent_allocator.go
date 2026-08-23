package biz

import "context"

// AgentAllocatorPort is the port interface for the AgentAllocator (Phase 2 of Spirit orchestration).
// Single responsibility: match each subtask in a TaskPlan to the best Agent or Team.
type AgentAllocatorPort interface {
	Allocate(ctx context.Context, taskPlan *TaskPlan) (*AllocationPlan, error)
	// AllocateExplicit assigns the caller-specified agent keys directly,
	// bypassing heuristic matching. Used when the Spirit LLM explicitly routes
	// a task to designated agents (IDENTITY.md contract: plan_and_execute +
	// agent_keys=["__system_admin__"] for system-butler tasks). Heuristic
	// layers can never select system agents (they are filtered at source), so
	// explicit routing is the only path to them.
	// agentKeys[0] is the executor (lead); remaining keys join as team members
	// in dag strategy.
	AllocateExplicit(ctx context.Context, taskPlan *TaskPlan, agentKeys []string) (*AllocationPlan, error)
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
	// DepartmentID is the home department for the assembled team (M78).
	DepartmentID string `json:"department_id,omitempty"`
	CompanyID    string `json:"company_id,omitempty"`
	// CrossDeptMemberKeys are team members whose department differs from
	// DepartmentID (borrow candidates). Empty when all members are in-dept.
	CrossDeptMemberKeys []string `json:"cross_dept_member_keys,omitempty"`
}

// AllocationPlanRepository is the repository interface for AllocationPlan persistence
type AllocationPlanRepository interface {
	Create(ctx context.Context, plan *AllocationPlan) (*AllocationPlan, error)
	GetByID(ctx context.Context, id string) (*AllocationPlan, error)
	Update(ctx context.Context, plan *AllocationPlan) (*AllocationPlan, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*AllocationPlan, error)
}

// StaffingAdvisor is an optional department-lead consult before AgentFactory
// (M78 R5 / ORGFAST-21). It must not re-decompose the user's original task.
// Stability:evolving
type StaffingAdvisor interface {
	Suggest(ctx context.Context, in StaffingAsk) (StaffingReply, error)
}

// StaffingAsk is the single staffing question: pick from candidates or Factory.
type StaffingAsk struct {
	DepartmentID  string
	DomainPath    string
	SubTaskName   string
	CandidateKeys []string
	// CandidateCards is optional "key|name|domain|mission" lines so the
	// lead can pick by specialty, not opaque keys.
	CandidateCards []string
}

// StaffingReply is the lead's suggestion. AgentKeys must be a subset of
// CandidateKeys; UseFactory means skip to AgentFactory.
type StaffingReply struct {
	AgentKeys  []string
	UseFactory bool
}
