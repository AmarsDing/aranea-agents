package biz

import (
	"context"
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
	StrategyDirect      OrchestrationStrategy = "direct"      // Spirit answers directly
	StrategySingleAgent OrchestrationStrategy = "single_agent" // Agent-as-Tool
	StrategyParallel    OrchestrationStrategy = "parallel"     // ParallelAgent
	StrategyDAG         OrchestrationStrategy = "dag"          // GraphAgent DAG
	StrategyCoordinator OrchestrationStrategy = "coordinator"  // Team(ModeCoordinator)
)

// TaskPlan is the output of the TaskPlanner
type TaskPlan struct {
	ID                string `json:"id"`
	SpiritSessionID   string `json:"spirit_session_id"`
	TraceID           string `json:"trace_id"`
	UserMessage       string `json:"user_message"`
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

	// Memory hit
	MemoryHit *MemoryHit `json:"memory_hit"`

	// Status (reuses PlanStatus from plan.go)
	Status PlanStatus `json:"status"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
}

// PlanTaskDAG represents the dependency graph of subtasks within a TaskPlan.
// This is distinct from the existing TaskDAG used for team orchestration.
type PlanTaskDAG struct {
	Nodes   []SubTask `json:"nodes"`
	RootIDs []string  `json:"root_ids"` // nodes with no dependencies
	LeafIDs []string  `json:"leaf_ids"` // nodes nothing depends on
}

// MemoryHit represents a memory cache hit from OrchestrationCache
type MemoryHit struct {
	CacheID       string  `json:"cache_id"`
	DQScore       float64 `json:"dq_score"`
	TopologyUsed  string  `json:"topology_used"`
	AgentKeysUsed []string `json:"agent_keys_used"`
}

// TaskPlanRepository is the repository interface for TaskPlan persistence
type TaskPlanRepository interface {
	Create(ctx context.Context, plan *TaskPlan) (*TaskPlan, error)
	GetByID(ctx context.Context, id string) (*TaskPlan, error)
	Update(ctx context.Context, plan *TaskPlan) (*TaskPlan, error)
	ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*TaskPlan, error)
}
