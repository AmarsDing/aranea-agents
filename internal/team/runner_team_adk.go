package team

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/agent/adksvc"
	"aranea-agents/internal/agent/intent"
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
	publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=start session_id=%s run_id=%s team_id=%s mode=%s members=%d streaming=%v",
		sess.ID, run.ID, teamRow.ID, mode, len(members), stream != nil))
	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_started", TeamID: teamRow.ID, RunID: run.ID, SessionID: sess.ID, Run: &cp})
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
	publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=workflow_built session_id=%s run_id=%s persist_members=%d stream_author=%q adk_streaming=%s",
		sess.ID, run.ID, len(plan.persistMembers), plan.streamAuthor,
		map[bool]string{true: "sse", false: "none"}[stream != nil]))
	switch mode {
	case "coordinator", "critic_loop", "adaptive":
		li := loopMaxIterations(mode, def)
		nMem := len(plan.persistMembers)
		if nMem > 0 {
			publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
				"team_turn phase=loop_plan session_id=%s run_id=%s mode=%s outer_loop_iterations=%d members_each_pass=%d (~%d_llm_chain_steps_before_tools_tokens)",
				sess.ID, run.ID, mode, li, nMem, int(li)*nMem))
		}
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
		Role:    anchorMem.Role,
	}
	userOpts, err := agent.UserOptionsJSON(firstAg, dialogMode, prov0, mod0, sess.ContextUsedRatio, anchor)
	if err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	sendText := content
	intRes := intent.Run(ctx, intent.IntentPassFromAgent(firstAg), r.catalog, r.llmHTTP, prov0, mod0, content)
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
	if merged, merr := mergeTeamUserADKMetaJSON(userOpts, content, sendText); merr != nil {
		slog.Warn("team run: merge team ADK user meta into options_json failed", "error", merr)
	} else {
		userOpts = merged
	}
	meta := intent.SSERunMeta{
		AgentID:   firstAg.ID,
		SessionID: sess.ID,
		RunID:     run.ID,
		TeamID:    teamRow.ID,
	}
	if r.broker != nil {
		r.broker.Publish(biz.TeamRunEvent{
			Type:      "intent_pass",
			TeamID:    teamRow.ID,
			RunID:     run.ID,
			SessionID: sess.ID,
			Payload:   intent.BuildIntentPassPayload(intRes, meta),
		})
	}
	intent.PublishMonitorLog(ctx, r.monitorLogs, intRes, "team", meta)
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
	if err := r.sessions.AppendChatMessage(ctx, sess.ID, userMsg, false); err != nil {
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	if streaming {
		_ = stream.Emit("user_message", userMsg)
	}

	msg := genai.NewContentFromText(sendText, genai.RoleUser)
	uid := agent.UserIDFromCtx(ctx)
	cfg := adkagent.RunConfig{StreamingMode: adkagent.StreamingModeSSE}
	if !streaming {
		cfg = adkagent.RunConfig{StreamingMode: adkagent.StreamingModeNone}
	}

	keyMeta := map[string]persistMetaEntry{}
	for _, m := range plan.persistMembers {
		ag, e := r.catalogAgent(ctx, m.AgentID)
		if e != nil {
			slog.Warn("team run: catalog agent for persist member failed; member excluded from author map",
				"agent_id", strings.TrimSpace(m.AgentID), "error", e)
			continue
		}
		registerPersistMetaKeys(keyMeta, m, ag)
	}
	for i, runName := range plan.persistRuntimeNames {
		if i >= len(plan.persistMembers) {
			break
		}
		m := plan.persistMembers[i]
		ag, e := r.catalogAgent(ctx, m.AgentID)
		if e != nil {
			continue
		}
		registerAuthorKeyIfAbsent(keyMeta, persistMetaEntry{Member: m, Agent: ag}, runName)
	}
	if sk := strings.ToLower(strings.TrimSpace(plan.streamAuthor)); sk != "" {
		if ent, ok := keyMeta[sk]; ok {
			registerPersistMetaKeys(keyMeta, ent.Member, ent.Agent)
		} else {
			publishTeamMonitor(ctx, r.monitorLogs, "WARN", fmt.Sprintf(
				"team_turn phase=keymap_missing_stream_author session_id=%s run_id=%s stream_author=%q persist_members=%d (ADK events for this author will not map to team members for persist/SSE leaf)",
				sess.ID, run.ID, plan.streamAuthor, len(plan.persistMembers)))
		}
	}
	registerWorkflowAuthorAliases(keyMeta, plan.workflowAuthorAliases, plan.streamAuthor)
	sampleKeys := teamMonitorAliasSample(keyMeta)
	publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=keymap_ready session_id=%s run_id=%s alias_count=%d sample_aliases=%s",
		sess.ID, run.ID, len(keyMeta), sampleKeys))

	synthKey := strings.ToLower(strings.TrimSpace(plan.streamAuthor))
	var (
		nEvents              int
		nPartial             int
		nSkipNonFinal        int
		nSkipUnknownAuthor   int
		nSkipToolOnlyFinal   int
		nAssistantOK         int
		lastStreamingAssistant biz.ChatMessage
	)
	var totalIn, totalOut int
	var lastUsage *genai.GenerateContentResponseUsageMetadata
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

	// Apply team definition timeout only to ADK execution. Intent pass, workflow build, and DB
	// prep run on the parent ctx so a long intent (e.g. 20–45s) does not consume the whole budget
	// and starve the first LLM call (seen as silence after keymap_ready in monitor).
	runCtx := ctx
	if dur := TurnDeadlineDuration(def); dur > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}
	publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=adk_run_start session_id=%s run_id=%s session_streaming=%v",
		sess.ID, run.ID, streaming))

	var lastCompressAg biz.Agent
	for ev, err := range rn.Run(runCtx, uid, sess.ID, msg, cfg) {
		if runCtx.Err() != nil {
			hint := ""
			switch {
			case errors.Is(runCtx.Err(), context.DeadlineExceeded):
				if dur := TurnDeadlineDuration(def); dur > 0 {
					hint = fmt.Sprintf(" hint=definition_timeout_hit effective=%s (ADK phase only; intent not counted)", dur)
				} else {
					hint = " hint=upstream_or_http_deadline check_server.http.timeout_and_reverse_proxy"
				}
			case errors.Is(runCtx.Err(), context.Canceled):
				hint = " hint=client_disconnect_or_abort"
			}
			publishTeamMonitor(ctx, r.monitorLogs, "WARN", fmt.Sprintf(
				"team_turn phase=cancelled session_id=%s run_id=%s err=%v events=%d partial=%d skip_non_final=%d skip_unknown_author=%d assistant_ok=%d%s",
				sess.ID, run.ID, runCtx.Err(), nEvents, nPartial, nSkipNonFinal, nSkipUnknownAuthor, nAssistantOK, hint))
			r.finishRunErr(ctx, &run, t0, runCtx.Err().Error())
			return userMsg, biz.ChatMessage{}, runCtx.Err()
		}
		if err != nil {
			publishTeamMonitor(ctx, r.monitorLogs, "WARN", fmt.Sprintf(
				"team_turn phase=adk_run_error session_id=%s run_id=%s err=%v events=%d partial=%d assistant_ok=%d",
				sess.ID, run.ID, err, nEvents, nPartial, nAssistantOK))
			r.finishRunErr(ctx, &run, t0, err.Error())
			return userMsg, biz.ChatMessage{}, err
		}
		if ev == nil {
			continue
		}
		nEvents++
		if ev.LLMResponse.UsageMetadata != nil {
			lastUsage = ev.LLMResponse.UsageMetadata
		}
		if toolRelay != nil {
			agEv := teamAgentForToolSSE(ev.Author, synthKey, keyMeta, len(plan.persistMembers), firstAg)
			if agent.SessionHasFunctionResponses(ev) {
				toolRelay.EmitToolResponses(agEv, ev)
			}
		}
		if streaming && ev.LLMResponse.Partial {
			nPartial++
			if toolRelay != nil {
				agEv := teamAgentForToolSSE(ev.Author, synthKey, keyMeta, len(plan.persistMembers), firstAg)
				if agent.SessionHasFunctionCalls(ev) {
					toolRelay.EmitToolCalls(agEv, ev)
				}
			}
			if metaIsStreamLeaf(ev.Author, synthKey, keyMeta, len(plan.persistMembers)) {
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
			nSkipNonFinal++
			continue
		}
		if toolRelay != nil && agent.SessionHasFunctionCalls(ev) {
			agEv := teamAgentForToolSSE(ev.Author, synthKey, keyMeta, len(plan.persistMembers), firstAg)
			toolRelay.EmitToolCalls(agEv, ev)
		}
		meta, ok := lookupPersistMeta(ev.Author, synthKey, keyMeta, len(plan.persistMembers))
		if !ok {
			mainPeek, _ := provider.TextsFromLLMResponse(&ev.LLMResponse)
			mainPeek = strings.TrimSpace(mainPeek)
			if streaming && mainPeek == "" {
				mainPeek = strings.TrimSpace(synthStreamReply.String())
			}
			if streaming && mainPeek == "" {
				mainPeek = strings.TrimSpace(synthStreamReason.String())
			}
			if synthKey != "" && mainPeek != "" {
				if entLeaf, ok2 := keyMeta[synthKey]; ok2 {
					meta = entLeaf
					ok = true
					publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
						"team_turn phase=author_fallback_stream_leaf session_id=%s run_id=%s author=%q",
						sess.ID, run.ID, strings.TrimSpace(ev.Author)))
				}
			}
		}
		if !ok {
			nSkipUnknownAuthor++
			mainPeek, _ := provider.TextsFromLLMResponse(&ev.LLMResponse)
			pvw := preview(strings.TrimSpace(mainPeek), 120)
			slog.Warn("team run: unknown LLM event author; skipping persist",
				"author", strings.TrimSpace(ev.Author), "stream_author", strings.TrimSpace(plan.streamAuthor),
				"final", ev.IsFinalResponse(), "partial", ev.LLMResponse.Partial)
			publishTeamMonitor(ctx, r.monitorLogs, "WARN", fmt.Sprintf(
				"team_turn phase=skip_unknown_author session_id=%s run_id=%s author=%q stream_author=%q is_final=%v partial=%v text_preview=%q keymap_sample=%q",
				sess.ID, run.ID, strings.TrimSpace(ev.Author), strings.TrimSpace(plan.streamAuthor),
				ev.IsFinalResponse(), ev.LLMResponse.Partial, pvw, teamMonitorAliasSample(keyMeta)))
			continue
		}
		main, rsnForMerge := provider.TextsFromLLMResponse(&ev.LLMResponse)
		main = strings.TrimSpace(main)
		rsnTrim := strings.TrimSpace(rsnForMerge)
		// ADK may end the turn with a final chunk that has function_call/response parts but no Text
		// parts, while the user-visible answer was already streamed via partials (synthStreamReply).
		onlyToolsNoText := main == "" && rsnTrim == "" && (agent.SessionHasFunctionCalls(ev) || agent.SessionHasFunctionResponses(ev))
		if onlyToolsNoText && streaming {
			if s := strings.TrimSpace(synthStreamReply.String()); s != "" {
				main = s
				onlyToolsNoText = false
			} else if s := strings.TrimSpace(synthStreamReason.String()); s != "" {
				rsnTrim = s
				rsnForMerge = s
				onlyToolsNoText = false
			}
		}
		if onlyToolsNoText {
			nSkipToolOnlyFinal++
			continue
		}
		displayMarkdown := main
		if displayMarkdown == "" && rsnTrim != "" {
			displayMarkdown = rsnTrim
		}
		opts, e := agent.AssistantOptionsJSON(meta.Agent, &agent.TeamMemberAnchor{
			AgentID: meta.Agent.ID,
			Name:    firstNonEmptyStr(meta.Agent.DisplayName, meta.Agent.AgentKey),
			Role:    meta.Member.Role,
		})
		if e != nil {
			r.finishRunErr(ctx, &run, t0, e.Error())
			return userMsg, biz.ChatMessage{}, e
		}
		if strings.TrimSpace(rsnForMerge) != "" {
			if opts, err = agent.MergeReasoningIntoAssistantOptionsJSON(opts, rsnForMerge); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		}
		ptok, ctok := 0, 0
		if um := ev.LLMResponse.UsageMetadata; um != nil {
			ptok = int(um.PromptTokenCount)
			ctok = int(um.CandidatesTokenCount)
		}
		am := biz.ChatMessage{
			ID:              uuid.NewString(),
			SessionID:       sess.ID,
			Role:            "assistant",
			ContentMarkdown: displayMarkdown,
			ModelName:       firstNonEmptyStr(modOpt, sess.Model, meta.Agent.Model),
			Status:          "ok",
			OptionsJSON:     opts,
			CreatedAt:       agent.RFC3339Now(),
			TokenIn:         ptok,
			TokenOut:        ctok,
		}
		// Sum per-final UsageMetadata; assumes each value is for that model invocation, not session-cumulative.
		totalIn += ptok
		totalOut += ctok
		if streaming {
			if err := r.sessions.AppendChatMessage(ctx, sess.ID, am, true); err != nil {
				publishTeamMonitor(ctx, r.monitorLogs, "WARN", fmt.Sprintf(
					"team_turn phase=append_assistant_failed session_id=%s run_id=%s err=%v", sess.ID, run.ID, err))
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		} else {
			unaryAssistants = append(unaryAssistants, am)
		}
		nAssistantOK++
		if streaming && strings.TrimSpace(am.ContentMarkdown) != "" {
			lastStreamingAssistant = am
		}
		if strings.TrimSpace(displayMarkdown) != "" {
			lastCompressAg = meta.Agent
		}
		r.persistStep(ctx, run, teamRow.ID, sortIndexForMember(plan.persistMembers, meta.Member), meta.Member, meta.Agent, content, am)
		leaf := metaIsStreamLeaf(ev.Author, synthKey, keyMeta, len(plan.persistMembers))
		if leaf && strings.TrimSpace(displayMarkdown) != "" {
			lastForClient = am
		}
	}
	_ = agent.SyncPersistedADKSessionToMemory(ctx, ss, agent.RunnerMemoryForRuntime(r.adk), adksvc.DefaultAppName, uid, sess.ID)

	publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
		"team_turn phase=adk_events_done session_id=%s run_id=%s events=%d partial_chunks=%d skip_non_final=%d skip_unknown_author=%d skip_tool_only_final=%d assistant_persisted=%d",
		sess.ID, run.ID, nEvents, nPartial, nSkipNonFinal, nSkipUnknownAuthor, nSkipToolOnlyFinal, nAssistantOK))

	assistantMsg = lastForClient
	if streaming && strings.TrimSpace(assistantMsg.ContentMarkdown) == "" && strings.TrimSpace(lastStreamingAssistant.ContentMarkdown) != "" {
		publishTeamMonitor(ctx, r.monitorLogs, "WARN", fmt.Sprintf(
			"team_turn phase=done_fallback session_id=%s run_id=%s reason=stream_leaf_empty using_last_persisted_assistant id=%s model=%q preview=%q",
			sess.ID, run.ID, lastStreamingAssistant.ID,
			strings.TrimSpace(lastStreamingAssistant.ModelName),
			preview(strings.TrimSpace(lastStreamingAssistant.ContentMarkdown), 120)))
		assistantMsg = lastStreamingAssistant
	}
	if !streaming {
		for i := 0; i < len(unaryAssistants); i++ {
			if err := r.sessions.AppendChatMessage(ctx, sess.ID, unaryAssistants[i], true); err != nil {
				r.finishRunErr(ctx, &run, t0, err.Error())
				return userMsg, biz.ChatMessage{}, err
			}
		}
		if len(unaryAssistants) > 0 {
			assistantMsg = unaryAssistants[len(unaryAssistants)-1]
		}
	}

	replyEmpty := strings.TrimSpace(assistantMsg.ContentMarkdown) == ""
	if nAssistantOK == 0 || replyEmpty {
		msg := "team workflow produced no usable assistant reply"
		if nAssistantOK == 0 {
			msg = "team workflow produced no assistant messages"
		}
		err := kerrors.InternalServer("CHAT_TEAM_NATIVE", msg)
		publishTeamMonitor(ctx, r.monitorLogs, "WARN", fmt.Sprintf(
			"team_turn phase=no_usable_reply session_id=%s run_id=%s nAssistantOK=%d reply_empty=%v skip_unknown_author=%d stream_author=%q",
			sess.ID, run.ID, nAssistantOK, replyEmpty, nSkipUnknownAuthor, plan.streamAuthor))
		r.finishRunErr(ctx, &run, t0, err.Error())
		return userMsg, biz.ChatMessage{}, err
	}

	if streaming && stream != nil {
		_ = stream.Emit("done", assistantMsg)
		publishTeamMonitor(ctx, r.monitorLogs, "INFO", fmt.Sprintf(
			"team_turn phase=sse_doneEmitted session_id=%s run_id=%s assistant_msg_id=%s content_len=%d token_out=%d",
			sess.ID, run.ID, assistantMsg.ID,
			len(strings.TrimSpace(assistantMsg.ContentMarkdown)), assistantMsg.TokenOut))
	}

	run.Status = "success"
	run.TokenIn = totalIn
	run.TokenOut = totalOut
	run.DurationMS = int(time.Since(t0).Milliseconds())
	run.OutputPreview = preview(assistantMsg.ContentMarkdown, 512)
	run.FinishedAt = agent.RFC3339Now()
	_ = r.teams.UpdateTeamRun(ctx, run)

	compressAg := firstAg
	if strings.TrimSpace(lastCompressAg.ID) != "" {
		compressAg = lastCompressAg
	}
	win := compressAg.ContextWindow
	if win <= 0 {
		win = 128000
	}
	// Prefer the last event's usage for session context ratio (typically the final model call in the turn).
	promptTok, completionTok := 0, 0
	if lastUsage != nil {
		promptTok = int(lastUsage.PromptTokenCount)
		completionTok = int(lastUsage.CandidatesTokenCount)
	}
	if promptTok <= 0 {
		promptTok = totalIn
	}
	if completionTok <= 0 {
		completionTok = totalOut
	}
	if promptTok <= 0 && strings.TrimSpace(assistantMsg.ContentMarkdown) != "" {
		promptTok = agent.RoughTokenEstimate(content + assistantMsg.ContentMarkdown)
	}
	if completionTok <= 0 && strings.TrimSpace(assistantMsg.ContentMarkdown) != "" {
		completionTok = agent.RoughTokenEstimate(assistantMsg.ContentMarkdown)
	}
	_ = r.sessions.UpdateSessionContextFromLLMUsage(ctx, sess.ID, promptTok, completionTok, win)
	if r.compress != nil {
		r.compress.AfterNativeTurn(ctx, sess.ID, compressAg)
	}

	if r.broker != nil {
		cp := run
		r.broker.Publish(biz.TeamRunEvent{Type: "run_finished", TeamID: teamRow.ID, RunID: run.ID, SessionID: sess.ID, Run: &cp})
	}
	biz.HintTeamRunSSE(ctx, r.broker, r.teams, teamRow.ID)
	return userMsg, assistantMsg, nil
}

