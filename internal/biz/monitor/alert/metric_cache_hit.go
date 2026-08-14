package alert

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/usage"
)

const (
	// DefaultCacheHitRatioLowThreshold is the low-water mark for the median
	// (P50) per-turn prompt-cache hit ratio (design 29-token §9.4): static
	// prefixes usually cover 60-80% of the prompt, so a median ratio below
	// 0.5 means the prefix is being busted or fell outside the cache TTL.
	DefaultCacheHitRatioLowThreshold = 0.5
	// cacheHitRatioLowMinSamples is the minimum sample count a
	// (provider, model) group needs before its hit ratio is alert-eligible.
	cacheHitRatioLowMinSamples = 20
	// cacheHitRatioLowThresholdEnv optionally overrides the default threshold
	// (system_setting has no key-value setting pattern; env is the minimal
	// override channel, same as MONITOR_ALERT_EVAL_INTERVAL).
	cacheHitRatioLowThresholdEnv = "MONITOR_LLM_CACHE_HIT_RATIO_THRESHOLD"
)

// CacheHitRatioBreach describes one (provider, model) group whose median
// per-turn cache hit ratio fell below the low-water threshold.
type CacheHitRatioBreach struct {
	Provider string
	Model    string
	Samples  int
	Ratio    float64
}

// CacheHitRatioLowMetric fires when any (provider, model) group has enough
// samples in the window and its median (P50) per-turn cache hit ratio is
// below the threshold. The metric value is the number of breaching groups
// (count semantics), so the standard alert state machine (fire when value >=
// rule threshold) applies unchanged with rule threshold 1.
type CacheHitRatioLowMetric struct {
	stats     usage.CacheHitRatioStatsRepo
	threshold float64

	// breaches holds the detail of the most recent Evaluate call, consumed
	// by BreachDetails when the alert engine fires the rule.
	mu       sync.Mutex
	breaches []CacheHitRatioBreach
}

func NewCacheHitRatioLowMetric(stats usage.CacheHitRatioStatsRepo) *CacheHitRatioLowMetric {
	return &CacheHitRatioLowMetric{stats: stats, threshold: cacheHitRatioLowThresholdFromEnv()}
}

func cacheHitRatioLowThresholdFromEnv() float64 {
	if raw := strings.TrimSpace(os.Getenv(cacheHitRatioLowThresholdEnv)); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 && v < 1 {
			return v
		}
	}
	return DefaultCacheHitRatioLowThreshold
}

func (m *CacheHitRatioLowMetric) Key() string { return "llm.cache_hit_ratio_low" }
func (m *CacheHitRatioLowMetric) Description() string {
	return "Number of (provider, model) groups with low prompt-cache hit ratio"
}
func (m *CacheHitRatioLowMetric) Catalog() AlertMetricInfo {
	return AlertMetricInfo{
		Key:  m.Key(),
		Name: "LLM cache hit ratio low",
		Description: fmt.Sprintf("Number of (provider, model) groups whose median (P50) per-turn prompt-cache hit ratio fell below %.2f within the window (at least %d samples each). Fires when at least one group breaches.",
			DefaultCacheHitRatioLowThreshold, cacheHitRatioLowMinSamples),
		Unit:                 "count",
		DefaultWindowMinutes: 60,
		SuggestedThreshold:   1,
	}
}

func (m *CacheHitRatioLowMetric) Evaluate(ctx context.Context, window time.Duration) (float64, error) {
	if m == nil || m.stats == nil {
		return 0, nil
	}
	stats, err := m.stats.CacheHitRatioStats(ctx, window)
	if err != nil {
		return 0, err
	}
	breaches := findCacheHitRatioBreaches(stats, m.threshold, cacheHitRatioLowMinSamples)
	m.mu.Lock()
	m.breaches = breaches
	m.mu.Unlock()
	return float64(len(breaches)), nil
}

// BreachDetails implements AlertBreachDetailer: the alert engine merges the
// summary and structured breach list into alert.fired events.
func (m *CacheHitRatioLowMetric) BreachDetails() (string, map[string]any) {
	if m == nil {
		return "", nil
	}
	m.mu.Lock()
	breaches := m.breaches
	m.mu.Unlock()
	if len(breaches) == 0 {
		return "", nil
	}
	worst := breaches[0]
	summary := fmt.Sprintf("%s: %d group(s) below %.2f — worst %s/%s p50 hit ratio %.2f (n=%d)",
		m.Key(), len(breaches), m.threshold, worst.Provider, worst.Model, worst.Ratio, worst.Samples)
	list := make([]map[string]any, 0, len(breaches))
	for _, b := range breaches {
		list = append(list, map[string]any{
			"provider": b.Provider, "model": b.Model, "samples": b.Samples, "hit_ratio": b.Ratio,
		})
	}
	return summary, map[string]any{"breaches": list, "hit_ratio_threshold": m.threshold}
}

// findCacheHitRatioBreaches returns (provider, model) groups whose P50
// per-turn hit ratio is below threshold, sorted by ascending ratio.
//
// N7 (2026-08-13 链路审查): the alert keys on the median per-turn ratio, not
// the token-weighted ratio. Compaction turns rewrite history into a fresh
// prompt that busts the cache; their large token counts dominate the weighted
// ratio and would false-positive whenever compression fires. P50 reflects the
// typical turn and is robust to those outliers. The repo aggregates one row
// per (provider, model) with the P50 computed at that grain in SQL.
func findCacheHitRatioBreaches(stats []usage.CacheHitRatioStat, threshold float64, minSamples int) []CacheHitRatioBreach {
	var out []CacheHitRatioBreach
	for _, s := range stats {
		if s.Samples < minSamples {
			continue
		}
		if s.P50Ratio < threshold {
			out = append(out, CacheHitRatioBreach{Provider: s.Provider, Model: s.Model, Samples: s.Samples, Ratio: s.P50Ratio})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ratio != out[j].Ratio {
			return out[i].Ratio < out[j].Ratio
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}
