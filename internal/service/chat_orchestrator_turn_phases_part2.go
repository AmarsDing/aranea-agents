package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"context"
	"strings"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

// buildTurnRunner constructs the agent deps, builds the agent, creates the runner,
// and wires the sub-agent service.
// Stability:evolving
func (o *ChatOrchestrator) buildTurnRunner(
	ctx context.Context,
	sess biz.Session,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	content string,
) (turnBuildResult, error) {
	sessionID := strings.TrimSpace(sess.ID)
	runID := admit.runID
	dialogMode := admit.dialogMode
	prov := admit.provider
	mod := admit.model

	depsStart := time.Now()
	deps, err := o.agentBuild.BuildTRPCDeps(ctx, AgentBuildParams{
		Session: sess, Agent: ag, RunID: runID,
		DialogMode: dialogMode, Provider: prov, Model: mod, Emitter: emitter,
	})
	if err != nil {
		emitter.LogError("chat.agent.build", "构建Agent依赖失败", event.P("agent_id", ag.ID), event.P("error", err.Error()))
		return turnBuildResult{}, TurnError(TurnErrAgentBuildFailed, err.Error())
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: BuildTRPCDeps",
		loggateway.StepID("chat.build_deps"),
		loggateway.Any("elapsed_ms", time.Since(depsStart).Milliseconds()))

	prefetchCh := make(chan *chatagent.TurnCuePrefetch, 1)
	safego.Go(ctx, "chat.cue.prefetch", func() {
		pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		prefetchCh <- chatagent.PrefetchTurnCues(pctx, deps, ag, content, sess.ID, chatagent.UserIDFromCtx(ctx))
	})

	agentStart := time.Now()
	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps, o.lg())
	if err != nil {
		emitter.LogError("chat.agent.build", "构建Agent实例失败", event.P("agent_id", ag.ID), event.P("error", err.Error()))
		return turnBuildResult{}, TurnError(TurnErrAgentBuildFailed, err.Error())
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: BuildTRPCAgentCached",
		loggateway.StepID("chat.build_agent"),
		loggateway.Any("elapsed_ms", time.Since(agentStart).Milliseconds()))
	emitter.LogDone("chat.agent.build", "Agent实例已构建", event.P("provider", prov), event.P("model", mod))

	var plugins []trpcplugin.Plugin
	wsID := workspace.IDFromContext(ctx)
	if o.rt().Plugin.Manager != nil {
		plugins = o.rt().Plugin.Manager.RunnerPluginsForAgent(ag.ID, wsID)
	} else if o.rt().Plugin.RT != nil {
		plugins = o.rt().Plugin.RT.PluginsForAgent(ag.ID, wsID)
	}
	emitter.LogDone("chat.plugins_load", "插件已加载", event.P("plugin_count", len(plugins)))
	deps.Plugins = plugins
	lookup := map[string]trpcagent.Agent{}
	if key := strings.TrimSpace(ag.AgentKey); key != "" {
		lookup[key] = root
	}
	rl := chatagent.ResolveRalphLoopTurn(ag.Settings)
	if rl.SkipErr != nil {
		emitter.LogWarn("chat.runner.ralph_loop", "Ralph Loop 配置无效，已跳过", "",
			event.P("agent_id", ag.ID), event.P("error", rl.SkipErr.Error()))
	}
	emitter.LogStart("chat.runner.create", "创建 Runner", event.P("agent_key", ag.AgentKey), event.P("plugin_count", len(plugins)))
	runnerMgr := o.tdPtr().CoalesceRunnerManager()
	runnerStart := time.Now()
	runner, err := runnerMgr.NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins: plugins, AwaitUserReplyRouting: deps.AwaitHook != nil,
		BuilderDeps: deps, AgentFactoryKeys: []string{ag.AgentKey},
		LookupAgents: lookup, RalphLoop: rl.Config,

		AppName: ag.ID,
	})
	if err != nil {
		emitter.LogError("chat.runner.create", "Runner 创建失败", event.P("error", err.Error()))
		return turnBuildResult{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: NewTurnRunner",
		loggateway.StepID("chat.runner_create"),
		loggateway.Any("elapsed_ms", time.Since(runnerStart).Milliseconds()))
	emitter.LogDone("chat.runner.create", "Runner 已创建")
	o.runs.StoreRunner(sessionID, runID, runner)
	if o.subAgentService() != nil {
		o.subAgentService().SetRunner(runner)

		o.subAgentService().SetAttribution(prov, mod, ag.ID, ag.AgentKey)
		if ag.Settings != nil {
			o.subAgentService().SetSessionRunes(sessionID, ag.Settings.SubagentsStoredResultRunes, ag.Settings.SubagentsStoredSummaryRunes)
		}
	}
	rbStart := time.Now()
	rollbackBoundary, rbErr := runnerMgr.MarkRollbackBoundary(ctx, sessionID, runID, "")
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: MarkRollbackBoundary",
		loggateway.StepID("chat.rollback_boundary"),
		loggateway.Any("elapsed_ms", time.Since(rbStart).Milliseconds()),
		loggateway.Any("has_error", rbErr != nil))
	if rbErr != nil {
		emitter.LogWarn("chat.runner.rollback_boundary", "Runner 回滚边界记录失败", "", event.P("error", rbErr.Error()))
	}
	var prefetch *chatagent.TurnCuePrefetch
	select {
	case prefetch = <-prefetchCh:
	case <-ctx.Done():
	}
	return turnBuildResult{
		deps:             deps,
		runner:           runner,
		rollbackBoundary: rollbackBoundary,
		prefetch:         prefetch,
	}, nil
}

// spiritWorkInFlight reports whether this Spirit session still has background
// orchestration. The root session must stay running until synthesis, instead
// of flipping to completed after the first plan_and_execute reply.
//
// Two signals are required because executeOrchestratePhase returns as soon as
// the PlanBoard is published — teams may not be persisted yet:
//  1. persisted teams in orchestrating / interrupted
//  2. newest non-direct TaskPlan still in a recoverable status (draft/executing)
func (o *ChatOrchestrator) spiritWorkInFlight(ctx context.Context, sessionID string) bool {
	if o == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	switch o.resolveSpiritTurnOrchestration(ctx, sessionID).Phase {
	case biz.SpiritPhaseOrchestrating, biz.SpiritPhaseInterrupted:
		return true
	}
	planner := o.team().TaskPlanner
	if planner == nil {
		return false
	}
	plans, err := planner.ListPlans(ctx, sessionID)
	if err != nil || len(plans) == 0 || plans[0] == nil {
		return false
	}
	p := plans[0]
	if p.Strategy == biz.StrategyDirect {
		return false
	}
	return biz.IsRecoverableTaskPlanStatus(p.Status)
}

// persistRememberIfRequested writes an explicit preference when the user said
// 记住/以后都/我的习惯是 but the LLM did not call memory_remember.
func (o *ChatOrchestrator) persistRememberIfRequested(ctx context.Context, sess biz.Session, ag biz.Agent, input biz.TurnInput, execResult turnExecuteResult) {
	if o == nil || !intent.LooksLikeRememberRequest(input.Content) {
		return
	}
	w := o.factWriter()
	if w == nil {
		return
	}
	sourceID := ""
	if execResult.userMsg.ID != "" {
		sourceID = execResult.userMsg.ID
	}
	w.WriteFacts(ctx, sess.ID, ag.ID, sess.UserID, sourceID, []biz.FactMark{{
		Type:       "preference",
		Confidence: "high",
		Content:    strings.TrimSpace(input.Content),
	}})
}
