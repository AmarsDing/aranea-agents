package data

import (
	"context"
	"strings"
	"time"

	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/apierror"
)

var _ bizusage.CacheHitRatioStatsRepo = (*usageRepo)(nil)

// CacheHitRatioStats aggregates model_token_usage_events by
// (provider, model) over the trailing window. The P50 must be computed at
// this grain in SQL: percentile_cont values from finer (per-agent_key) groups
// cannot be merged into a correct (provider, model) median in Go.
// PG-only: percentile_cont has no SQLite equivalent; the sole caller is the
// monitor alert engine (production Postgres), CLI SQLite tools never invoke it.
func (r *usageRepo) CacheHitRatioStats(ctx context.Context, window time.Duration) ([]bizusage.CacheHitRatioStat, error) {
	if window <= 0 {
		window = time.Hour
	}
	since := time.Now().UTC().Add(-window).Format(time.RFC3339)
	q := r.data.Dialect().RenumberPlaceholders(`SELECT provider_code, model_api_id,
	 COUNT(*),
	 COALESCE(SUM(input_tokens), 0),
	 COALESCE(SUM(cached_input_tokens), 0),
	 COALESCE(1.0 * SUM(cached_input_tokens) / NULLIF(SUM(input_tokens), 0), 0),
	 COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY 1.0 * cached_input_tokens / input_tokens), 0)
	 FROM model_token_usage_events
	 WHERE occurred_at >= ? AND input_tokens >= ? AND ` + sqlUsageBillableKind + `
	 GROUP BY provider_code, model_api_id
	 ORDER BY provider_code, model_api_id`)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, since, bizusage.MinCacheablePromptTokens)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainData)
	}
	defer rows.Close()
	var out []bizusage.CacheHitRatioStat
	for rows.Next() {
		var s bizusage.CacheHitRatioStat
		if err = rows.Scan(&s.Provider, &s.Model, &s.Samples, &s.PromptTok, &s.CachedTok, &s.WeightedRatio, &s.P50Ratio); err != nil {
			return nil, entErrToBizErr(err, apierror.DomainData)
		}
		out = append(out, s)
	}
	return out, entErrToBizErr(rows.Err(), apierror.DomainData)
}

var _ bizusage.RunCacheHitRatioRepo = (*usageRepo)(nil)

// scanUsageSingleRow runs q via QueryContext and scans at most one row into
// dest. Reports whether a row was present. Execer has no QueryRowContext, so
// single-row reads go through this helper.
func scanUsageSingleRow(ctx context.Context, db Execer, q string, args []any, dest ...any) (bool, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	found := false
	if rows.Next() {
		found = true
		err = rows.Scan(dest...)
	}
	closeErr := rows.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		return false, err
	}
	return found, nil
}

// RunCacheHitRatio derives one team run's prompt/cached tokens from the usage
// event store (79-runtime-governance Phase 0 task 0.1，§2.4 取值源决策：事件面
// 是 cached tokens 的唯一权威，team_runs 不落 cached 列——避免同一事实双份
// 持久化的同步发散）。
//
// 双分支：
//  1. team_turn 对账行（成功/HITL 路径，message_id = run id）——run 总账单行，
//     LIMIT 1 防御性去重（写入路径保证单行，重复行内容恒等）；
//  2. 回退：genuine team_member 行（usage_attribution 为空）按 step 归属求和——
//     覆盖预算熔断/失败 run（无 team_turn 行）。attribution 非空的镜像行
//     （member_level_stream / run_level_anchor_fallback / stream_anchor_remainder）
//     与 team_turn 总账同额，必须排除以防双计（P2-1 语义）。
func (r *usageRepo) RunCacheHitRatio(ctx context.Context, runID string) (bizusage.RunCacheHitRatio, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return bizusage.RunCacheHitRatio{}, nil
	}
	db := r.data.RWDB().ReadDB(ctx)

	var out bizusage.RunCacheHitRatio
	found, err := scanUsageSingleRow(ctx, db, r.data.Dialect().RenumberPlaceholders(
		`SELECT input_tokens, cached_input_tokens
		   FROM model_token_usage_events
		  WHERE usage_kind = 'team_turn' AND message_id = ?
		  LIMIT 1`), []any{runID}, &out.PromptTok, &out.CachedTok)
	if err != nil {
		return bizusage.RunCacheHitRatio{}, entErrToBizErr(err, apierror.DomainData)
	}
	if found {
		out.Found = true
	} else {
		// 失败/取消 run 无 team_turn 行 → 回退 genuine 成员行求和。
		attribution := r.data.Dialect().JSONExtract("metadata_json", "usage_attribution")
		var n int64
		_, err = scanUsageSingleRow(ctx, db, r.data.Dialect().RenumberPlaceholders(
			`SELECT COALESCE(SUM(u.input_tokens), 0), COALESCE(SUM(u.cached_input_tokens), 0), COUNT(*)
			   FROM model_token_usage_events u
			  WHERE u.usage_kind = 'team_member'
			    AND COALESCE(`+attribution+`, '') = ''
			    AND u.message_id IN (SELECT id FROM team_run_steps WHERE run_id = ?)`), []any{runID},
			&out.PromptTok, &out.CachedTok, &n)
		if err != nil {
			return bizusage.RunCacheHitRatio{}, entErrToBizErr(err, apierror.DomainData)
		}
		out.Found = n > 0
	}
	if out.PromptTok > 0 {
		out.Ratio = float64(out.CachedTok) / float64(out.PromptTok)
	}
	return out, nil
}
