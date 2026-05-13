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
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/strutil"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

func (r *Runner) runTeamTRPC(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, teamRow biz.Team, def Definition, mode string, stream agent.StreamEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
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
	publishTeamMonitor(ctx, r.td.MonitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=start session_id=%s run_id=%s team_id=%s mode=%s members=%d streaming=%v",
		sess.ID, run.ID, teamRow.ID, mode, len(members), stream != nil))
	if r.td.TeamSSE != nil {
		cp := run
		r.td.TeamSSE.Publish(biz.TeamRunEvent{Type: "run_started", TeamID: teamRow.ID, RunID: run.ID, SessionID: sess.ID, Run: &cp})
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
		Catalog:    r.td.LLMCatalog,
		AgentUC:    r.td.AgentsUC,
		Agents:     r.td.Agents,
		RT:         r.td.RoundTrip(),
		SkillUC:    r.td.SkillUC,
		Sys:        r.td.Sys,
		Provider:   prov0,
		Model:      mod0,
		DialogMode: dialogMode,
	}
	teamDeps := TRPCTeamBuilderDeps{BuilderDeps: builderDeps}
	root, err := BuildTRPCTeam(ctx, def, teamDeps, r.catalogAgent)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	publishTeamMonitor(ctx, r.td.MonitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=team_built session_id=%s run_id=%s mode=%s",
		sess.ID, run.ID, mode))

	runnerDeps := agent.TRPCRunnerDeps{
		SessionService: agent.NewInMemoryTRPCSessionService(),
	}
	if r.td.RT != nil && r.td.RT.SessionMemory != nil {
		runnerDeps.MemoryService = memtrpc.NewSQLiteMemoryService(r.td.RT.SessionMemory)
	}
	runner, err := agent.NewTRPCRunner(root, runnerDeps)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	defer runner.Close()

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
	intRes := intent.Run(ctx, intent.IntentPassFromAgent(firstAg), r.td.LLMCatalog, r.td.LLMHTTP, prov0, mod0, content)
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
	meta := intent.SSERunMeta{
		AgentID:   firstAg.ID,
		SessionID: sess.ID,
		RunID:     run.ID,
		TeamID:    teamRow.ID,
	}
	if r.td.TeamSSE != nil {
		r.td.TeamSSE.Publish(biz.TeamRunEvent{
			Type:      "intent_pass",
			TeamID:    teamRow.ID,
			RunID:     run.ID,
			SessionID: sess.ID,
			Payload:   intent.BuildIntentPassPayload(intRes, meta),
		})
	}
	intent.PublishMonitorLog(ctx, r.td.MonitorLogs, intRes, "team", meta)

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

	streaming := stream != nil
	if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	if streaming {
		_ = stream.Emit("user_message", userMsg)
	}

	uid := agent.UserIDFromCtx(ctx)

	runCtx := ctx
	if dur := TurnDeadlineDuration(def); dur > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}
	publishTeamMonitor(ctx, r.td.MonitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=trpc_run_start session_id=%s run_id=%s streaming=%v",
		sess.ID, run.ID, streaming))

	events, err := agent.RunTRPCUserTurn(runCtx, runner, uid, sess.ID, sendText)
	if err != nil {
		publishTeamMonitor(ctx, r.td.MonitorLogs, "WARN", fmt.Sprintf(
			"team_turn phase=run_error session_id=%s run_id=%s err=%v",
			sess.ID, run.ID, err))
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	var (
		reply         strings.Builder
		reasoning     strings.Builder
		promptTok     int
		completionTok int
	)

	for ev := range events {
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
			publishTeamMonitor(ctx, r.td.MonitorLogs, "WARN", fmt.Sprintf(
				"team_turn phase=cancelled session_id=%s run_id=%s err=%v%s",
				sess.ID, run.ID, runCtx.Err(), hint))
			r.finishRunErr(ctx, &run, t0, runCtx.Err().Error())
			return userMsg, biz.ChatMessage{}, runCtx.Err()
		}
		if ev == nil || ev.Response == nil {
			continue
		}
		if ev.IsRunnerCompletion() {
			continue
		}
		if usage := ev.Response.Usage; usage != nil {
			promptTok = usage.PromptTokens
			completionTok = usage.CompletionTokens
		}
		for _, choice := range ev.Response.Choices {
			msg := choice.Message
			if text := strings.TrimSpace(msg.Content); text != "" {
				if streaming && ev.Response.IsPartial {
					if d := provider.VisibleStreamingDelta(&reply, text); d != "" {
						_ = stream.Emit("delta", map[string]string{"content": d})
					}
				} else {
					_ = provider.VisibleStreamingDelta(&reply, text)
				}
			}
			if rc := strings.TrimSpace(msg.ReasoningContent); rc != "" {
				if streaming && ev.Response.IsPartial {
					if d := provider.VisibleStreamingDelta(&reasoning, rc); d != "" {
						_ = stream.Emit("delta", map[string]string{"reasoning_content": d})
					}
				} else {
					_ = provider.VisibleStreamingDelta(&reasoning, rc)
				}
			}
			if streaming && len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					_ = stream.Emit("tool.call", map[string]any{
						"session_id":   sess.ID,
						"tool_name":    tc.Function.Name,
						"tool_call_id": tc.ID,
					})
				}
			}
		}
	}

	publishTeamMonitor(ctx, r.td.MonitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=events_done session_id=%s run_id=%s",
		sess.ID, run.ID))

	replyText := strings.TrimSpace(reply.String())
	reasoningText := strings.TrimSpace(reasoning.String())
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

	if streaming {
		if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, assistantMsg, true); err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		_ = stream.Emit("done", assistantMsg)
	} else {
		if err := r.td.Sessions.AppendChatMessage(ctx, sess.ID, assistantMsg, true); err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
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

	if r.td.TeamSSE != nil {
		cp := run
		r.td.TeamSSE.Publish(biz.TeamRunEvent{Type: "run_finished", TeamID: teamRow.ID, RunID: run.ID, SessionID: sess.ID, Run: &cp})
	}
	biz.HintTeamRunSSE(ctx, r.td.TeamSSE, r.teams, teamRow.ID)
	return userMsg, assistantMsg, nil
}
