package team

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/agent/adksvc"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/tools"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	adkagent "google.golang.org/adk/agent"

	"google.golang.org/genai"
)

func (r *Runner) runTeamADK(ctx context.Context, sess biz.Session, req *chatv1.SendChatMessageRequest, teamRow biz.Team, def Definition, mode string, stream agent.StreamEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "content is required")
	}
	dialogMode, provOpt, modOpt, attN := extractOpts(req)
	dialogMode = firstNonEmptyStr(dialogMode, sess.DialogMode, "default")

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
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_started", TeamID: teamRow.ID, RunID: run.ID, SessionID: sess.ID, Run: &cp})
	}

	t0 := time.Now()
	firstAg, err := r.catalogAgent(ctx, members[0].AgentID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = kerrors.NotFound("AGENT", "team member agent not found")
		}
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	deps := agent.BuilderDeps{
		Catalog:             r.catalog,
		AgentUC:             r.agentsUC,
		Agents:              r.agents,
		ToolsCatalog:        r.toolsCatalog,
		RT:                  &provider.RoundTrip{HTTP: r.llmHTTP},
		SQLiteSessionMemory: r.adk != nil && r.adk.SessionMemory != nil,
	}
	mount := tools.TurnMount{
		AgentsUC:     r.agentsUC,
		Agents:       r.agents,
		ToolsCatalog: r.toolsCatalog,
		SkillUC:      r.skillUC,
		Sys:          r.sys,
	}
	if r.adk != nil {
		mount.MCP = r.adk.AgentMCP
	}
	root, plan, err := BuildWorkflowRoot(ctx, mode, def, deps, mount, content, sess, provOpt, modOpt, r.catalogAgent)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	ss := adksvc.NewBizSessionForUsecase(r.sessions, func(c context.Context, agentID string) (string, error) {
		a, e := r.catalogAgent(c, agentID)
		if e != nil {
			return "", e
		}
		return strings.TrimSpace(a.AgentKey), nil
	})

	rn, err := agent.NewADKRunnerForRuntime(root, ss, r.adk)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	prov0 := firstNonEmptyStr(provOpt, sess.Provider, firstAg.Provider)
	mod0 := firstNonEmptyStr(modOpt, sess.Model, firstAg.Model)
	anchor := &agent.TeamMemberAnchor{
		AgentID: firstAg.ID,
		Name:    firstNonEmptyStr(firstAg.DisplayName, firstAg.AgentKey),
		Role:    members[0].Role,
	}
	userOpts, err := agent.UserOptionsJSON(firstAg, dialogMode, prov0, mod0, sess.ContextUsedRatio, anchor)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
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

	streaming := stream != nil
	if streaming {
		if err := r.sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		_ = stream.Emit("user_message", userMsg)
	}

	msg := genai.NewContentFromText(content, genai.RoleUser)
	uid := agent.UserIDFromCtx(ctx)
	cfg := adkagent.RunConfig{StreamingMode: adkagent.StreamingModeSSE}
	if !streaming {
		cfg = adkagent.RunConfig{StreamingMode: adkagent.StreamingModeNone}
	}

	keyMeta := map[string]struct {
		Member MemberDef
		Agent  biz.Agent
	}{}
	for _, m := range plan.persistMembers {
		ag, e := r.catalogAgent(ctx, m.AgentID)
		if e != nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(ag.AgentKey))
		if key != "" {
			keyMeta[key] = struct {
				Member MemberDef
				Agent  biz.Agent
			}{Member: m, Agent: ag}
		}
	}

	synthKey := strings.ToLower(strings.TrimSpace(plan.streamAuthor))
	var totalIn, totalOut int
	var lastForClient biz.ChatMessage
	var unaryAssistants []biz.ChatMessage

	var toolRelay *agent.ChatToolSSERelay
	var synthStreamReply strings.Builder
	var synthStreamReason strings.Builder
	mono := func(p agent.ChatToolUseSSE) {
		if r.broker == nil {
			return
		}
		typ := "tool.result"
		if p.Phase == "before" {
			typ = "tool.call"
		}
		r.broker.Publish(biz.TeamRunEvent{
			Type:      typ,
			TeamID:    teamRow.ID,
			RunID:     run.ID,
			SessionID: sess.ID,
			Payload:   agent.ChatToolPayloadMap(p),
		})
	}
	if streaming || r.broker != nil {
		toolRelay = agent.NewChatToolSSERelay(stream, mono)
	}

	for ev, err := range rn.Run(ctx, uid, sess.ID, msg, cfg) {
		if ctx.Err() != nil {
			r.finishRunErr(ctx, &run, t0, ctx.Err().Error())
			return userMsg, biz.ChatMessage{}, ctx.Err()
		}
		if err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		if ev == nil {
			continue
		}
		if toolRelay != nil {
			agEv := teamAgentForToolSSE(ev.Author, keyMeta, firstAg)
			if agent.SessionHasFunctionResponses(ev) {
				toolRelay.EmitToolResponses(agEv, ev)
			}
		}
		if streaming && ev.LLMResponse.Partial {
			if toolRelay != nil {
				agEv := teamAgentForToolSSE(ev.Author, keyMeta, firstAg)
				if agent.SessionHasFunctionCalls(ev) {
					toolRelay.EmitToolCalls(agEv, ev)
				}
			}
			auth := strings.ToLower(strings.TrimSpace(ev.Author))
			if auth == synthKey {
				main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
				if d := provider.VisibleStreamingDelta(&synthStreamReply, main); d != "" {
					_ = stream.Emit("delta", map[string]string{"content": d})
				}
				if dr := provider.VisibleStreamingDelta(&synthStreamReason, rsn); dr != "" {
					_ = stream.Emit("delta", map[string]string{"reasoning_content": dr})
				}
			}
			continue
		}
		if !ev.IsFinalResponse() || ev.LLMResponse.Partial {
			continue
		}
		auth := strings.ToLower(strings.TrimSpace(ev.Author))
		if toolRelay != nil && agent.SessionHasFunctionCalls(ev) {
			agEv := teamAgentForToolSSE(ev.Author, keyMeta, firstAg)
			toolRelay.EmitToolCalls(agEv, ev)
		}
		meta, ok := keyMeta[auth]
		if !ok {
			continue
		}
		main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
		main = strings.TrimSpace(main)
		opts, e := agent.AssistantOptionsJSON(meta.Agent, &agent.TeamMemberAnchor{
			AgentID: meta.Agent.ID,
			Name:    firstNonEmptyStr(meta.Agent.DisplayName, meta.Agent.AgentKey),
			Role:    meta.Member.Role,
		})
		if e != nil {
			r.finishRunErr(ctx, &run, t0, e.Error())
			return userMsg, biz.ChatMessage{}, e
		}
		if rsn != "" {
			if opts, err = agent.MergeReasoningIntoAssistantOptionsJSON(opts, rsn); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		}
		am := biz.ChatMessage{
			ID:              uuid.NewString(),
			SessionID:       sess.ID,
			Role:            "assistant",
			ContentMarkdown: main,
			ModelName:       firstNonEmptyStr(modOpt, sess.Model, meta.Agent.Model),
			Status:          "ok",
			OptionsJSON:     opts,
			CreatedAt:       agent.RFC3339Now(),
		}
		totalIn += am.TokenIn
		totalOut += am.TokenOut
		if streaming {
			if err := r.sessions.AppendChatMessage(ctx, sess.ID, am, true); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		} else {
			unaryAssistants = append(unaryAssistants, am)
		}
		r.persistStep(ctx, run, teamRow.ID, sortIndexForMember(plan.persistMembers, meta.Member), meta.Member, meta.Agent, content, am)
		if auth == synthKey {
			lastForClient = am
		}
	}
	_ = agent.SyncPersistedADKSessionToMemory(ctx, ss, agent.RunnerMemoryForRuntime(r.adk), adksvc.DefaultAppName, uid, sess.ID)

	assistantMsg = lastForClient
	if !streaming {
		if len(unaryAssistants) == 0 {
			err := kerrors.InternalServer("CHAT_TEAM_NATIVE", "team workflow produced no assistant messages")
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		if err := r.sessions.AppendChatTurn(ctx, sess.ID, userMsg, unaryAssistants[0]); err != nil {
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		for i := 1; i < len(unaryAssistants); i++ {
			if err := r.sessions.AppendChatMessage(ctx, sess.ID, unaryAssistants[i], true); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		}
		assistantMsg = unaryAssistants[len(unaryAssistants)-1]
	} else if stream != nil {
		_ = stream.Emit("done", assistantMsg)
	}

	run.Status = "success"
	run.TokenIn = totalIn
	run.TokenOut = totalOut
	run.DurationMS = int(time.Since(t0).Milliseconds())
	run.OutputPreview = preview(assistantMsg.ContentMarkdown, 512)
	run.FinishedAt = agent.RFC3339Now()
	_ = r.teams.UpdateTeamRun(ctx, run)

	win := firstAg.ContextWindow
	if win <= 0 {
		win = 128000
	}
	promptTok := totalIn
	completionTok := totalOut
	if promptTok <= 0 && strings.TrimSpace(assistantMsg.ContentMarkdown) != "" {
		promptTok = agent.RoughTokenEstimate(content + assistantMsg.ContentMarkdown)
	}
	if completionTok <= 0 && strings.TrimSpace(assistantMsg.ContentMarkdown) != "" {
		completionTok = agent.RoughTokenEstimate(assistantMsg.ContentMarkdown)
	}
	_ = r.sessions.UpdateSessionContextFromLLMUsage(ctx, sess.ID, promptTok, completionTok, win)
	if r.compress != nil {
		r.compress.AfterNativeTurn(ctx, sess.ID, firstAg)
	}

	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_finished", TeamID: teamRow.ID, RunID: run.ID, SessionID: sess.ID, Run: &cp})
	}
	biz.HintTeamRunSSE(ctx, r.broker, r.teams, teamRow.ID)
	return userMsg, assistantMsg, nil
}

