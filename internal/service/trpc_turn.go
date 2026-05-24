package service

import (
	"context"

	"aranea-agents/internal/biz"
)

// runSingleAgentViaTRPC delegates to ChatOrchestrator.
func (s *ChatService) runSingleAgentViaTRPC(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	dialogMode, prov, mod string,
	attN int,
) (biz.ChatMessage, biz.ChatMessage, error) {
	return s.orch.runSingleAgentViaTRPC(ctx, sess, input, ag, dialogMode, prov, mod, attN)
}

// processPendingQueue delegates to ChatOrchestrator.
func (s *ChatService) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string) {
	s.orch.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod)
}

// recordSessionTurn delegates to ChatOrchestrator.
func (s *ChatService) recordSessionTurn(ctx context.Context, sessionID string, ag biz.Agent, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	s.orch.recordSessionTurn(ctx, sessionID, ag, userMsgID, assistantMsgID, prov, mod, promptTok, completionTok, contentPreview)
}

// recordTeamSessionTurn delegates to ChatOrchestrator.
func (s *ChatService) recordTeamSessionTurn(ctx context.Context, sessionID, teamID, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	s.orch.recordTeamSessionTurn(ctx, sessionID, teamID, userMsgID, assistantMsgID, prov, mod, promptTok, completionTok, contentPreview)
}
