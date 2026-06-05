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
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

func (r *Runner) runTeamTRPCFromInput(ctx context.Context, sess biz.Session, input biz.TurnInput, teamRow biz.Team, def Definition, mode string) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, _ := extractOptsFromInput(input)
	dialogMode = strutil.FirstNonEmpty(dialogMode, sess.DialogMode, "default")

	members := EnabledMembers(def)
	if len(members) == 0 {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "team has no enabled members")
	}

	run := biz.TeamRun{
		ID:                     uuid.NewString(),
		TeamID:                 teamRow.ID,
		SessionID:              sess.ID,
		Mode:                   mode,
		Status:                 biz.TeamRunStatusRunning,
		InputPreview:           preview(content, 512),
		TopologyJSON:           topologyJSON(def),
		DefinitionSnapshotJSON: strings.TrimSpace(teamRow.DefinitionJSON),
		StartedAt:              agent.RFC3339Now(),
		CreatedAt:              agent.RFC3339Now(),
		UpdatedAt:              agent.RFC3339Now(),
	}
	run, err = r.teams.CreateTeamRun(ctx, run)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	turnStatus := biz.TeamMemberStepStatusOK
	var teamBridge *turntrace.Bridge
	ctx, teamBridge, _ = turntrace.Start(ctx, turntrace.Config{
		Domain:    turntrace.DomainTeam,
		SpanName:  "team.run",
		SessionID: sess.ID,
		RunID:     run.ID,
		AgentKey:  teamRow.ID,
	})
	ctx = turntrace.WithBridge(ctx, teamBridge)
	defer func() { teamBridge.Finish(err) }()

	var teamEmitter *event.TraceEmitter
	if r.td.Pipeline.Bus != nil {
		teamEmitter = event.NewTraceEmitterForRun(event.TraceEmitterOpts{
			Ctx: ctx, Bus: r.td.Pipeline.Bus, Buffer: r.td.Pipeline.Buffer,
			SessionID: sess.ID, RunID: run.ID, AgentKey: teamRow.ID,
			Domain: event.TraceDomainTeam, LG: r.lg,
		})
		teamEmitter.SetOtelRefs(teamBridge.TraceID(), teamBridge.RootSpanID())
		ctx = event.WithTraceEmitter(ctx, teamEmitter)
		if tid := strings.TrimSpace(teamBridge.TraceID()); tid != "" {
			if uerr := r.teams.UpdateTeamRunTraceID(ctx, run.ID, tid); uerr != nil {
				r.lg.Warn("trace_id 持久化失败", loggateway.StepID("team.run.trace_id"), loggateway.Err(uerr))
			}
		}
		teamEmitter.LogStart("team.run.start", "开始团队协作",
			event.P("team_id", teamRow.ID), event.P("mode", mode), event.P("members", len(members)))
		defer func() {
			if teamEmitter != nil {
				teamEmitter.FinishRoot(turnStatus)
			}
		}()
	}

	if r.td.Pipeline.Bus != nil {
		cp := run
		env := event.NewEnvelope(event.EnvelopeTypeTeamRunStarted, "team-runner", sess.ID)
		env.TeamID = teamRow.ID
		env.Metadata = map[string]any{"run_id": run.ID, "run": cp}
		r.td.Pipeline.Bus.Publish(ctx, env)
	}

	t0 := time.Now()
	biz.DefaultTurnCompletionBridge().RegisterTurnStart(sess.ID, run.ID, t0)

	ar, turnStatus, err := r.resolveAnchorAndAttachments(ctx, members, def.IntentAnchorAgentID, sess, input, provOpt, modOpt, &run, t0)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	builderDeps := agent.TRPCBuilderDeps{
		Catalog:               r.td.Catalog.LLM,
		AgentUC:               r.td.Catalog.AgentsUC,
		Agents:                r.td.Catalog.Agents,
		RT:                    r.td.RoundTrip(),
		SkillUC:               r.td.Catalog.SkillUC,
		MCPTooling:            r.td.Persist.AgentMCP,
		ToolUC:                r.td.Catalog.ToolUC,
		Sessions:              r.td.Sessions,
		Sys:                   r.td.Catalog.Settings,
		Provider:              ar.prov,
		Model:                 ar.mod,
		DialogMode:            dialogMode,
		SkillDBRepo:           r.skillDBRepo,
		HasMemory:             r.td.Persist.Memory.Available(),
		MemoryService:         r.td.Persist.Memory.TRPC,
		PluginManager:         r.cfg.PluginManager,
		MemoryAdmin:           r.td.Persist.Memory.Admin,
		MemoryL2Recall:        r.td.Persist.Memory.L2Recall,
		MemoryL3Recall:        r.td.Persist.Memory.L3Recall,
		MemoryCompositeRecall: r.td.Persist.Memory.CompositeRecall,
		KnowledgeRetriever:    r.cfg.Knowledge.Retriever,
		CodeExecFactory:       r.codeExecFactory,
	}
	if r.cfg.AwaitHookProvider != nil {
		builderDeps.AwaitHook = r.cfg.AwaitHookProvider(ctx, sess.ID, run.ID)
	}
	teamDeps := TRPCTeamBuilderDeps{BuilderDeps: builderDeps, UseCache: true}

	root, memberLookup, graphExecID, compiledTeam, err := r.compileTeamRuntime(ctx, sess, teamRow, def, mode, teamDeps, teamEmitter, run.ID)
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if graphExecID != "" {
		run.GraphExecutionID = graphExecID
		if uerr := r.teams.UpdateTeamRunGraphExecutionID(ctx, run.ID, graphExecID); uerr != nil {
			r.lg.Warn("graph_execution_id 持久化失败", loggateway.StepID("team.graph_runtime.persist"), loggateway.Err(uerr))
		}
	}

	var plugins []trpcplugin.Plugin
	if r.cfg.PluginManager != nil {
		plugins = r.cfg.PluginManager.RunnerPluginsForAgent(ar.agent.ID)
	} else if r.cfg.PluginRT != nil {
		plugins = r.cfg.PluginRT.PluginsForAgent(ar.agent.ID)
	}
	builderDeps.Plugins = plugins
	memberKeys, err := memberAgentKeys(ctx, def, r.catalogAgent)
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

	utOpts, turnStatus, err := r.prepareUserTurnOptions(ctx, ar, content, sess, &run, teamRow, dialogMode, t0)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	userMsg = biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sess.ID,
		Role:             "user",
		ContentMarkdown:  content,
		Status:           biz.TeamMemberStepStatusOK,
		OptionsJSON:      utOpts.userOpts,
		CreatedAt:        agent.RFC3339Now(),
		AttachmentsCount: utOpts.attN,
	}

	if err := r.sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	uid := agent.UserIDFromCtx(ctx)

	runCtx := ctx
	if dur := TurnDeadlineDuration(def); dur > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}
	if builderDeps.AwaitHook != nil {
		runCtx = serviceawaitreply.WithReplyFunc(runCtx, builderDeps.AwaitHook)
	}
	if r.cfg.Knowledge != nil && r.cfg.Knowledge.Retriever != nil {
		runCtx = knowledgetool.WithRetriever(runCtx, r.cfg.Knowledge.Retriever)
	}
	if r.cfg.Knowledge != nil && r.cfg.Knowledge.Router != nil {
		runCtx = knowledgetool.WithAdaptiveRouter(runCtx, r.cfg.Knowledge.Router)
	}
	if r.cfg.Knowledge != nil && r.cfg.Knowledge.FederatedRetriever != nil {
		runCtx = knowledgetool.WithFederatedRetriever(runCtx, r.cfg.Knowledge.FederatedRetriever)
	}
	if r.cfg.Knowledge != nil && r.cfg.Knowledge.Evaluator != nil {
		runCtx = knowledgetool.WithRetrievalEvaluator(runCtx, r.cfg.Knowledge.Evaluator)
	}
	if len(input.Options.KnowledgeBases) > 0 {
		runCtx = knowledgetool.WithKnowledgeCollections(runCtx, input.Options.KnowledgeBases)
	}
	if teamEmitter != nil {
		teamEmitter.LogStart("team.run.execute", "执行团队任务", event.P("mode", mode))
	}
	var turnArtCollector *artifactbiz.TurnCollector
	runCtx, turnArtCollector = artifactbiz.WithTurnCollector(runCtx)

	userTurnMsg, err := agent.BuildUserMessageFromArtifacts(runCtx, r.td.Persist.ArtifactUC, sess.ID, content, input.Options.AttachmentIDs)
	if err != nil {
		if teamEmitter != nil {
			teamEmitter.LogError("team.run.attachments", err.Error(), event.P("mode", mode))
		}
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		rollbackRunnerSession()
		return userMsg, biz.ChatMessage{}, err
	}

	runOpts := append([]trpcagent.RunOption{skillruntime.RunOptionWithTurnQuery(content)}, utOpts.intentRunOpts...)
	events, err := agent.RunTRPCUserTurnMsg(runCtx, runner, uid, sess.ID, userTurnMsg, runOpts...)
	if err != nil {
		if teamEmitter != nil {
			teamEmitter.LogError("team.run.execute", err.Error(), event.P("mode", mode))
		}
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		rollbackRunnerSession()
		return userMsg, biz.ChatMessage{}, err
	}
	events = event.WrapFrameworkEventsWithOtel(events, teamEmitter, teamBridge, teamBridge)

	memberKeySet := make(map[string]struct{}, len(memberKeys))
	for _, k := range memberKeys {
		memberKeySet[k] = struct{}{}
	}
	traceID := ""
	if teamEmitter != nil {
		traceID = teamEmitter.TraceID()
	}
	contextWin := sessctx.ResolveContextWindowTokens(ctx, r.teamLLMCatalog(), sess, ar.agent, ar.prov, ar.mod)
	var streamPromptTok, streamCompletionTok int
	var contextUsagePatched bool
	defer func() {
		if !contextUsagePatched && turnStatus != biz.TeamMemberStepStatusOK && streamPromptTok > 0 {
			sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, ar.agent, ar.prov, ar.mod, streamPromptTok, streamCompletionTok, r.lg)
		}
	}()

	projectMeta := agent.ProjectMeta{
		SessionID:        sess.ID,
		RequestID:        run.ID,
		InvocationID:     run.ID,
		RunID:            run.ID,
		TraceID:          traceID,
		TeamID:           teamRow.ID,
		AgentID:          ar.agent.ID,
		AgentDisplayName: ar.agent.DisplayName,
		MemberAgentKeys:  memberKeySet,
		ContextWindow:    contextWin,
		Source:           event.EnvelopeSourceFromContext(ctx),
	}
	streamOpts := r.newStreamConsumeOptions()
	result, streamErr := agent.ConsumeWithFirstByteGuard(runCtx, agent.DefaultFirstByteTimeout, events, r.td.Pipeline.Bus, projectMeta, streamOpts, r.lg)
	streamPromptTok = result.PromptTok
	streamCompletionTok = result.CompletionTok
	if streamErr != nil {
		turnStatus = biz.TeamMemberStepStatusError
		if teamEmitter != nil {
			teamEmitter.LogError("team.run.execute", streamErr.Error(), event.P("mode", mode))
		}
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
		if teamEmitter != nil {
			teamEmitter.LogError("team.run.execute", runCtx.Err().Error()+hint, event.P("mode", mode))
		}
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, runCtx.Err().Error())
		return userMsg, biz.ChatMessage{}, runCtx.Err()
	}

	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.execute", "团队任务执行完成", event.P("mode", mode))
	}

	reasoningText := strings.TrimSpace(result.Reasoning.String())
	displayMarkdown := agent.DisplayMarkdownFromStream(result)
	promptTok, completionTok := agent.EstimateTokensIfMissing(result.PromptTok, result.CompletionTok, content, displayMarkdown)

	assistantOptsStr, err := agent.AssistantOptionsJSON(ar.agent, &agent.TeamMemberAnchor{
		AgentID: ar.agent.ID,
		Name:    strutil.FirstNonEmpty(ar.agent.DisplayName, ar.agent.AgentKey),
		Role:    ar.member.Role,
	})
	if err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	if reasoningText != "" {
		if assistantOptsStr, err = agent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, reasoningText); err != nil {
			turnStatus = biz.TeamMemberStepStatusError
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
	}
	if turnArtCollector != nil {
		if merged, merr := artifactbiz.MergeRefsIntoOptionsJSON(assistantOptsStr, turnArtCollector.Refs()); merr != nil {
			turnStatus = biz.TeamMemberStepStatusError
			r.finishRunErr(ctx, &run, t0, merr.Error())
			return userMsg, biz.ChatMessage{}, merr
		} else {
			assistantOptsStr = merged
		}
	}
	assistantAttN := 0
	if turnArtCollector != nil {
		assistantAttN = len(turnArtCollector.Refs())
	}

	assistantMsg = biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sess.ID,
		Role:             "assistant",
		ContentMarkdown:  displayMarkdown,
		ModelName:        strutil.FirstNonEmpty(modOpt, sess.DefaultModel, ar.agent.Model),
		Status:           biz.TeamMemberStepStatusOK,
		OptionsJSON:      assistantOptsStr,
		CreatedAt:        agent.RFC3339Now(),
		TokenIn:          promptTok,
		TokenOut:         completionTok,
		AttachmentsCount: assistantAttN,
	}

	if displayMarkdown == "" {
		fallback := "The team workflow produced no usable assistant reply. This may indicate a configuration issue."
		if result.HasError {
			fallback = fmt.Sprintf("Team AI service error: %s. Please check your configuration or try again later.", result.LastError)
		} else if result.HasContent {
			fallback = "The team workflow completed but produced no text output. This may indicate a configuration issue with the model."
		}
		err := kerrors.InternalServer("CHAT_TEAM_NATIVE", fallback)
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	if err := r.sessions.AppendChatMessage(ctx, sess.ID, assistantMsg, true); err != nil {
		turnStatus = biz.TeamMemberStepStatusError
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	event.BumpAndPublishSessionRevision(
		ctx,
		r.td.Sessions,
		r.td.Pipeline.Bus,
		sess.ID,
		run.ID,
		userMsg.ID,
		event.EnvelopeSourceFromContext(ctx),
		r.lg,
	)

	if graphExecID != "" && r.mediator != nil {
		if deferred, derr := r.mediator.DeferTeamRunSuccessIfHITL(ctx, graphExecID, &run); derr != nil {
			r.lg.Warn("HITL defer 失败", loggateway.StepID("team.graph_runtime.hitl"), loggateway.Err(derr))
		} else if deferred {
			r.recordTeamRunUsage(ctx, run, teamRow.ID, ar.agent, promptTok, completionTok, ar.prov, ar.mod, dialogMode)
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
		Content:        content,
		AssistantMsg:   assistantMsg,
		Result:         result,
		PromptTok:      promptTok,
		CompletionTok:  completionTok,
		Prov:           ar.prov,
		Mod:            ar.mod,
		DialogMode:     dialogMode,
		GraphExecID:    graphExecID,
		AnchorMem:      ar.member,
		AnchorAg:       ar.agent,
	}
	if graphExecID != "" {
		r.finalizeGraphRunStepsFallback(ctx, finishIn)
	} else {
		r.persistNativeBulkMemberSteps(ctx, finishIn, members)
	}

	r.finalizeTeamRun(ctx, &run, teamRow, ar, assistantMsg, promptTok, completionTok, dialogMode, graphExecID, t0, teamEmitter)

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
