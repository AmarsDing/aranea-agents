package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/pkg/loggateway"
)

// NewChatStreamConsumeOptions wires catalog lookup and activity persistence for a chat turn.
func NewChatStreamConsumeOptions(tools biz.TeamToolLookup, agents biz.AgentRepository, sessions biz.SessionTurnExtrasPort, activityWriter biz.ActivityWriter, activityBus biz.ActivityEventBus, lg loggateway.Logger) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(tools, agents, sessions, activityWriter, activityBus, lg)
}
