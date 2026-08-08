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
//   - 注入 EventBus 到 PlanExecutor 并启动订阅，让 PlanExecutor 自动响应
//     PlanBoardCreatedEvent 触发 DAG 执行。
//   - 注入 SpiritTeamAssembler 和 TeamStarter 到 RealTeamOrchestrator，
//     让 PlanExecutor.dispatchStep 能创建真实的 team + team_run。
//   - 注入 AgentReader 到 RealTeamOrchestrator，让 Orchestrate 能查询
//     active agent 列表作为团队成员（PlanStep 不携带 AgentKeys）。
//
// 2026-07-04 问题 1 修复：注入 v2 Sequencer 到 GraphOrchestrationProjector，
// 让 PublishGraphTaskStatus 的 system.notice 经过持久化（EventRouter →
// UpsertActivity），避免刷新后丢失。graphProj 在 wire 中先于 sequencer 创建，
// 需后注入打破循环。
func ProvideChatService(deps ChatOrchestratorDeps, planExec *PlanExecutor, v2Bus biz.EventBus, realOrch *RealTeamOrchestrator, agentReader biz.AgentReader, graphProj *GraphOrchestrationProjector, mbWaker biz.MailboxWaker) *ChatService {
	deps.Turn.AfterTurn = biz.NoopNativeTurnAfter{}
	cs := NewChatService(deps)
	// Backfill turnGateway into TeamStarter to break the Wire cycle:
	// ChatService → TeamStarterPort → TurnGateway → ChatService.
	// TeamStarter needs TurnGateway for the system-push pattern
	// (checkAllTeamsCompleted → SynthesizeResults → ExecuteTurn).
	if setter, ok := deps.Team.TeamStarter.(interface{ SetTurnGateway(biz.TurnGateway) }); ok {
		setter.SetTurnGateway(cs)
	}
	// M71: backfill TurnExecutorGateway into the dept mailbox waker to break
	// the Wire cycle: ChatService → RuntimeTooling → DeptMailboxUsecase →
	// MailboxWaker → TurnExecutorGateway → ChatService.
	if setter, ok := mbWaker.(interface{ SetTurnGateway(biz.TurnExecutorGateway) }); ok {
		setter.SetTurnGateway(cs)
	}
	// Inject the v2 PlanExecutor into TeamStarter. May be nil (v1-only mode);
	// the setter nil-checks internally and the field is never read in v1 paths.
	if planExec != nil {
		if setter, ok := deps.Team.TeamStarter.(interface{ SetPlanExecutor(*PlanExecutor) }); ok {
			setter.SetPlanExecutor(planExec)
		}
		// 2026-07-27 总结重复触发修复：synthesis 唯一触发点移到 dagRun 终态，
		// PlanExecutor 经 AllTeamsCompletedNotifier 回调 TeamStarter。
		if notifier, ok := deps.Team.TeamStarter.(biz.AllTeamsCompletedNotifier); ok {
			planExec.SetCompletionNotifier(notifier)
		}
		// C-18: cancel_orchestration / check_progress fall back to PlanBoard.ID.
		cs.orch.SetPlanBoardOrch(planExec)
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
	// 2026-07-04 问题 1 修复：从 V2ProjectorFactory 提取 seq，注入到
	// GraphOrchestrationProjector，让 graph_stage 事件经过 sequencer 持久化。
	if graphProj != nil {
		if pf := deps.Infra.V2ProjectorFactory; pf != nil {
			if seq := pf.Seq(); seq != nil {
				graphProj.SetSeq(seq)
			}
		}
	}
	// 2026-07-04 问题 P5/D1 修复：注入 ProjectorFactory 到 PlanExecutor 作为
	// TeamDispatchMarker，让 dispatchStep 标记 task 已派发 team，
	// OnTurnEnd 据此延迟 task.completed 直到 synthesis turn 完成。
	if planExec != nil && deps.Infra.V2ProjectorFactory != nil {
		planExec.SetTeamDispatchMarker(deps.Infra.V2ProjectorFactory)
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
	agents *biz.AgentUsecase,
	bus biz.EventBus,
	lg loggateway.Logger,
) *evaluation.Runner {
	if chat == nil || turns == nil || evalUC == nil || catalog == nil || sys == nil {
		return nil
	}
	runner := NewEvaluationRunner(evalUC, turns, catalog, sys, lg)
	// P2-2: online score-drop alert — nil-safe when agents/bus are unavailable.
	if agents != nil && bus != nil {
		runner.WithDropAlerter(evaluation.NewScoreDropAlerter(evalUC, evalAgentConfigReader{agents: agents}, bus, lg))
	}
	chat.AttachNativeTurnAfterHook(NewEvaluationAfterTurnTrigger(evalUC, runner))
	return runner
}

// ProvidePublishGate builds the P2-1 publish regression gate. Nil when the
// evaluation stack is unavailable; Check is nil-safe so consumers can call it
// unconditionally.
func ProvidePublishGate(evalUC *biz.EvalUsecase, runner *evaluation.Runner, bus biz.EventBus, lg loggateway.Logger) *evaluation.PublishGate {
	if evalUC == nil || runner == nil {
		return nil
	}
	return evaluation.NewPublishGate(evalUC, runner, bus, lg)
}

// AttachNativeTurnAfterHook sets the post-turn hook after evaluation runner is constructed.
func (s *ChatService) AttachNativeTurnAfterHook(hook biz.NativeTurnAfterHook) {
	if s == nil || hook == nil {
		return
	}
	s.orch.AttachNativeTurnAfterHook(hook)
}
