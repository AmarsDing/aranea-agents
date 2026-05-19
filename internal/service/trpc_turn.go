package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	rt "aranea-agents/internal/runtime"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

const (
	defaultTurnTimeout = 5 * time.Minute
)

func (s *ChatService) runSingleAgentViaTRPC(
	ctx context.Context,
	sess biz.Session,
	req *chatv1.SendChatMessageRequest,
	ag biz.Agent,
	dialogMode, prov, mod string,
	attN int,
) (biz.ChatMessage, biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	content := strings.TrimSpace(req.GetContent())
	if ak := strings.TrimSpace(req.GetAgentKey()); ak != "" && !strings.EqualFold(ak, ag.AgentKey) {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_AGENT", "agent_key does not match this session")
	}

	runID := uuid.NewString()
	flow := event.NewFlowLogger(s.td.Pipeline.Bus, sessionID, ag.AgentKey)

	// Apply a turn-level timeout so a hanging LLM does not block the HTTP
	// caller indefinitely. If the parent context already has a shorter
	// deadline we keep it as-is.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTurnTimeout)
		defer cancel()
	}

	deps := chatagent.TRPCBuilderDeps{
		Catalog:     s.td.Catalog.LLM,
		AgentUC:     s.td.Catalog.AgentsUC,
		Agents:      s.td.Catalog.Agents,
		RT:          s.td.RoundTrip(),
		SkillUC:     s.td.Catalog.SkillUC,
		MCPTooling:  s.td.Persist.AgentMCP,
		ToolUC:      s.td.Catalog.ToolUC,
		Sessions:    s.td.Sessions,
		Sys:         s.td.Catalog.Settings,
		Provider:    prov,
		Model:       mod,
		DialogMode:  dialogMode,
		SkillDBRepo: s.skillDBRepo,
		// EP-RT-02: inject await-reply hook so the tool blocks mid-turn.
		AwaitHook: s.makeAwaitReplyFunc(ctx, sessionID, runID),
		// EP-RT-05: signal whether a MemoryService will back the runner.
		HasMemory:     s.td.Persist.Memory.Available(),
		PluginManager: s.pluginManager,
		MemoryAdmin:   s.td.Persist.Memory.Admin,
	}
	root, err := chatagent.BuildTRPCLLMAgentCached(ctx, ag, deps)
	if err != nil {
		flow.LogError("chat.agent_build", "构建Agent实例失败", event.P("agent_id", ag.ID), event.P("error", err.Error()))
		s.runs.Finish(sessionID)
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.agent_build", "Agent实例已构建", event.P("provider", prov), event.P("model", mod))

	var plugins []trpcplugin.Plugin
	if s.pluginManager != nil {
		plugins = s.pluginManager.RunnerPluginsForAgent(ag.ID)
	} else if s.pluginRT != nil {
		plugins = s.pluginRT.PluginsForAgent(ag.ID)
	}
	flow.LogDone("chat.plugins_load", "插件已加载", event.P("plugin_count", len(plugins)))
	deps.Plugins = plugins
	if s.td.RunnerMgr == nil {
		s.td.RunnerMgr = rt.NewRunnerManagerFromPersist(s.td.Persist)
	}
	runner, err := s.td.RunnerMgr.NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:               plugins,
		AwaitUserReplyRouting: deps.AwaitHook != nil,
		BuilderDeps:           deps,
		AgentFactoryKeys:      []string{ag.AgentKey},
	})
	if err != nil {
		s.runs.Finish(sessionID)
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	s.runs.StoreRunner(sessionID, runID, runner)
	s.setRunStatus(sessionID, runID, "running", "")
	flow.LogStart("chat.turn_execute", "开始执行Turn", event.P("run_id", runID))
	turnStart := time.Now()
	defer func() {
		s.runs.Finish(sessionID)
		runner.Close()
		s.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod)
	}()

	userOpts, err := chatagent.UserOptionsJSON(ag, dialogMode, prov, mod, sess.ContextUsedRatio, nil)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	sendText := content
	intRes := intent.Run(ctx, intent.IntentPassFromAgent(ag), s.td.Catalog.LLM, s.td.LLMHTTP, prov, mod, content)
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
	meta := intent.RunMeta{AgentID: ag.ID, SessionID: sessionID}
	intentPayload := intent.BuildIntentPassPayload(intRes, meta)
	if s.td.Pipeline.Bus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeIntentPass, ag.ID, sessionID)
		env.Metadata = intentPayload
		s.td.Pipeline.Bus.Publish(ctx, env)
	}
	if s.td.Pipeline.Bus != nil {
		level, msg := intent.MonitorLogEntry(intRes, "chat", meta)
		env := chatagent.NewEventProjector(s.td.Pipeline.Bus).BuildLogEnvelope(level, msg, "intent-pass", sessionID)
		s.td.Pipeline.Bus.Publish(ctx, env)
	}

	now := chatagent.RFC3339Now()
	userMsg := biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		Role:             "user",
		ContentMarkdown:  content,
		Status:           "ok",
		OptionsJSON:      userOpts,
		CreatedAt:        now,
		AttachmentsCount: attN,
	}
	if err := s.td.Sessions.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.user_msg_persist", "用户消息已持久化")

	uid := chatagent.UserIDFromCtx(ctx)
	runOpts := []trpcagent.RunOption{trpcagent.WithRequestID(sessionID)}
	if ag.Settings != nil {
		if vars := chatagent.ParseVariablesJSON(ag.Settings.VariablesJSON); vars != nil {
			runOpts = append(runOpts, trpcagent.MergeRuntimeState(vars))
		}
	}
	// Apply a turn-level timeout so a hanging LLM call does not block the
	// HTTP caller indefinitely. If the parent context already has a shorter
	// deadline we keep it as-is.
	const defaultTurnTimeout = 5 * time.Minute
	if deadline, hasDeadline := ctx.Deadline(); !hasDeadline || time.Until(deadline) > defaultTurnTimeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTurnTimeout)
		defer cancel()
	}
	// EP-RT-02: inject the await-reply hook into the run context so the
	// ServiceTool can retrieve it at call time.
	runCtx := serviceawaitreply.WithReplyFunc(ctx, deps.AwaitHook)
	runCtx = s.injectA2AContext(runCtx, ag.ID)

	// Debug: fire a goroutine that logs if the LLM call is still running after 60s.
	go func() {
		select {
		case <-time.After(60 * time.Second):
			flow.Log("chat.llm_call", event.FlowPhaseStart, "LLM调用超过60秒仍在等待", event.P("run_id", runID))
		case <-runCtx.Done():
		}
	}()
	flow.LogStart("chat.llm_call", "调用LLM模型")
	events, err := chatagent.RunTRPCUserTurn(runCtx, runner, uid, sessionID, sendText, runOpts...)
	if err != nil {
		flow.LogError("chat.llm_call", "LLM调用失败", event.P("error", err.Error()))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "error").Observe(time.Since(turnStart).Seconds())
		s.setRunStatus(sessionID, runID, "failed", err.Error())
		return userMsg, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.llm_call", "LLM调用返回，开始消费事件流")

	projectMeta := chatagent.ProjectMeta{
		SessionID: sessionID,
		RequestID: sessionID,
	}
	result := chatagent.ConsumeEventStream(ctx, events, s.td.Pipeline.Bus, projectMeta)
	flow.LogDone("chat.stream_consume", "事件流消费完成",
		event.P("reply_len", result.Reply.Len()),
		event.P("has_error", result.HasError),
		event.P("has_content", result.HasContent),
		event.P("prompt_tok", result.PromptTok),
		event.P("completion_tok", result.CompletionTok),
	)
	if ctx.Err() != nil {
		flow.LogError("chat.turn_timeout", "请求超时", event.P("timeout", defaultTurnTimeout.String()))
		fallback := fmt.Sprintf("Request timed out after %v. The AI service may be unavailable. Please try again.", defaultTurnTimeout)
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "timeout").Observe(time.Since(turnStart).Seconds())
		s.setRunStatus(sessionID, runID, "failed", fallback)
		return userMsg, biz.ChatMessage{}, kerrors.InternalServer("CHAT_AGENT", fallback)
	}

	// Empty reply detection: when the agent produces no visible text output,
	// surface the underlying error or a user-friendly fallback instead of
	// returning a silent empty message.
	replyText := strings.TrimSpace(result.Reply.String())
	if replyText == "" {
		flow.LogError("chat.empty_reply", "Agent未产生任何响应", event.P("has_error", result.HasError), event.P("last_error", result.LastError), event.P("has_content", result.HasContent))
		fallback := "I received your message but was unable to generate a response."
		if result.HasError {
			fallback = fmt.Sprintf("AI service error: %s. Please check your configuration or try again later.", result.LastError)
		} else if !result.HasContent {
			fallback = "The AI model did not produce any output. This may indicate a configuration issue with the model or provider."
		}
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "empty_reply").Observe(time.Since(turnStart).Seconds())
		s.setRunStatus(sessionID, runID, "failed", fallback)
		return userMsg, biz.ChatMessage{}, kerrors.InternalServer("CHAT_AGENT", fallback)
	}

	promptTok, completionTok := result.PromptTok, result.CompletionTok
	if promptTok <= 0 && completionTok <= 0 && replyText != "" {
		promptTok = roughTokenEstimateFromText(content + replyText)
		completionTok = roughTokenEstimateFromText(replyText)
	}

	assistantOptsStr, err := chatagent.AssistantOptionsJSON(ag, nil)
	if err != nil {
		return userMsg, biz.ChatMessage{}, err
	}
	if s := result.Reasoning.String(); s != "" {
		if assistantOptsStr, err = chatagent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, s); err != nil {
			return userMsg, biz.ChatMessage{}, err
		}
	}

	assistantMsg := biz.ChatMessage{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		Role:            "assistant",
		ContentMarkdown: replyText,
		ModelName:       mod,
		Status:          "ok",
		OptionsJSON:     assistantOptsStr,
		CreatedAt:       chatagent.RFC3339Now(),
		TokenIn:         promptTok,
		TokenOut:        completionTok,
	}
	if err := s.td.Sessions.AppendChatMessage(ctx, sessionID, assistantMsg, true); err != nil {
		return userMsg, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.assistant_msg_persist", "助手消息已持久化", event.P("reply_len", len(replyText)))
	patchSessionContextUsage(ctx, s, sessionID, ag, promptTok, completionTok)

	arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "ok").Observe(time.Since(turnStart).Seconds())
	s.recordSessionTurn(ctx, sessionID, ag, userMsg.ID, assistantMsg.ID, prov, mod, promptTok, completionTok, assistantMsg.ContentMarkdown)
	s.setRunStatus(sessionID, runID, "completed", "")
	flow.LogDone("chat.turn_execute", "Turn执行完成",
		event.P("run_id", runID),
		event.P("reply_len", len(replyText)),
		event.P("prompt_tok", promptTok),
		event.P("completion_tok", completionTok),
	)

	return userMsg, assistantMsg, nil
}