func publishTeamMonitor(ctx context.Context, b *biz.MonitorLogBroker, level, msg string) {
	if b == nil || strings.TrimSpace(msg) == "" {
		return
	}
	b.Publish(ctx, level, msg, "team-runner")
}

func teamMonitorAliasSample(m map[string]persistMetaEntry) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	const max = 15
	if len(keys) > max {
		return strings.Join(keys[:max], ",") + fmt.Sprintf(",…(+%d)", len(keys)-max)
	}
	return strings.Join(keys, ",")
}

func sortIndexForMember(order []MemberDef, m MemberDef) int {
	for i, x := range order {
		if strings.TrimSpace(x.AgentID) == strings.TrimSpace(m.AgentID) && strings.TrimSpace(x.Role) == strings.TrimSpace(m.Role) {
			return i
		}
	}
	return 0
}

// persistMetaEntry ties a catalog agent row to a team member for ADK event authors.
type persistMetaEntry struct {
	Member MemberDef
	Agent  biz.Agent
}

func registerAuthorKeyIfAbsent(m map[string]persistMetaEntry, ent persistMetaEntry, name string) {
	k := strings.ToLower(strings.TrimSpace(name))
	if k == "" {
		return
	}
	if _, exists := m[k]; !exists {
		m[k] = ent
	}
}

