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
	}
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
		if rberr := runnerMgr.RollbackToBoundary(context.Background(), rollbackBoundary); rberr != nil {
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
	if streamOpts != nil && streamOpts.V2Projector != nil {
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

	runOpts := append(teamTurnBaseRunOptions(teamRow.ID, ti.content), utOpts.intentRunOpts...)
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
	events = teeGraphStageNotices(events, r.td.Pipeline.EventBus, sess.ID, deriveSpiritSessionID(sess), ResolveLinkedGraphID(teamRow.LinkedGraphID, teamRow.DefinitionJSON), graphExecID, r.lg)
	events = event.WrapFrameworkEventsWithOtel(events, teamEmitter, teamBridge, teamBridge)

	// Phase 7: Consume stream (streamOpts already has the pre-created projector)
	var streamPromptTok, streamCompletionTok int
	var contextUsagePatched bool
	defer func() {
		if !contextUsagePatched && turnStatus != biz.TeamMemberStepStatusOK && streamPromptTok > 0 {
			sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, ar.agent, ar.prov, ar.mod, streamPromptTok, streamCompletionTok, r.lg)
		}
	}()

	result, streamErr := agent.ConsumeWithFirstByteGuard(runCtx, agent.DefaultFirstByteTimeout, events, projectMeta, streamOpts, r.lg)
	streamPromptTok = result.PromptTok
	streamCompletionTok = result.CompletionTok
	if streamErr != nil {
		turnStatus = biz.TeamMemberStepStatusError
		logTeamRunError(teamEmitter, "team.run.execute", streamErr.Error(), mode)
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
			r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, result.CachedTok, ar.prov, ar.mod, ti.dialogMode)
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
		Prov:           ar.prov,
		Mod:            ar.mod,
		DialogMode:     ti.dialogMode,
		GraphExecID:    graphExecID,
		AnchorMem:      ar.member,
		AnchorAg:       ar.agent,
	}
	r.finalizeGraphRunStepsFallback(ctx, finishIn)

	// Persist swarm active member for CrossRequestTransfer (next turn entry override).
	if def.Swarm != nil && def.Swarm.CrossRequestTransfer && ar.agent.AgentKey != "" {
		writeSwarmActiveAgent(ctx, r.td.Sessions, sess, ar.agent.AgentKey)
	}

	run = r.finalizeTeamRun(ctx, sess, run, teamRow, ar, assistantMsg, promptTok, completionTok, result.CachedTok, ti.dialogMode, graphExecID, t0, teamEmitter)

	sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, ar.agent, ar.prov, ar.mod, promptTok, completionTok, r.lg)
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
