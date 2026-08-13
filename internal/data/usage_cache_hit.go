package data

import (
	"context"
	"time"

	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/apierror"
)

var _ bizusage.CacheHitRatioStatsRepo = (*usageRepo)(nil)

// CacheHitRatioStats aggregates model_token_usage_events by
// (provider, model, agent_key) over the trailing window.
// PG-only: percentile_cont has no SQLite equivalent; the sole caller is the
// monitor alert engine (production Postgres), CLI SQLite tools never invoke it.
func (r *usageRepo) CacheHitRatioStats(ctx context.Context, window time.Duration) ([]bizusage.CacheHitRatioStat, error) {
	if window <= 0 {
		window = time.Hour
	}
	since := time.Now().UTC().Add(-window).Format(time.RFC3339)
	q := r.data.Dialect().RenumberPlaceholders(`SELECT provider_code, model_api_id, agent_key,
	 COUNT(*),
	 COALESCE(SUM(input_tokens), 0),
	 COALESCE(SUM(cached_input_tokens), 0),
	 COALESCE(1.0 * SUM(cached_input_tokens) / NULLIF(SUM(input_tokens), 0), 0),
	 COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY 1.0 * cached_input_tokens / input_tokens), 0)
	 FROM model_token_usage_events
	 WHERE occurred_at >= ? AND input_tokens >= ? AND ` + sqlUsageBillableKind + `
	 GROUP BY provider_code, model_api_id, agent_key
	 ORDER BY provider_code, model_api_id, agent_key`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, since, bizusage.MinCacheablePromptTokens)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var out []bizusage.CacheHitRatioStat
	for rows.Next() {
		var s bizusage.CacheHitRatioStat
		if err = rows.Scan(&s.Provider, &s.Model, &s.AgentKey, &s.Samples, &s.PromptTok, &s.CachedTok, &s.WeightedRatio, &s.P50Ratio); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		out = append(out, s)
	}
	return out, entErrToBizErr(rows.Err(), apierror.DomainData)
}