func registerPersistMetaKeys(m map[string]persistMetaEntry, mem MemberDef, ag biz.Agent) {
	ent := persistMetaEntry{Member: mem, Agent: ag}
	add := func(s string) {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" {
			return
		}
		if _, exists := m[k]; !exists {
			m[k] = ent
		}
	}
	add(ag.AgentKey)
	add(ag.DisplayName)
	add(mem.Name)
	add(ag.ID)
}

// registerWorkflowAuthorAliases maps ADK parent workflow agent names to the user-visible stream leaf,
// so final responses credited to e.g. team_loop_coordinator still persist under the last chain member.
func registerWorkflowAuthorAliases(m map[string]persistMetaEntry, aliases []string, streamAuthor string) {
	sk := strings.ToLower(strings.TrimSpace(streamAuthor))
	if sk == "" {
		return
	}
	ent, ok := m[sk]
	if !ok {
		return
	}
	for _, name := range aliases {
		k := strings.ToLower(strings.TrimSpace(name))
		if k == "" {
			continue
		}
		if _, exists := m[k]; !exists {
			m[k] = ent
		}
	}
}

func lookupPersistMeta(author, synthKey string, keyMeta map[string]persistMetaEntry, persistN int) (persistMetaEntry, bool) {
	auth := strings.ToLower(strings.TrimSpace(author))
	sk := strings.ToLower(strings.TrimSpace(synthKey))
	if ent, ok := keyMeta[auth]; ok {
		return ent, true
	}
	if auth == "" && sk != "" {
		if ent, ok := keyMeta[sk]; ok {
			return ent, true
		}
	}
	if persistN == 1 {
		for _, ent := range keyMeta {
			return ent, true
		}
	}
	return persistMetaEntry{}, false
}

