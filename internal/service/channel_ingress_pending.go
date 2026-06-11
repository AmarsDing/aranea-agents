package service

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func (h *ChannelIngress) bindChannelPendingMode(sessionID, configJSON string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || h == nil || h.chat == nil {
		return
	}
	h.chat.SetSessionPendingMergeFollowup(sessionID, biz.ChannelBusyInputFollowup(configJSON))
}

func (h *ChannelIngress) clearChannelPendingMode(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || h == nil || h.chat == nil {
		return
	}
	h.chat.SetSessionPendingMergeFollowup(sessionID, false)
}

func (h *ChannelIngress) tryAcquireChannelConcurrent(chRow biz.Channel, ev port.InboundEvent, ltCfg biz.ChannelLongTaskConfig) (release func(), ok bool) {
	if h == nil || h.concurrentGate == nil {
		return func() {}, true
	}
	isGroup := biz.InboundEventIsGroup(ev.OutboundMeta)
	peerID := strings.TrimSpace(ev.PeerID)
	limit := ltCfg.MaxConcurrentInbound(isGroup)
	return h.concurrentGate.TryAcquire(chRow.ID, peerID, isGroup, limit)
}
