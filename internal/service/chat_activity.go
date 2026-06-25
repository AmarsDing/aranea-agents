package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/pkg/loggateway"
)

// NewChatStreamConsumeOptions wires catalog lookup and the ActivityProjector for a chat turn.
//
// Phase 1c: the legacy `sessions` parameter (SessionTurnExtrasPort) has been
// removed — its UpsertChatActivityMessage path was a no-op backed by
// NoopMessageWriter. Activity persistence is owned exclusively by the
// ActivityProjector.
func NewChatStreamConsumeOptions(tools biz.TeamToolLookup, agents biz.AgentRepository, activityWriter biz.ActivityWriter, activityBus biz.ActivityEventBus, lg loggateway.Logger) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(tools, agents, activityWriter, activityBus, lg)
}
