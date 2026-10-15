package tools

import (
	"aranea-agents/internal/biz"
	biztypes "aranea-agents/internal/biz/types"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/telemetry/turntrace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"context"
	"strings"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// startBackgroundPlanCompletion resumes LLM decompose + allocate + board
// after plan_and_execute returned planning_in_progress so Spirit can speak.
func startBackgroundPlanCompletion(ctx context.Context, draft *biz.TaskPlan, taskPrompt, spiritSessionID, mode string, explicitKeys []string, deps planAndExecuteDeps) {
	if draft == nil || deps.planner == nil {
		return
	}
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backgroundPlanCompleteTimeout)
	safego.Go(bgCtx, "spirit.plan.complete_decompose", func() {
		defer cancel()
		resumeInput := biz.PlanInput{
			UserMessage:     taskPrompt,
			SpiritSessionID: spiritSessionID,
			Mode:            mode,
			AgentKeys:       explicitKeys,
			ResumePlanID:    draft.ID,
		}
		resumeInput = biz.MergeCommitted(bgCtx, resumeInput)
		finished, err := deps.planner.Plan(bgCtx, resumeInput)
		if err != nil {
			publishOrchestrationFailed(deps.bus, bgCtx, spiritSessionID, "plan", err.Error())
			return
		}
		if finished.NeedsClarification() {
			if deps.bus != nil {
				deps.bus.Publish(bgCtx, biz.NewSystemNoticeEvent(spiritSessionID, "orchestration_progress", "", map[string]any{
					"phase":     "needs_clarification",
					"questions": finished.ClarificationQuestions,
				}))
			}
			return
		}
		if finished.DecomposeFailed() && finished.Strategy != biz.StrategyDirect {
			publishOrchestrationFailed(deps.bus, bgCtx, spiritSessionID, "decompose", biz.DecomposeFailedUserHint)
			return
		}
		if finished.Strategy == biz.StrategyDirect {
			if _, pubErr := deps.planner.PublishV2Board(bgCtx, finished, nil, ""); pubErr != nil {
				publishOrchestrationFailed(deps.bus, bgCtx, spiritSessionID, "publish_board", pubErr.Error())
			}
			return
		}
		allocPlan, _, allocErr := executeAllocatePhase(bgCtx, finished, explicitKeys, deps)
		if allocErr != nil {
			publishOrchestrationFailed(deps.bus, bgCtx, spiritSessionID, "allocate", allocErr.Error())
			return
		}
		board, pubErr := deps.planner.PublishV2Board(bgCtx, finished, allocPlan, "")
		if pubErr != nil {
			publishOrchestrationFailed(deps.bus, bgCtx, spiritSessionID, "publish_board", pubErr.Error())
			return
		}
		if _, _, orchErr := executeOrchestratePhase(bgCtx, finished, allocPlan, board.ID, deps); orchErr != nil {
			publishOrchestrationFailed(deps.bus, bgCtx, spiritSessionID, "orchestrate", orchErr.Error())
		}
	})
}

// executeAllocatePhase runs Phase 2 of plan_and_execute: agent allocation.
// When explicitKeys is non-empty, heuristic matching is bypassed and the
// specified agents are assigned directly (system-butler routing contract).
func executeAllocatePhase(ctx context.Context, taskPlan *biz.TaskPlan, explicitKeys []string, deps planAndExecuteDeps) (allocPlan *biz.AllocationPlan, step biztypes.OrchestrationStepRecord, err error) {

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

	if deps.startupWaiter != nil {
		started, reason := deps.startupWaiter.WaitBoardStartup(ctx, planBoardID, planBoardStartupWait)
		if !started {
			deps.lg.Warn("plan_and_execute: 启动对账失败（假启动拦截）",
				loggateway.StepID("spirit.plan_and_execute.startup_drift"),
				loggateway.Str("plan_id", taskPlan.ID),
				loggateway.Str("plan_board_id", planBoardID),
				loggateway.Str("reason", reason),
			)
			return nil, biztypes.OrchestrationStepRecord{
					StepName:   "orchestrate",
					Status:     "failed",
					Error:      "startup reconciliation failed: " + reason,
					StartedAt:  start,
					FinishedAt: time.Now().UTC(),
				}, apierror.Internal(apierror.DomainSpirit,
					"团队组建失败（启动对账未通过）：%s。请如实告知用户编排未能启动，不要声称已组建团队。", reason)
		}
	}
	handle := &biz.OrchestrationHandle{
		ID:              planBoardID,
		TaskPlanID:      taskPlan.ID,
		AllocationID:    allocPlan.ID,
		SpiritSessionID: taskPlan.SpiritSessionID,
		TraceID:         taskPlan.TraceID,
		Strategy:        taskPlan.Strategy,
		Status:          biz.OrchestrationStatusRunning,
	}

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
