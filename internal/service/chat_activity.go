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
//
// The legacy `seqAlloc` parameter has been removed (GlobalSeqAllocator
// removed — ordering is now governed by Timestamp ASC per design doc §B.3.3).
func NewChatStreamConsumeOptions(tools biz.TeamToolLookup, toolRegistry biz.ToolRegistryReader, agents biz.AgentRepository, activityUpserter biz.ActivityUpserter, activityBus biz.ActivityEventBus, lg loggateway.Logger) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(tools, toolRegistry, agents, activityUpserter, activityBus, lg)
}