func sortIndexForMember(order []MemberDef, m MemberDef) int {
	for i, x := range order {
		if strings.TrimSpace(x.AgentID) == strings.TrimSpace(m.AgentID) && strings.TrimSpace(x.Role) == strings.TrimSpace(m.Role) {
			return i
		}
	}
	return 0
}

func teamAgentForToolSSE(author string, keyMeta map[string]struct {
	Member MemberDef
	Agent  biz.Agent
}, fallback biz.Agent) biz.Agent {
	auth := strings.ToLower(strings.TrimSpace(author))
	if meta, ok := keyMeta[auth]; ok {
		return meta.Agent
	}
	return agent.AgentForToolSSELabel(author, fallback)
}

func (r *Runner) persistStep(ctx context.Context, run biz.TeamRun, teamID string, sortIdx int, m MemberDef, ag biz.Agent, userContent string, asst biz.ChatMessage) {
	step := biz.TeamRunStep{
		ID:            uuid.NewString(),
		RunID:         run.ID,
		TeamID:        teamID,
		AgentID:       ag.ID,
		AgentKey:      ag.AgentKey,
		AgentName:     firstNonEmptyStr(ag.DisplayName, ag.AgentKey),
		Role:          m.Role,
		SortOrder:     sortIdx,
		Status:        asst.Status,
		InputPreview:  preview(userContent, 400),
		OutputPreview: preview(asst.ContentMarkdown, 400),
		TokenIn:       asst.TokenIn,
		TokenOut:      asst.TokenOut,
		CostMicroUSD:  0,
		DurationMS:    asst.LatencyMS,
		ErrorMessage:  asst.ErrorMessage,
		StartedAt:     asst.CreatedAt,
		FinishedAt:    asst.CreatedAt,
		CreatedAt:     agent.RFC3339Now(),
	}
	saved, err := r.teams.CreateTeamRunStep(ctx, step)
	if err != nil {
		return
	}
	if r.broker != nil {
		r.broker.Publish(biz.TeamRunEvent{Type: "step_finished", TeamID: teamID, RunID: run.ID, SessionID: run.SessionID, Step: &saved})
	}
}

