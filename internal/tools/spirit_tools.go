package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	biztypes "aranea-agents/internal/biz/types"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"

	"aranea-agents/pkg/apierror"
)

// ---------------------------------------------------------------------------
// New three-phase orchestration tools (T3.1)
// ---------------------------------------------------------------------------

// PlanAndExecuteInput is the input for the plan_and_execute tool.
type PlanAndExecuteInput struct {
	TaskPrompt string `json:"task_prompt" jsonschema:"description=The task to plan and execute"`
	Mode       string `json:"mode,omitempty" jsonschema:"description=Execution mode: direct (Spirit answers directly, no delegation), parallel (N independent single-agent subtasks run concurrently, 1 agent per subtask, NO multi-member teams), dag (N teams with dependency graph, each team has >=2 members collaborating). Must be explicitly set; auto/single/coordinator are deprecated. Default: direct"`
	// AgentKeys explicitly routes the task to designated agents, bypassing
	// heuristic allocation (IDENTITY.md contract: system-butler tasks use
	// agent_keys=["__system_admin__"]). Heuristic matching can never select
	// system agents, so explicit routing is the only path to them.
	// agent_keys[0] executes; remaining keys join as team members in dag mode.
	AgentKeys []string `json:"agent_keys,omitempty" jsonschema:"description=Explicit agent routing: agent_keys[0] executes the task (e.g. [\"__system_admin__\"] for Skill/MCP/industry management, [\"__memory__\"] for memory tasks, [\"__skills__\"] for skill-evolution tasks). Bypasses heuristic agent matching. Required when delegating to system butlers."`
}

