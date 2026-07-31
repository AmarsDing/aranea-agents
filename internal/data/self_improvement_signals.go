package data

import (
	"context"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/evalrun"
	"aranea-agents/pkg/apierror"
)

// ── Platform self-improvement signal adapters (73-self-iteration-v3, T1.9) ──
//
// SelfImprovementSignalRepo implements the three DB-backed biz signal ports:
//   - biz.ErrorClusterReader — model_token_usage_events 失败事件按 error_code 聚类
//   - biz.PerfMetricsReader  — 同表 latency/total_tokens 基线（7d）vs 当前（24h）
//   - biz.EvalBaselineReader — eval_runs 最近两次同 ds+agent completed 基线
//
// model_token_usage_events 为 raw-DDL 表（20260717 迁移，非 Ent Schema），
// P95 用 Postgres percentile_cont（生产唯一驱动，DB-R1/R6 合规）。

// SelfImprovementSignalRepo adapts usage/eval tables to biz signal ports.
type SelfImprovementSignalRepo struct {
	data *Data
}

var _ biz.ErrorClusterReader = (*SelfImprovementSignalRepo)(nil)
var _ biz.PerfMetricsReader = (*SelfImprovementSignalRepo)(nil)
var _ biz.EvalBaselineReader = (*SelfImprovementSignalRepo)(nil)
var _ biz.SIMetricsReader = (*SelfImprovementSignalRepo)(nil)

// NewSelfImprovementSignalRepo creates the adapter. Logger is unnecessary:
// reads are passive queries; errors are returned to the caller (trigger).
func NewSelfImprovementSignalRepo(d *Data) *SelfImprovementSignalRepo {
	return &SelfImprovementSignalRepo{data: d}
}

