package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/evaluation"
	"aranea-agents/pkg/loggateway"
)

// ProvideChatService constructs ChatService with a noop AfterTurn hook (real hook attached by ProvideEvaluationRunner).
// planExec is the v2 forward DAG scheduler (may be nil in v1-only deployments).
// When non-nil, it is injected into TeamStarter via SetPlanExecutor to break
// the Wire cycle: PlanExecutor → TeamOrchestrator → SpiritTeamAssembler
// → TeamStarter → ... → ChatService.
// 2026-07-04 问题 4 修复：
// - 注入 EventBus 到 PlanExecutor 并启动订阅，让 PlanExecutor 自动响应
//   PlanBoardCreatedEvent 触发 DAG 执行。
// - 注入 SpiritTeamAssembler 和 TeamStarter 到 RealTeamOrchestrator，
//   让 PlanExecutor.dispatchStep 能创建真实的 team + team_run。
// - 注入 AgentReader 到 RealTeamOrchestrator，让 Orchestrate 能查询
//   active agent 列表作为团队成员（PlanStep 不携带 AgentKeys）。
func ProvideChatService(deps ChatOrchestratorDeps, planExec *PlanExecutor, v2Bus biz.EventBus, realOrch *RealTeamOrchestrator, agentReader biz.AgentReader) *ChatService {
	deps.Turn.AfterTurn = biz.NoopNativeTurnAfter{}
	cs := NewChatService(deps)
	// Backfill turnGateway into TeamStarter to break the Wire cycle:
	// ChatService → TeamStarterPort → TurnGateway → ChatService.
	// TeamStarter needs TurnGateway for the system-push pattern
	// (checkAllTeamsCompleted → SynthesizeResults → ExecuteTurn).
	if setter, ok := deps.Team.TeamStarter.(interface{ SetTurnGateway(biz.TurnGateway) }); ok {
		setter.SetTurnGateway(cs)
	}
	// Inject the v2 PlanExecutor into TeamStarter. May be nil (v1-only mode);
	// the setter nil-checks internally and the field is never read in v1 paths.
	if planExec != nil {
		if setter, ok := deps.Team.TeamStarter.(interface{ SetPlanExecutor(*PlanExecutor) }); ok {
			setter.SetPlanExecutor(planExec)
		}
		// 2026-07-04 问题 4 修复：注入 EventBus 并启动订阅，
		// 让 PlanExecutor 自动响应 PlanBoardCreatedEvent 触发 DAG 执行。
		if v2Bus != nil {
			planExec.SetEventBus(v2Bus)
			planExec.StartSubscription()
		}
	}
	// 2026-07-04 问题 4 修复：注入 SpiritTeamAssembler、TeamStarter 和
	// AgentReader 到 RealTeamOrchestrator，打破 Wire 循环：
	// PlanExecutor → RealTeamOrchestrator → SpiritTeamAssembler → TeamStarter → PlanExecutor。
	// AgentReader 用于查询 active agent 列表作为团队成员（PlanStep 不携带 AgentKeys）。
	if realOrch != nil {
		if deps.Team.SpiritAssembler != nil {
			realOrch.SetAssembler(deps.Team.SpiritAssembler)
		}
		if deps.Team.TeamStarter != nil {
			realOrch.SetStarter(deps.Team.TeamStarter)
		}
		if agentReader != nil {
			realOrch.SetAgentReader(agentReader)
		}
	}
	return cs
}

// ProvideEvaluationRunner builds the evaluation runner and attaches the AfterTurn hook to chat.
func ProvideEvaluationRunner(
	chat *ChatService,
	turns EvalTurnGateway,
	evalUC *biz.EvalUsecase,
	catalog *biz.LlmProviderModelUsecase,
	sys biz.SystemSettingRepo,
	lg loggateway.Logger,
) *evaluation.Runner {
	if chat == nil || turns == nil || evalUC == nil || catalog == nil || sys == nil {
		return nil
	}
	runner := NewEvaluationRunner(evalUC, turns, catalog, sys, lg)
	chat.AttachNativeTurnAfterHook(NewEvaluationAfterTurnTrigger(evalUC, runner))
	return runner
}

// AttachNativeTurnAfterHook sets the post-turn hook after evaluation runner is constructed.
func (s *ChatService) AttachNativeTurnAfterHook(hook biz.NativeTurnAfterHook) {
	if s == nil || hook == nil {
		return
	}
	s.orch.AttachNativeTurnAfterHook(hook)
}
