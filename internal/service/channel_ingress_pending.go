package service

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func (h *ChannelIngress) bindChannelPendingMode(sessionID, configJSON string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || h == nil {
		return
	}
	svc, ok := h.chat.(*ChatService)
	if !ok || svc.orch == nil {
		return
	}
	svc.orch.SetSessionPendingMergeFollowup(sessionID, biz.ChannelBusyInputFollowup(configJSON))
}

func (h *ChannelIngress) clearChannelPendingMode(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || h == nil {
		return
	}
	svc, ok := h.chat.(*ChatService)
	if !ok || svc.orch == nil {
		return
	}
	svc.orch.SetSessionPendingMergeFollowup(sessionID, false)
}

func (h *ChannelIngress) tryAcquireChannelConcurrent(chRow biz.Channel, ev port.InboundEvent, ltCfg biz.ChannelLongTaskConfig) (release func(), ok bool) {
	if h == nil || h.concurrentGate == nil {
		return func() {}, true
	}
	isGroup := inboundEventIsGroup(ev)
	limit := ltCfg.MaxConcurrentInbound(isGroup)
	if !h.concurrentGate.TryAcquire(chRow.ID, isGroup, limit) {
		return nil, false
	}
	return func() { h.concurrentGate.Release(chRow.ID, isGroup) }, true
}
