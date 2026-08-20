package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	sessctx "aranea-agents/internal/session"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// teamTurnBaseRunOptions builds the base run options for a team turn.
// C5: team_id is injected into the root invocation's RuntimeState; the graph
// runtime propagates it to member invocations (merged into graph initial
// state, then carried into each member's RunOptions.RuntimeState), enabling
// team-scope L3 memory recall via memoryRuntimeContext's RuntimeState fallback.
func teamTurnBaseRunOptions(teamID, turnQuery string) []trpcagent.RunOption {
	return []trpcagent.RunOption{
		skillruntime.RunOptionWithTurnQuery(turnQuery),
		trpcagent.MergeRuntimeState(map[string]any{"team_id": teamID}),
		// 框架 v1.11 修复管线：参数 JSON 修复 + 文本工具调用提取（与
		// chat 主路径 buildTurnRunOptions 对齐）。
		trpcagent.WithToolCallArgumentsJSONRepairEnabled(true),
		trpcagent.WithToolCallTextRepairEnabled(true),
	}
}

// resolveUpstreamDeliverableSeed resolves the cross-team deliverable seed for
// a DAG downstream team turn (2026-08-08 问题3c). Returns nil when the runner
// has no seed resolver wired, the team is standalone (no DagNodeID), or the
// team has no DependsOn. Seed read failure degrades to nil with a Warn — a
// seed error must never fail the turn (members still have the prompt-inlined
// digests and the read_upstream_deliverable tool as fallback).
func (r *Runner) resolveUpstreamDeliverableSeed(ctx context.Context, teamRow biz.Team) map[string]any {
	if r.upstreamSeedFn == nil || teamRow.DagNodeID == "" || len(teamRow.DependsOn) == 0 {
		return nil
	}
	seed, err := r.upstreamSeedFn(ctx, teamRow)
	if err != nil {
		r.lg.Warn("上游交付物种子解析失败，降级为无种子启动",
			loggateway.StepID("team.run.upstream_seed"),
			loggateway.Str("team_id", teamRow.ID),
			loggateway.Err(err))
		return nil
	}
	return seed
}

