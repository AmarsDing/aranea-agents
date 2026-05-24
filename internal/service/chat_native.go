package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
)

// nativeDialogModeChatOptions returns the dialog mode options for native chat.
func nativeDialogModeChatOptions() []*chatv1.ChatOption {
	return []*chatv1.ChatOption{
		{Type: "dialog_mode", Key: "default", Label: "标准对话", Enabled: true, SortOrder: 1},
		{Type: "dialog_mode", Key: "plan", Label: "深度思考", Enabled: true, SortOrder: 2},
		{Type: "dialog_mode", Key: "code", Label: "仅代码", Enabled: true, SortOrder: 3},
	}
}

// RunNativeTurn implements biz.NativeTurnGateway — the biz-level turn entry point
// that avoids proto dependency. All internal callers (Channel, Cron, A2A) should
// use this instead of proto-based methods.
func (s *ChatService) RunNativeTurn(ctx context.Context, input biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	return s.orch.RunNativeAgentTurnFromInput(ctx, input)
}

// RunNativeTurnWithOutcome implements biz.NativeTurnGateway with explicit turn classification.
func (s *ChatService) RunNativeTurnWithOutcome(ctx context.Context, input biz.TurnInput) (biz.NativeTurnResult, error) {
	return s.orch.RunNativeAgentTurnWithOutcome(ctx, input)
}

// RunAgentTurn implements a2a.AgentTurnRunner for call_agent and HTTP Invoke dispatch (EP-A2A-01).
func (s *ChatService) RunAgentTurn(ctx context.Context, agentID, input string, timeoutSec int) (string, error) {
	return s.orch.RunAgentTurn(ctx, agentID, input, timeoutSec)
}

// RunCronTurn dispatches a cron-triggered turn through the in-process agent runner.
func (s *ChatService) RunCronTurn(ctx context.Context, sessionID, content, teamID string) (userMsgID, agentMsgID string, err error) {
	return s.orch.RunCronTurn(ctx, sessionID, content, teamID)
}

// hydratedAgent loads and returns an Agent by ID.
func (s *ChatService) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	return s.orch.hydratedAgent(ctx, agentID)
}

// patchSessionContextUsage updates session context usage after a turn.
func patchSessionContextUsage(ctx context.Context, o *ChatOrchestrator, sessionID string, ag biz.Agent, promptTok, completionTok int) {
	o.patchSessionContextUsage(ctx, sessionID, ag, promptTok, completionTok)
}

// notifyNativeTurnHooks runs post-turn side effects.
func notifyNativeTurnHooks(ctx context.Context, o *ChatOrchestrator, sessionID string, ag biz.Agent, userInput, assistantOutput string) {
	o.notifyNativeTurnHooks(ctx, sessionID, ag, userInput, assistantOutput)
}

// chatMessageToMap converts a ChatMessage to a map for proto serialization.
func chatMessageToMap(m biz.ChatMessage) map[string]any {
	return map[string]any{
		"id":                m.ID,
		"session_id":        m.SessionID,
		"parent_message_id": m.ParentMessageID,
		"turn_index":        m.TurnIndex,
		"role":              m.Role,
		"content_markdown":  m.ContentMarkdown,
		"model_name":        m.ModelName,
		"token_in":          m.TokenIn,
		"token_out":         m.TokenOut,
		"latency_ms":        m.LatencyMS,
		"status":            m.Status,
		"attachments_count": m.AttachmentsCount,
		"options_json":      m.OptionsJSON,
		"error_message":     m.ErrorMessage,
		"created_at":        m.CreatedAt,
	}
}

func chatNowRFC3339() string {
	return strings.TrimSpace(chatagent.RFC3339Now())
}
