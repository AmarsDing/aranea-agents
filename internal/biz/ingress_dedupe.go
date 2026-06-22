package biz

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

const (
	DefaultMessageDedupeTTL = 5 * time.Minute
	DefaultIngressDebounce  = 800 * time.Millisecond
	inflightEntryTTL        = 30 * time.Minute
)

// IngressMessageDedupe suppresses duplicate platform message ids within a TTL window (CH-BOR-06).
// It embeds an inflightSet to fully implement IngressDeduplicator.
type IngressMessageDedupe struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
	*inflightSet
}

// inflightSet tracks in-flight dedup keys with TTL-based cleanup.
type inflightSet struct {
	mu     sync.Mutex
	m      map[string]inflightEntry
	done   chan struct{}
	closeO sync.Once
}

type inflightEntry struct {
	acquiredAt time.Time
}

// NewIngressMessageDedupe creates a new deduplicator with the given TTL.
func NewIngressMessageDedupe(ttl time.Duration) *IngressMessageDedupe {
	if ttl <= 0 {
		ttl = DefaultMessageDedupeTTL
	}
	is := &inflightSet{m: make(map[string]inflightEntry), done: make(chan struct{})}
	safego.Go(context.Background(), "inflightSet.cleanupLoop", is.cleanupLoop)
	return &IngressMessageDedupe{seen: make(map[string]time.Time), ttl: ttl, inflightSet: is}
}

// ClaimMessage implements IngressDeduplicator.
// Returns false when the message was already seen within TTL.
func (d *IngressMessageDedupe) ClaimMessage(channelID, messageID string) bool {
	key := IngressMessageDedupeKey(channelID, messageID)
	return d.claim(key, time.Now())
}

// TryAcquireInflight implements IngressDeduplicator.
// Returns false when the dedup key is already being processed.
func (d *IngressMessageDedupe) TryAcquireInflight(dedupKey string) bool {
	return d.inflightSet.tryAcquire(dedupKey)
}

// ReleaseInflight implements IngressDeduplicator.
func (d *IngressMessageDedupe) ReleaseInflight(dedupKey string) {
	d.inflightSet.release(dedupKey)
}

// Stop implements IngressDeduplicator.
// It terminates the background cleanup goroutine of the inflight set.
func (d *IngressMessageDedupe) Stop() {
	d.inflightSet.Stop()
}

// claim returns false when the message id was seen within TTL.
func (d *IngressMessageDedupe) claim(key string, now time.Time) bool {
	if d == nil || key == "" {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen == nil {
		d.seen = make(map[string]time.Time)
	}
	d.purgeLocked(now)
	if seenAt, ok := d.seen[key]; ok && now.Sub(seenAt) < d.ttl {
		return false
	}
	d.seen[key] = now
	return true
}

func (d *IngressMessageDedupe) purgeLocked(now time.Time) {
	for k, seenAt := range d.seen {
		if now.Sub(seenAt) >= d.ttl {
			delete(d.seen, k)
		}
	}
}

func (s *inflightSet) tryAcquire(key string) bool {
	if s == nil {
		return true
	}
	if key == "" {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if entry, ok := s.m[key]; ok && now.Sub(entry.acquiredAt) < inflightEntryTTL {
		return false
	}
	s.m[key] = inflightEntry{acquiredAt: now}
	return true
}

func (s *inflightSet) release(key string) {
	if s == nil {
		return
	}
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

func (s *inflightSet) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for k, v := range s.m {
				if now.Sub(v.acquiredAt) >= inflightEntryTTL {
					delete(s.m, k)
				}
			}
			s.mu.Unlock()
		case <-s.done:
			return
		}
	}
}

// Stop terminates the background cleanup goroutine.
// Safe to call multiple times.
func (s *inflightSet) Stop() {
	if s != nil && s.done != nil {
		s.closeO.Do(func() { close(s.done) })
	}
}