func (r *Runner) runTeamTRPCFromInput(ctx context.Context, sess biz.Session, input biz.TurnInput, teamRow biz.Team, def Definition, mode string) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	// Phase 1: Validate input and extract options
	ti, err := r.validateTeamTurnInput(input, sess, def)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	// Phase 2: Create and persist the initial TeamRun record
	run, err := r.createInitialTeamRun(ctx, sess, teamRow, def, mode, ti.content)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	// Phase 3: Setup tracing and event emitter
	ts := r.setupTeamTracing(ctx, sess, teamRow, run, mode, len(ti.members))
	ctx = ts.ctx
	teamBridge := ts.bridge
	teamEmitter := ts.emitter
	turnStatus := ts.turnStatus
	defer func() { teamBridge.Finish(err) }()
	if teamEmitter != nil {
		defer func() {
			teamEmitter.FinishRoot(turnStatus)
		}()
	}
	r.publishTeamRunStartedEvent(ctx, sess, teamRow, run, def)

	t0 := time.Now()
	biz.DefaultTurnCompletionBridge().RegisterTurnStart(sess.ID, run.ID, t0)

	// Phase 4: Resolve anchor agent and attachments
	ar, turnStatus, err := r.resolveAnchorAndAttachments(ctx, ti.members, def.IntentAnchorAgentID, sess, input, ti.provOpt, ti.modOpt, &run, t0)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	// Phase 5: Build TRPC dependencies and compile team runtime
	builderDeps := r.buildTeamBuilderDeps(ctx, sess, run, ar, ti.dialogMode)
	teamDeps := TRPCTeamBuilderDeps{BuilderDeps: builderDeps, UseCache: true, MemberCustomTools: r.cfg.MemberCustomTools}

	root, memberLookup, graphExecID, compiledTeam, err := r.compileTeamRuntime(ctx, sess, teamRow, def, mode, teamDeps, teamEmitter, run.ID)
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if graphExecID != "" {
		run.GraphExecutionID = graphExecID
		if uerr := r.runWriter.UpdateTeamRunGraphExecutionID(ctx, run.ID, graphExecID); uerr != nil {
			r.lg.Warn("graph_execution_id 持久化失败", loggateway.StepID("team.graph_runtime.persist"), loggateway.Err(uerr))
		}
		// G2（ADR-F D2）：replanner 计数随 run 收口释放（含 HITL 暂停路径——
		// 暂停即事件流结束；resume 重跑时重新注入回调并重新计数，与 graph run
		// 域 Resume 语义对齐）。防 ManagedMap entry 泄漏（A5）。
		if r.cfg.Replanner != nil {
			defer r.cfg.Replanner.ReleaseExecution(graphExecID)
		}
	}

	plugins := r.resolveTeamPlugins(ctx, ar.agent.ID)
	builderDeps.Plugins = plugins
	memberKeys, err := memberAgentKeys(ctx, def, r.lookupAgent)
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	obs := r.startObservers(ctx, sess, teamRow, def, run, graphExecID, compiledTeam)
	defer obs.stopAll()

	rl := agent.ResolveRalphLoopTurn(ar.agent.Settings)
	if rl.SkipErr != nil {
		r.lg.Warn("Ralph Loop 配置无效，已跳过",
			loggateway.StepID("team.runner.ralph_loop"), loggateway.Str("agent_id", ar.agent.ID), loggateway.Err(rl.SkipErr))
	}
	runnerMgr := r.td.CoalesceRunnerManager()
	runner, err := runnerMgr.NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:               plugins,
		AwaitUserReplyRouting: builderDeps.AwaitHook != nil,
		BuilderDeps:           builderDeps,
		AgentFactoryKeys:      memberKeys,
		LookupAgents:          memberLookup,
		RalphLoop:             rl.Config,
		// P0-03 fix: use manager agent ID as AppName for memory scope.
		// Member agents share the manager's Runner session; their proactive
		// recall uses their own agentID. Full per-member scope isolation
		// requires per-member Runners (future work).
		AppName: ar.agent.ID,
	})
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if r.cfg.Runs != nil {
		r.cfg.Runs.StoreRunner(sess.ID, run.ID, runner)
	}
	rollbackBoundary, _ := runnerMgr.MarkRollbackBoundary(ctx, sess.ID, run.ID, "")
	rollbackDone := false
	rollbackRunnerSession := func() {
		if rollbackDone {
			return
		}
		rollbackDone = true
		// defer 阶段请求 ctx 可能已取消：脱离取消语义（保留 trace 值）但必须有界。
		rbCtx, rbCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer rbCancel()
		if rberr := runnerMgr.RollbackToBoundary(rbCtx, rollbackBoundary); rberr != nil {
			r.lg.Warn("RollbackToBoundary failed", loggateway.StepID("team.run.rollback_fail"), loggateway.Str("boundary", rollbackBoundary.BoundaryID), loggateway.Err(rberr))
		}
	}
	defer func() {
		if turnStatus != biz.TeamMemberStepStatusOK {
			rollbackRunnerSession()
		}
		if r.cfg.Runs != nil {
			r.cfg.Runs.Finish(sess.ID, run.ID)
		}
		runner.Close()
	}()

	utOpts, turnStatus, err := r.prepareUserTurnOptions(ctx, ar, ti.content, sess, &run, teamRow, ti.dialogMode, t0)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	userMsg = biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sess.ID,
		Role:             "user",
		ContentMarkdown:  ti.content,
		Status:           biz.TeamMemberStepStatusOK,
		OptionsJSON:      utOpts.userOpts,
		CreatedAt:        agent.RFC3339Now(),
		AttachmentsCount: utOpts.attN,
	}

	if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	uid := agent.UserIDFromCtx(ctx)

	// Phase 6: Build execution context with deadline, knowledge tools, and artifact collector
	runCtx, runCancel, turnArtCollector := r.buildTeamRunContext(ctx, def, builderDeps, input)
	if runCancel != nil {
		defer runCancel()
	}
	runCtx, abortRun := context.WithCancel(runCtx)
	defer abortRun()
	if teamEmitter != nil {
		teamEmitter.LogStart("team.run.execute", "执行团队任务", event.P("mode", mode))
	}

	// Phase 6.5: Build project metadata and pre-configure the v2 ActivityProjector.
	// N-21/N-03: The v2 projector is pre-configured and injected into runCtx so
	// that plugins (cost_guard, model_router) and hooks (tool_confirmation) can
	// emit notice/confirm events via biz.ActivityEmitterFromContext during the
	// LLM call. Reset clears stale state; Configure sets meta without emitting
	// events. OnTurnStart (called later by the stream consumer) will emit
	// task.created + turn.started.
	traceID := ""
	if teamEmitter != nil {
		traceID = teamEmitter.TraceID()
	}
	projectMeta := r.buildTeamProjectMeta(ctx, sess, run, teamRow, def, ar, memberKeys, ti.content, traceID)
	streamOpts := r.newStreamConsumeOptions()
	if streamOpts == nil {
		streamOpts = &agent.StreamConsumeOptions{}
	}
	streamOpts.AbortOnStall = abortRun
	if streamOpts.V2Projector != nil {
		v2Meta := agent.V2ProjectMetaFromV1(projectMeta)
		streamOpts.V2Projector.Configure(v2Meta)
		runCtx = biz.WithActivityEmitter(runCtx, streamOpts.V2Projector)
	}

	// Publish session activities for ALL members at the start of execution,
	// regardless of graph/non-graph mode. Without these, the frontend
	// AgentCard has no parent node to render, and member thinking/action/reply
	// activities become orphaned at the root level.
	//
	// In graph mode, PublishTeamStepStarted may also fire later for each node
	// start — the deterministic activity ID (SessionActivityID) ensures the
	// frontend deduplicates and treats subsequent publishes as status updates
	// rather than new AgentCards. Members that never execute stay in "running"
	// until FinalizeGraphTeamRun or timeout transitions them.
	if r.hasPublisher() {
		for _, m := range ti.members {
			ag, lookupErr := r.lookupAgent(ctx, m.AgentID)
			if lookupErr != nil {
				r.lg.Warn("session activity publish: agent lookup failed",
					loggateway.StepID("team.run.session_publish"),
					loggateway.Str("agent_id", m.AgentID),
					loggateway.Err(lookupErr))
				continue
			}
			agentName := strutil.FirstNonEmpty(ag.DisplayName, ag.AgentKey)
			r.publishTeamStepActivity(ctx, run, teamRow.ID, ag.AgentKey, agentName,
				biz.ActivityEventCreated, biz.ActivityStatusRunning, "executing", nil)
		}
	}

	userTurnMsg, err := agent.BuildUserMessageFromArtifacts(runCtx, r.td.Persist.ArtifactUC, r.cfg.ToolResultGate, sess.ID, ti.content, input.Options.AttachmentIDs)
	if err != nil {
		logTeamRunError(teamEmitter, "team.run.attachments", err.Error(), mode)
		r.finishTeamRunWithError(ctx, &run, t0, err.Error(), &turnStatus, rollbackRunnerSession)
		return userMsg, biz.ChatMessage{}, err
	}

	// 批次 B：以锚点 agent 的 skill runtime policy 安装概览预算渲染器——
	// graph runtime 会把 root RunOptions 传播到成员 invocation，一次安装全链生效。
	var anchorSkillRuntime skillruntime.RuntimeSettings
	if ar.agent.Settings != nil {
		anchorSkillRuntime = ar.agent.Settings
	}
	runOpts := append(teamTurnBaseRunOptions(teamRow.ID, ti.content),
		skillruntime.RunOptionWithOverviewBudget(anchorSkillRuntime))
	runOpts = append(runOpts, utOpts.intentRunOpts...)
	// 2026-08-08 问题3c：DAG 下游团队把上游已完成团队的交付物种子注入
	// graph 初始 state（deliverable StateField，MergeReducer）。graph runtime
	// 会把 root RuntimeState 合并进 initialState（graph/trpc/builder.go），
	// 成员节点的 node-start RuntimeState 快照随之携带种子——get_deliverable
	// 直接读到上游 topic，不再因 per-execution state 隔离而 found=false。
	// 种子回流不冒充本团队产出：biz 层闸门/信封写入用同一解析结果减种子。
	if seed := r.resolveUpstreamDeliverableSeed(ctx, teamRow); len(seed) > 0 {
		runOpts = append(runOpts, trpcagent.MergeRuntimeState(map[string]any{biz.DeliverableStateKey: seed}))
	}
	// P2-1 模型级联 / P3-4 评测态 profile：definition 配置 model_cascade 时安装
	// run 级 ModelSelector——leader（synthesizer/意图锚点）保持高档 base，其余
	// 成员路由到成本档；配置 eval_profile 时钉住全成员模型/工具/生成参数
	// （ADR-E：两者互斥，eval_profile 胜出——可复现性优先于成本优化）。
	// agent-node 克隆父 RunOptions，成员 invocation 以各自 AgentName 应用
	// selector；run 级 selector 优先于成员构建级 selector（团队显式管理策略，
	// 覆盖成员自身的模型路由插件）。
	runOpts = append(runOpts, r.modelGovernanceRunOptions(ctx, def, sess.ID)...)
	// G2（ADR-F D2）：team 图执行注入 replanner 全局回调——节点静态声明
	// （fallback_agent/on_failure=skip）未恢复的失败经 RuntimeReplanner 决策
	// 落地（Reflexion 重试 / reroute→skip / insert_fallback→HITL）。非 graph
	// 模式或未配置 replanner 时返回 nil（append nil option 无效，须条件追加）。
	if opt := r.replanCallbacksRunOption(sess.ID, deriveSpiritSessionID(sess),
		ResolveLinkedGraphID(teamRow.LinkedGraphID, teamRow.DefinitionJSON), graphExecID); opt != nil {
		runOpts = append(runOpts, opt)
	}
	events, err := agent.RunTRPCUserTurnMsg(runCtx, runner, uid, sess.ID, userTurnMsg, runOpts...)
	if err != nil {
		logTeamRunError(teamEmitter, "team.run.execute", err.Error(), mode)
		r.finishTeamRunWithError(ctx, &run, t0, err.Error(), &turnStatus, rollbackRunnerSession)
		return userMsg, biz.ChatMessage{}, err
	}
	// F-C: 团队 Graph 路径绕过 trpcGraphRuntime.Run（唯一 EventBridge 宿主），
	// node_start/node_end 到不了 coordinator 的 step watch，per-member
	// team_run_steps 不持久化。tee 框架事件流，把图节点生命周期以 graph_stage
	// system notice 重发到 EventBus（watch 按 spiritSessionID + execution_id 过滤）。
	// Y3: HITL interrupt 经 tee 内联同步标记（先于流结束），避免 async bus
	// 握手与 run 收尾竞态（DeferTeamRunSuccessIfHITL 读到陈旧 Running 把暂停误判完成）。
	var onGraphInterrupt func(nodeID, lineageID string)
	if r.mediator != nil && graphExecID != "" {
		onGraphInterrupt = func(nodeID, lineageID string) {
			// HITL 标记可能触发于 run ctx 取消后：WithoutCancel 保留值、WithTimeout 兜底有界。
			markCtx, markCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer markCancel()
			if err := r.mediator.MarkTeamGraphInterrupt(markCtx, graphExecID, nodeID, lineageID); err != nil {
				r.lg.Warn("team graph: inline interrupt mark failed",
					loggateway.StepID("team.graph.interrupt_mark_fail"),
					loggateway.Str("exec_id", graphExecID),
					loggateway.Str("node_id", nodeID),
					loggateway.Err(err))
			}
		}
	}
	events = teeGraphStageNotices(ctx, events, r.td.Pipeline.EventBus, sess.ID, deriveSpiritSessionID(sess), ResolveLinkedGraphID(teamRow.LinkedGraphID, teamRow.DefinitionJSON), graphExecID, onGraphInterrupt, r.lg)
	events = event.WrapFrameworkEventsWithOtel(events, teamEmitter, teamBridge, teamBridge)

	// Phase 7: Consume stream (streamOpts already has the pre-created projector)
	var streamLastRoundPromptTok, streamLastRoundCompletionTok int
	var contextUsagePatched bool
	defer func() {
		if !contextUsagePatched && turnStatus != biz.TeamMemberStepStatusOK && streamLastRoundPromptTok > 0 {
			// Context patching uses the final round's tokens (window occupancy),
			// not the billing totals summed across rounds.
			sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, ar.agent, ar.prov, ar.mod, streamLastRoundPromptTok, streamLastRoundCompletionTok, r.lg)
		}
	}()

	result, streamErr := agent.ConsumeWithFirstByteGuard(runCtx, agent.DefaultFirstByteTimeout, events, projectMeta, streamOpts, r.lg)
	streamLastRoundPromptTok = result.LastRoundPromptTok
	streamLastRoundCompletionTok = result.LastRoundCompletionTok
	if streamErr != nil {
		turnStatus = biz.TeamMemberStepStatusError
		logTeamRunError(teamEmitter, "team.run.execute", streamErr.Error(), mode)
		rt.PublishLLMFailureNotice(r.td.Pipeline.EventBus, r.lg, ctx, deriveSpiritSessionID(sess), streamErr)
		r.finishRunErr(ctx, &run, t0, streamErr.Error())
		return userMsg, biz.ChatMessage{}, streamErr
	}
	if runCtx.Err() != nil {
		hint := ""
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			if dur := TurnDeadlineDuration(def); dur > 0 {
				hint = fmt.Sprintf(" hint=definition_timeout_hit effective=%s", dur)
			}
		case errors.Is(runCtx.Err(), context.Canceled):
			hint = " hint=client_disconnect_or_abort"
		}
		logTeamRunError(teamEmitter, "team.run.execute", runCtx.Err().Error()+hint, mode)
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, runCtx.Err().Error())
		return userMsg, biz.ChatMessage{}, runCtx.Err()
	}

	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.execute", "团队任务执行完成", event.P("mode", mode))
	}

	// Phase 8: Build assistant message from stream result
	abResult, err := r.buildAssistantMessageFromResult(result, ar, sess, ti.modOpt, turnArtCollector, ti.content)
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	assistantMsg = abResult.msg
	promptTok := abResult.promptTok
	completionTok := abResult.completionTok
	usageSource := abResult.usageSource

	// Phase 9: Validate output and persist assistant message
	if verr := validateAssistantOutput(result); verr != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, verr.Error())
		return userMsg, biz.ChatMessage{}, verr
	}

	if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, assistantMsg, true); err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	event.BumpSessionRevision(ctx, r.td.Sessions, sess.ID, r.lg)

	// Phase 10: Handle HITL defer, finalize run, and patch context usage
	if graphExecID != "" && r.mediator != nil {
		if deferred, derr := r.mediator.DeferTeamRunSuccessIfHITL(ctx, graphExecID, &run); derr != nil {
			r.lg.Warn("HITL defer 失败", loggateway.StepID("team.graph_runtime.hitl"), loggateway.Err(derr))
		} else if deferred {
			r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, result.CachedTok, ar.prov, ar.mod, ti.dialogMode, usageSource)
			if teamEmitter != nil {
				teamEmitter.LogDone("team.run.finish", "团队任务等待人工", event.P("status", run.Status))
			}
			return userMsg, assistantMsg, nil
		}
	}

	finishIn := TeamRunFinishInput{
		Run:            run,
		TeamID:         teamRow.ID,
		DefinitionJSON: teamRow.DefinitionJSON,
		Content:        ti.content,
		AssistantMsg:   assistantMsg,
		Result:         result,
		PromptTok:      promptTok,
		CompletionTok:  completionTok,
		UsageSource:    usageSource,
		Prov:           ar.prov,
		Mod:            ar.mod,
		DialogMode:     ti.dialogMode,
		GraphExecID:    graphExecID,
		AnchorMem:      ar.member,
		AnchorAg:       ar.agent,
	}
	// P2-1b (2026-08-19): persist genuine per-member usage rows from the
	// stream's MemberUsage — graph watch writes member steps with zero tokens,
	// so without this pass a watch-healthy team run is invisible to billable
	// aggregates (team_turn rows are excluded by design). When rows were
	// written, the anchor-fallback usage row is suppressed (double-count guard).
	usageFromStream := r.recordGraphMemberUsageFromResult(ctx, finishIn)
	r.finalizeGraphRunStepsFallback(ctx, finishIn, usageFromStream)

	// Persist swarm active member for CrossRequestTransfer (next turn entry override).
	if def.Swarm != nil && def.Swarm.CrossRequestTransfer && ar.agent.AgentKey != "" {
		writeSwarmActiveAgent(ctx, r.td.Sessions, sess, ar.agent.AgentKey)
	}

	run = r.finalizeTeamRun(ctx, sess, run, teamRow, ar, assistantMsg, promptTok, completionTok, result.CachedTok, ti.dialogMode, usageSource, graphExecID, t0, teamEmitter)

	// Context patching uses the final round's tokens (window occupancy), not
	// the billing totals; fall back to turn totals when usage was estimated.
	lastRoundPrompt, lastRoundCompletion := result.LastRoundPromptTok, result.LastRoundCompletionTok
	if lastRoundPrompt <= 0 && promptTok > 0 {
		lastRoundPrompt, lastRoundCompletion = promptTok, completionTok
	}
	sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, ar.agent, ar.prov, ar.mod, lastRoundPrompt, lastRoundCompletion, r.lg)
	contextUsagePatched = true

	return userMsg, assistantMsg, nil
}

func refsContainImageAttachment(refs []artifactbiz.Ref) bool {
	for _, ref := range refs {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref.MimeType)), "image/") {
			return true
		}
	}
	return false
}

func refsContainFileAttachment(refs []artifactbiz.Ref) bool {
	for _, ref := range refs {
		mime := strings.ToLower(strings.TrimSpace(ref.MimeType))
		if mime != "" && !strings.HasPrefix(mime, "image/") {
			return true
		}
	}
	return false
}
