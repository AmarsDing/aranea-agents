package service

import (
	"context"
	"fmt"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// forcedPlanningSystemPrompt is the system message injected when the pre-planning
// gate forces the planning path. It instructs the Spirit LLM to invoke the
// plan_and_execute tool instead of answering directly.
//
// The prompt is intentionally minimal: it conveys the gate's decision and the
// complexity signal without overriding the agent's domain instructions.
const forcedPlanningSystemPrompt = `Pre-Planning Gate decision: this task has been assessed as %s complexity (score %.2f).
Reason: %s.

You MUST invoke the plan_and_execute tool to decompose the task before producing the final answer.
Do not answer directly even if you believe you can; route through plan_and_execute so the
orchestration layer can persist a plan, allocate agents, and emit planning timeline events.`

// runPrePlanningGate evaluates task complexity after the intent pass and returns
// a gate decision. When the planner is unavailable (e.g., single-agent sessions
// without team orchestration deps), it returns a zero-value decision with no
// error, leaving the turn on the default direct-answer path.
//
// The gate is non-fatal: any error from QuickAssess is logged and propagated,
// but the caller treats it as "no force planning" (see chat_orchestrator_turn.go).
//
// 续跑 turn（synthesis/澄清续答，ParentTaskID 非空）跳过门控，与
// runClarificationGate 同款防循环：复杂度在根 turn 已评估，重评会重复发布
// session 级孤儿 notice（2026-07-27 排查：单会话 7 对重复 = 2 根 turn
// + 2 澄清续跑 + 3 总结 turn），且 forcedPlanning 系统提示注入 synthesis
// turn 会强制其再走规划路径。
func (o *ChatOrchestrator) runPrePlanningGate(
	ctx context.Context,
	input biz.TurnInput,
	intentArt *intent.Artifact,
) (GateDecision, error) {
	if strings.TrimSpace(input.ParentTaskID) != "" {
		return GateDecision{}, nil
	}
	planner := o.team().TaskPlanner
	if planner == nil {
		// No planner wired (e.g., single-agent deployment without Spirit orchestration).
		// Skip the gate silently — direct-answer path remains the default.
		return GateDecision{}, nil
	}

	bus := o.td().Pipeline.EventBus
	gate := NewPrePlanningGate(planner, bus, o.v2Seq, o.lg())

	planInput := biz.PlanInput{
		UserMessage:     input.Content,
		SpiritSessionID: input.SessionID,
		// TaskID 复用 turn 入口预解析的 RootTaskActivityID（chat_orchestrator_turn.go
		// 在 BUILD/IntentPass 并行后注入 ctx），与澄清门同款模式；ctx 缺失时为空，
		// notice 退化为 session 级（行为同修复前，不阻断）。
		TaskID:         string(chatagent.RootTaskActivityIDFromCtx(ctx)),
		IntentArtifact: intentArtifactToBiz(intentArt),
	}
	if traceID, ok := biz.SpiritTraceIDFromContext(ctx); ok {
		planInput.TraceID = traceID
	}

	decision, err := gate.Evaluate(ctx, planInput)
	if err != nil {
		o.lg().Warn("预规划门控评估失败，回退到直接回答路径",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.SessionID(input.SessionID),
			loggateway.Err(err),
		)
		return GateDecision{}, err
	}
	return decision, nil
}

// resolveSpiritTurnOrchestration classifies the Spirit session from persisted
// teams and builds the runtime brief. Fail-soft to Idle when TeamUC is missing
// or the list query fails.
func (o *ChatOrchestrator) resolveSpiritTurnOrchestration(ctx context.Context, sessionID string) biz.SpiritTurnOrchestration {
	out := biz.SpiritTurnOrchestration{Phase: biz.SpiritPhaseIdle}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return out
	}
	teamUC := o.team().TeamUC
	if teamUC == nil {
		return out
	}
	teams, err := teamUC.ListBySpiritSessionID(ctx, sessionID)
	if err != nil {
		o.lg().Warn("查询会话团队失败，编排阶段按 idle 处理",
			loggateway.StepID("chat.orch_phase.resolve"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err),
		)
		return out
	}
	out.Phase = biz.ResolveSpiritSessionPhase(teams)
	out.Brief = biz.FormatOrchestrationBrief(out.Phase, teams)
	return out
}