// ListErrorClusters implements biz.ErrorClusterReader.
func (r *SelfImprovementSignalRepo) ListErrorClusters(ctx context.Context, since time.Time, minCount int) ([]biz.ErrorCluster, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	if minCount <= 0 {
		minCount = 1
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT error_code, COUNT(*) AS cnt,
			MAX(occurred_at) AS last_seen,
			(array_agg(error_message ORDER BY occurred_at DESC))[1] AS sample_message,
			(array_agg(agent_key ORDER BY occurred_at DESC))[1] AS component
		FROM model_token_usage_events
		WHERE status = 'failed' AND error_code <> '' AND occurred_at >= ?
		GROUP BY error_code
		HAVING COUNT(*) >= ?
		ORDER BY cnt DESC
		LIMIT 100`),
		since.UTC().Format(time.RFC3339Nano), minCount,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	defer rows.Close()

	var out []biz.ErrorCluster
	for rows.Next() {
		var c biz.ErrorCluster
		var lastSeen string
		if err := rows.Scan(&c.ErrorCode, &c.Count, &lastSeen, &c.SampleMessage, &c.Component); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		c.LastSeen = parseSITime(lastSeen)
		out = append(out, c)
	}
	return out, entErrToBizErr(rows.Err(), "SELF_IMPROVE")
}

// GetStepLatencyStats implements biz.PerfMetricsReader. Windows are disjoint:
// current = [now-cur, now), baseline = [now-base, now-cur).
func (r *SelfImprovementSignalRepo) GetStepLatencyStats(ctx context.Context, baselineWindow, currentWindow string) ([]biz.StepLatencyStat, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	baseDur, curDur := parseSIWindow(baselineWindow), parseSIWindow(currentWindow)
	if baseDur <= 0 || curDur <= 0 {
		return nil, apierror.BadRequest("SELF_IMPROVE", "invalid window: "+baselineWindow+"/"+currentWindow)
	}
	now := time.Now().UTC()
	curStart := now.Add(-curDur).Format(time.RFC3339Nano)
	baseStart := now.Add(-baseDur).Format(time.RFC3339Nano)

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`WITH cur AS (
			SELECT agent_key, percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) AS p95, COUNT(*) AS n
			FROM model_token_usage_events
			WHERE occurred_at >= ? AND agent_key <> ''
			GROUP BY agent_key
		), base AS (
			SELECT agent_key, percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) AS p95
			FROM model_token_usage_events
			WHERE occurred_at >= ? AND occurred_at < ? AND agent_key <> ''
			GROUP BY agent_key
		)
		SELECT c.agent_key, COALESCE(b.p95, 0), c.p95, c.n
		FROM cur c LEFT JOIN base b ON b.agent_key = c.agent_key`),
		curStart, baseStart, curStart,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	defer rows.Close()

	var out []biz.StepLatencyStat
	for rows.Next() {
		var s biz.StepLatencyStat
		if err := rows.Scan(&s.StepID, &s.BaselineP95MS, &s.CurrentP95MS, &s.SampleCount); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		out = append(out, s)
	}
	return out, entErrToBizErr(rows.Err(), "SELF_IMPROVE")
}

// GetTokenUsageStats implements biz.PerfMetricsReader (same window split as
// latency; compares per-agent average total_tokens).
func (r *SelfImprovementSignalRepo) GetTokenUsageStats(ctx context.Context, baselineWindow, currentWindow string) ([]biz.TokenAnomaly, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	baseDur, curDur := parseSIWindow(baselineWindow), parseSIWindow(currentWindow)
	if baseDur <= 0 || curDur <= 0 {
		return nil, apierror.BadRequest("SELF_IMPROVE", "invalid window: "+baselineWindow+"/"+currentWindow)
	}
	now := time.Now().UTC()
	curStart := now.Add(-curDur).Format(time.RFC3339Nano)
	baseStart := now.Add(-baseDur).Format(time.RFC3339Nano)

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`WITH cur AS (
			SELECT agent_key, AVG(total_tokens) AS avg_tokens, COUNT(*) AS n
			FROM model_token_usage_events
			WHERE occurred_at >= ? AND agent_key <> ''
			GROUP BY agent_key
		), base AS (
			SELECT agent_key, AVG(total_tokens) AS avg_tokens
			FROM model_token_usage_events
			WHERE occurred_at >= ? AND occurred_at < ? AND agent_key <> ''
			GROUP BY agent_key
		)
		SELECT c.agent_key, COALESCE(b.avg_tokens, 0), c.avg_tokens, c.n
		FROM cur c LEFT JOIN base b ON b.agent_key = c.agent_key`),
		curStart, baseStart, curStart,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	defer rows.Close()

	var out []biz.TokenAnomaly
	for rows.Next() {
		var a biz.TokenAnomaly
		if err := rows.Scan(&a.Scope, &a.BaselineTokens, &a.CurrentTokens, &a.SampleCount); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
		out = append(out, a)
	}
	return out, entErrToBizErr(rows.Err(), "SELF_IMPROVE")
}

// GetLatestBaseline implements biz.EvalBaselineReader: the newest completed
// eval run (any dataset/agent).
func (r *SelfImprovementSignalRepo) GetLatestBaseline(ctx context.Context) (*biz.EvalBaseline, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	runs, err := r.data.RW().Read(ctx).EvalRun.Query().
		Where(evalrun.StatusEQ("completed")).
		Order(ent.Desc(evalrun.FieldCreatedAt)).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return entEvalRunToBaseline(runs[0]), nil
}

// GetPreviousBaseline implements biz.EvalBaselineReader: the newest completed
// run sharing dataset+agent with the latest one, strictly older than it.
// Nil when the latest run's suite has no prior completed run.
func (r *SelfImprovementSignalRepo) GetPreviousBaseline(ctx context.Context) (*biz.EvalBaseline, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	latest, err := r.GetLatestBaseline(ctx)
	if err != nil || latest == nil {
		return nil, err
	}
	runs, err := r.data.RW().Read(ctx).EvalRun.Query().
		Where(
			evalrun.StatusEQ("completed"),
			evalrun.DatasetIDEQ(latest.DatasetID),
			evalrun.AgentIDEQ(latest.AgentID),
			evalrun.IDNEQ(latest.RunID),
		).
		Order(ent.Desc(evalrun.FieldCreatedAt)).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return entEvalRunToBaseline(runs[0]), nil
}

// entEvalRunToBaseline averages the four component scores into one composite.
func entEvalRunToBaseline(r *ent.EvalRun) *biz.EvalBaseline {
	return &biz.EvalBaseline{
		RunID:     r.ID,
		DatasetID: r.DatasetID,
		AgentID:   r.AgentID,
		Score:     float64(r.ExactMatchScore+r.ContainsMatchScore+r.LlmJudgeScore+r.ToolCallAccuracy) / 4,
		CreatedAt: parseSITime(r.CreatedAt),
	}
}

// Snapshot implements biz.SIMetricsReader: aggregates error rate (failed /
// total) and p95 latency over [now-window, now) from
// model_token_usage_events. AlertCount stays 0 in P4 — fired alerts have no
// persistent table yet (deviation recorded in development.md); the rollback
// decision uses error_rate / p95 only (design D7).
func (r *SelfImprovementSignalRepo) Snapshot(ctx context.Context, window time.Duration) (*biz.MetricsSnapshot, error) {
	if r == nil || r.data == nil {
		return nil, apierror.Internal("SELF_IMPROVE", "database not configured")
	}
	if window <= 0 {
		window = time.Hour
	}
	since := time.Now().UTC().Add(-window).Format(time.RFC3339Nano)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT
			COALESCE(AVG(CASE WHEN status = 'failed' THEN 1.0 ELSE 0.0 END), 0) AS error_rate,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) AS p95
		FROM model_token_usage_events
		WHERE occurred_at >= ?`),
		since,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "SELF_IMPROVE")
	}
	defer rows.Close()
	snap := &biz.MetricsSnapshot{}
	if rows.Next() {
		if err := rows.Scan(&snap.ErrorRate, &snap.P95MS); err != nil {
			return nil, entErrToBizErr(err, "SELF_IMPROVE")
		}
	}
	return snap, entErrToBizErr(rows.Err(), "SELF_IMPROVE")
}

// parseSIWindow parses "7d" / "24h" style windows. Unknown/zero → 0.
func parseSIWindow(s string) time.Duration {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return 0
}

// parseSITime parses RFC3339(Nano) text timestamps; unparseable → zero time.
func parseSITime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
