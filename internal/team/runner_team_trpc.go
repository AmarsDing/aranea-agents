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
	teamDeps := TRPCTeamBuilderDeps{BuilderDeps: builderDeps, UseCache: true}

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

	plugins := r.resolveTeamPlugins(ar.agent.ID)
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
			r.cfg.Runs.Finish(sess.ID)
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

	// Phase 6.5: Build project metadata and pre-create ActivityProjector.
	// N-21/N-03: The projector is pre-created and injected into runCtx so that
	// plugins (cost_guard, model_router) and hooks (tool_confirmation) can emit
	// notice/confirm Activities via biz.ActivityEmitterFromContext during the
	// LLM call. The same projector is reused in ConsumeWithFirstByteGuard.
	traceID := ""
	if teamEmitter != nil {
		traceID = teamEmitter.TraceID()
	}
	projectMeta := r.buildTeamProjectMeta(ctx, sess, run, teamRow, ar, memberKeys, ti.content, traceID)
	streamOpts := r.newStreamConsumeOptions()
	if streamOpts != nil && streamOpts.ActivityProjector != nil {
		streamOpts.ActivityProjector.Reset()
		runCtx = streamOpts.ActivityProjector.OnTurnStart(runCtx, projectMeta)
		runCtx = biz.WithActivityEmitter(runCtx, streamOpts.ActivityProjector)
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
	if r.td.Pipeline.ActivityBus != nil {
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

	userTurnMsg, err := agent.BuildUserMessageFromArtifacts(runCtx, r.td.Persist.ArtifactUC, sess.ID, ti.content, input.Options.AttachmentIDs)
	if err != nil {
		logTeamRunError(teamEmitter, "team.run.attachments", err.Error(), mode)
		r.finishTeamRunWithError(ctx, &run, t0, err.Error(), &turnStatus, rollbackRunnerSession)
		return userMsg, biz.ChatMessage{}, err
	}

	runOpts := append([]trpcagent.RunOption{skillruntime.RunOptionWithTurnQuery(ti.content)}, utOpts.intentRunOpts...)
	events, err := agent.RunTRPCUserTurnMsg(runCtx, runner, uid, sess.ID, userTurnMsg, runOpts...)
	if err != nil {
		logTeamRunError(teamEmitter, "team.run.execute", err.Error(), mode)
		r.finishTeamRunWithError(ctx, &run, t0, err.Error(), &turnStatus, rollbackRunnerSession)
		return userMsg, biz.ChatMessage{}, err
	}
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
			r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, ar.prov, ar.mod, ti.dialogMode)
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

	// Publish completed session activities for non-graph mode.
	if graphExecID == "" && r.td.Pipeline.ActivityBus != nil {
		for _, m := range ti.members {
			ag, lookupErr := r.lookupAgent(ctx, m.AgentID)
			if lookupErr != nil {
				continue
			}
			agentName := strutil.FirstNonEmpty(ag.DisplayName, ag.AgentKey)
			r.publishTeamStepActivity(ctx, run, teamRow.ID, ag.AgentKey, agentName,
				biz.ActivityEventCompleted, biz.ActivityStatusCompleted, "completed", nil)
		}
	}

	run = r.finalizeTeamRun(ctx, sess, run, teamRow, ar, assistantMsg, promptTok, completionTok, ti.dialogMode, graphExecID, t0, teamEmitter)

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