func (o *ChatOrchestrator) orchestrationLooksLikeNewTask(ctx context.Context, input biz.TurnInput, intentArt *intent.Artifact) bool {
	refined := ""
	if intentArt != nil {
		refined = intentArt.RefinedGoal
	}
	last := latestPlanUserMessage(ctx, o.team().TaskPlanner, input.SessionID)
	return biz.OrchestrationLooksLikeNewTask(input.Content, last, refined)
}

func latestPlanUserMessage(ctx context.Context, planner biz.TaskPlannerPort, sessionID string) string {
	if planner == nil || strings.TrimSpace(sessionID) == "" {
		return ""
	}
	plans, err := planner.ListPlans(ctx, sessionID)
	if err != nil || len(plans) == 0 || plans[0] == nil {
		return ""
	}
	return strings.TrimSpace(plans[0].UserMessage)
}

// intentArtifactToBiz converts the intent pass artifact (internal/agent/intent)
// to the biz-layer mirror (biz.IntentArtifact). Returns nil when the source is nil
// to preserve the "no intent pass ran" signal downstream.
func intentArtifactToBiz(art *intent.Artifact) *biz.IntentArtifact {
	if art == nil {
		return nil
	}
	out := &biz.IntentArtifact{
		RefinedGoal:     art.RefinedGoal,
		IntentKind:      art.IntentKind,
		SuccessCriteria: art.SuccessCriteria,
		Ambiguities:     art.Ambiguities,
		SearchHints:     art.SearchHints,
		// 2026-08-28 方案①：顶层与子意图 hints 并集透传 task planner。
		ToolHints: art.AllToolHints(),
		RiskFlags: art.RiskFlags,
	}
	if n := len(art.SubIntents); n >= 2 {
		out.SubIntents = make([]biz.SubIntent, n)
		for i, s := range art.SubIntents {
			out.SubIntents[i] = biz.SubIntent{
				Goal:       s.Goal,
				IntentKind: s.IntentKind,
				ToolHints:  append([]string(nil), s.ToolHints...),
			}
		}
	}
	return out
}

// forcedPlanningRunOption returns a trpc-agent RunOption that injects a system
// message instructing the Spirit LLM to invoke plan_and_execute. The injected
// message is appended to the run's context messages (same mechanism as the
// intent pass injection), so it does not replace the agent's system prompt.
//
// SP-2a：提示注入仍是首选路径（LLM 遵从时零额外开销）；LLM 未遵从时由
// ForcePlanningRoute 硬路由钩子直调（internal/agent/force_planning_route.go）。
func forcedPlanningRunOption(decision GateDecision) trpcagent.RunOption {
	msg := trpcmodel.NewSystemMessage(fmt.Sprintf(forcedPlanningSystemPrompt,
		string(decision.Level),
		decision.Score,
		strings.TrimSpace(decision.Reason),
	))
	return trpcagent.WithInjectedContextMessages([]trpcmodel.Message{msg})
}

// forcePlanningTaskPrompt 为 SP-2a 硬路由选择 task_prompt：意图精化目标优先
// （语义更清晰、利于 planner 分解），回退门控透传的 biz 镜像，最终回退原始
// 用户消息。返回空串时 ContextWithForcePlanningRoute 不标记（与门控跳过
// 等价，硬路由不触发）。
func forcePlanningTaskPrompt(decision GateDecision, art *intent.Artifact, userMessage string) string {
	if art != nil {
		if goal := strings.TrimSpace(art.RefinedGoal); goal != "" {
			return goal
		}
	}
	if decision.IntentArtifact != nil {
		if goal := strings.TrimSpace(decision.IntentArtifact.RefinedGoal); goal != "" {
			return goal
		}
	}
	return strings.TrimSpace(userMessage)
}
