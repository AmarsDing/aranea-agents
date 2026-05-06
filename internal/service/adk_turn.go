package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/adkadapter"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"

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

	ss := &adkadapter.BizSessionService{
		Repo:    adkadapter.UsecaseSessionRepo{UC: s.sessions},
		AppName: adkadapter.DefaultAppName,
	}
	ss.ResolveAssistantAuthor = func(c context.Context, agentID string) (string, error) {
		a, err := s.agents.GetAgentByID(c, agentID)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(a.AgentKey), nil
	}

	rt := &provider.RoundTrip{HTTP: s.llmHTTP}
	root, err := adkadapter.BuildLLMAgent(ctx, ag, adkadapter.BuilderDeps{
		Catalog:  s.llmCatalog,
		AgentUC:  s.agentsUC,
		Agents:   s.agents,
		RT:       rt,
		Provider: prov,
		Model:    mod,
	})
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	rn, err := runner.New(runner.Config{
		AppName:           adkadapter.DefaultAppName,
		Agent:             root,
		SessionService:    ss,
		MemoryService:     adkadapter.NewADKMemoryService(),
		AutoCreateSession: false,
		PluginConfig: runner.PluginConfig{
			Plugins: adkadapter.DefaultRunnerPlugins(),
		},
	})
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
	uid := adkadapter.UserIDFromCtx(ctx)
	cfg := adkagent.RunConfig{StreamingMode: adkagent.StreamingModeSSE}
	if stream == nil {
		cfg = adkagent.RunConfig{StreamingMode: adkagent.StreamingModeNone}
	}

	var reply strings.Builder
	var reasoning strings.Builder
	for ev, err := range rn.Run(ctx, uid, sessionID, msg, cfg) {
		if err != nil {
			return userMsg, biz.ChatMessage{}, err
		}
		if ev == nil {
			continue
		}
		if stream != nil && ev.LLMResponse.Partial {
			main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
			if main != "" {
				_ = stream.Emit("delta", map[string]string{"content": main})
			}
			if rsn != "" {
				_ = stream.Emit("delta", map[string]string{"reasoning_content": rsn})
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(ev.Author), "user") {
			continue
		}
		main, rsn := provider.TextsFromLLMResponse(&ev.LLMResponse)
		if main != "" {
			reply.WriteString(main)
		}
		if rsn != "" {
			reasoning.WriteString(rsn)
		}
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
	}
	if stream == nil {
		if err := s.sessions.AppendChatTurn(ctx, sessionID, userMsg, assistantMsg); err != nil {
			return userMsg, biz.ChatMessage{}, err
		}
		return userMsg, assistantMsg, nil
	}
	if err := s.sessions.AppendChatMessage(ctx, sessionID, assistantMsg, true); err != nil {
		return userMsg, biz.ChatMessage{}, err
	}
	_ = stream.Emit("done", assistantMsg)
	return userMsg, assistantMsg, nil
}
