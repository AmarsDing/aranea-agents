package biz

import "context"

// TaskPlannerPort is the port interface for the TaskPlanner (Phase 1 of Spirit orchestration).
// Single responsibility: assess task complexity + decompose tasks + output strategy.
type TaskPlannerPort interface {
	Plan(ctx context.Context, input PlanInput) (*TaskPlan, error)
	GetPlan(ctx context.Context, planID string) (*TaskPlan, error)
	ConfirmPlan(ctx context.Context, planID string, adjustments PlanAdjustments) (*TaskPlan, error)
}

// IntentArtifact mirrors the intent pass output fields consumed by TaskPlanner.
// Defined in biz to avoid import cycles (internal/agent/intent → internal/agent → internal/biz).
type IntentArtifact struct {
	RefinedGoal       string   `json:"refined_goal"`
	IntentKind        string   `json:"intent_kind"`
	SuccessCriteria   []string `json:"success_criteria"`
	Ambiguities       []string `json:"ambiguities"`
	SearchHints       []string `json:"search_hints"`
	RiskFlags         []string `json:"risk_flags"`
	ComplexityScore   float64  `json:"complexity_score,omitempty"`
	ComplexitySignals []string `json:"complexity_signals,omitempty"`
	SuggestedAgents   []string `json:"suggested_agents,omitempty"`
	SuggestedTopology string   `json:"suggested_topology,omitempty"`
}

// PlanInput is the input to TaskPlanner.Plan
type PlanInput struct {
	UserMessage     string
	SpiritSessionID string
	TraceID         string
	IntentArtifact  *IntentArtifact // from intent pass, converted to biz type
	HistoryDQScore  float64         // from OrchestrationCache, 0 if no history
}

// PlanAdjustments allows Spirit LLM to adjust the plan
type PlanAdjustments struct {
	MergeSubTasks    []string    // subtask IDs to merge
	SplitSubTask     string      // subtask ID to split
	AddSubTask       *SubTaskSpec // new subtask to add
	RemoveSubTask    string      // subtask ID to remove
	StrategyOverride string      // override the suggested strategy
	Reason           string
}

// SubTaskSpec is a specification for a new subtask
type SubTaskSpec struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	RequiredCapabilities []string `json:"required_capabilities"`
	DependsOn            []string `json:"depends_on"`
	Priority             int      `json:"priority"`
}
