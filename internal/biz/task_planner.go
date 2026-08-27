package biz

import "context"

// TaskPlannerPort is the port interface for the TaskPlanner (Phase 1 of Spirit orchestration).
// Single responsibility: assess task complexity + decompose tasks + output strategy.
//
// Stability: evolving
type TaskPlannerPort interface {
	// Plan assesses complexity, optionally decomposes, persists, and outputs a strategy.
	Plan(ctx context.Context, input PlanInput) (*TaskPlan, error)
	// QuickAssess performs a pure-computation complexity assessment (no LLM, no DB),
	// used by the pre-planning gate (P1-2) to force planning for Moderate/Complex tasks.
	QuickAssess(ctx context.Context, input PlanInput) (ComplexityLevel, float64, error)
	GetPlan(ctx context.Context, planID string) (*TaskPlan, error)
	// ListPlans returns all plans for a spirit session, newest first (T3.2).
	ListPlans(ctx context.Context, spiritSessionID string) ([]*TaskPlan, error)
	ConfirmPlan(ctx context.Context, planID string, adjustments PlanAdjustments) (*TaskPlan, error)
	// PublishV2Board publishes v2 PlanBoard/PlanStep/GraphStage/GraphNode creation
	// events via the v2 Sequencer. Must be called AFTER Phase 2 (Allocate) so
	// PlanStep.AgentKeys can be filled from allocPlan. For direct strategy
	// (no team execution), pass nil allocPlan — AgentKeys stays empty.
	// 2026-07-05 Step 3 修复：原先在 Plan() 内部 Phase 1 发布，导致 allocPlan
	// 不存在时 PlanStep.AgentKeys 为空，RealTeamOrchestrator 退回查 DB 取错 agent。
	//
	// C-18: returns the created PlanBoard (ID is the canonical orchestration_id).
	// When publishing is skipped (nil seq/plan, empty SubTasks), returns a zero
	// PlanBoard and nil error.
	PublishV2Board(ctx context.Context, plan *TaskPlan, allocPlan *AllocationPlan, chatSessionID string) (PlanBoard, error)
}

// IntentArtifact mirrors the intent pass output fields consumed by TaskPlanner.
// Defined in biz to avoid import cycles (internal/agent/intent → internal/agent → internal/biz).
type IntentArtifact struct {
	RefinedGoal     string   `json:"refined_goal"`
	IntentKind      string   `json:"intent_kind"`
	SuccessCriteria []string `json:"success_criteria"`
	Ambiguities     []string `json:"ambiguities"`
	SearchHints     []string `json:"search_hints"`
	ToolHints       []string `json:"tool_hints,omitempty"`
	RiskFlags       []string `json:"risk_flags"`
}

// PlanInput is the input to TaskPlanner.Plan
type PlanInput struct {
	UserMessage     string
	SpiritSessionID string
	// TaskID 是本 turn 所属根 Task 的 ID（根 turn 为预生成 ID，续跑 turn 继承
	// 父 Task），由调用方从 ctx RootTaskActivityID 解析。门控 notice 经此挂接
	// 到 Task，避免成为永不渲染的 session 级孤儿步骤。
	TaskID string
	// ChatSessionID is the chat (parent) session ID used when publishing plan
	// activities. The plan must appear in the chat session timeline so the
	// frontend can receive real-time status updates via WebSocket (the WS
	// subscription filters by chat session ID). Falls back to SpiritSessionID
	// when empty (pre-existing behavior).
	ChatSessionID  string
	TraceID        string
	IntentArtifact *IntentArtifact // from intent pass, converted to biz type
	HistoryDQScore float64         // from OrchestrationCache, 0 if no history
	// Mode is the explicit execution mode from plan_and_execute tool input.
	// Values: "" / "auto" (system decides via complexity), "direct", "single",
	// "single_agent", "parallel", "dag", "coordinator".
	// When set to a team-forming mode (parallel/dag/coordinator), it overrides
	// complexity-based strategy selection and forces subtask decomposition.
	Mode string
	// AgentKeys 是 plan_and_execute 的显式路由键（IDENTITY.md 契约：系统
	// 管家任务 agent_keys=["__system_admin__"]）。组队证据闸（包C Q2-C2）
	// 跳过显式路由——键路由本身即用户/系统契约的组队证据，无需 lexical
	// 证据兜底。仅工具入口在键路由升级 mode=parallel 时透传；其他调用方
	// 留空，证据闸照常生效。
	AgentKeys []string
}

// PlanAdjustments allows Spirit LLM to adjust the plan
type PlanAdjustments struct {
	MergeSubTasks    []string     // subtask IDs to merge
	SplitSubTask     string       // subtask ID to split
	AddSubTask       *SubTaskSpec // new subtask to add
	RemoveSubTask    string       // subtask ID to remove
	StrategyOverride string       // override the suggested strategy
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
