package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	biztypes "aranea-agents/internal/biz/types"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
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
	AgentKeys []string `json:"agent_keys,omitempty" jsonschema:"description=Explicit agent routing: real agent_keys (e.g. [\"__system_admin__\"] for Skill/MCP/industry management, [\"__memory__\"] for memory tasks). Department/company display names (e.g. \"技术部\") are also accepted and resolved to that org unit's lead agent. In parallel mode, when len(agent_keys)==len(subtasks) they map 1:1 in order; otherwise agent_keys[0] executes all subtasks and remaining keys join as team members in dag mode. Bypasses heuristic agent matching. Required when delegating to system butlers."`
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
	// ClarificationQuestions 分解层澄清出口（Q7, session-eval-20260827 P4）：
	// planner 判定任务存在阻塞性信息缺失时挂出的问题，NextAction=
	// await_user_clarification。收到后必须逐条向用户提问，带答案重新调用，
	// 禁止自行补全答案。
	ClarificationQuestions []string `json:"clarification_questions,omitempty"`
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
	// startupWaiter P2-② 假启动对账：orchestrate 阶段返回前验证看板真实
	// 启动了团队（首个 team_run 落地），失败则把真实原因回传给 Spirit。
	// 可选：nil 时跳过对账（测试 / 旧装配路径）。
	startupWaiter BoardStartupWaiter
}

// BoardStartupWaiter 对账一个新发布 PlanBoard 的启动结局（P2-②）。
// 由 service.PlanExecutor 实现（tools 不反向依赖 service）。
type BoardStartupWaiter interface {
	// WaitBoardStartup 返回 (true, "")=首个 team 已创建或无法判定（超时/
	// 挂起，不阻断）；(false, reason)=board 启动失败。
	WaitBoardStartup(ctx context.Context, planBoardID string, timeout time.Duration) (bool, string)
}

// planBoardStartupWait 是 orchestrate 阶段启动对账的等待上限。正常首个
// team 创建为 DB 事务（毫秒级）；HITL 挂起/慢创建超时后放行 running。
const planBoardStartupWait = 10 * time.Second

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

// shouldRejectDirectAnswerPlan blocks obvious direct-answer requests
// (recommendations / explanations / opinions) from entering plan assembly
// （包C Q2-C1, session-eval-20260827 S11-t5 根修）。「推荐三本书」被
// Spirit LLM 自主送入 plan_and_execute，工具自判 simple/direct 仍完成
// 计划装配（计划板+事件写时间线），单轮 input +120%。与事实查询拦截同
// 契约：显式 agent_keys / parallel / dag / force_new 一律放行（butler
// 路由与显式组队诉求不受影响）；任务信号 veto 在 biz.LooksLikeDirectAnswer
// 内完成（含任务动作词的复合任务不拦截）。
func shouldRejectDirectAnswerPlan(taskPrompt, mode string, forceNew bool, explicitKeys []string) bool {
	if forceNew || len(explicitKeys) > 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "parallel", "dag":
		return false
	}
	return biz.LooksLikeDirectAnswer(taskPrompt)
}