func (r *Runner) finishRunErr(ctx context.Context, run *biz.TeamRun, t0 time.Time, msg string) {
	if run == nil {
		return
	}
	run.Status = "failed"
	run.ErrorMessage = msg
	run.FinishedAt = agent.RFC3339Now()
	run.DurationMS = int(time.Since(t0).Milliseconds())
	_ = r.teams.UpdateTeamRun(ctx, *run)
	if r.broker != nil {
		cp := *run
		r.broker.Publish(biz.TeamRunEvent{
			Type:      "run_finished",
			TeamID:    run.TeamID,
			RunID:     run.ID,
			SessionID: strings.TrimSpace(run.SessionID),
			Run:       &cp,
		})
		r.broker.Publish(biz.TeamRunEvent{
			Type:      "run.failed",
			TeamID:    run.TeamID,
			RunID:     run.ID,
			SessionID: strings.TrimSpace(run.SessionID),
			Run:       &cp,
			Payload: map[string]any{
				"error_message": msg,
			},
		})
	}
	if r.monitorLogs != nil {
		r.monitorLogs.Publish(ctx, "WARN", fmt.Sprintf("team_run failed team_id=%s run_id=%s session_id=%s: %s", run.TeamID, run.ID, strings.TrimSpace(run.SessionID), msg), "team-runner")
	}
}
