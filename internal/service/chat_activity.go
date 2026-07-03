package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
)

// NewChatStreamConsumeOptions wires the v2 projector and activity bus for a
// chat turn.
//
// v2 phase: v2Projector is the singleton v2 projector (wired via Wire DI).
// The legacy catalog ActivityMetaResolver, activityWriter, toolRegistry, and
// logger parameters have been removed — they were dead after the v1
// ActivityProjector removal (the resolver was set but never read).
func NewChatStreamConsumeOptions(activityBus biz.ActivityEventBus, v2Projector *v2.ActivityProjector) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(activityBus, v2Projector)
}
