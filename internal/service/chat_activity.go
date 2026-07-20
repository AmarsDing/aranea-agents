package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/chatactivity"
)

// NewChatStreamConsumeOptions wires the v2 projector for a chat turn.
func NewChatStreamConsumeOptions(v2Projector *v2.ActivityProjector) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(v2Projector)
}
