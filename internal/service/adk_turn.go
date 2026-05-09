package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/adksvc"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/tools"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	adkagent "google.golang.org/adk/agent"

	"google.golang.org/genai"
)

// runSingleAgentViaADK executes one catalog-agent turn using pkg/adk-go Runner + llmagent.
func (s *ChatService) runSingleAgentViaADK(
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

	ss := adksvc.NewBizSessionForUsecase(s.sessions, func(c context.Context, agentID string) (string, error) {
		a, err := s.agents.GetAgentByID(c, agentID)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(a.AgentKey), nil
	})

	rt := &provider.RoundTrip{HTTP: s.llmHTTP}
	mount := tools.TurnMount{
		AgentsUC: s.agentsUC, Agents: s.agents, ToolsCatalog: s.toolsCatalog,
		SkillUC: s.skillUC, Sys: s.sys,
	}
	if s.adk != nil {
		mount.MCP = s.adk.AgentMCP
	}
	deps := chatagent.BuilderDeps{
		Catalog:             s.llmCatalog,
		AgentUC:             s.agentsUC,
		Agents:              s.agents,
		ToolsCatalog:        s.toolsCatalog,
		RT:                  rt,
		Provider:            prov,
		Model:               mod,
		SQLiteSessionMemory: s.adk != nil && s.adk.SessionMemory != nil,
	}
	if err := mount.Attach(ctx, ag, content, &deps.Tools, &deps.Toolsets); err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	root, err := chatagent.BuildLLMAgent(ctx, ag, deps)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	rn, err := chatagent.NewADKRunnerForRuntime(root, ss, s.adk)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	userOpts, err := chatagent.UserOptionsJSON(ag, dialogMode, prov, mod, sess.ContextUsedRatio, nil)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
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

	if stream != nil {
		if err := s.sessions.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
		_ = stream.Emit("user_message", userMsg)
	}

	msg := genai.NewContentFromText(content, genai.RoleUser)
	uid := chatagent.UserIDFromCtx(ctx)
	cfg := adkagent.RunConfig{StreamingMode: adkagent.StreamingModeSSE}
	if stream == nil {
		cfg = adkagent.RunConfig{StreamingMode: adkagent.StreamingModeNone}
	}

	mono := func(p chatagent.ChatToolUseSSE) {
		if s.teamSSE == nil {
			return
		}
		typ := "tool.result"
		if p.Phase == "before" {
			typ = "tool.call"
		}
		s.teamSSE.Publish(biz.TeamRunEvent{
			Type:      typ,
			SessionID: sessionID,
			Payload:   chatagent.ChatToolPayloadMap(p),
		})
	}
	toolRelay := chatagent.NewChatToolSSERelay(stream, mono)
	var reply strings.Builder
	var reasoning strings.Builder
	var lastUsage *genai.GenerateContentResponseUsageMetadata
	for ev, err := range rn.Run(ctx, uid, sessionID, msg, cfg) {
		if ctx.Err() != nil {
			return userMsg, biz.ChatMessage{}, ctx.Err()
		}
		if err != nil {
			return userMsg, biz.ChatMessage{}, err
		}
		if ev == nil {
			continue
		}
		if ev.LLMResponse.UsageMetadata != nil {
			lastUsage = ev.LLMResponse.UsageMetadata
		}
		agEv := chatagent.AgentForToolSSELabel(ev.Author, ag)
		if toolRelay != nil && chatagent.SessionHasFunctionResponses(ev) {
			toolRelay.EmitToolResponses(agEv, ev)
		}
		if stream != nil && ev.LLMResponse.Partial {
			if toolRelay != nil && chatagent.SessionHasFunctionCalls(ev) {
				toolRelay.EmitToolCalls(agEv, ev)
			}
			main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
			if d := provider.VisibleStreamingDelta(&reply, main); d != "" {
				_ = stream.Emit("delta", map[string]string{"content": d})
			}
			if dr := provider.VisibleStreamingDelta(&reasoning, rsn); dr != "" {
				_ = stream.Emit("delta", map[string]string{"reasoning_content": dr})
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(ev.Author), "user") {
			continue
		}
		if toolRelay != nil && chatagent.SessionHasFunctionCalls(ev) {
			toolRelay.EmitToolCalls(agEv, ev)
		}
		main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
		_ = provider.VisibleStreamingDelta(&reply, main)
		_ = provider.VisibleStreamingDelta(&reasoning, rsn)
	}
	_ = chatagent.SyncPersistedADKSessionToMemory(ctx, ss, chatagent.RunnerMemoryForRuntime(s.adk), adksvc.DefaultAppName, uid, sessionID)

	promptTok, completionTok := 0, 0
	if lastUsage != nil {
		promptTok = int(lastUsage.PromptTokenCount)
		completionTok = int(lastUsage.CandidatesTokenCount)
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
		if err := s.sessions.AppendChatTurn(ctx, sessionID, userMsg, assistantMsg); err != nil {
			return userMsg, biz.ChatMessage{}, err
		}
		patchSessionContextUsage(ctx, s, sessionID, ag, promptTok, completionTok)
		return userMsg, assistantMsg, nil
	}
	if err := s.sessions.AppendChatMessage(ctx, sessionID, assistantMsg, true); err != nil {
		return userMsg, biz.ChatMessage{}, err
	}
	patchSessionContextUsage(ctx, s, sessionID, ag, promptTok, completionTok)
	_ = stream.Emit("done", assistantMsg)
	return userMsg, assistantMsg, nil
}

func patchSessionContextUsage(ctx context.Context, s *ChatService, sessionID string, ag biz.Agent, promptTok, completionTok int) {
	if s == nil || s.sessions == nil {
		return
	}
	win := ag.ContextWindow
	if win <= 0 {
		win = 128000
	}
	_ = s.sessions.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTok, completionTok, win)
	if s.compress != nil {
		s.compress.AfterNativeTurn(ctx, sessionID, ag)
	}
}
