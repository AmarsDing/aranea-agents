package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/chatactivity"
)

// NewChatStreamConsumeOptions wires the v2 projector for a chat turn.
//
// v2 phase: v2Projector is the singleton v2 projector (wired via Wire DI).
// The legacy catalog ActivityMetaResolver, activityWriter, toolRegistry,
// logger, and activityBus parameters have been removed — they were dead
// after the v1 ActivityProjector removal (the resolver was set but never
// read) and Phase 3b-D Tier 4 v1 bus removal (opts.ActivityBus was never
// read by the framework).
func NewChatStreamConsumeOptions(v2Projector *v2.ActivityProjector) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(v2Projector)
}