// metaIsStreamLeaf returns true when ev.Author refers to the same team member as plan.streamAuthor
// (the leaf whose SSE stream is user-visible), allowing aliases (display name, member.Name, etc.).
func metaIsStreamLeaf(evAuthor, synthKey string, keyMeta map[string]persistMetaEntry, persistN int) bool {
	sk := strings.ToLower(strings.TrimSpace(synthKey))
	if sk == "" {
		return false
	}
	streamEnt, ok := keyMeta[sk]
	if !ok {
		return strings.ToLower(strings.TrimSpace(evAuthor)) == sk
	}
	evEnt, ok := lookupPersistMeta(evAuthor, synthKey, keyMeta, persistN)
	if !ok {
		return false
	}
	return strings.TrimSpace(evEnt.Agent.ID) == strings.TrimSpace(streamEnt.Agent.ID)
}

func teamAgentForToolSSE(author, synthKey string, keyMeta map[string]persistMetaEntry, persistN int, fallback biz.Agent) biz.Agent {
	auth := strings.ToLower(strings.TrimSpace(author))
	if meta, ok := keyMeta[auth]; ok {
		return meta.Agent
	}
	if ent, ok := lookupPersistMeta(author, synthKey, keyMeta, persistN); ok {
		return ent.Agent
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
