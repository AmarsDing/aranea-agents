package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
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
//
// v2 phase: v2Projector is the singleton v2 projector (wired via Wire DI).
// When non-nil, every chat turn triggers the v2 dual-path alongside v1.
func NewChatStreamConsumeOptions(tools biz.TeamToolLookup, toolRegistry biz.ToolRegistryReader, agents biz.AgentRepository, activityUpserter biz.ActivityUpserter, activityBus biz.ActivityEventBus, v2Projector *v2.ActivityProjector, lg loggateway.Logger) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(tools, toolRegistry, agents, activityUpserter, activityBus, v2Projector, lg)
}
