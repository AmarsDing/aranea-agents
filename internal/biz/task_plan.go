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

	// ClarificationQuestions 是分解层澄清出口的阻塞性问题（Q7，
	// session-eval-20260827 P4 根修）：分解 LLM 判定任务存在阻塞性信息
	// 缺失（产品名/日期/渠道等）时选择提问而非虚构。内存字段，不持久化——
	// 澄清计划不进入执行管线，问题由 plan_and_execute 立即透传给 Spirit
	// 向用户提问，落库仅留 DecomposeReason=needs_clarification 痕迹。
	ClarificationQuestions []string `json:"-"`
}

// NeedsClarification reports whether the plan is parked pending user
// clarification (decomposition-layer clarification exit, Q7). Such plans
// must not be allocated/orchestrated — the caller surfaces the questions.
func (p *TaskPlan) NeedsClarification() bool {
	return p != nil && len(p.ClarificationQuestions) > 0
}

// Decompose reason values persisted on TaskPlan.DecomposeReason.
const (
	DecomposeReasonFailed       = "decompose_failed"
	DecomposeReasonEmpty        = "decompose_empty"
	DecomposeReasonVerifyFailed = "verify_failed"
	// DecomposeReasonDeferred means assess+draft finished; LLM decompose
	// continues on ResumePlanID so Spirit can speak during planning.
	DecomposeReasonDeferred = "deferred_decompose"
)

// PlanningInProgressNextAction is returned by plan_and_execute when the
// planner LLM is still running in the background after the tool ACK.
const PlanningInProgressNextAction = "planning_in_progress"

// PlanningInProgressUserHint tells Spirit not to re-enter plan_and_execute.
const PlanningInProgressUserHint = "任务正在分解规划中，请向用户说明正在组建方案，等待编排进度事件后再汇报结果。不要立刻再次调用 plan_and_execute。"

// AwaitOrchestrationNextAction is returned when a committed PlanTeam lane
// has started (or is starting) orchestration. Spirit must not write the
// user-facing deliverable in this turn — the team owns that work.
const AwaitOrchestrationNextAction = "await_orchestration"

// AwaitOrchestrationUserHint is the user-facing status Spirit should relay.
const AwaitOrchestrationUserHint = "编排已启动，团队正在执行。请向用户说明已派发，等待团队交付后再汇报结果。禁止在本轮直接撰写交付物正文。"

// DecomposeFailedNextAction is returned by plan_and_execute when medium+
// decomposition fails. Spirit must not treat StrategyDirect as success.
const DecomposeFailedNextAction = "decompose_failed"

// DecomposeFailedUserHint explains the fail-closed decompose path to Spirit.
const DecomposeFailedUserHint = "任务分解未完成，不能改为单人直接作答。请向用户说明后重试 plan_and_execute（可指定 mode=parallel/dag），或改用 build_orchestration_graph 显式组队。"

// DecomposeFailed reports whether decomposition was attempted and did not
// produce an executable plan. Callers must not silently run StrategyDirect.
func (p *TaskPlan) DecomposeFailed() bool {
	if p == nil {
		return false
	}
	switch p.DecomposeReason {
	case DecomposeReasonFailed, DecomposeReasonEmpty, DecomposeReasonVerifyFailed:
		return true
	}
	return false
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
	// GraphTemplateID optionally routes this stage through an existing M53
	// template. Empty = ordinary Team Turn. Never invents a second engine.
	GraphTemplateID string `json:"graph_template_id,omitempty"`
	// ConfirmBefore is R18 playbook_confirm_before: hold dispatch until the
	// user approves. Default stage handoff stays Brief-only.
	ConfirmBefore bool `json:"confirm_before,omitempty"`
	// CollectionIDs are knowledge bases this stage may search. Empty = no
	// pre-scope (agent tools fall back to department/default routing).
	CollectionIDs []string `json:"collection_ids,omitempty"`
	// DepartmentKey is the playbook-declared department for this stage
	// (used to resolve the correct dept_lead agent). Empty = no override.
	DepartmentKey string `json:"department_key,omitempty"`
}

// PlanTaskDAG represents the dependency graph of subtasks within a TaskPlan.
type PlanTaskDAG struct {
	Nodes   []SubTask `json:"nodes"`
	RootIDs []string  `json:"root_ids"` // nodes with no dependencies
	LeafIDs []string  `json:"leaf_ids"` // nodes nothing depends on
}

// MemoryHit represents a memory cache hit from OrchestrationCache
type MemoryHit struct {
	CacheID               string   `json:"cache_id"`
	DQScore               float64  `json:"dq_score"`
	TopologyUsed          string   `json:"topology_used"`
	AgentKeysUsed         []string `json:"agent_keys_used"`
	DomainPath            string   `json:"domain_path,omitempty"`
	Specialties           []string `json:"specialties,omitempty"`
	ConstraintFingerprint string   `json:"constraint_fingerprint,omitempty"`
	PlaybookID            string   `json:"playbook_id,omitempty"`
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
