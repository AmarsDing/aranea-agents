package service

import (
	"context"
	"fmt"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

// EvalTurnGateway runs evaluation agent turns without coupling evaluation to ChatService wiring.
type EvalTurnGateway interface {
	RunEvalAgentTurn(ctx context.Context, agentID, input string) (reply string, err error)
}

// RunEvalAgentTurn implements EvalTurnGateway using native trpc-agent-go turn execution.
func (s *ChatService) RunEvalAgentTurn(ctx context.Context, agentID, input string) (string, error) {
	if s == nil || s.td.Sessions == nil {
		return "", fmt.Errorf("eval: chat service not configured")
	}
	agentID = strings.TrimSpace(agentID)
	input = strings.TrimSpace(input)
	if agentID == "" || input == "" {
		return "", fmt.Errorf("eval: agent_id and input are required")
	}
	sess, err := s.td.Sessions.Create(ctx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   agentID,
		OwnerType: "agent",
		Title:     fmt.Sprintf("eval-%s", agentID),
		UserID:    "1",
	})
	if err != nil {
		return "", fmt.Errorf("eval: create session: %w", err)
	}
	_, asst, err := s.RunNativeTurnUnary(ctx, &chatv1.SendChatMessageRequest{
		SessionId: sess.ID,
		Content:   input,
	})
	if err != nil {
		return "", err
	}
	return asst.ContentMarkdown, nil
}
