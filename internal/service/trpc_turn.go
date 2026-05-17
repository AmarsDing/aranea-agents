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

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"

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
) (biz.ChatMessage, biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	content := strings.TrimSpace(req.GetContent())
	if ak := strings.TrimSpace(req.GetAgentKey()); ak != "" && !strings.EqualFold(ak, ag.AgentKey) {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_AGENT", "agent_key does not match this session")
	}

	deps := chatagent.TRPCBuilderDeps{
		Catalog:     s.td.Catalog.LLM,
		AgentUC:     s.td.Catalog.AgentsUC,
		Agents:      s.td.Catalog.Agents,
		RT:          s.td.RoundTrip(),
		SkillUC:     s.td.Catalog.SkillUC,
		MCPTooling:  s.td.Persist.AgentMCP,
		ToolUC:      s.td.Catalog.ToolUC,
		Sys:         s.td.Catalog.Settings,
		Provider:    prov,
		Model:       mod,
		DialogMode:  dialogMode,
		SkillDBRepo: s.skillDBRepo,
	}
	root, err := chatagent.BuildTRPCLLMAgentCached(ctx, ag, deps)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	var plugins []trpcplugin.Plugin
	if s.pluginRT != nil {
		plugins = s.pluginRT.Plugins()
	}
	runnerDeps := chatagent.NewRunnerDepsFromRuntime(s.td.Persist.Session, s.td.Persist.Memory, plugins...)
	runner, err := chatagent.NewTRPCRunner(root, runnerDeps)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	s.activeRuns.Store(sessionID, runner)
	defer func() {
		s.activeRuns.Delete(sessionID)
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

	projectMeta := chatagent.ProjectMeta{
		SessionID: sessionID,
		RequestID: sessionID,
	}
	result := chatagent.ConsumeEventStream(ctx, events, s.td.Pipeline.Bus, projectMeta)
	if ctx.Err() != nil {
		return userMsg, biz.ChatMessage{}, ctx.Err()
	}

	promptTok, completionTok := result.PromptTok, result.CompletionTok
	if promptTok <= 0 && completionTok <= 0 && strings.TrimSpace(result.Reply.String()) != "" {
		promptTok = roughTokenEstimateFromText(content + result.Reply.String())
		completionTok = roughTokenEstimateFromText(result.Reply.String())
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
		ContentMarkdown: strings.TrimSpace(result.Reply.String()),
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
	patchSessionContextUsage(ctx, s, sessionID, ag, promptTok, completionTok)

	s.recordSessionTurn(ctx, sessionID, ag, userMsg.ID, assistantMsg.ID, prov, mod, promptTok, completionTok, assistantMsg.ContentMarkdown)

	return userMsg, assistantMsg, nil
}

func (s *ChatService) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string) {
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
		_, _, err := s.runSingleAgentViaTRPC(bgCtx, sess, req, ag, dialogMode, prov, mod, 0)
		if err != nil && s.td.Pipeline.Bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeError, "pending-queue", sessionID)
			env.Error = &event.EnvelopeError{
				Type:    "pending_failed",
				Message: fmt.Sprintf("pending message failed: %s", err.Error()),
			}
			env.Metadata = map[string]any{"pending_id": entry.ID}
			s.td.Pipeline.Bus.Publish(bgCtx, env)
		}
	}()
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