// SubTaskSummary is a summary of a subtask in the plan.
type SubTaskSummary struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	AgentKey  string   `json:"agent_key,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// PlanAndExecuteOutput is the output for the plan_and_execute tool.
type PlanAndExecuteOutput struct {
	PlanID          string                             `json:"plan_id"`
	Strategy        string                             `json:"strategy"`
	ComplexityLevel string                             `json:"complexity_level"`
	SubtaskCount    int                                `json:"subtask_count"`
	SubTasks        []SubTaskSummary                   `json:"sub_tasks,omitempty"`
	OrchestrationID string                             `json:"orchestration_id,omitempty"`
	MemoryHit       bool                               `json:"memory_hit"`
	Steps           []biztypes.OrchestrationStepRecord `json:"steps,omitempty"`
}

// planAndExecuteDeps holds the shared dependencies for plan_and_execute phases.
type planAndExecuteDeps struct {
	planner            biz.TaskPlannerPort
	allocator          biz.AgentAllocatorPort
	orchestrator       biz.TaskOrchestratorPort
	teamQuery          SpiritTeamQueryPort
	sessionModelLookup SessionModelLookup
	bus                biz.EventBus // Phase 3b-D: v2 EventBus
	lg                 loggateway.Logger
}

// SessionModelLookup resolves the effective provider/model for a Spirit session.
// Used by plan_and_execute to inject the session model into the context for
// "inherit" mode LLM resolution in the planner/allocator.
type SessionModelLookup interface {
	GetSessionModel(ctx context.Context, sessionID string) (provider, model string)
}

// NewPlanAndExecuteTool creates the plan_and_execute tool that replaces
// assess_complexity + assemble_team + list_butlers + query_butler_status.
func NewPlanAndExecuteTool(planner biz.TaskPlannerPort, allocator biz.AgentAllocatorPort, orchestrator biz.TaskOrchestratorPort, teamQuery SpiritTeamQueryPort, sessionModelLookup SessionModelLookup, bus biz.EventBus, lg loggateway.Logger) *trpcfunction.FunctionTool[PlanAndExecuteInput, PlanAndExecuteOutput] {
	deps := planAndExecuteDeps{
		planner:            planner,
		allocator:          allocator,
		orchestrator:       orchestrator,
		teamQuery:          teamQuery,
		sessionModelLookup: sessionModelLookup,
		bus:                bus,
		lg:                 lg,
	}

	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input PlanAndExecuteInput) (PlanAndExecuteOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return PlanAndExecuteOutput{}, apierror.BadRequest(apierror.DomainSpirit, "spirit session id not found in context")
			}

			taskPrompt := strings.TrimSpace(input.TaskPrompt)
			if taskPrompt == "" {
				return PlanAndExecuteOutput{}, apierror.BadRequest(apierror.DomainSpirit, "task_prompt is required")
			}

			mode := input.Mode
			explicitKeys := normalizeExplicitAgentKeys(input.AgentKeys)
			// Explicit agent routing implies delegation; direct mode skips the
			// allocation phase entirely, so the keys would be silently ignored.
			// Upgrade to the weakest delegating mode (parallel) so the routing
			// actually takes effect.
			if len(explicitKeys) > 0 {
				switch strings.ToLower(strings.TrimSpace(mode)) {
				case "", "auto", "direct":
					mode = "parallel"
				}
			}

			// Inject the session's effective provider/model into the context so
			// the planner/allocator can resolve their LLM via "inherit" mode.
			if deps.sessionModelLookup != nil {
				if p, m := deps.sessionModelLookup.GetSessionModel(ctx, spiritSessionID); p != "" && m != "" {
					ctx = biz.WithPlannerSessionModel(ctx, p, m)
				}
			}

			// Emit ButlerOrchestrationStarted event.
			if deps.bus != nil {
				meta := map[string]any{
					"task_prompt": taskPrompt,
					"mode":        mode,
					"agent_key":   "plan_and_execute",
				}
				if len(explicitKeys) > 0 {
					meta["agent_keys"] = explicitKeys
				}
				deps.bus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_started", "", meta))
			}

			// Check parallel quota before proceeding.
			if deps.teamQuery != nil {
				maxParallel := deps.teamQuery.GetMaxParallelTeams(ctx, spiritSessionID)
				if maxParallel > 0 {
					activeTeams, listErr := deps.teamQuery.ListActiveTeams(ctx, spiritSessionID)
					if listErr != nil {
						deps.lg.Warn("查询活跃团队列表失败，跳过配额检查",
							loggateway.StepID("spirit.quota_check_err"),
							loggateway.Err(listErr),
						)
					} else if len(activeTeams) >= maxParallel {
						return PlanAndExecuteOutput{}, apierror.BadRequest(apierror.DomainSpirit, fmt.Sprintf("并行团队数已达上限 (%d/%d)，请等待当前团队完成后再试", len(activeTeams), maxParallel))
					}
				}
			}

			var steps []biztypes.OrchestrationStepRecord

			// Phase 1: Plan
			taskPlan, planStep, err := executePlanPhase(ctx, taskPrompt, spiritSessionID, mode, deps)
			steps = append(steps, planStep)
			if err != nil {
				publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "plan", err.Error())
				return PlanAndExecuteOutput{Steps: steps}, err
			}

			out := PlanAndExecuteOutput{
				PlanID:          taskPlan.ID,
				Strategy:        string(taskPlan.Strategy),
				ComplexityLevel: string(taskPlan.ComplexityLevel),
				SubtaskCount:    len(taskPlan.SubTasks),
				MemoryHit:       taskPlan.MemoryHit != nil,
				Steps:           steps,
			}
			for _, st := range taskPlan.SubTasks {
				out.SubTasks = append(out.SubTasks, SubTaskSummary{
					ID:        st.ID,
					Name:      st.Name,
					DependsOn: st.DependsOn,
				})
			}

			// For direct strategy, no allocation or orchestration needed.
			if taskPlan.Strategy == biz.StrategyDirect {
				// 2026-07-05 Step 3: direct 路径也发布 v2 PlanBoard（如有 SubTasks），
				// allocPlan=nil 表示无 agent 分配，PlanStep.AgentKeys 为空。
				// C-18: PublishV2Board returns (PlanBoard, error) — do not ignore.
				board, pubErr := deps.planner.PublishV2Board(ctx, taskPlan, nil, "")
				if pubErr != nil {
					publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "publish_board", pubErr.Error())
					return out, pubErr
				}
				if board.ID != "" {
					out.OrchestrationID = board.ID
				}
				// B-04 fix: do NOT publish orchestration_completed here — the DAG
				// hasn't executed yet. PlanExecutor.publishPlanBoardTerminal will
				// emit orchestration_completed/orchestration_failed when the DAG
				// reaches a terminal state.
				return out, nil
			}

			// Phase 2: Allocate
			allocPlan, allocStep, allocErr := executeAllocatePhase(ctx, taskPlan, explicitKeys, deps)
			out.Steps = append(out.Steps, allocStep)
			if allocErr != nil {
				publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "allocate", allocErr.Error())
				return out, allocErr
			}

			// Fill agent keys from allocation into subtask summaries.
			for i := range out.SubTasks {
				for _, alloc := range allocPlan.Allocations {
					if alloc.SubTaskID == out.SubTasks[i].ID {
						out.SubTasks[i].AgentKey = alloc.AssignedKey
						break
					}
				}
			}

			// 2026-07-05 Step 3: Phase 2 之后发布 v2 PlanBoard，使 PlanStep.AgentKeys
			// 能从 allocPlan.Allocations 填充。PlanExecutor 订阅 PlanBoardCreatedEvent
			// 后读取 PlanStep.AgentKeys 组建 team，避免查 DB 取错 agent。
			// C-18: PlanBoard.ID is the canonical orchestration_id (matches
			// PlanExecutor terminal meta.orchestration_id).
			board, pubErr := deps.planner.PublishV2Board(ctx, taskPlan, allocPlan, "")
			if pubErr != nil {
				publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "publish_board", pubErr.Error())
				return out, pubErr
			}

			// Phase 3: Orchestrate (delegated to PlanExecutor; ID = PlanBoard.ID)
			handle, orchStep, orchErr := executeOrchestratePhase(ctx, taskPlan, allocPlan, board.ID, deps)
			out.Steps = append(out.Steps, orchStep)
			if orchErr != nil {
				publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "orchestrate", orchErr.Error())
				return out, orchErr
			}

			out.OrchestrationID = handle.ID
			// B-04 fix: do NOT publish orchestration_completed here — the DAG
			// hasn't executed yet. PlanExecutor.publishPlanBoardTerminal will
			// emit the terminal orchestration event when the DAG finishes.
			return out, nil
		},
		trpcfunction.WithName("plan_and_execute"),
		trpcfunction.WithDescription("规划并执行任务。自动评估复杂度、分配 Agent、启动编排。简单任务直接回答，复杂任务自动组建团队。"),
	)
}

// executePlanPhase runs Phase 1 of plan_and_execute: task planning.
func executePlanPhase(ctx context.Context, taskPrompt, spiritSessionID, mode string, deps planAndExecuteDeps) (plan *biz.TaskPlan, step biztypes.OrchestrationStepRecord, err error) {
	// Start plan phase span (P3-2): Trace propagation across Spirit→Team→Graph.
	bridge := turntrace.FromContext(ctx)
	if bridge != nil {
		ctx, _ = bridge.StartPhase(ctx, turntrace.PhasePlan)
	}
	defer func() {
		if bridge != nil {
			bridge.EndPhase(turntrace.PhasePlan, err)
		}
	}()

	start := time.Now().UTC()
	defer func() {
		metrics.SpiritPlanDuration.Observe(time.Since(start).Seconds())
	}()
	planInput := biz.PlanInput{
		UserMessage:     taskPrompt,
		SpiritSessionID: spiritSessionID,
		Mode:            mode,
	}
	taskPlan, planErr := deps.planner.Plan(ctx, planInput)
	if planErr != nil {
		return nil, biztypes.OrchestrationStepRecord{
			StepName:   "plan",
			Status:     "failed",
			Error:      planErr.Error(),
			StartedAt:  start,
			FinishedAt: time.Now().UTC(),
		}, apierror.Internal(apierror.DomainSpirit, "plan failed: "+planErr.Error())
	}
	return taskPlan, biztypes.OrchestrationStepRecord{
		StepName:   "plan",
		Status:     "completed",
		StartedAt:  start,
		FinishedAt: time.Now().UTC(),
	}, nil
}

// executeAllocatePhase runs Phase 2 of plan_and_execute: agent allocation.
// When explicitKeys is non-empty, heuristic matching is bypassed and the
// specified agents are assigned directly (system-butler routing contract).
func executeAllocatePhase(ctx context.Context, taskPlan *biz.TaskPlan, explicitKeys []string, deps planAndExecuteDeps) (allocPlan *biz.AllocationPlan, step biztypes.OrchestrationStepRecord, err error) {
	// Start allocate phase span (P3-2): Trace propagation across Spirit→Team→Graph.
	bridge := turntrace.FromContext(ctx)
	if bridge != nil {
		ctx, _ = bridge.StartPhase(ctx, turntrace.PhaseAlloc)
	}
	defer func() {
		if bridge != nil {
			bridge.EndPhase(turntrace.PhaseAlloc, err)
		}
	}()

	start := time.Now().UTC()
	defer func() {
		metrics.SpiritAllocDuration.Observe(time.Since(start).Seconds())
	}()
	if len(explicitKeys) > 0 {
		allocPlan, err = deps.allocator.AllocateExplicit(ctx, taskPlan, explicitKeys)
	} else {
		allocPlan, err = deps.allocator.Allocate(ctx, taskPlan)
	}
	if err != nil {
		deps.lg.Warn("plan_and_execute: allocation failed, returning plan only",
			loggateway.StepID("spirit.plan_and_execute.alloc_fail"),
			loggateway.Str("plan_id", taskPlan.ID),
			loggateway.Err(err),
		)
		return nil, biztypes.OrchestrationStepRecord{
			StepName:   "allocate",
			Status:     "failed",
			Error:      err.Error(),
			StartedAt:  start,
			FinishedAt: time.Now().UTC(),
		}, err
	}
	return allocPlan, biztypes.OrchestrationStepRecord{
		StepName:   "allocate",
		Status:     "completed",
		StartedAt:  start,
		FinishedAt: time.Now().UTC(),
	}, nil
}

// executeOrchestratePhase runs Phase 3 of plan_and_execute: task orchestration.
// 2026-07-05 Step 1 止血修复：不再调用 TaskOrchestratorImpl.Orchestrate 创建 team。
// team 创建完全交给 PlanExecutor（通过 PlanBoardCreatedEvent 订阅触发）。
// 设计依据：docs/superpowers/plans/2026-07-05-fix-double-execution-plan-step-agent-keys.md
// 原因：TaskOrchestratorImpl.orchestrateDAG 会与 PlanExecutor 双重执行，
// 且不调用 MarkTeamDispatched，破坏 system-push 模式的 Task 延迟关闭机制。
//
// C-18: planBoardID is the canonical orchestration_id (PlanBoard.ID). Must not
// mint a separate orch_* UUID — check_progress/cancel and PlanExecutor terminal
// events all key off the same ID.
func executeOrchestratePhase(ctx context.Context, taskPlan *biz.TaskPlan, allocPlan *biz.AllocationPlan, planBoardID string, deps planAndExecuteDeps) (*biz.OrchestrationHandle, biztypes.OrchestrationStepRecord, error) {
	start := time.Now().UTC()
	planBoardID = strings.TrimSpace(planBoardID)
	if planBoardID == "" {
		return nil, biztypes.OrchestrationStepRecord{
			StepName:   "orchestrate",
			Status:     "failed",
			Error:      "plan board id is required (PublishV2Board produced no board)",
			StartedAt:  start,
			FinishedAt: time.Now().UTC(),
		}, apierror.Internal(apierror.DomainSpirit, "plan board id is required for orchestration")
	}
	deps.lg.Info("plan_and_execute: orchestration delegated to PlanExecutor",
		loggateway.StepID("spirit.plan_and_execute.orch_delegated"),
		loggateway.Str("plan_id", taskPlan.ID),
		loggateway.Str("plan_board_id", planBoardID),
		loggateway.Str("spirit_session_id", taskPlan.SpiritSessionID),
		loggateway.Int("subtask_count", len(taskPlan.SubTasks)),
	)
	handle := &biz.OrchestrationHandle{
		ID:              planBoardID,
		TaskPlanID:      taskPlan.ID,
		AllocationID:    allocPlan.ID,
		SpiritSessionID: taskPlan.SpiritSessionID,
		TraceID:         taskPlan.TraceID,
		Strategy:        taskPlan.Strategy,
		Status:          biz.OrchestrationStatusRunning,
	}
	// B-04: Phase 3 only accepts/starts orchestration — DAG has not finished.
	// Report "running" (not "completed") so callers do not treat tool success
	// as durable execution completion. Terminal events come from PlanExecutor.
	return handle, biztypes.OrchestrationStepRecord{
		StepName:  "orchestrate",
		Status:    "running",
		StartedAt: start,
	}, nil
}

// normalizeExplicitAgentKeys trims blanks and dedupes agent keys, preserving order.
func normalizeExplicitAgentKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// publishOrchestrationFailed emits a ButlerOrchestrationFailed event.
func publishOrchestrationFailed(bus biz.EventBus, ctx context.Context, sessionID, phase, errMsg string) {
	if bus == nil {
		return
	}
	meta := map[string]any{
		"phase":     phase,
		"error":     errMsg,
		"agent_key": "plan_and_execute",
	}
	bus.Publish(ctx, biz.NewSystemNoticeEvent(sessionID, "orchestration_failed", "", meta))
}

// CheckOrchestrationProgressInput is the input for the check_progress tool.
type CheckOrchestrationProgressInput struct {
	OrchestrationID string `json:"orchestration_id" jsonschema:"description=The orchestration ID to check progress for"`
}

// TaskProgressView is a view of a single task's progress.
type TaskProgressView struct {
	SubTaskID   string  `json:"sub_task_id"`
	SubTaskName string  `json:"sub_task_name"`
	AgentKey    string  `json:"agent_key"`
	Status      string  `json:"status"`
	Progress    float64 `json:"progress"`
}

// CheckOrchestrationProgressOutput is the output for the check_progress tool.
type CheckOrchestrationProgressOutput struct {
	OrchestrationID string             `json:"orchestration_id"`
	Status          string             `json:"status"`
	Tasks           []TaskProgressView `json:"tasks"`
}

// PlanBoardOrchFallback resolves progress/cancel by PlanBoard.ID when the
// legacy OrchestrationRepository has no row (C-18: canonical ID is PlanBoard.ID).
type PlanBoardOrchFallback interface {
	CheckPlanBoardProgress(ctx context.Context, planBoardID string) ([]biz.TaskProgress, error)
	CancelPlanBoard(ctx context.Context, planBoardID string) error
}

// NewCheckOrchestrationProgressTool creates the check_progress tool.
// boards may be nil; when set, a PlanBoard.ID lookup is used if orchestrator.GetByID fails.
func NewCheckOrchestrationProgressTool(orchestrator biz.TaskOrchestratorPort, boards PlanBoardOrchFallback, lg loggateway.Logger) *trpcfunction.FunctionTool[CheckOrchestrationProgressInput, CheckOrchestrationProgressOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input CheckOrchestrationProgressInput) (CheckOrchestrationProgressOutput, error) {
			orchestrationID := strings.TrimSpace(input.OrchestrationID)
			if orchestrationID == "" {
				return CheckOrchestrationProgressOutput{}, apierror.BadRequest(apierror.DomainSpirit, "orchestration_id is required")
			}

			progress, err := orchestrator.CheckProgress(ctx, orchestrationID)
			if err != nil && boards != nil {
				if pbProgress, pbErr := boards.CheckPlanBoardProgress(ctx, orchestrationID); pbErr == nil {
					progress, err = pbProgress, nil
				}
			}
			if err != nil {
				return CheckOrchestrationProgressOutput{}, apierror.Internal(apierror.DomainSpirit, "check progress: "+err.Error())
			}

			views := make([]TaskProgressView, 0, len(progress))
			for _, p := range progress {
				views = append(views, TaskProgressView{
					SubTaskID:   p.SubTaskID,
					SubTaskName: p.SubTaskName,
					AgentKey:    p.AgentKey,
					Status:      p.Status,
					Progress:    p.Progress,
				})
			}
			return CheckOrchestrationProgressOutput{
				OrchestrationID: orchestrationID,
				Status:          deriveOrchestrationStatus(views),
				Tasks:           views,
			}, nil
		},
		trpcfunction.WithName("check_progress"),
		trpcfunction.WithDescription("查询编排执行进度。基于 orchestration_id（= PlanBoard.ID）查询。返回编排状态和每个子任务的进度（含 agent_key、status、progress 百分比）。"),
	)
}

// deriveOrchestrationStatus infers the overall orchestration status from
// individual task statuses. This prevents the caller from having to
// poll indefinitely when the orchestration has already completed or failed.
func deriveOrchestrationStatus(tasks []TaskProgressView) string {
	if len(tasks) == 0 {
		return "pending"
	}
	allCompleted := true
	anyFailed := false
	anyCancelled := false
	for _, t := range tasks {
		switch t.Status {
		case "failed":
			anyFailed = true
			allCompleted = false
		case "cancelled":
			anyCancelled = true
			allCompleted = false
		case "completed":
			// ok
		default:
			allCompleted = false
		}
	}
	if anyFailed {
		return "failed"
	}
	if anyCancelled {
		return "cancelled"
	}
	if allCompleted {
		return "completed"
	}
	return "running"
}

// CancelOrchestrationInput is the input for the cancel_orchestration tool.
type CancelOrchestrationInput struct {
	OrchestrationID string `json:"orchestration_id" jsonschema:"description=The orchestration ID to cancel"`
}

// CancelOrchestrationOutput is the output for the cancel_orchestration tool.
type CancelOrchestrationOutput struct {
	OrchestrationID string `json:"orchestration_id"`
	Status          string `json:"status"`
}

// NewCancelOrchestrationTool creates the cancel_orchestration tool.
// boards may be nil; when set, cancel falls back to PlanBoard.ID if orchestrator.Cancel fails.
func NewCancelOrchestrationTool(orchestrator biz.TaskOrchestratorPort, boards PlanBoardOrchFallback, lg loggateway.Logger) *trpcfunction.FunctionTool[CancelOrchestrationInput, CancelOrchestrationOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input CancelOrchestrationInput) (CancelOrchestrationOutput, error) {
			orchestrationID := strings.TrimSpace(input.OrchestrationID)
			if orchestrationID == "" {
				return CancelOrchestrationOutput{}, apierror.BadRequest(apierror.DomainSpirit, "orchestration_id is required")
			}
			err := orchestrator.Cancel(ctx, orchestrationID)
			if err != nil && boards != nil {
				if pbErr := boards.CancelPlanBoard(ctx, orchestrationID); pbErr == nil {
					err = nil
				}
			}
			if err != nil {
				return CancelOrchestrationOutput{}, err
			}
			return CancelOrchestrationOutput{OrchestrationID: orchestrationID, Status: "cancelled"}, nil
		},
		trpcfunction.WithName("cancel_orchestration"),
		trpcfunction.WithDescription("取消正在运行的编排。orchestration_id 为 PlanBoard.ID。取消后释放资源。"),
	)
}

// ---------------------------------------------------------------------------
// Port interfaces — still used by SpiritTeamAssembler and TaskOrchestratorImpl.
// ---------------------------------------------------------------------------

// SpiritTeamAssemblerPort assembles teams for the Spirit agent.
// Stability:evolving
type SpiritTeamAssemblerPort interface {
	// AssembleTeam returns the assembled team, its session, and a map of
	// agentKey → member session ID for each team member.
	AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, map[string]string, error)
	SuggestTopology(ctx context.Context, taskDescription string) (string, bool)
}

// SpiritTeamQueryPort queries team state for the Spirit agent.
// Stability:evolving
type SpiritTeamQueryPort interface {
	ListActiveTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
	ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
	GetMaxParallelTeams(ctx context.Context, spiritSessionID string) int
}

// SpiritTeamControllerPort controls team lifecycle (cancel, progress check).
// Stability:evolving
type SpiritTeamControllerPort interface {
	CancelTeam(ctx context.Context, teamID string) error
	CheckTeamProgress(ctx context.Context, spiritSessionID string) ([]biz.TeamProgress, error)
}

// SpiritSynthesisPort synthesizes results from multiple teams.
// Stability:evolving
type SpiritSynthesisPort interface {
	SynthesizeResults(ctx context.Context, spiritSessionID string, strategy string) (*biz.SynthesisOutput, error)
}

// SynthesizeResultsInput is the input for the synthesize_results tool.
type SynthesizeResultsInput struct {
	Strategy string `json:"strategy,omitempty" jsonschema:"description=合成策略,enum=template,enum=prompt,enum=hybrid"`
}

// SynthesizeResultsOutput is the output for the synthesize_results tool.
type SynthesizeResultsOutput struct {
	Content           string                    `json:"content"`
	Strategy          string                    `json:"strategy"`
	TeamResults       []biz.TeamSynthesisResult `json:"team_results"`
	NeedsLLMSynthesis bool                      `json:"needs_llm_synthesis"`
}

// NewSynthesizeResultsTool creates the synthesize_results tool.
func NewSynthesizeResultsTool(synthesis SpiritSynthesisPort) *trpcfunction.FunctionTool[SynthesizeResultsInput, SynthesizeResultsOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input SynthesizeResultsInput) (SynthesizeResultsOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return SynthesizeResultsOutput{}, apierror.BadRequest(apierror.DomainSpirit, "spirit session id not found in context")
			}
			output, err := synthesis.SynthesizeResults(ctx, spiritSessionID, input.Strategy)
			if err != nil {
				return SynthesizeResultsOutput{}, apierror.Internal(apierror.DomainSpirit, "synthesize results: "+err.Error())
			}
			return SynthesizeResultsOutput{
				Content:           output.Content,
				Strategy:          string(output.Strategy),
				TeamResults:       output.TeamResults,
				NeedsLLMSynthesis: output.Strategy == biz.SynthesisStrategyPrompt || output.Strategy == biz.SynthesisStrategyHybrid,
			}, nil
		},
		trpcfunction.WithName("synthesize_results"),
		trpcfunction.WithDescription("合成所有已完成团队的执行结果。前置条件：所有并行团队均已完成——系统会在全部完成后主动通知你，收到通知后再调用本工具。若收到“团队仍在执行中”的提示，请耐心等待系统通知，不要重试或轮询。调用后将各团队结果整合为综合报告。"),
	)
}

func spiritSessionIDFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if inv.Session != nil {
		return inv.Session.ID
	}
	return ""
}

// ---------------------------------------------------------------------------
// Batch tool execution integration (B5: ParallelToolExecutor integration)
// ---------------------------------------------------------------------------

// BatchExecuteSpiritTools runs a batch of tool calls using ParallelToolExecutor
// when available, falling back to serial execution when the executor is nil or
// Execute returns an error (e.g., dependency cycle, context cancellation).
//
// The Wire-bound executor provides configuration (maxConcurrency, worktree
// isolator, transaction sandbox); the caller-supplied handler dispatches each
// call to the appropriate tool implementation. A fresh executor is constructed
// with the handler so the Wire-bound singleton is never mutated.
//
// Use this helper for batch tool call scenarios such as multi_tool_use.parallel
// patterns where multiple independent (or dependency-ordered) tool calls should
// run concurrently for lower latency.
func BatchExecuteSpiritTools(
	ctx context.Context,
	exec *ParallelToolExecutor,
	handler ToolHandler,
	calls []ToolCall,
	lg loggateway.Logger,
) []ToolResult {
	if len(calls) == 0 {
		return nil
	}
	if exec != nil && handler != nil {
		// Reuse the Wire-bound executor's concurrency setting AND isolation
		// plumbing (worktree isolator / transaction sandbox); the handler is
		// caller-supplied because tool dispatch is agent/session-specific.
		parallelExec := NewParallelToolExecutor(handler, lg,
			WithMaxConcurrency(exec.maxConcurrency),
			WithWorktreeIsolator(exec.worktreeIso),
			WithTransactionSandbox(exec.txSandbox),
		)
		results, err := parallelExec.Execute(ctx, calls)
		if err == nil {
			return results
		}
		if lg != nil {
			lg.Warn("ParallelToolExecutor 执行失败，降级为串行执行",
				loggateway.StepID("spirit.batch.fallback"),
				loggateway.Err(err),
			)
		}
	}
	return executeToolCallsSerial(ctx, handler, calls, lg)
}

// executeToolCallsSerial runs tool calls one by one, respecting ctx cancellation.
// A nil handler produces failure results so callers can distinguish "no handler"
// from "handler returned error".
func executeToolCallsSerial(ctx context.Context, handler ToolHandler, calls []ToolCall, lg loggateway.Logger) []ToolResult {
	results := make([]ToolResult, 0, len(calls))
	for _, call := range calls {
		if ctx.Err() != nil {
			results = append(results, ToolResult{
				CallID:  call.ID,
				Name:    call.Name,
				Success: false,
				Error:   ctx.Err().Error(),
			})
			break
		}
		if handler == nil {
			results = append(results, ToolResult{
				CallID:  call.ID,
				Name:    call.Name,
				Success: false,
				Error:   "no tool handler configured",
			})
			continue
		}
		results = append(results, handler(ctx, call))
	}
	return results
}
