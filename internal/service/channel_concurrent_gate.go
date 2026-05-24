package service

import (
	"strings"
	"sync"

	"aranea-agents/internal/channel/port"
)

type channelConcurrentKey struct {
	channelID string
	group     bool
}

type channelConcurrentGate struct {
	mu     sync.Mutex
	active map[channelConcurrentKey]int
}

func newChannelConcurrentGate() *channelConcurrentGate {
	return &channelConcurrentGate{active: make(map[channelConcurrentKey]int)}
}

func (g *channelConcurrentGate) TryAcquire(channelID string, isGroup bool, limit int) bool {
	if g == nil || limit <= 0 {
		return true
	}
	key := channelConcurrentKey{channelID: channelID, group: isGroup}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[key] >= limit {
		return false
	}
	g.active[key]++
	return true
}

func (g *channelConcurrentGate) Release(channelID string, isGroup bool) {
	if g == nil {
		return
	}
	key := channelConcurrentKey{channelID: channelID, group: isGroup}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[key] <= 1 {
		delete(g.active, key)
		return
	}
	g.active[key]--
}

func inboundEventIsGroup(ev port.InboundEvent) bool {
	if ev.OutboundMeta == nil {
		return false
	}
	meta := ev.OutboundMeta
	switch strings.ToLower(strings.TrimSpace(meta["chat_type"])) {
	case "group", "supergroup":
		return true
	default:
		return strings.TrimSpace(meta["group_id"]) != ""
	}
}