// NewPlanAndExecuteTool creates the plan_and_execute tool that replaces
// assess_complexity + assemble_team + list_butlers + query_butler_status.
func NewPlanAndExecuteTool(planner biz.TaskPlannerPort, allocator biz.AgentAllocatorPort, orchestrator biz.TaskOrchestratorPort, teamQuery SpiritTeamQueryPort, sessionModelLookup SessionModelLookup, bus biz.EventBus, lg loggateway.Logger, startupWaiter BoardStartupWaiter) *trpcfunction.FunctionTool[PlanAndExecuteInput, PlanAndExecuteOutput] {
	deps := planAndExecuteDeps{
		planner:            planner,
		allocator:          allocator,
		orchestrator:       orchestrator,
		teamQuery:          teamQuery,
		sessionModelLookup: sessionModelLookup,
		bus:                bus,
		lg:                 lg,
		startupWaiter:      startupWaiter,
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
					"fact query: answer with datetime / web_research; do not call plan_and_execute")
			}
			if shouldRejectDirectAnswerPlan(taskPrompt, mode, input.ForceNew, explicitKeys) {
				return PlanAndExecuteOutput{}, apierror.BadRequest(apierror.DomainSpirit,
					"direct-answer request: answer directly in one pass; plan_and_execute is for multi-step deliverable production, not recommendations/explanations")
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
				taskPlan, planStep, err = executePlanPhase(ctx, taskPrompt, spiritSessionID, mode, explicitKeys, deps)
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

			// Q7 分解层澄清出口（session-eval-20260827 P4 根修）：planner 判定
			// 存在阻塞性信息缺失——不分配、不建板、不编排，问题透传给 Spirit
			// 逐条向用户提问，带答案重新调用。杜绝 S07 型虚构（分解 LLM 曾自行
			// 编造产品名/日期/渠道塞进 subtask 描述）。
			if taskPlan.NeedsClarification() {
				out.NextAction = "await_user_clarification"
				out.ClarificationQuestions = append([]string(nil), taskPlan.ClarificationQuestions...)
				return out, nil
			}

			if taskPlan.DecomposeFailed() && taskPlan.Strategy != biz.StrategyDirect {
				out.NextAction = biz.DecomposeFailedNextAction
				out.ReuseReason = biz.DecomposeFailedUserHint
				return out, nil
			}

			if taskPlan.DecomposeReason == biz.DecomposeReasonDeferred {
				out.NextAction = biz.PlanningInProgressNextAction
				out.ReuseReason = biz.PlanningInProgressUserHint
				if committedPlanTeam(ctx) {
					// Committed PlanTeam must not treat the defer ACK as
					// "Spirit may now write the deliverable".
					out.NextAction = biz.AwaitOrchestrationNextAction
					out.ReuseReason = biz.AwaitOrchestrationUserHint
				}
				startBackgroundPlanCompletion(ctx, taskPlan, taskPrompt, spiritSessionID, mode, explicitKeys, deps)
				return out, nil
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
					// N2 自动重规划：roster_miss 且计划来自记忆命中时，作废记忆
					// 缓存重规划一次（LLM 分解），仍失败才回 NextAction。
					if errors.Is(allocErr, biz.ErrRosterMiss) && taskPlan.MemoryHit != nil {
						deps.lg.Info("roster_miss 且计划来自记忆命中，自动重规划",
							loggateway.StepID("spirit.plan_and_execute.replan"),
							loggateway.Str("plan_id", taskPlan.ID),
							loggateway.Str("spirit_session_id", spiritSessionID),
						)
						replanInput := biz.PlanInput{
							UserMessage:       taskPrompt,
							SpiritSessionID:   spiritSessionID,
							Mode:              mode,
							AgentKeys:         explicitKeys,
							DeferLLMDecompose: deferLLMDecompose(ctx),
							SkipMemory:        true,
						}
						replanPlan, replanErr := deps.planner.Plan(ctx, replanInput)
						if replanErr == nil && replanPlan != nil && replanPlan.MemoryHit == nil {
							// 重规划成功且不再命中记忆——用新计划重试分配
							taskPlan = replanPlan
							out.PlanID = taskPlan.ID
							out.Strategy = string(taskPlan.Strategy)
							out.ComplexityLevel = string(taskPlan.ComplexityLevel)
							out.SubtaskCount = len(taskPlan.SubTasks)
							out.MemoryHit = false
							out.SubTasks = nil
							for _, st := range taskPlan.SubTasks {
								out.SubTasks = append(out.SubTasks, SubTaskSummary{
									ID:        st.ID,
									Name:      st.Name,
									DependsOn: st.DependsOn,
								})
							}
							allocPlan, allocStep, allocErr = executeAllocatePhase(ctx, taskPlan, explicitKeys, deps)
							out.Steps = append(out.Steps, allocStep)
						}
					}
					if allocErr != nil {
						// 包B B3b（session-eval-20260825, P-ROSTER-GAP）：roster miss
						// 结构化降级——通用角色名在花名册无映射时，返回 NextAction 引导
						// LLM 改道 build_orchestration_graph 显式组队（authorize_playbook
						// 先例；Plan-and-Act R4：计划失败是正常事件，重规划是一等公民），
						// 不再向 LLM 裸露 BAD_REQUEST 导致首选编排路径硬失败。
						if errors.Is(allocErr, biz.ErrRosterMiss) {
							out.NextAction = biz.RosterMissNextAction
							out.ReuseReason = biz.RosterMissUserHint
							return out, nil
						}
						return out, allocErr
					}
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
			if committedPlanTeam(ctx) {
				out.NextAction = biz.AwaitOrchestrationNextAction
				out.ReuseReason = biz.AwaitOrchestrationUserHint
			}
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
		fallback := currentOrchestrationCohort(teams)
		if !reuseFallbackAllowed(taskPrompt, fallback) {
			return PlanAndExecuteOutput{}, false
		}
		cohort = fallback
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

// deferLLMDecompose is true when Plan may ACK a draft and finish LLM
// decompose in the background so Spirit can speak. A committed PlanTeam
// or PlanSolo lane must not open that gap: Spirit would write the
// deliverable while allocate/orchestrate has not run (0831-r2 S06).
func deferLLMDecompose(ctx context.Context) bool {
	d, ok := biz.RouteDecisionFromContext(ctx)
	if !ok {
		return true
	}
	switch d.Lane {
	case biz.RouteLanePlanTeam, biz.RouteLanePlanSolo:
		return false
	}
	return true
}

func committedPlanTeam(ctx context.Context) bool {
	d, ok := biz.RouteDecisionFromContext(ctx)
	return ok && d.Lane == biz.RouteLanePlanTeam
}

// executePlanPhase runs Phase 1 of plan_and_execute: task planning.
func executePlanPhase(ctx context.Context, taskPrompt, spiritSessionID, mode string, explicitKeys []string, deps planAndExecuteDeps) (plan *biz.TaskPlan, step biztypes.OrchestrationStepRecord, err error) {
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
	if deps.bus != nil {
		deps.bus.Publish(ctx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_progress", "", map[string]any{
			"phase": "planning",
		}))
	}
	planInput := biz.PlanInput{
		UserMessage:       taskPrompt,
		SpiritSessionID:   spiritSessionID,
		Mode:              mode,
		AgentKeys:         explicitKeys,
		DeferLLMDecompose: deferLLMDecompose(ctx),
	}
	planInput = biz.MergeCommitted(ctx, planInput)
	if m := strings.ToLower(strings.TrimSpace(planInput.Mode)); (m == "" || m == "auto") && planInput.Committed.Mode != "" {
		planInput.Mode = planInput.Committed.Mode
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

const backgroundPlanCompleteTimeout = 90 * time.Second

// startBackgroundPlanCompletion resumes LLM decompose + allocate + board
// after plan_and_execute returned planning_in_progress so Spirit can speak.

// executeAllocatePhase runs Phase 2 of plan_and_execute: agent allocation.
// When explicitKeys is non-empty, heuristic matching is bypassed and the
// specified agents are assigned directly (system-butler routing contract).

// Start allocate phase span (P3-2): Trace propagation across Spirit→Team→Graph.

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

// P2-② 假启动拦截（session-eval-20260829-r2 R4-Q1）：返回前对账看板
// 真实启动结局。S07 事故：assembly 校验失败（无效 agent_keys）→ 零
// team_runs，但工具返回 running → Spirit 终复谎称「编排已组建」。
// 现在启动失败会把真实原因作为工具错误回传，Spirit 必须如实告知用户。

// B-04: Phase 3 only accepts/starts orchestration — DAG has not finished.
// Report "running" (not "completed") so callers do not treat tool success
// as durable execution completion. Terminal events come from PlanExecutor.

// normalizeExplicitAgentKeys trims blanks and dedupes agent keys, preserving order.

// publishOrchestrationFailed emits a ButlerOrchestrationFailed event.

// PlanBoardOrchFallback resolves cancel by PlanBoard.ID when the
// legacy OrchestrationRepository has no row (C-18: canonical ID is PlanBoard.ID).

// CancelOrchestrationInput is the input for the cancel_orchestration tool.

// CancelOrchestrationOutput is the output for the cancel_orchestration tool.

// NewCancelOrchestrationTool creates the cancel_orchestration tool.
// boards may be nil; when set, cancel falls back to PlanBoard.ID if orchestrator.Cancel fails.

// ---------------------------------------------------------------------------
// Port interfaces — still used by SpiritTeamAssembler and TaskOrchestratorImpl.
// ---------------------------------------------------------------------------

// SpiritTeamAssemblerPort assembles teams for the Spirit agent.
// Stability:evolving

// AssembleTeam returns the assembled team, its session, and a map of
// agentKey → member session ID for each team member.

// SpiritTeamQueryPort queries team state for the Spirit agent.
// Stability:evolving

// SpiritTeamControllerPort controls team lifecycle (cancel, progress check).
// Stability:evolving

// SpiritSynthesisPort synthesizes results from multiple teams.
// Stability:evolving

// SynthesizeResultsInput is the input for the synthesize_results tool.

// SynthesizeResultsOutput is the output for the synthesize_results tool.

// NewSynthesizeResultsTool creates the synthesize_results tool.

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

// Reuse the Wire-bound executor's concurrency setting AND isolation
// plumbing (worktree isolator / transaction sandbox); the handler is
// caller-supplied because tool dispatch is agent/session-specific.

// BatchExecuteAssembledTools runs a batch through the same decorated
// CallableTool.Call as the LLM path. IsolationStrategy is cleared and the
// Wire worktree/transaction isolators are not copied, so assembled file tools
// are not wrapped in a second git worktree.

// executeToolCallsSerial runs tool calls one by one, respecting ctx cancellation.
// A nil handler produces failure results so callers can distinguish "no handler"
// from "handler returned error".
