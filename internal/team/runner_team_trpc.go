package team

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/tools/serviceawaitreply"
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
	publishTeamMonitor(ctx, r.td.Pipeline.Bus, "INFO", fmt.Sprintf(
		"team_turn phase=start session_id=%s run_id=%s team_id=%s mode=%s members=%d",
		sess.ID, run.ID, teamRow.ID, mode, len(members)), sess.ID)
	if r.td.Pipeline.Bus != nil {
		cp := run
		env := event.NewEnvelope(event.EnvelopeTypeTeamRunStarted, "team-runner", sess.ID)
		env.TeamID = teamRow.ID
		env.Metadata = map[string]any{"run_id": run.ID, "run": cp}
		r.td.Pipeline.Bus.Publish(ctx, env)
	}

	t0 := time.Now()
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
			slog.Warn("team run: intent_anchor_agent_id not in enabled members; using first member",
				"intent_anchor_agent_id", want, "team_id", teamRow.ID)
		}
	}
	firstAg, err := r.catalogAgent(ctx, anchorMem.AgentID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = kerrors.NotFound("AGENT", "team member agent not found")
		}
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
		MemoryAdmin:   r.td.Persist.Memory.Admin,
	}
	if r.awaitHookProvider != nil {
		builderDeps.AwaitHook = r.awaitHookProvider(ctx, sess.ID, run.ID)
	}
	teamDeps := TRPCTeamBuilderDeps{BuilderDeps: builderDeps, UseCache: true}
	root, err := BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	publishTeamMonitor(ctx, r.td.Pipeline.Bus, "INFO", fmt.Sprintf(
		"team_turn phase=team_built session_id=%s run_id=%s mode=%s",
		sess.ID, run.ID, mode), sess.ID)

	var plugins []trpcplugin.Plugin
	if r.pluginManager != nil {
		plugins = r.pluginManager.RunnerPluginsForAgent(firstAg.ID)
	} else if r.pluginRT != nil {
		plugins = r.pluginRT.PluginsForAgent(firstAg.ID)
	}
	builderDeps.Plugins = plugins
	memberKeys, err := memberAgentKeys(ctx, def, r.catalogAgent)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if r.td.RunnerMgr == nil {
		r.td.RunnerMgr = rt.NewRunnerManagerFromPersist(r.td.Persist)
	}
	runner, err := r.td.RunnerMgr.NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:               plugins,
		AwaitUserReplyRouting: builderDeps.AwaitHook != nil,
		BuilderDeps:           builderDeps,
		AgentFactoryKeys:      memberKeys,
	})
	if err != nil {
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
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	sendText := content
	intRes := intent.Run(ctx, intent.IntentPassFromAgent(firstAg), r.td.Catalog.LLM, r.td.LLMHTTP, prov0, mod0, content)
	if intRes.Artifact != nil {
		if strings.TrimSpace(intRes.RawJSON) != "" {
			merged, merr := intent.MergeIntoUserOptionsJSON(userOpts, intRes.RawJSON)
			if merr != nil {
				slog.Warn("intent merge into user options_json failed; continuing without intent_artifact", "error", merr)
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
	if r.td.Pipeline.Bus != nil {
		level, msg := intent.MonitorLogEntry(intRes, "team", meta)
		logEnv := event.NewEnvelope(event.EnvelopeTypeLog, "intent-pass", sess.ID)
		logEnv.Metadata = map[string]any{"level": level, "source": "intent-pass"}
		logEnv.Content = &event.EnvelopeContent{Text: msg}
		r.td.Pipeline.Bus.Publish(ctx, logEnv)
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
	publishTeamMonitor(ctx, r.td.Pipeline.Bus, "INFO", fmt.Sprintf(
		"team_turn phase=trpc_run_start session_id=%s run_id=%s",
		sess.ID, run.ID), sess.ID)

	events, err := agent.RunTRPCUserTurn(runCtx, runner, uid, sess.ID, sendText)
	if err != nil {
		publishTeamMonitor(ctx, r.td.Pipeline.Bus, "WARN", fmt.Sprintf(
			"team_turn phase=run_error session_id=%s run_id=%s err=%v",
			sess.ID, run.ID, err), sess.ID)
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	memberKeySet := make(map[string]struct{}, len(memberKeys))
	for _, k := range memberKeys {
		memberKeySet[k] = struct{}{}
	}
	projectMeta := agent.ProjectMeta{
		SessionID:       sess.ID,
		RequestID:       run.ID,
		TeamID:          teamRow.ID,
		MemberAgentKeys: memberKeySet,
	}
	result := agent.ConsumeEventStream(runCtx, events, r.td.Pipeline.Bus, projectMeta)
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
		publishTeamMonitor(ctx, r.td.Pipeline.Bus, "WARN", fmt.Sprintf(
			"team_turn phase=cancelled session_id=%s run_id=%s err=%v%s",
			sess.ID, run.ID, runCtx.Err(), hint), sess.ID)
		r.finishRunErr(ctx, &run, t0, runCtx.Err().Error())
		return userMsg, biz.ChatMessage{}, runCtx.Err()
	}

	publishTeamMonitor(ctx, r.td.Pipeline.Bus, "INFO", fmt.Sprintf(
		"team_turn phase=events_done session_id=%s run_id=%s",
		sess.ID, run.ID), sess.ID)

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
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	if reasoningText != "" {
		if assistantOptsStr, err = agent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, reasoningText); err != nil {
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
		err := kerrors.InternalServer("CHAT_TEAM_NATIVE", "team workflow produced no usable assistant reply")
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, assistantMsg, true); err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	for i, m := range members {
		ag, e := r.catalogAgent(ctx, m.AgentID)
		if e != nil {
			continue
		}
		stepMsg := assistantMsg
		if i == 0 {
			stepMsg.TokenIn = promptTok
			stepMsg.TokenOut = completionTok
		} else {
			stepMsg.TokenIn = 0
			stepMsg.TokenOut = 0
		}
		r.persistStep(ctx, run, teamRow.ID, i, m, ag, content, stepMsg)
	}

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

	if r.td.Pipeline.Bus != nil {
		cp := run
		env := event.NewEnvelope(event.EnvelopeTypeTeamRunFinished, "team-runner", sess.ID)
		env.TeamID = teamRow.ID
		env.Metadata = map[string]any{"run_id": run.ID, "run": cp}
		r.td.Pipeline.Bus.Publish(ctx, env)
	}
	return userMsg, assistantMsg, nil
}
