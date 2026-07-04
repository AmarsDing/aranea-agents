package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/chatactivity"
)

// NewChatStreamConsumeOptions wires the v2 projector for a chat turn.
//
// 2026-07-04 问题 4 修复：v2Projector 是 per-turn 实例（由 V2ProjectorFactory
// 创建），不再是全局单例。每个 chat turn 持有独立 Projector 实例，避免与
// 并发的 team member turn 共享状态导致互相清空。
func NewChatStreamConsumeOptions(v2Projector *v2.ActivityProjector) *chatagent.StreamConsumeOptions {
	return chatactivity.NewStreamConsumeOptions(v2Projector)
}
