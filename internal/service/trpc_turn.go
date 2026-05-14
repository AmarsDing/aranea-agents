package service

import (
	"context"
	"log/slog"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/internal/provider"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

func (s *ChatService) runSingleAgentViaTRPC(
	ctx context.Context,
	sess biz.Session,
	req *chatv1.SendChatMessageRequest,
	ag biz.Agent,
	dialogMode, prov, mod string,
	attN int,
	stream *streamWriter,
) (biz.ChatMessage, biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	content := strings.TrimSpace(req.GetContent())
	if ak := strings.TrimSpace(req.GetAgentKey()); ak != "" && !strings.EqualFold(ak, ag.AgentKey) {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_AGENT", "agent_key does not match this session")
	}

	deps := chatagent.TRPCBuilderDeps{
		Catalog:    s.td.LLMCatalog,
		AgentUC:    s.td.AgentsUC,
		Agents:     s.td.Agents,
		RT:         s.td.RoundTrip(),
		SkillUC:    s.td.SkillUC,
		Sys:        s.td.Sys,
		Provider:   prov,
		Model:      mod,
		DialogMode: dialogMode,
	}
	root, err := chatagent.BuildTRPCLLMAgent(ctx, ag, deps)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	runnerDeps := chatagent.TRPCRunnerDeps{
		SessionService: chatagent.NewInMemoryTRPCSessionService(),
	}
	if s.td.RT != nil && s.td.RT.SessionMemory != nil {
		runnerDeps.MemoryService = memtrpc.NewSQLiteMemoryService(s.td.RT.SessionMemory)
	}
	runner, err := chatagent.NewTRPCRunner(root, runnerDeps)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	s.activeRuns.Store(sessionID, runner)
	defer func() {
		s.activeRuns.Delete(sessionID)
		runner.Close()
		s.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod, stream)
	}()

	userOpts, err := chatagent.UserOptionsJSON(ag, dialogMode, prov, mod, sess.ContextUsedRatio, nil)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	sendText := content
	intRes := intent.Run(ctx, intent.IntentPassFromAgent(ag), s.td.LLMCatalog, s.td.LLMHTTP, prov, mod, content)
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
	meta := intent.SSERunMeta{AgentID: ag.ID, SessionID: sessionID}
	if s.td.TeamSSE != nil {
		s.td.TeamSSE.Publish(biz.TeamRunEvent{
			Type:      "intent_pass",
			SessionID: sessionID,
			Payload:   intent.BuildIntentPassPayload(intRes, meta),
		})
	}
	intent.PublishMonitorLog(ctx, s.td.MonitorLogs, intRes, "chat", meta)

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
	if stream != nil {
		if err := s.td.Sessions.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
		_ = stream.Emit("user_message", userMsg)
	}

	uid := chatagent.UserIDFromCtx(ctx)
	events, err := chatagent.RunTRPCUserTurn(ctx, runner, uid, sessionID, sendText,
		trpcagent.WithRequestID(sessionID),
	)
	if err != nil {
		return userMsg, biz.ChatMessage{}, err
	}

	var reply strings.Builder
	var reasoning strings.Builder
	promptTok, completionTok := 0, 0

	for ev := range events {
		if ctx.Err() != nil {
			return userMsg, biz.ChatMessage{}, ctx.Err()
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
				if stream != nil && ev.Response.IsPartial {
					if d := provider.VisibleStreamingDelta(&reply, text); d != "" {
						_ = stream.Emit("delta", map[string]string{"content": d})
					}
				} else {
					_ = provider.VisibleStreamingDelta(&reply, text)
				}
			}
			if rc := strings.TrimSpace(msg.ReasoningContent); rc != "" {
				if stream != nil && ev.Response.IsPartial {
					if d := provider.VisibleStreamingDelta(&reasoning, rc); d != "" {
						_ = stream.Emit("delta", map[string]string{"reasoning_content": d})
					}
				} else {
					_ = provider.VisibleStreamingDelta(&reasoning, rc)
				}
			}
			if stream != nil && len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					_ = stream.Emit("tool.call", map[string]any{
						"session_id":   sessionID,
						"tool_name":    tc.Function.Name,
						"tool_call_id": tc.ID,
					})
				}
			}
			delta := choice.Delta
			if stream != nil && len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					_ = stream.Emit("tool.call", map[string]any{
						"session_id":   sessionID,
						"tool_name":    tc.Function.Name,
						"tool_call_id": tc.ID,
					})
				}
			}
		}
	}

	if promptTok <= 0 && completionTok <= 0 && strings.TrimSpace(reply.String()) != "" {
		promptTok = roughTokenEstimateFromText(content + reply.String())
		completionTok = roughTokenEstimateFromText(reply.String())
	}

	assistantOptsStr, err := chatagent.AssistantOptionsJSON(ag, nil)
	if err != nil {
		return userMsg, biz.ChatMessage{}, err
	}
	if s := reasoning.String(); s != "" {
		if assistantOptsStr, err = chatagent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, s); err != nil {
			return userMsg, biz.ChatMessage{}, err
		}
	}

	assistantMsg := biz.ChatMessage{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		Role:            "assistant",
		ContentMarkdown: strings.TrimSpace(reply.String()),
		ModelName:       mod,
		Status:          "ok",
		OptionsJSON:     assistantOptsStr,
		CreatedAt:       chatagent.RFC3339Now(),
		TokenIn:         promptTok,
		TokenOut:        completionTok,
	}
	if stream == nil {
		if err := s.td.Sessions.AppendChatTurn(ctx, sessionID, userMsg, assistantMsg); err != nil {
			return userMsg, biz.ChatMessage{}, err
		}
		patchSessionContextUsage(ctx, s, sessionID, ag, promptTok, completionTok)
		return userMsg, assistantMsg, nil
	}
	if err := s.td.Sessions.AppendChatMessage(ctx, sessionID, assistantMsg, true); err != nil {
		return userMsg, biz.ChatMessage{}, err
	}
	patchSessionContextUsage(ctx, s, sessionID, ag, promptTok, completionTok)
	_ = stream.Emit("done", assistantMsg)
	return userMsg, assistantMsg, nil
}

func (s *ChatService) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string, stream *streamWriter) {
	entry, ok := s.dequeuePending(sessionID)
	if !ok {
		return
	}
	go func() {
		bgCtx := context.Background()
		req := &chatv1.SendChatMessageRequest{
			SessionId: sessionID,
			Content:   entry.Content,
		}
		_, _, _ = s.runSingleAgentViaTRPC(bgCtx, sess, req, ag, dialogMode, prov, mod, 0, stream)
	}()
}
