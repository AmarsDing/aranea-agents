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

// GateStaleActiveMaxAge is the safety-net threshold for force-resetting
// entries whose count > 0 but haven't been touched for this duration.
// This prevents permanent blocking when a release call is lost (panic, bug).
const GateStaleActiveMaxAge = 2 * time.Hour

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
		age := now.Sub(entry.lastAcq)
		if entry.count == 0 && age > GateEntryMaxAge {
			delete(g.active, key)
			continue
		}
		// Safety net: force-reset stale active entries to prevent permanent blocking.
		if entry.count > 0 && age > GateStaleActiveMaxAge {
			entry.count = 0
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
// limit <= 0 means "no concurrency limit" — all requests are allowed.
func (g *ChannelConcurrentGate) TryAcquire(channelID, peerID string, isGroup bool, limit int) (release func(), ok bool) {
	if g == nil {
		return func() {}, true
	}
	if limit <= 0 {
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
	entry.lastAcq = time.Now() // refresh to prevent stale detection from evicting active entries
}
