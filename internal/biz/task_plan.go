package biz

import (
	"context"
	"strings"
	"time"
)

// ComplexityLevel represents task complexity
type ComplexityLevel string

const (
	ComplexitySimple   ComplexityLevel = "simple"
	ComplexityModerate ComplexityLevel = "moderate"
	ComplexityComplex  ComplexityLevel = "complex"
)

// OrchestrationStrategy represents the execution strategy
type OrchestrationStrategy string

const (
	StrategyDirect      OrchestrationStrategy = "direct"       // Spirit answers directly
	StrategySingleAgent OrchestrationStrategy = "single_agent" // Agent-as-Tool
	StrategyParallel    OrchestrationStrategy = "parallel"     // ParallelAgent
	StrategyDAG         OrchestrationStrategy = "dag"          // GraphAgent DAG
	StrategyCoordinator OrchestrationStrategy = "coordinator"  // Team(ModeCoordinator)
)

// TaskPlanStatus represents the lifecycle state of a TaskPlan.
// (Migrated from plan.go's LegacyPlanStatus — the values are unchanged
// to preserve DB compatibility with the task_plans.status column.)
type TaskPlanStatus string

const (
	TaskPlanStatusDraft     TaskPlanStatus = "draft"
	TaskPlanStatusApproved  TaskPlanStatus = "approved"
	TaskPlanStatusConfirmed TaskPlanStatus = "confirmed"
	TaskPlanStatusExecuting TaskPlanStatus = "executing"
	TaskPlanStatusCompleted TaskPlanStatus = "completed"
	TaskPlanStatusFailed    TaskPlanStatus = "failed"
)

// TaskPlan is the output of the TaskPlanner
type TaskPlan struct {
	ID                 string `json:"id"`
	SpiritSessionID    string `json:"spirit_session_id"`
	TraceID            string `json:"trace_id"`
	UserMessage        string `json:"user_message"`
	IntentArtifactJSON string `json:"intent_artifact_json"`

	// Complexity assessment
	ComplexityLevel ComplexityLevel `json:"complexity_level"`
	ComplexityScore float64         `json:"complexity_score"`
	Dimensions      DimensionScores `json:"dimensions"`

	// Task decomposition (only for complex)
	SubTasks        []SubTask    `json:"sub_tasks"`
	TaskDAG         *PlanTaskDAG `json:"task_dag"`
	DecomposeReason string       `json:"decompose_reason"`

	// Strategy
	Strategy       OrchestrationStrategy `json:"strategy"`
	StrategyReason string                `json:"strategy_reason"`
	TopologyHint   TopologyType          `json:"topology_hint"`

	// DomainPath 是 plan 级主导域（PrimaryDomainPath(SubTasks) 推导，内存字段，
	// 不持久化——重读 plan 时由 SubTasks 重推导）。
	DomainPath string `json:"domain_path,omitempty"`

	// Memory hit
	MemoryHit *MemoryHit `json:"memory_hit"`

	// Status is the lifecycle state of the plan.
	Status TaskPlanStatus `json:"status"`

	// P2-B: tenant isolation. empty = legacy (treated as default workspace); non-empty = tenant-private.
	WorkspaceID string `json:"workspace_id"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// StreamPublished 标记 PlanBoard/PlanStep/GraphStage/GraphNode 事件
	// 是否已通过流式分解（decomposeTaskStream）渐进发布。为 true 时
	// PublishV2Board 仅做 AgentKeys 更新（PlanStepUpdatedEvent），不重复
	// 发布 Created 事件。内存字段，不持久化。
	StreamPublished bool `json:"-"`
}

// DimensionScores holds the six-dimension complexity assessment
type DimensionScores struct {
	Semantic   float64 `json:"semantic"`   // 0.25 weight
	Structural float64 `json:"structural"` // 0.15 weight
	Domain     float64 `json:"domain"`     // 0.15 weight (Phase 3: cross-domain)
	Tool       float64 `json:"tool"`       // 0.10 weight
	Context    float64 `json:"context"`    // 0.10 weight
	Historical float64 `json:"historical"` // 0.25 weight
}

// SubTask represents a decomposed sub-task
type SubTask struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	DependsOn            []string `json:"depends_on"`
	RequiredCapabilities []string `json:"required_capabilities"`
	Priority             int      `json:"priority"`
	EstimatedComplexity  float64  `json:"estimated_complexity"`
	// P1 形式契约（B.10.15.2）：LLM 输出 + 确定性兜底派生；advisory，不阻断。
	Deliverables  []DeliverableContract `json:"deliverables,omitempty"`
	InputContract []DeliverableContract `json:"input_contract,omitempty"`
	// DomainPath 是 planner LLM 顺带输出的归一化领域路径（B.10.21.4）。
	// advisory：空不阻断，匹配管线落回旧行为。
	DomainPath string `json:"domain_path,omitempty"`
}

// PlanTaskDAG represents the dependency graph of subtasks within a TaskPlan.
type PlanTaskDAG struct {
	Nodes   []SubTask `json:"nodes"`
	RootIDs []string  `json:"root_ids"` // nodes with no dependencies
	LeafIDs []string  `json:"leaf_ids"` // nodes nothing depends on
}

// MemoryHit represents a memory cache hit from OrchestrationCache
type MemoryHit struct {
	CacheID       string   `json:"cache_id"`
	DQScore       float64  `json:"dq_score"`
	TopologyUsed  string   `json:"topology_used"`
	AgentKeysUsed []string `json:"agent_keys_used"`
}

// RecoverableTaskPlanStatuses are non-terminal plan states that may be
// reloaded after process interruption (P1-10). completed/failed are terminal
// and must not be resumed as the active plan.
var RecoverableTaskPlanStatuses = []TaskPlanStatus{
	TaskPlanStatusDraft,
	TaskPlanStatusApproved,
	TaskPlanStatusConfirmed,
	TaskPlanStatusExecuting,
}

// IsRecoverableTaskPlanStatus reports whether a plan may be restored after interruption.
func IsRecoverableTaskPlanStatus(s TaskPlanStatus) bool {
	switch s {
	case TaskPlanStatusDraft, TaskPlanStatusApproved, TaskPlanStatusConfirmed, TaskPlanStatusExecuting:
		return true
	}
	return false
}

// TaskPlanRepository is the repository interface for TaskPlan persistence
type TaskPlanRepository interface {
	Create(ctx context.Context, plan *TaskPlan) (*TaskPlan, error)
	GetByID(ctx context.Context, id string) (*TaskPlan, error)
	Update(ctx context.Context, plan *TaskPlan) (*TaskPlan, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*TaskPlan, error)
	// ListByStatuses returns plans whose status is in the given set, newest first.
	// Used by startup recovery to reload interrupted Phase 1 drafts.
	ListByStatuses(ctx context.Context, statuses []TaskPlanStatus) ([]*TaskPlan, error)
}

// SpiritTeamModeForStep maps planner/board strategy + member count to the
// intra-team Graph mode. Inter-team DAG stays on PlanExecutor; this only
// chooses how one step's members are wired (sequential / parallel / coordinator).
func SpiritTeamModeForStep(strategy string, agentCount int) string {
	if agentCount <= 1 {
		return TeamModeSequential
	}
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "parallel":
		return TeamModeParallel
	case "coordinator":
		return TeamModeCoordinator
	default:
		return TeamModeCoordinator
	}
}
