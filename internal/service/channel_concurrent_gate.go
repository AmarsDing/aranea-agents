package service

import (
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/channel/port"
	"aranea-agents/pkg/safego"
)

type channelConcurrentKey struct {
	channelID string
	peerID    string
	group     bool
}

type channelConcurrentEntry struct {
	count    int
	lastAcq  time.Time
}

const gateCleanupInterval = 5 * time.Minute
const gateEntryMaxAge = 30 * time.Minute

type channelConcurrentGate struct {
	mu     sync.Mutex
	active map[channelConcurrentKey]*channelConcurrentEntry
	done   chan struct{}
}

func newChannelConcurrentGate() *channelConcurrentGate {
	g := &channelConcurrentGate{
		active: make(map[channelConcurrentKey]*channelConcurrentEntry),
		done:   make(chan struct{}),
	}
	g.startCleanup()
	return g
}

func (g *channelConcurrentGate) startCleanup() {
	ticker := time.NewTicker(gateCleanupInterval)
	safego.Go(nil, "channel_concurrent_gate.cleanup", func() {
		for {
			select {
			case <-ticker.C:
				g.cleanupExpired()
			case <-g.done:
				ticker.Stop()
				return
			}
		}
	})
}

func (g *channelConcurrentGate) cleanupExpired() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for key, entry := range g.active {
		if now.Sub(entry.lastAcq) > gateEntryMaxAge {
			delete(g.active, key)
		}
	}
}

func (g *channelConcurrentGate) Close() {
	if g != nil && g.done != nil {
		close(g.done)
	}
}

func (g *channelConcurrentGate) TryAcquire(channelID, peerID string, isGroup bool, limit int) bool {
	if g == nil || limit <= 0 {
		return true
	}
	key := channelConcurrentKey{channelID: channelID, peerID: peerID, group: isGroup}
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.active[key]
	if !ok {
		entry = &channelConcurrentEntry{count: 0, lastAcq: time.Now()}
		g.active[key] = entry
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	entry.lastAcq = time.Now()
	return true
}

func (g *channelConcurrentGate) Release(channelID, peerID string, isGroup bool) {
	if g == nil {
		return
	}
	key := channelConcurrentKey{channelID: channelID, peerID: peerID, group: isGroup}
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.active[key]
	if !ok {
		return
	}
	if entry.count <= 1 {
		delete(g.active, key)
		return
	}
	entry.count--
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
