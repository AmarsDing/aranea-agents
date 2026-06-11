package service

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
		if entry.count == 0 && now.Sub(entry.lastAcq) > gateEntryMaxAge {
			delete(g.active, key)
		}
	}
}

// Close implements biz.ConcurrencyGate.
func (g *channelConcurrentGate) Close() {
	if g != nil && g.done != nil {
		close(g.done)
	}
}

// TryAcquire implements biz.ConcurrencyGate.
// It returns a release function on success, nil on failure.
func (g *channelConcurrentGate) TryAcquire(channelID, peerID string, isGroup bool, limit int) (release func(), ok bool) {
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

func (g *channelConcurrentGate) release(channelID, peerID string, isGroup bool) {
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
