package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
)

// NewChatStreamConsumeOptions wires catalog lookup and activity persistence for a chat turn.
func NewChatStreamConsumeOptions(tools *biz.ToolUsecase, agents biz.AgentRepository, sessions *biz.SessionUsecase) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(tools, agents, sessions)
}

// toolUCCast extracts the concrete *biz.ToolUsecase from the narrow interface.
// TECH-DEBT: chatactivity needs the full Usecase API; remove once chatactivity
// is refactored to use narrow interfaces.
func toolUCCast(uc biz.TeamToolLookup) *biz.ToolUsecase {
	if c, ok := uc.(*biz.ToolUsecase); ok {
		return c
	}
	return nil
}
