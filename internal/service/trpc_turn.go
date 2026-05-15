package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

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
		MCPTooling: s.td.RT.AgentMCP,
		ToolUC:     s.td.ToolUC,
		Sys:        s.td.Sys,
		Provider:   prov,
		Model:      mod,
		DialogMode: dialogMode,
	}
	root, err := chatagent.BuildTRPCLLMAgent(ctx, ag, deps)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	runnerDeps := chatagent.TRPCRunnerDeps{}
	if s.td.RT != nil {
		if s.td.RT.TRPCSession != nil {
			runnerDeps.SessionService = s.td.RT.TRPCSession
		}
		if s.td.RT.SessionMemory != nil {
			runnerDeps.MemoryService = memtrpc.NewSQLiteMemoryService(s.td.RT.SessionMemory)
		}
	}
	if runnerDeps.SessionService == nil {
		runnerDeps.SessionService = chatagent.NewInMemoryTRPCSessionService()
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
	intentPayload := intent.BuildIntentPassPayload(intRes, meta)
	if s.td.TeamSSE != nil {
		s.td.TeamSSE.Publish(biz.TeamRunEvent{
			Type:      "intent_pass",
			SessionID: sessionID,
			Payload:   intentPayload,
		})
	}
	if stream != nil {
		_ = stream.Emit("intent_pass", intentPayload)
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
	runOpts := []trpcagent.RunOption{trpcagent.WithRequestID(sessionID)}
	if ag.Settings != nil {
		if vars := chatagent.ParseVariablesJSON(ag.Settings.VariablesJSON); vars != nil {
			runOpts = append(runOpts, trpcagent.MergeRuntimeState(vars))
		}
	}
	events, err := chatagent.RunTRPCUserTurn(ctx, runner, uid, sessionID, sendText, runOpts...)
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
		if ev == nil {
			continue
		}
		if ev.IsRunnerCompletion() {
			continue
		}
		if stream != nil {
			if len(ev.StateDelta) > 0 {
				_ = stream.Emit("state_delta", map[string]any{
					"session_id":  sessionID,
					"state_delta": ev.StateDelta,
				})
			}
			if len(ev.Extensions) > 0 {
				_ = stream.Emit("extensions", map[string]any{
					"session_id": sessionID,
					"extensions": ev.Extensions,
				})
			}
			if ev.Branch != "" {
				_ = stream.Emit("branch", map[string]string{
					"session_id": sessionID,
					"branch":     ev.Branch,
				})
			}
			if ev.FilterKey != "" {
				_ = stream.Emit("filter_key", map[string]string{
					"session_id": sessionID,
					"filter_key": ev.FilterKey,
				})
			}
			if ev.Tag != "" {
				_ = stream.Emit("tag", map[string]string{
					"session_id": sessionID,
					"tag":        ev.Tag,
				})
			}
		}
		if ev.Response == nil {
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
		bgCtx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
		s.pendingCancels.Store(sessionID, cancel)
		defer func() {
			cancel()
			s.pendingCancels.Delete(sessionID)
		}()
		req := &chatv1.SendChatMessageRequest{
			SessionId: sessionID,
			Content:   entry.Content,
		}
		_, _, _ = s.runSingleAgentViaTRPC(bgCtx, sess, req, ag, dialogMode, prov, mod, 0, stream)
	}()
}
