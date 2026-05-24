package service

import (
	"sync"
	"time"
)

const (
	defaultIngressDebounce     = 800 * time.Millisecond
	defaultMessageDedupeTTL    = 5 * time.Minute
)

// ingressMessageDedupe suppresses duplicate platform message ids within a TTL window (CH-BOR-06).
type ingressMessageDedupe struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func newIngressMessageDedupe(ttl time.Duration) *ingressMessageDedupe {
	if ttl <= 0 {
		ttl = defaultMessageDedupeTTL
	}
	return &ingressMessageDedupe{seen: make(map[string]time.Time), ttl: ttl}
}

func ingressMessageDedupeKey(channelID, messageID string) string {
	if channelID == "" || messageID == "" {
		return ""
	}
	return channelID + ":" + messageID
}

// claim returns false when the message id was seen within TTL.
func (d *ingressMessageDedupe) claim(key string, now time.Time) bool {
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

func (d *ingressMessageDedupe) purgeLocked(now time.Time) {
	for k, seenAt := range d.seen {
		if now.Sub(seenAt) >= d.ttl {
			delete(d.seen, k)
		}
	}
}

func shouldSkipRecentDuplicate(lastSeen time.Time, ttl time.Duration, now time.Time) bool {
	if lastSeen.IsZero() || ttl <= 0 {
		return false
	}
	return now.Sub(lastSeen) < ttl
}
