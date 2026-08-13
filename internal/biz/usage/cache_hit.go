package usage

import (
	"context"
	"time"
)

// MinCacheablePromptTokens is the minimum prompt size at which providers
// enable prompt caching. Smaller prompts can never hit the cache, so they
// are excluded from hit-ratio aggregation to avoid false low-water alerts.
const MinCacheablePromptTokens = 1024

// CacheHitRatioStat aggregates prompt-cache efficiency for one
// (provider, model, agent_key) group over a time window.
type CacheHitRatioStat struct {
	Provider string
	Model    string
	AgentKey string
	// Samples is the number of turns with prompt_tokens >= MinCacheablePromptTokens.
	Samples   int
	PromptTok int64
	CachedTok int64
	// WeightedRatio = CachedTok / PromptTok (0 when PromptTok == 0).
	WeightedRatio float64
	// P50Ratio is the median per-turn cached/prompt ratio.
	P50Ratio float64
}

// CacheHitRatioStatsRepo reads aggregated prompt-cache hit ratios from the
// usage event store.
//
// Stability:evolving
type CacheHitRatioStatsRepo interface {
	// CacheHitRatioStats aggregates usage rows within the trailing window by
	// (provider, model, agent_key). Turns with prompt_tokens below
	// MinCacheablePromptTokens and team_turn reconciliation rows are excluded.
	CacheHitRatioStats(ctx context.Context, window time.Duration) ([]CacheHitRatioStat, error)
}
