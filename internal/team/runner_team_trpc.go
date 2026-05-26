package team

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"aranea-agents/internal/event"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/metrics"
	rt "aranea-agents/internal/runtime"
	sessctx "aranea-agents/internal/session"
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/strutil"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

func (r *Runner) runTeamTRPCFromInput(ctx context.Context, sess biz.Session, input biz.TurnInput, teamRow biz.Team, def Definition, mode string) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, attN := extractOptsFromInput(input)
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
		Status:                 "running",
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

	turnStatus := "ok"
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
			Domain: event.TraceDomainTeam,
		})
		teamEmitter.SetOtelRefs(teamBridge.TraceID(), teamBridge.RootSpanID())
		ctx = event.WithTraceEmitter(ctx, teamEmitter)
		if tid := strings.TrimSpace(teamBridge.TraceID()); tid != "" {
			if uerr := r.teams.UpdateTeamRunTraceID(ctx, run.ID, tid); uerr != nil {
				event.CtxFlowLogWarn(ctx, "team.run.trace_id", "trace_id 持久化失败", event.P("error", uerr.Error()))
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
	anchorMem := members[0]
	if want := strings.TrimSpace(def.IntentAnchorAgentID); want != "" {
		found := false
		for _, m := range members {
			if strings.TrimSpace(m.AgentID) == want {
				anchorMem = m
				found = true
				break
			}
		}
		if !found {
			event.CtxFlowLogWarn(ctx, "team.intent_anchor_fallback", "团队意图锚点不在成员列表，使用首个成员",
				event.P("intent_anchor_agent_id", want), event.P("team_id", teamRow.ID))
		}
	}
	firstAg, err := r.catalogAgent(ctx, anchorMem.AgentID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = kerrors.NotFound("AGENT", "team member agent not found")
		}
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	prov0 := strutil.FirstNonEmpty(provOpt, sess.DefaultProvider, firstAg.Provider)
	mod0 := strutil.FirstNonEmpty(modOpt, sess.DefaultModel, firstAg.Model)
	builderDeps := agent.TRPCBuilderDeps{
		Catalog:            r.td.Catalog.LLM,
		AgentUC:            r.td.Catalog.AgentsUC,
		Agents:             r.td.Catalog.Agents,
		RT:                 r.td.RoundTrip(),
		SkillUC:            r.td.Catalog.SkillUC,
		MCPTooling:         r.td.Persist.AgentMCP,
		ToolUC:             r.td.Catalog.ToolUC,
		Sessions:           r.td.Sessions,
		Sys:                r.td.Catalog.Settings,
		Provider:           prov0,
		Model:              mod0,
		DialogMode:         dialogMode,
		SkillDBRepo:        r.skillDBRepo,
		HasMemory:          r.td.Persist.Memory.Available(),
		PluginManager:      r.pluginManager,
		MemoryAdmin:        r.td.Persist.Memory.Admin,
		MemoryL2Recall:        r.td.Persist.Memory.L2Recall,
		MemoryL3Recall:        r.td.Persist.Memory.L3Recall,
		MemoryCompositeRecall: r.td.Persist.Memory.CompositeRecall,
		KnowledgeRetriever: r.knowledgeRetriever,
		CodeExecFactory:    r.codeExecFactory,
	}
	if r.awaitHookProvider != nil {
		builderDeps.AwaitHook = r.awaitHookProvider(ctx, sess.ID, run.ID)
	}
	teamDeps := TRPCTeamBuilderDeps{BuilderDeps: builderDeps, UseCache: true}

	graphExecID := ""
	var root trpcagent.Agent
	var memberLookup map[string]trpcagent.Agent
	var compiledGraphCfg biz.GraphBuildConfig
	graphAttempted := false
	graphCompileErr := ""
	graphBuildErr := ""
	useGraph := r.graphRoot != nil && TeamGraphRuntimeEnabledForTeam(def, teamRow.ID) && SupportsTeamGraphRuntimeMode(mode)
	if useGraph {
		graphAttempted = true
		graphExecID = uuid.NewString()
		cfg, cerr := CompileToGraphRuntimeConfigFromJSON(ctx, def, teamRow.DefinitionJSON, func(agentID string) string {
			ag, gerr := r.catalogAgent(ctx, agentID)
			if gerr != nil {
				return ""
			}
			return strings.TrimSpace(ag.AgentKey)
		}, r.graphLoader)
		if cerr != nil {
			graphCompileErr = cerr.Error()
			event.CtxFlowLogWarn(ctx, "team.graph_runtime.compile", "Graph 编译失败", event.P("error", cerr.Error()))
			metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "compile_error").Inc()
		} else {
			compiledGraphCfg = cfg
			groot, gerr := r.graphRoot.BuildTeamGraphRoot(ctx, cfg)
			if gerr != nil {
				graphBuildErr = gerr.Error()
				event.CtxFlowLogWarn(ctx, "team.graph_runtime.build", "GraphAgent 构建失败", event.P("error", gerr.Error()))
				metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "build_error").Inc()
			} else {
				root = groot
				_, memberLookup, err = BuildTeamMemberAgents(ctx, def, teamDeps, r.catalogAgent)
				if err != nil {
					turnStatus = "error"
					r.finishRunErr(ctx, &run, t0, err.Error())
					return biz.ChatMessage{}, biz.ChatMessage{}, err
				}
				run.GraphExecutionID = graphExecID
				if uerr := r.teams.UpdateTeamRunGraphExecutionID(ctx, run.ID, graphExecID); uerr != nil {
					event.CtxFlowLogWarn(ctx, "team.graph_runtime.persist", "graph_execution_id 持久化失败", event.P("error", uerr.Error()))
				}
				if r.teamGraphCoord != nil {
					if regErr := r.teamGraphCoord.RegisterTeamGraphExecution(ctx, graphExecID, sess.ID, teamRow.ID, run.ID, compiledGraphCfg); regErr != nil {
						event.CtxFlowLogWarn(ctx, "team.graph_runtime.register", "graph execution 注册失败", event.P("error", regErr.Error()))
					}
				}
				if teamEmitter != nil {
					teamEmitter.LogDone("team.run.graph", "Team GraphAgent 已构建", event.P("graph_execution_id", graphExecID))
				}
				metrics.TeamGraphRuntimeTotal.WithLabelValues("graph", "success").Inc()
			}
		}
	}
	if root == nil {
		decision := DecideNativeFallback(def, teamRow.ID, graphAttempted, graphCompileErr, graphBuildErr, mode, r.graphRoot != nil)
		if decision.UseNative {
			root, memberLookup, err = BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
			if err != nil {
				turnStatus = "error"
				r.finishRunErr(ctx, &run, t0, err.Error())
				return biz.ChatMessage{}, biz.ChatMessage{}, err
			}
			metrics.TeamGraphRuntimeTotal.WithLabelValues("native", decision.MetricLabel).Inc()
			if teamEmitter != nil {
				teamEmitter.LogDone("team.run.build", "团队 Native 应急路径已构建", event.P("mode", mode), event.P("graph_attempted", graphAttempted))
			}
		} else {
			turnStatus = "error"
			r.finishRunErr(ctx, &run, t0, decision.ErrorMessage)
			return biz.ChatMessage{}, biz.ChatMessage{}, decision.Error()
		}
	}

	var plugins []trpcplugin.Plugin
	if r.pluginManager != nil {
		plugins = r.pluginManager.RunnerPluginsForAgent(firstAg.ID)
	} else if r.pluginRT != nil {
		plugins = r.pluginRT.PluginsForAgent(firstAg.ID)
	}
	builderDeps.Plugins = plugins
	memberKeys, err := memberAgentKeys(ctx, def, r.catalogAgent)
	if err != nil {
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	var stopObsProjector context.CancelFunc
	var stopTaskBridge context.CancelFunc
	var stopExecTracker context.CancelFunc
	var stopGraphStepWatch context.CancelFunc
	var activityFlusher *ActivityStepFlusher
	obs := r.startObservers(ctx, sess, teamRow, def, run, graphExecID, compiledGraphCfg)
	stopObsProjector = obs.stopObsProjector
	stopTaskBridge = obs.stopTaskBridge
	stopExecTracker = obs.stopExecTracker
	stopGraphStepWatch = obs.stopGraphStepWatch
	activityFlusher = obs.activityFlusher
	if activityFlusher != nil {
		defer activityFlusher.Stop()
	}
	if stopObsProjector != nil {
		defer stopObsProjector()
	}
	if stopTaskBridge != nil {
		defer stopTaskBridge()
	}
	if stopExecTracker != nil {
		defer stopExecTracker()
	}
	if stopGraphStepWatch != nil {
		defer stopGraphStepWatch()
	}

	// Team Ralph Loop uses the first member agent's runtime settings (orchestrator / lead).
	rl := agent.ResolveRalphLoopTurn(firstAg.Settings)
	if rl.SkipErr != nil {
		event.CtxFlowLogWarn(ctx, "team.runner.ralph_loop", "Ralph Loop 配置无效，已跳过",
			event.P("agent_id", firstAg.ID), event.P("error", rl.SkipErr.Error()))
	}
	runner, err := r.td.CoalesceRunnerManager().NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:               plugins,
		AwaitUserReplyRouting: builderDeps.AwaitHook != nil,
		BuilderDeps:           builderDeps,
		AgentFactoryKeys:      memberKeys,
		LookupAgents:          memberLookup,
		RalphLoop:             rl.Config,
	})
	if err != nil {
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if r.runs != nil {
		r.runs.StoreRunner(sess.ID, run.ID, runner)
	}
	defer func() {
		if r.runs != nil {
			r.runs.Finish(sess.ID)
		}
		runner.Close()
	}()

	anchor := &agent.TeamMemberAnchor{
		AgentID: firstAg.ID,
		Name:    strutil.FirstNonEmpty(firstAg.DisplayName, firstAg.AgentKey),
		Role:    anchorMem.Role,
	}
	userOpts, err := agent.UserOptionsJSON(firstAg, dialogMode, prov0, mod0, sess.ContextUsedRatio, anchor)
	if err != nil {
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	var intentRunOpts []trpcagent.RunOption
	var intRes intent.RunResult
	if intent.ShouldRun(firstAg, content) {
		intRes = intent.RunForAgent(ctx, firstAg, r.td.Catalog.LLM, r.td.LLMHTTP, prov0, mod0, content)
		if intRes.Artifact != nil {
			if strings.TrimSpace(intRes.RawJSON) != "" {
				merged, merr := intent.MergeIntoUserOptionsJSON(userOpts, intRes.RawJSON)
				if merr != nil {
					event.CtxFlowLogWarn(ctx, "team.intent.merge_fail", "团队意图合并失败，将继续执行", event.P("error", merr))
				} else {
					userOpts = merged
				}
			}
			intentRunOpts = append(intentRunOpts, intent.RunOptionInject(intRes.Artifact))
		}
	}
	if r.td.Persist.ArtifactUC != nil && len(artifactbiz.NormalizeAttachmentIDs(input.Options.AttachmentIDs)) > 0 {
		refs, rerr := artifactbiz.ResolveAttachmentRefs(ctx, r.td.Persist.ArtifactUC, sess.ID, input.Options.AttachmentIDs)
		if rerr != nil {
			turnStatus = "error"
			r.finishRunErr(ctx, &run, t0, rerr.Error())
			return biz.ChatMessage{}, biz.ChatMessage{}, rerr
		}
		attN = len(refs)
		var merr error
		userOpts, merr = artifactbiz.MergeRefsIntoOptionsJSON(userOpts, refs)
		if merr != nil {
			turnStatus = "error"
			r.finishRunErr(ctx, &run, t0, merr.Error())
			return biz.ChatMessage{}, biz.ChatMessage{}, merr
		}
	}
	if intent.ShouldRun(firstAg, content) && r.td.Pipeline.Bus != nil {
		meta := intent.RunMeta{
			AgentID:   firstAg.ID,
			SessionID: sess.ID,
			RunID:     run.ID,
			TeamID:    teamRow.ID,
		}
		env := event.NewEnvelope(event.EnvelopeTypeIntentPass, firstAg.ID, sess.ID)
		env.TeamID = teamRow.ID
		env.Metadata = intent.BuildIntentPassPayload(intRes, meta)
		r.td.Pipeline.Bus.Publish(ctx, env)
	}

	userMsg = biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sess.ID,
		Role:             "user",
		ContentMarkdown:  content,
		Status:           "ok",
		OptionsJSON:      userOpts,
		CreatedAt:        agent.RFC3339Now(),
		AttachmentsCount: attN,
	}

	if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
		turnStatus = "error"
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
	if r.knowledgeRetriever != nil {
		runCtx = knowledgetool.WithRetriever(runCtx, r.knowledgeRetriever)
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
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	runOpts := append([]trpcagent.RunOption{skillruntime.RunOptionWithTurnQuery(content)}, intentRunOpts...)
	events, err := agent.RunTRPCUserTurnMsg(runCtx, runner, uid, sess.ID, userTurnMsg, runOpts...)
	if err != nil {
		if teamEmitter != nil {
			teamEmitter.LogError("team.run.execute", err.Error(), event.P("mode", mode))
		}
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
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
	contextWin := sessctx.ResolveContextWindowTokens(ctx, r.teamLLMCatalog(), sess, firstAg, prov0, mod0)
	var streamPromptTok, streamCompletionTok int
	var contextUsagePatched bool
	defer func() {
		if !contextUsagePatched && turnStatus != "ok" && streamPromptTok > 0 {
			sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, firstAg, prov0, mod0, streamPromptTok, streamCompletionTok)
		}
	}()

	projectMeta := agent.ProjectMeta{
		SessionID:        sess.ID,
		RequestID:        run.ID,
		InvocationID:     run.ID,
		RunID:            run.ID,
		TraceID:          traceID,
		TeamID:           teamRow.ID,
		AgentID:          firstAg.ID,
		AgentDisplayName: firstAg.DisplayName,
		MemberAgentKeys:  memberKeySet,
		ContextWindow:    contextWin,
		Source:           event.EnvelopeSourceFromContext(ctx),
	}
	streamOpts := r.newStreamConsumeOptions()
	result, streamErr := agent.ConsumeWithFirstByteGuard(runCtx, agent.DefaultFirstByteTimeout, events, r.td.Pipeline.Bus, projectMeta, streamOpts)
	streamPromptTok = result.PromptTok
	streamCompletionTok = result.CompletionTok
	if streamErr != nil {
		turnStatus = "error"
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
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, runCtx.Err().Error())
		return userMsg, biz.ChatMessage{}, runCtx.Err()
	}

	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.execute", "团队任务执行完成", event.P("mode", mode))
	}

	reasoningText := strings.TrimSpace(result.Reasoning.String())
	displayMarkdown := agent.DisplayMarkdownFromStream(result)
	promptTok, completionTok := agent.EstimateTokensIfMissing(result.PromptTok, result.CompletionTok, content, displayMarkdown)

	assistantOptsStr, err := agent.AssistantOptionsJSON(firstAg, anchor)
	if err != nil {
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	if reasoningText != "" {
		if assistantOptsStr, err = agent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, reasoningText); err != nil {
			turnStatus = "error"
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
	}
	if turnArtCollector != nil {
		if merged, merr := artifactbiz.MergeRefsIntoOptionsJSON(assistantOptsStr, turnArtCollector.Refs()); merr != nil {
			turnStatus = "error"
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
		ID:              uuid.NewString(),
		SessionID:       sess.ID,
		Role:            "assistant",
		ContentMarkdown: displayMarkdown,
		ModelName:       strutil.FirstNonEmpty(modOpt, sess.DefaultModel, firstAg.Model),
		Status:          "ok",
		OptionsJSON:     assistantOptsStr,
		CreatedAt:       agent.RFC3339Now(),
		TokenIn:         promptTok,
		TokenOut:        completionTok,
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
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, assistantMsg, true); err != nil {
		turnStatus = "error"
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
	)

	// Graph HITL: defer before team_run_steps bulk persist (steps come from graph events + resume finisher).
	if graphExecID != "" && r.teamGraphCoord != nil {
		if deferred, derr := r.teamGraphCoord.DeferTeamRunSuccessIfHITL(ctx, graphExecID, &run); derr != nil {
			event.CtxFlowLogWarn(ctx, "team.graph_runtime.hitl", "HITL defer 失败", event.P("error", derr.Error()))
		} else if deferred {
			r.recordTeamRunUsage(ctx, run, teamRow.ID, firstAg, promptTok, completionTok, prov0, mod0, dialogMode)
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
		Prov:           prov0,
		Mod:            mod0,
		DialogMode:     dialogMode,
		GraphExecID:    graphExecID,
		AnchorMem:      anchorMem,
		AnchorAg:       firstAg,
	}
	if graphExecID == "" {
		r.persistNativeBulkMemberSteps(ctx, finishIn, members)
	} else {
		r.finalizeGraphRunStepsFallback(ctx, finishIn)
	}
	r.recordTeamRunUsage(ctx, run, teamRow.ID, firstAg, promptTok, completionTok, prov0, mod0, dialogMode)

	run.Status = "success"
	run.TokenIn = promptTok
	run.TokenOut = completionTok
	run.DurationMS = int(time.Since(t0).Milliseconds())
	run.OutputPreview = preview(assistantMsg.ContentMarkdown, 512)
	run.FinishedAt = agent.RFC3339Now()
	_ = r.teams.UpdateTeamRun(ctx, run)

	compressAg := firstAg
	sessctx.PatchContextFromLLMUsage(ctx, r.td.Sessions, r.td.Compress, r.teamLLMCatalog(), sess.ID, sess, compressAg, prov0, mod0, promptTok, completionTok)
	contextUsagePatched = true

	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.finish", "团队任务结束", event.P("status", run.Status))
	}
	if r.td.Pipeline.Bus != nil {
		cp := run
		env := event.NewEnvelope(event.EnvelopeTypeTeamRunFinished, "team-runner", sess.ID)
		env.TeamID = teamRow.ID
		env.Metadata = map[string]any{"run_id": run.ID, "run": cp}
		r.td.Pipeline.Bus.Publish(ctx, env)
		r.publishTeamRunSummary(ctx, run)
	}
	return userMsg, assistantMsg, nil
}
