package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// NewChatStreamConsumeOptions wires catalog lookup and activity persistence for a chat turn.
func NewChatStreamConsumeOptions(tools biz.TeamToolLookup, agents biz.AgentRepository, sessions biz.SessionTurnExtrasPort, activityWriter biz.ActivityWriter, eventBus event.Bus, lg loggateway.Logger) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(tools, agents, sessions, activityWriter, eventBus, lg)
}
