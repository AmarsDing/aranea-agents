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
// (provider, model) group over a time window.
type CacheHitRatioStat struct {
	Provider string
	Model    string
	// Samples is the number of turns with prompt_tokens >= MinCacheablePromptTokens.
	Samples   int
	PromptTok int64
	CachedTok int64
	// WeightedRatio = CachedTok / PromptTok (0 when PromptTok == 0). Diagnostic
	// only: a single huge cache-busted turn (e.g. post-compaction history
	// rewrite) dominates it, so alerting must not key on it.
	WeightedRatio float64
	// P50Ratio is the median per-turn cached/prompt ratio. The
	// llm.cache_hit_ratio_low alert keys on it: it reflects the typical turn
	// and is robust to compaction outliers (N7, 2026-08-13 链路审查).
	P50Ratio float64
}

// CacheHitRatioStatsRepo reads aggregated prompt-cache hit ratios from the
// usage event store.
//
// Stability:evolving
type CacheHitRatioStatsRepo interface {
	// CacheHitRatioStats aggregates usage rows within the trailing window by
	// (provider, model). The P50 is computed at this grain directly in SQL —
	// per-agent_key percentiles cannot be merged correctly afterwards.
	// Turns with prompt_tokens below MinCacheablePromptTokens and team_turn
	// reconciliation rows are excluded.
	CacheHitRatioStats(ctx context.Context, window time.Duration) ([]CacheHitRatioStat, error)
}

// CacheHitRatioStats serves the RPC/observability read path. The narrow
// capability is resolved via type assertion (same pattern as ContextBudgetStats)
// so the composite usage.Repo stays untouched; a repo without the capability
// yields empty stats.
func (u *Usecase) CacheHitRatioStats(ctx context.Context, window time.Duration) ([]CacheHitRatioStat, error) {
	repo, ok := u.repo.(CacheHitRatioStatsRepo)
	if !ok {
		return nil, nil
	}
	return repo.CacheHitRatioStats(ctx, window)
}

// RunCacheHitRatio is the per-run prompt-cache efficiency, derived at read
// time from the usage event store (the single authoritative plane for cached
// tokens — team_runs has no cached column by design, 79-runtime-governance
// §2.4 取值源决策 2026-08-25).
type RunCacheHitRatio struct {
	// Found reports whether any usage row exists for the run. False ⇒ the
	// caller must treat the ratio as "no data", not as 0% hit.
	Found     bool
	PromptTok int64
	CachedTok int64
	// Ratio = CachedTok / PromptTok (0 when PromptTok == 0).
	Ratio float64
}

// RunCacheHitRatioRepo reads per-run prompt-cache hit ratios from the usage
// event store.
//
// Stability:evolving
type RunCacheHitRatioRepo interface {
	// RunCacheHitRatio aggregates one run's prompt/cached tokens:
	//  1. the team_turn reconciliation row (success/HITL paths — message_id
	//     equals run id);
	//  2. fallback: SUM of genuine team_member rows (attribution empty) whose
	//     step ids belong to the run — covers failed/cancelled runs that never
	//     wrote a team_turn row (e.g. token-budget trips).
	RunCacheHitRatio(ctx context.Context, runID string) (RunCacheHitRatio, error)
}

// RunCacheHitRatio serves the run-detail read path (79-runtime-governance
// Phase 0 task 0.1). Same narrow-interface resolution as CacheHitRatioStats;
// a repo without the capability yields Found=false.
func (u *Usecase) RunCacheHitRatio(ctx context.Context, runID string) (RunCacheHitRatio, error) {
	repo, ok := u.repo.(RunCacheHitRatioRepo)
	if !ok {
		return RunCacheHitRatio{}, nil
	}
	return repo.RunCacheHitRatio(ctx, runID)
}