func (s *ChatService) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string) {
	entry, ok := s.pending.Dequeue(sessionID)
	if !ok {
		return
	}
	safego.Go(context.Background(), "pending-queue", func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
		s.runs.SetPendingCancel(sessionID, cancel)
		defer func() {
			cancel()
			s.runs.ClearPendingCancel(sessionID)
		}()
		req := &chatv1.SendChatMessageRequest{
			SessionId: sessionID,
			Content:   entry.Content,
		}
		var err error
		if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
			_, _, err = s.teamsNative.RunTurn(bgCtx, sess, req)
		} else {
			_, _, err = s.runSingleAgentViaTRPC(bgCtx, sess, req, ag, dialogMode, prov, mod, 0)
		}
		if err != nil && s.td.Pipeline.Bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeError, "pending-queue", sessionID)
			env.Error = &event.EnvelopeError{
				Type:      "pending_failed",
				Message:   fmt.Sprintf("pending message failed: %s", err.Error()),
				PendingID: entry.ID,
			}
			s.td.Pipeline.Bus.Publish(bgCtx, env)
		}
	})
}

func (s *ChatService) recordSessionTurn(ctx context.Context, sessionID string, ag biz.Agent, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	if s == nil || s.td.Sessions == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	preview := contentPreview
	if len(preview) > 200 {
		preview = preview[:200]
	}
	turn := biz.SessionTurn{
		SessionID:           sessionID,
		UserMessageID:       userMsgID,
		AssistantMessageID:  assistantMsgID,
		OwnerType:           "agent",
		AgentID:             ag.ID,
		Status:              "completed",
		StartedAt:           now,
		EndedAt:             now,
		InputTokens:         promptTok,
		OutputTokens:        completionTok,
		TotalTokens:         promptTok + completionTok,
		ModelCallCount:      1,
		FinalProvider:       prov,
		FinalModel:          mod,
		FinalContentPreview: preview,
	}
	if _, err := s.td.Sessions.CreateTurn(ctx, turn); err != nil {
		slog.Warn("recordSessionTurn failed", "session_id", sessionID, "error", err)
	}
}

func (s *ChatService) recordTeamSessionTurn(ctx context.Context, sessionID, teamID, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	if s == nil || s.td.Sessions == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	preview := contentPreview
	if len(preview) > 200 {
		preview = preview[:200]
	}
	turn := biz.SessionTurn{
		SessionID:           sessionID,
		UserMessageID:       userMsgID,
		AssistantMessageID:  assistantMsgID,
		OwnerType:           "team",
		TeamID:              teamID,
		Status:              "completed",
		StartedAt:           now,
		EndedAt:             now,
		InputTokens:         promptTok,
		OutputTokens:        completionTok,
		TotalTokens:         promptTok + completionTok,
		ModelCallCount:      1,
		FinalProvider:       prov,
		FinalModel:          mod,
		FinalContentPreview: preview,
	}
	if _, err := s.td.Sessions.CreateTurn(ctx, turn); err != nil {
		slog.Warn("recordTeamSessionTurn failed", "session_id", sessionID, "team_id", teamID, "error", err)
	}
}
