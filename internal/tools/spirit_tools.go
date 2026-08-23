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
	// ForceNew starts a brand-new DAG even when this spirit session already has
	// overlapping running/completed teams for the same goal. Default false.
	ForceNew bool `json:"force_new,omitempty" jsonschema:"description=Set true only when the user explicitly asked to start a NEW independent analysis. Default false: if this session already has teams, reuse them instead of decomposing again."`
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
	ReuseExisting   bool                               `json:"reuse_existing,omitempty"`
	ReuseReason     string                             `json:"reuse_reason,omitempty"`
	ExistingTeams   []ExistingTeamSummary              `json:"existing_teams,omitempty"`
	NextAction      string                             `json:"next_action,omitempty"`
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

// shouldRejectFactQueryPlan blocks light-gear lookups (weather, time, FX)
// from starting a team. Explicit agent_keys / parallel / dag / force_new
// still go through so butler routing is unchanged.
func shouldRejectFactQueryPlan(taskPrompt, mode string, forceNew bool, explicitKeys []string) bool {
	if forceNew || len(explicitKeys) > 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "parallel", "dag":
		return false
	}
	return biz.LooksLikeFactQuery(taskPrompt)
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
			if shouldRejectFactQueryPlan(taskPrompt, mode, input.ForceNew, explicitKeys) {
				return PlanAndExecuteOutput{}, apierror.BadRequest(apierror.DomainSpirit,
					"fact query: answer with datetime / duckduckgo_search / web_research; do not call plan_and_execute")
			}
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

			// Same-goal follow-up: if this spirit session already has overlapping
			// running/completed teams, skip LLM decompose. Caller must pass
			// force_new / say 「重新组建」 to start a genuinely new analysis.
			if out, ok := tryReuseExistingOrchestration(ctx, spiritSessionID, taskPrompt, input.ForceNew, deps); ok {
				return out, nil
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

			// Phase 1: Plan — reuse a restored draft when RecoverAllInterrupted
			// loaded one for this spirit session + user message (P1-10).
			var taskPlan *biz.TaskPlan
			var recoveredAlloc *biz.AllocationPlan
			if rp, ra, ok := consumeRecoveredPlan(deps.orchestrator, spiritSessionID, taskPrompt); ok && rp != nil {
				taskPlan = rp
				recoveredAlloc = ra
				now := time.Now().UTC()
				steps = append(steps, biztypes.OrchestrationStepRecord{
					StepName:   "plan",
					Status:     "recovered",
					StartedAt:  now,
					FinishedAt: now,
				})
				deps.lg.Info("plan_and_execute: using recovered TaskPlan",
					loggateway.StepID("spirit.plan_and_execute.plan_recovered"),
					loggateway.Str("plan_id", taskPlan.ID),
					loggateway.Str("spirit_session_id", spiritSessionID),
					loggateway.Bool("allocation_restored", recoveredAlloc != nil),
				)
			} else {
				var planStep biztypes.OrchestrationStepRecord
				var err error
				taskPlan, planStep, err = executePlanPhase(ctx, taskPrompt, spiritSessionID, mode, deps)
				steps = append(steps, planStep)
				if err != nil {
					publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "plan", err.Error())
					return PlanAndExecuteOutput{Steps: steps}, err
				}
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

			if taskPlan.StrategyReason == biz.PlaybookFillRequiredReason {
				out.NextAction = "authorize_playbook"
				out.ReuseReason = biz.PlaybookFillUserHint
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

			// Phase 2: Allocate — skip LLM/heuristic matching when Phase 2 was restored.
			var allocPlan *biz.AllocationPlan
			if recoveredAlloc != nil {
				allocPlan = recoveredAlloc
				now := time.Now().UTC()
				out.Steps = append(out.Steps, biztypes.OrchestrationStepRecord{
					StepName:   "allocate",
					Status:     "recovered",
					StartedAt:  now,
					FinishedAt: now,
				})
			} else {
				var allocStep biztypes.OrchestrationStepRecord
				var allocErr error
				allocPlan, allocStep, allocErr = executeAllocatePhase(ctx, taskPlan, explicitKeys, deps)
				out.Steps = append(out.Steps, allocStep)
				if allocErr != nil {
					publishOrchestrationFailed(deps.bus, ctx, spiritSessionID, "allocate", allocErr.Error())
					return out, allocErr
				}
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
		trpcfunction.WithDescription("规划并执行任务。自动评估复杂度、分配 Agent、启动编排。简单任务直接回答，复杂任务自动组建团队。会话已有编排时默认复用（reuse_existing=true）并返回 existing_teams，禁止再开一套 DAG。仅当用户明确要求新的独立分析时传 force_new=true。"),
	)
}

// consumeRecoveredPlan returns a Phase 1/2 bundle restored by RecoverAllInterrupted
// when the orchestrator implements RecoveredPlanConsumer. No-op on other impls.
func consumeRecoveredPlan(orch biz.TaskOrchestratorPort, spiritSessionID, userMessage string) (*biz.TaskPlan, *biz.AllocationPlan, bool) {
	if orch == nil {
		return nil, nil, false
	}
	c, ok := orch.(biz.RecoveredPlanConsumer)
	if !ok {
		return nil, nil, false
	}
	return c.ConsumeRecoveredPlan(spiritSessionID, userMessage)
}

func tryReuseExistingOrchestration(ctx context.Context, spiritSessionID, taskPrompt string, forceNew bool, deps planAndExecuteDeps) (PlanAndExecuteOutput, bool) {
	if wantsForceNewOrchestration(forceNew, taskPrompt) || deps.teamQuery == nil {
		return PlanAndExecuteOutput{}, false
	}
	teams, err := deps.teamQuery.ListAllTeams(ctx, spiritSessionID)
	if err != nil {
		if deps.lg != nil {
			deps.lg.Warn("查询会话团队失败，跳过复用短路",
				loggateway.StepID("spirit.plan_and_execute.reuse_list_err"),
				loggateway.Err(err),
			)
		}
		return PlanAndExecuteOutput{}, false
	}
	phase := biz.ResolveSpiritSessionPhase(teams)
	if phase == biz.SpiritPhaseIdle {
		return PlanAndExecuteOutput{}, false
	}
	if biz.LooksLikeFreshOrchestrationAsk(taskPrompt) && biz.OrchestrationGoalShifted(taskPrompt, teams) {
		return PlanAndExecuteOutput{}, false
	}
	overlap := reusableOverlappingTeams(taskPrompt, teams)
	cohort := expandToOrchestrationCohort(overlap, teams)
	if len(cohort) == 0 {
		cohort = currentOrchestrationCohort(teams)
	}
	if len(cohort) == 0 {
		return PlanAndExecuteOutput{}, false
	}
	if deps.bus != nil {
		deps.bus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_progress", "", map[string]any{
			"phase":      "reused",
			"team_count": len(cohort),
		}))
	}
	if deps.lg != nil {
		deps.lg.Info("plan_and_execute: reusing existing session teams",
			loggateway.StepID("spirit.plan_and_execute.reuse_existing"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Int("team_count", len(cohort)),
		)
	}
	return PlanAndExecuteOutput{
		Strategy:        reuseStrategy,
		ComplexityLevel: string(biz.ComplexityModerate),
		ReuseExisting:   true,
		ReuseReason:     "session phase " + string(phase) + "; reuse existing teams instead of starting a new DAG",
		ExistingTeams:   summarizeExistingTeams(cohort),
		NextAction:      reuseNextAction(cohort),
	}, true
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
	keys := append([]string(nil), explicitKeys...)
	if len(keys) == 0 && taskPlan != nil && taskPlan.MemoryHit != nil {
		keys = biz.FilterRecipeAgentKeys(taskPlan.MemoryHit.AgentKeysUsed)
	}
	if len(keys) > 0 {
		allocPlan, err = deps.allocator.AllocateExplicit(ctx, taskPlan, keys)
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
// 原因：旧编排器 DAG 建团路径会与 PlanExecutor 双重执行，
// 且不调用 MarkTeamDispatched，破坏 system-push 模式的 Task 延迟关闭机制。
// （ADR-2，2026-08-20：旧 Orchestrate 及其建团 helper 已整体删除。）
//
// C-18: planBoardID is the canonical orchestration_id (PlanBoard.ID). Must not
// mint a separate orch_* UUID — cancel_orchestration and PlanExecutor terminal
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

// PlanBoardOrchFallback resolves cancel by PlanBoard.ID when the
// legacy OrchestrationRepository has no row (C-18: canonical ID is PlanBoard.ID).
type PlanBoardOrchFallback interface {
	CancelPlanBoard(ctx context.Context, planBoardID string) error
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
			err := orchestrator.Cancel(ctx, orchestrationID, biz.CancelReasonUser)
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
	CancelTeam(ctx context.Context, teamID string, reason biz.CancelReason) error
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
// The Wire-bound executor provides maxConcurrency. Worktree/transaction
// isolators apply only when a ToolCall sets IsolationStrategy; assembled
// tools must use BatchExecuteAssembledTools (or NewAssembledToolHandler with
// IsolationStrategy empty) so file isolation stays inside CallableTool.Call
// (path locks / optional git wrap).
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

// BatchExecuteAssembledTools runs a batch through the same decorated
// CallableTool.Call as the LLM path. IsolationStrategy is cleared and the
// Wire worktree/transaction isolators are not copied, so assembled file tools
// are not wrapped in a second git worktree.
func BatchExecuteAssembledTools(
	ctx context.Context,
	exec *ParallelToolExecutor,
	assembled *AssembledToolsets,
	calls []ToolCall,
	lg loggateway.Logger,
) []ToolResult {
	if len(calls) == 0 {
		return nil
	}
	if assembled == nil {
		return executeToolCallsSerial(ctx, nil, calls, lg)
	}
	handler := NewAssembledToolHandler(assembled.Tools, assembled.ToolSets)
	cleaned := stripIsolationStrategy(calls)
	var stripped *ParallelToolExecutor
	if exec != nil {
		stripped = NewParallelToolExecutor(nil, lg, WithMaxConcurrency(exec.maxConcurrency))
	}
	return BatchExecuteSpiritTools(ctx, stripped, handler, cleaned, lg)
}

func stripIsolationStrategy(calls []ToolCall) []ToolCall {
	out := make([]ToolCall, len(calls))
	for i, c := range calls {
		c.IsolationStrategy = ""
		out[i] = c
	}
	return out
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
