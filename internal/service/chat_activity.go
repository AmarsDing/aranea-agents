package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
)

// NewChatStreamConsumeOptions wires catalog lookup and activity persistence for a chat turn.
func NewChatStreamConsumeOptions(tools biz.TeamToolLookup, agents biz.AgentRepository, sessions biz.SessionTurnExtrasPort) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(tools, agents, sessions)
}
