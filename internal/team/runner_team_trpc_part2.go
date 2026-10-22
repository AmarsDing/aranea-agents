package team

import (
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/sandbox"
	sessctx "aranea-agents/internal/session"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

func (r *Runner) runTeamTRPCFromInput(ctx context.Context, sess biz.Session, input biz.TurnInput, teamRow biz.Team, def Definition, mode string) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {

	ti, err := r.validateTeamTurnInput(input, sess, def)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	run, err := r.createInitialTeamRun(ctx, sess, teamRow, def, mode, ti.content)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	r.registerRunTokenBudget(run.ID, resolveRunTokenBudget(def))
	defer r.releaseRunTokenBudget(run.ID)

	r.registerNoProgressAudit(run.ID, r.cfg.NoProgressAudit)
	defer r.releaseNoProgressAudit(run.ID)

	if r.cfg.SandboxManager != nil {
		defer r.cfg.SandboxManager.ReleaseRun(run.ID)
	}

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

	ar, turnStatus, err := r.resolveAnchorAndAttachments(ctx, ti.members, def.IntentAnchorAgentID, sess, input, ti.provOpt, ti.modOpt, &run, t0)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	pendingIntent := r.startTeamIntentPass(ctx, ar, ti.content)
	defer pendingIntent.stop()

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

	utOpts, turnStatus, err := r.prepareUserTurnOptions(ctx, ar, ti.content, sess, &run, teamRow, ti.dialogMode, t0, pendingIntent)
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

	runCtx, runCancel, turnArtCollector := r.buildTeamRunContext(ctx, def, builderDeps, input)
	if runCancel != nil {
		defer runCancel()
	}

	runCtx = sandbox.WithRunID(runCtx, run.ID)

	runCtx = decision.WithGateRunID(runCtx, run.ID)

	runCtx = decision.WithGateSessionID(runCtx, run.SessionID)

	// P0-4（A10）：首字节/stall 预算注入移入下方 attempt 闭包——invalid-retry
	// 每次重试按 (attemptNo-1) 递减 stall 重连池（InvalidStallReconnects）
	// 并同步重算消费端守卫总时限，防止模型层重连与编排层整轮重发的恢复
	// 预算被乘数放大。首次尝试保持全量（与接线前行为逐字节一致）。
	firstByteBudget := agent.ResolveFirstByteTimeout(runCtx, r.td.ReadDeps.LLM, ar.prov, ar.mod)
	stallTimeout, stallMaxReconnects := agent.ResolveStallPolicy(runCtx, r.td.ReadDeps.LLM, ar.prov, ar.mod)
	if teamEmitter != nil {
		teamEmitter.LogStart("team.run.execute", "执行团队任务", event.P("mode", mode))
	}

	traceID := ""
	if teamEmitter != nil {
		traceID = teamEmitter.TraceID()
	}
	projectMeta := r.buildTeamProjectMeta(ctx, sess, run, teamRow, def, ar, memberKeys, ti.content, traceID)
	// v2 投影 meta 跨尝试共享（projectMeta 不变）；projector 实例按尝试
	// 新建（newStreamConsumeOptions 每次返回独立实例，attempt 间无共享态）。
	v2Meta := agent.V2ProjectMetaFromV1(projectMeta)

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

	runOpts = append(runOpts, trpcagent.WithRunInvocationID(run.ID))
	runOpts = append(runOpts, utOpts.intentRunOpts...)

	if seed := r.resolveUpstreamDeliverableSeed(ctx, teamRow); len(seed) > 0 {
		runOpts = append(runOpts, trpcagent.MergeRuntimeState(map[string]any{biz.DeliverableStateKey: seed}))
	}

	runOpts = append(runOpts, r.modelGovernanceRunOptions(ctx, def, sess.ID)...)

	if opt := r.replanCallbacksRunOption(sess.ID, deriveSpiritSessionID(sess),
		StageGraphAssetID(teamRow.LinkedGraphID, teamRow.DefinitionJSON), graphExecID); opt != nil {
		runOpts = append(runOpts, opt)
	}
	// F-C/Y3: HITL interrupt 内联同步标记闭包（上移至 attempt 闭包前——每次
	// 重试的 tee 都复用它；语义保持「流期间同步标记，先于流结束」）。
	var onGraphInterrupt func(nodeID, lineageID string)
	if r.mediator != nil && graphExecID != "" {
		onGraphInterrupt = func(nodeID, lineageID string) {
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

	// Phase 7: Consume stream (per-attempt fresh streamOpts/projector)
	var streamLastRoundPromptTok, streamLastRoundCompletionTok int
	var contextUsagePatched bool
	defer func() {
		if !contextUsagePatched && turnStatus != biz.TeamMemberStepStatusOK && streamLastRoundPromptTok > 0 {

			sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, ar.agent, ar.prov, ar.mod, streamLastRoundPromptTok, streamLastRoundCompletionTok, r.lg)
		}
	}()

	var lastRunCtx context.Context
	invokeFailed := false
	attemptFn := func(attemptCtx context.Context, attemptNo int) (agent.EventStreamResult, error) {
		// invalid-retry stall 共账（P0-4 §12.2③）：第 1 次尝试保持全量
		// stall 重连池（equals 接线前行为，OFF 直通时逐字节一致）；后续
		// 每次重试递减重连预算，经 provider.WithStallBudget 同一 ctx key
		// 注入，消费端守卫总时限按剩余重连数同步重算。
		remainingReconnects := agent.InvalidStallReconnects(stallMaxReconnects, attemptNo)
		attemptRunCtx := provider.WithFirstByteRetryBudget(attemptCtx, firstByteBudget)
		attemptRunCtx = provider.WithStallBudget(attemptRunCtx, stallTimeout, remainingReconnects)
		attemptRunCtx, abortRun := context.WithCancel(attemptRunCtx)
		defer abortRun()
		lastRunCtx = attemptRunCtx

		// 每次尝试新建 consume options（内嵌独立 v2 projector，attempt 间
		// 无共享态；factory 语义见 newStreamConsumeOptions）。
		streamOpts := r.newStreamConsumeOptions()
		if streamOpts == nil {
			streamOpts = &agent.StreamConsumeOptions{}
		}
		streamOpts.AbortOnStall = abortRun

		streamOpts.OnPromptTokensAccumulated = func(delta int) {
			if r.accumulateRunTokenBudgetFromStream(attemptRunCtx, run, teamRow.ID, delta) {
				abortRun()
			}
		}

		if r.heartbeatWriter != nil && run.TeamRunV2ID != "" {
			pinger := newTeamRunHeartbeatPinger(r.heartbeatWriter, run.TeamRunV2ID, r.lg)
			streamOpts.OnStreamActivity = func() { pinger.Ping(attemptRunCtx) }
		}
		if streamOpts.V2Projector != nil {
			streamOpts.V2Projector.Configure(v2Meta)
			attemptRunCtx = biz.WithActivityEmitter(attemptRunCtx, streamOpts.V2Projector)
		}

		attemptEvents, runErr := agent.RunTRPCUserTurnMsg(attemptRunCtx, runner, uid, sess.ID, userTurnMsg, runOpts...)
		if runErr != nil {
			invokeFailed = true
			return agent.EventStreamResult{}, runErr
		}
		invokeFailed = false
		// F-C: 团队 Graph 路径绕过 trpcGraphRuntime.Run（唯一 EventBridge 宿主），
		// node_start/node_end 到不了 coordinator 的 step watch，per-member
		// team_run_steps 不持久化。tee 框架事件流，把图节点生命周期以 graph_stage
		// system notice 重发到 EventBus（watch 按 spiritSessionID + execution_id 过滤）。
		// Y3: HITL interrupt 经 tee 内联同步标记（先于流结束），避免 async bus
		// 握手与 run 收尾竞态（DeferTeamRunSuccessIfHITL 读到陈旧 Running 把暂停误判完成）。
		attemptEvents = teeGraphStageNotices(ctx, attemptEvents, r.td.Pipeline.EventBus, sess.ID, deriveSpiritSessionID(sess), StageGraphAssetID(teamRow.LinkedGraphID, teamRow.DefinitionJSON), graphExecID, onGraphInterrupt, r.lg)
		attemptEvents = event.WrapFrameworkEventsWithOtel(attemptEvents, teamEmitter, teamBridge, teamBridge)

		firstByteTimeout := provider.FirstByteGuardWithRetry(firstByteBudget, remainingReconnects)
		return agent.ConsumeWithFirstByteGuard(attemptRunCtx, firstByteTimeout, attemptEvents, projectMeta, streamOpts, r.lg)
	}

	// invalid-retry（P0-4 A10）：默认关闭（env 开关），关闭时单次直通——
	// 与接线前逐字节一致。
	retryOpts := agent.InvalidRetryOptionsFromEnv()
	retryOpts.Emitter = teamEmitter
	result, streamErr := agent.RunTurnWithInvalidRetry(runCtx, attemptFn, retryOpts, r.lg)
	streamLastRoundPromptTok = result.LastRoundPromptTok
	streamLastRoundCompletionTok = result.LastRoundCompletionTok
	if streamErr != nil && invokeFailed {
		// LLM 调用发起失败（RunTRPCUserTurnMsg err）：保留原错误路径。
		logTeamRunError(teamEmitter, "team.run.execute", streamErr.Error(), mode)
		r.finishTeamRunWithError(ctx, &run, t0, streamErr.Error(), &turnStatus, rollbackRunnerSession)
		return userMsg, biz.ChatMessage{}, streamErr
	}
	if streamErr != nil {
		turnStatus = biz.TeamMemberStepStatusError
		logTeamRunError(teamEmitter, "team.run.execute", streamErr.Error(), mode)
		rt.PublishLLMFailureNotice(r.td.Pipeline.EventBus, r.lg, ctx, deriveSpiritSessionID(sess), streamErr)
		r.finishRunErr(ctx, &run, t0, streamErr.Error())
		return userMsg, biz.ChatMessage{}, streamErr
	}
	if lastRunCtx != nil && lastRunCtx.Err() != nil {
		hint := ""
		errMsg := lastRunCtx.Err().Error()
		switch {
		case errors.Is(lastRunCtx.Err(), context.DeadlineExceeded):
			if dur := TurnDeadlineDuration(def); dur > 0 {
				hint = fmt.Sprintf(" hint=definition_timeout_hit effective=%s", dur)
			}
		case errors.Is(lastRunCtx.Err(), context.Canceled):
			if reason := r.cancelReason(sess.ID); reason != "" {
				hint = " hint=cancel_reason:" + reason
				errMsg += " (cancel_reason=" + reason + ")"
			} else {
				hint = " hint=client_disconnect_or_abort"
			}
		}
		logTeamRunError(teamEmitter, "team.run.execute", errMsg+hint, mode)
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, errMsg)
		return userMsg, biz.ChatMessage{}, lastRunCtx.Err()
	}

	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.execute", "团队任务执行完成", event.P("mode", mode))
	}

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

	if graphExecID != "" && r.mediator != nil {
		if deferred, derr := r.mediator.DeferTeamRunSuccessIfHITL(ctx, graphExecID, &run); derr != nil {
			r.lg.Warn("HITL defer 失败", loggateway.StepID("team.graph_runtime.hitl"), loggateway.Err(derr))
		} else if deferred {
			r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, result.CachedTok, result.CacheWriteTok, result.ReasoningTok, ar.prov, ar.mod, ti.dialogMode, usageSource)
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

	usageFromStream := r.recordGraphMemberUsageFromResult(ctx, finishIn)
	r.finalizeGraphRunStepsFallback(ctx, finishIn, usageFromStream)

	if def.Swarm != nil && def.Swarm.CrossRequestTransfer && ar.agent.AgentKey != "" {
		writeSwarmActiveAgent(ctx, r.td.Sessions, sess, ar.agent.AgentKey)
	}

	run = r.finalizeTeamRun(ctx, sess, run, teamRow, ar, assistantMsg, promptTok, completionTok, result.CachedTok, result.CacheWriteTok, result.ReasoningTok, ti.dialogMode, usageSource, graphExecID, t0, teamEmitter)

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
