package team

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"aranea-agents/internal/event"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/strutil"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

func (r *Runner) runTeamTRPC(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, teamRow biz.Team, def Definition, mode string) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, attN := extractOpts(req)
	dialogMode = strutil.FirstNonEmpty(dialogMode, sess.DialogMode, "default")

	members := EnabledMembers(def)
	if len(members) == 0 {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("TEAM", "team has no enabled members")
	}

	run := biz.TeamRun{
		ID:           uuid.NewString(),
		TeamID:       teamRow.ID,
		SessionID:    sess.ID,
		Mode:         mode,
		Status:       "running",
		InputPreview: preview(content, 512),
		TopologyJSON: topologyJSON(def),
		StartedAt:    agent.RFC3339Now(),
		CreatedAt:    agent.RFC3339Now(),
		UpdatedAt:    agent.RFC3339Now(),
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
		Catalog:     r.td.Catalog.LLM,
		AgentUC:     r.td.Catalog.AgentsUC,
		Agents:      r.td.Catalog.Agents,
		RT:          r.td.RoundTrip(),
		SkillUC:     r.td.Catalog.SkillUC,
		MCPTooling:  r.td.Persist.AgentMCP,
		ToolUC:      r.td.Catalog.ToolUC,
		Sessions:    r.td.Sessions,
		Sys:         r.td.Catalog.Settings,
		Provider:    prov0,
		Model:       mod0,
		DialogMode:  dialogMode,
		SkillDBRepo: r.skillDBRepo,
		HasMemory:     r.td.Persist.Memory.Available(),
		PluginManager: r.pluginManager,
		MemoryAdmin:        r.td.Persist.Memory.Admin,
		KnowledgeRetriever: r.knowledgeRetriever,
		CodeExecFactory:    r.codeExecFactory,
	}
	if r.awaitHookProvider != nil {
		builderDeps.AwaitHook = r.awaitHookProvider(ctx, sess.ID, run.ID)
	}
	teamDeps := TRPCTeamBuilderDeps{BuilderDeps: builderDeps, UseCache: true}
	root, memberLookup, err := BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
	if err != nil {
		turnStatus = "error"
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if teamEmitter != nil {
		teamEmitter.LogDone("team.run.build", "团队 Agent 已构建", event.P("mode", mode))
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
	sendText := content
	intRes := intent.Run(ctx, intent.IntentPassFromAgent(firstAg), r.td.Catalog.LLM, r.td.LLMHTTP, prov0, mod0, content)
	if intRes.Artifact != nil {
		if strings.TrimSpace(intRes.RawJSON) != "" {
			merged, merr := intent.MergeIntoUserOptionsJSON(userOpts, intRes.RawJSON)
			if merr != nil {
				event.CtxFlowLogWarn(ctx, "team.intent.merge_fail", "团队意图合并失败，将继续执行", event.P("error", merr))
			} else {
				userOpts = merged
			}
		}
		sendText = intent.WrapUserMessage(content, intRes.Artifact)
	}
	meta := intent.RunMeta{
		AgentID:   firstAg.ID,
		SessionID: sess.ID,
		RunID:     run.ID,
		TeamID:    teamRow.ID,
	}
	if r.td.Pipeline.Bus != nil {
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

	events, err := agent.RunTRPCUserTurn(runCtx, runner, uid, sess.ID, sendText, skillruntime.RunOptionWithTurnQuery(sendText))
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
	projectMeta := agent.ProjectMeta{
		SessionID:       sess.ID,
		RequestID:       run.ID,
		InvocationID:    run.ID,
		RunID:           run.ID,
		TraceID:         traceID,
		TeamID:          teamRow.ID,
		AgentID:         firstAg.ID,
		AgentDisplayName: firstAg.DisplayName,
		MemberAgentKeys: memberKeySet,
	}
	streamOpts := chatactivity.NewStreamConsumeOptions(r.td.Catalog.ToolUC, r.td.Catalog.Agents, r.td.Sessions)
	result := agent.ConsumeEventStream(runCtx, events, r.td.Pipeline.Bus, projectMeta, streamOpts)
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

	replyText := strings.TrimSpace(result.Reply.String())
	reasoningText := strings.TrimSpace(result.Reasoning.String())
	promptTok, completionTok := result.PromptTok, result.CompletionTok
	if promptTok <= 0 && completionTok <= 0 && replyText != "" {
		promptTok = agent.RoughTokenEstimate(content + replyText)
		completionTok = agent.RoughTokenEstimate(replyText)
	}

	displayMarkdown := replyText
	if displayMarkdown == "" && reasoningText != "" {
		displayMarkdown = reasoningText
	}

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

	for i, m := range members {
		ag, e := r.catalogAgent(ctx, m.AgentID)
		if e != nil {
			continue
		}
		stepMsg := assistantMsg
		stepMsg.TokenIn, stepMsg.TokenOut = stepTokensForMember(ag.AgentKey, i, result, promptTok, completionTok)
		toolCalls := 0
		if result.MemberToolCalls != nil {
			toolCalls = result.MemberToolCalls[ag.AgentKey]
		}
		r.persistStep(ctx, run, teamRow.ID, i, m, ag, content, stepMsg, prov0, mod0, dialogMode, toolCalls)
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
	win := compressAg.ContextWindow
	if win <= 0 {
		win = 128000
	}
	_ = r.td.Sessions.UpdateSessionContextFromLLMUsage(ctx, sess.ID, promptTok, completionTok, win)
	if r.td.Compress != nil {
		r.td.Compress.AfterNativeTurn(ctx, sess.ID, compressAg)
	}

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
