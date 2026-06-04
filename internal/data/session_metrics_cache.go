package data

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type metricsCacheEntry struct {
	metrics  *biz.SessionMetrics
	expireAt time.Time
}

// SessionMetricsCache wraps SessionMetricsReader with a simple TTL-based cache.
type SessionMetricsCache struct {
	reader   biz.SessionMetricsReader
	entries  sync.Map
	ttl      time.Duration
	capacity int
	lg       loggateway.Logger
}

var _ biz.SessionMetricsReader = (*SessionMetricsCache)(nil)

func NewSessionMetricsCache(reader biz.SessionMetricsReader, lg loggateway.Logger) *SessionMetricsCache {
	return &SessionMetricsCache{
		reader:   reader,
		ttl:      30 * time.Second,
		capacity: 500,
		lg:       lg,
	}
}

func (c *SessionMetricsCache) GetSessionMetrics(ctx context.Context, sessionID string) (*biz.SessionMetrics, error) {
	if v, ok := c.entries.Load(sessionID); ok {
		entry := v.(*metricsCacheEntry)
		if time.Now().Before(entry.expireAt) {
			return entry.metrics, nil
		}
		c.entries.Delete(sessionID)
	}

	metrics, err := c.reader.GetSessionMetrics(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if metrics != nil {
		c.entries.Store(sessionID, &metricsCacheEntry{
			metrics:  metrics,
			expireAt: time.Now().Add(c.ttl),
		})
	}
	return metrics, nil
}

func (c *SessionMetricsCache) ListSessionMetricsByIDs(ctx context.Context, ids []string) (map[string]*biz.SessionMetrics, error) {
	result := make(map[string]*biz.SessionMetrics, len(ids))
	var missed []string

	for _, id := range ids {
		if v, ok := c.entries.Load(id); ok {
			entry := v.(*metricsCacheEntry)
			if time.Now().Before(entry.expireAt) {
				result[id] = entry.metrics
				continue
			}
			c.entries.Delete(id)
		}
		missed = append(missed, id)
	}

	if len(missed) == 0 {
		return result, nil
	}

	fromDB, err := c.reader.ListSessionMetricsByIDs(ctx, missed)
	if err != nil {
		return nil, err
	}
	for id, m := range fromDB {
		result[id] = m
		if m != nil {
			c.entries.Store(id, &metricsCacheEntry{
				metrics:  m,
				expireAt: time.Now().Add(c.ttl),
			})
		}
	}
	return result, nil
}

// Invalidate removes a single session's metrics from the cache.
func (c *SessionMetricsCache) Invalidate(sessionID string) {
	c.entries.Delete(sessionID)
}

// InvalidateAll clears the entire cache.
func (c *SessionMetricsCache) InvalidateAll() {
	c.entries.Range(func(key, _ any) bool {
		c.entries.Delete(key)
		return true
	})
}
