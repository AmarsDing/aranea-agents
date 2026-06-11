package biz

import (
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

type channelConcurrentKey struct {
	channelID string
	peerID    string
	group     bool
}

type channelConcurrentEntry struct {
	count   int
	lastAcq time.Time
}

const GateCleanupInterval = 5 * time.Minute
const GateEntryMaxAge = 30 * time.Minute

// ChannelConcurrentGate limits the number of concurrent inbound turns per channel+peer.
type ChannelConcurrentGate struct {
	mu     sync.Mutex
	active map[channelConcurrentKey]*channelConcurrentEntry
	done   chan struct{}
	closeO sync.Once
}

// NewChannelConcurrentGate creates a new concurrency gate.
func NewChannelConcurrentGate() *ChannelConcurrentGate {
	g := &ChannelConcurrentGate{
		active: make(map[channelConcurrentKey]*channelConcurrentEntry),
		done:   make(chan struct{}),
	}
	g.startCleanup()
	return g
}

func (g *ChannelConcurrentGate) startCleanup() {
	ticker := time.NewTicker(GateCleanupInterval)
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

func (g *ChannelConcurrentGate) cleanupExpired() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for key, entry := range g.active {
		if entry.count == 0 && now.Sub(entry.lastAcq) > GateEntryMaxAge {
			delete(g.active, key)
		}
	}
}

// Close implements ConcurrencyGate.
// Safe to call multiple times.
func (g *ChannelConcurrentGate) Close() {
	if g != nil && g.done != nil {
		g.closeO.Do(func() { close(g.done) })
	}
}

// TryAcquire implements ConcurrencyGate.
// It returns a release function on success, nil on failure.
func (g *ChannelConcurrentGate) TryAcquire(channelID, peerID string, isGroup bool, limit int) (release func(), ok bool) {
	if g == nil || limit <= 0 {
		return func() {}, true
	}
	key := channelConcurrentKey{channelID: channelID, peerID: peerID, group: isGroup}
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, exists := g.active[key]
	if !exists {
		entry = &channelConcurrentEntry{count: 0, lastAcq: time.Now()}
		g.active[key] = entry
	}
	if entry.count >= limit {
		return nil, false
	}
	entry.count++
	entry.lastAcq = time.Now()
	return func() { g.release(channelID, peerID, isGroup) }, true
}

func (g *ChannelConcurrentGate) release(channelID, peerID string, isGroup bool) {
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
