package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/skillrecommend"
)

// skillHealthCacheTTL bounds how long a cached health-metrics lookup stays
// valid. Ranking runs per turn and does not need second-level freshness;
// 5 minutes keeps DB aggregate queries off the hot path while metrics still
// reflect recent invocations.
const skillHealthCacheTTL = 5 * time.Minute

// skillHealthCacheMaxEntries is a soft cap; on overflow expired entries are
// purged, and the map is reset if still over the cap (bounded memory).
const skillHealthCacheMaxEntries = 1024

// skillHealthCacheEntry caches one GetHealthMetrics lookup outcome.
// metrics == nil means "no data for this window" (a cacheable miss).
type skillHealthCacheEntry struct {
	metrics   *biz.SkillHealthMetrics
	expiresAt time.Time
}

// SkillHealthMetricsAdapter adapts the Biz layer's SkillHealthAggregator
// to both biz.HealthMetricsProvider and skillrecommend.HealthMetricsProvider.
// Placed in the service layer because it depends on both biz and tools packages.
//
// Lookups are memoized in-process per (skillID, days) with a TTL: a single
// ranking pass queries every candidate twice (success rate + avg duration),
// and consecutive turns repeat the same aggregates. Errors are NOT cached so
// transient failures retry on the next access.
type SkillHealthMetricsAdapter struct {
	agg biz.SkillHealthAggregator

	// now is overridable in tests.
	now func() time.Time

	mu    sync.Mutex
	cache map[string]skillHealthCacheEntry
}

// NewSkillHealthMetricsAdapter creates a new adapter.
func NewSkillHealthMetricsAdapter(agg biz.SkillHealthAggregator) *SkillHealthMetricsAdapter {
	return &SkillHealthMetricsAdapter{
		agg:   agg,
		now:   time.Now,
		cache: make(map[string]skillHealthCacheEntry),
	}
}

// GetRecentSuccessRate returns the success rate (0-1) for a skill over the
// last N days by delegating to SkillHealthAggregator.GetHealthMetrics.
func (a *SkillHealthMetricsAdapter) GetRecentSuccessRate(ctx context.Context, skillID string, days int) (float64, error) {
	metrics, err := a.lookup(ctx, skillID, days)
	if err != nil || metrics == nil {
		return 0, err
	}
	return metrics.SuccessRate, nil
}

// GetRecentAvgDuration returns the average duration in milliseconds for a
// skill over the last N days by delegating to SkillHealthAggregator.GetHealthMetrics.
func (a *SkillHealthMetricsAdapter) GetRecentAvgDuration(ctx context.Context, skillID string, days int) (float64, error) {
	metrics, err := a.lookup(ctx, skillID, days)
	if err != nil || metrics == nil {
		return 0, err
	}
	return metrics.AvgDurationMS, nil
}

// lookup returns cached metrics for (skillID, days), querying the aggregator
// on miss. A nil metrics result is a cached "no data" outcome.
func (a *SkillHealthMetricsAdapter) lookup(ctx context.Context, skillID string, days int) (*biz.SkillHealthMetrics, error) {
	key := skillID + "\x1f" + strconv.Itoa(days)
	now := a.now()

	a.mu.Lock()
	if entry, ok := a.cache[key]; ok && now.Before(entry.expiresAt) {
		a.mu.Unlock()
		return entry.metrics, nil
	}
	a.mu.Unlock()

	since := now.UTC().AddDate(0, 0, -days)
	metrics, err := a.agg.GetHealthMetrics(ctx, skillID, since)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	if len(a.cache) >= skillHealthCacheMaxEntries {
		a.purgeExpiredLocked(now)
	}
	if len(a.cache) >= skillHealthCacheMaxEntries {
		// Still full after purge (all entries fresh): reset to keep memory bounded.
		a.cache = make(map[string]skillHealthCacheEntry)
	}
	a.cache[key] = skillHealthCacheEntry{metrics: metrics, expiresAt: now.Add(skillHealthCacheTTL)}
	a.mu.Unlock()
	return metrics, nil
}

func (a *SkillHealthMetricsAdapter) purgeExpiredLocked(now time.Time) {
	for k, entry := range a.cache {
		if !now.Before(entry.expiresAt) {
			delete(a.cache, k)
		}
	}
}

// Compile-time checks that the adapter satisfies both interfaces.
var _ biz.HealthMetricsProvider = (*SkillHealthMetricsAdapter)(nil)
var _ skillrecommend.HealthMetricsProvider = (*SkillHealthMetricsAdapter)(nil)
