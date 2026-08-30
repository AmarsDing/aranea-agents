package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/sessionmetrics"
	"aranea-agents/pkg/loggateway"
)

type sessionMetricsRepo struct {
	data *Data
}

var (
	_ biz.SessionMetricsReader = (*sessionMetricsRepo)(nil)
	_ biz.SessionMetricsWriter = (*sessionMetricsRepo)(nil)
)

func NewSessionMetricsRepo(data *Data) biz.SessionMetricsWriter {
	return &sessionMetricsRepo{data: data}
}

// NewSessionMetricsReader provides a SessionMetricsReader from the same sessionMetricsRepo.
// The returned value also implements SessionMetricsWriter.
func NewSessionMetricsReader(data *Data) biz.SessionMetricsReader {
	return &sessionMetricsRepo{data: data}
}

func entSessionMetricsToBiz(e *ent.SessionMetrics) *biz.SessionMetrics {
	if e == nil {
		return nil
	}
	return &biz.SessionMetrics{
		SessionID:           e.ID,
		MessageCount:        e.MessageCount,
		RunCount:            e.RunCount,
		ModelCallCount:      e.ModelCallCount,
		ToolCallCount:       e.ToolCallCount,
		SkillCallCount:      e.SkillCallCount,
		MCPCallCount:        e.McpCallCount,
		InputTokens:         e.InputTokens,
		OutputTokens:        e.OutputTokens,
		TotalTokens:         e.TotalTokens,
		TotalCostMicroUSD:   e.TotalCostMicroUsd,
		AvgLatencyMs:        e.AvgLatencyMs,
		ErrorCount:          e.ErrorCount,
		ContextUsedTokens:   e.ContextUsedTokens,
		ContextUsedRatio:    e.ContextUsedRatio,
		MaxContextUsedRatio: e.MaxContextUsedRatio,
		ContextStatus:       e.ContextStatus,
		LastMessageAt:       e.LastMessageAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

func (r *sessionMetricsRepo) GetSessionMetrics(ctx context.Context, sessionID string) (*biz.SessionMetrics, error) {
	c := r.data.RW().Read(ctx)
	row, err := c.SessionMetrics.Get(ctx, sessionID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, entErrToBizErr(err, "SESSION_METRICS")
	}
	return entSessionMetricsToBiz(row), nil
}

func (r *sessionMetricsRepo) ListSessionMetricsByIDs(ctx context.Context, ids []string) (map[string]*biz.SessionMetrics, error) {
	if len(ids) == 0 {
		return map[string]*biz.SessionMetrics{}, nil
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.SessionMetrics.Query().
		Where(sessionmetrics.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "SESSION_METRICS")
	}
	result := make(map[string]*biz.SessionMetrics, len(rows))
	for _, row := range rows {
		result[row.ID] = entSessionMetricsToBiz(row)
	}
	return result, nil
}

func (r *sessionMetricsRepo) UpsertSessionMetrics(ctx context.Context, sessionID string, delta *session.SessionMetricsDelta) error {
	if delta == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	c := r.data.RW().Write(ctx)
	now := nowRFC3339()

	// INSERT 直接应用 delta 值（R4-Q10 修复：此前 INSERT 写全零、delta 仅
	// 在 ON CONFLICT 分支生效，导致每个 session 首个 delta 批次整体丢失——
	// S01 实测 2 轮只记 1）。ON CONFLICT 分支对已存在行做增量累加。
	builder := c.SessionMetrics.Create().
		SetID(sessionID).
		SetMessageCount(delta.MessageCount).
		SetRunCount(delta.RunCount).
		SetModelCallCount(delta.ModelCallCount).
		SetToolCallCount(delta.ToolCallCount).
		SetSkillCallCount(delta.SkillCallCount).
		SetMcpCallCount(delta.McpCallCount).
		SetInputTokens(int(delta.InputTokens)).
		SetOutputTokens(int(delta.OutputTokens)).
		SetTotalTokens(int(delta.TotalTokens)).
		SetTotalCostMicroUsd(delta.TotalCostMicroUsd).
		SetAvgLatencyMs(0).
		SetErrorCount(0).
		SetContextUsedTokens(delta.ContextUsedTokens).
		SetContextUsedRatio(delta.ContextUsedRatio).
		SetMaxContextUsedRatio(delta.MaxContextUsedRatio).
		SetContextStatus("").
		SetLastMessageAt(delta.LastMessageAt).
		SetUpdatedAt(now)

	err := builder.
		OnConflictColumns(sessionmetrics.FieldID).
		Update(func(u *ent.SessionMetricsUpsert) {
			if delta.MessageCount != 0 {
				u.AddMessageCount(delta.MessageCount)
			}
			if delta.RunCount != 0 {
				u.AddRunCount(delta.RunCount)
			}
			if delta.ModelCallCount != 0 {
				u.AddModelCallCount(delta.ModelCallCount)
			}
			if delta.ToolCallCount != 0 {
				u.AddToolCallCount(delta.ToolCallCount)
			}
			if delta.SkillCallCount != 0 {
				u.AddSkillCallCount(delta.SkillCallCount)
			}
			if delta.McpCallCount != 0 {
				u.AddMcpCallCount(delta.McpCallCount)
			}
			if delta.InputTokens != 0 {
				u.AddInputTokens(int(delta.InputTokens))
			}
			if delta.OutputTokens != 0 {
				u.AddOutputTokens(int(delta.OutputTokens))
			}
			if delta.TotalTokens != 0 {
				u.AddTotalTokens(int(delta.TotalTokens))
			}
			if delta.TotalCostMicroUsd != 0 {
				u.AddTotalCostMicroUsd(delta.TotalCostMicroUsd)
			}
			if delta.ErrorCount != 0 {
				u.AddErrorCount(delta.ErrorCount)
			}
			if delta.LastMessageAt != "" {
				u.SetLastMessageAt(delta.LastMessageAt)
			}
			if delta.ContextUsedTokens != 0 {
				u.SetContextUsedTokens(delta.ContextUsedTokens)
			}
			if delta.ContextUsedRatio != 0 {
				u.SetContextUsedRatio(delta.ContextUsedRatio)
			}
			if delta.MaxContextUsedRatio != 0 {
				u.SetMaxContextUsedRatio(delta.MaxContextUsedRatio)
			}
			u.SetUpdatedAt(now)
		}).
		Exec(ctx)
	if err != nil {
		r.data.lg.Warn("upsert session metrics failed", loggateway.StepID("data.session_metrics.upsert"), loggateway.Err(err))
		return entErrToBizErr(err, "SESSION_METRICS")
	}
	// R4-Q10：avg_latency_ms 滚动平均。样本基数 = run_count（每个 run 记账时
	// 同批携带墙钟耗时之和 LatencySumMs，未观测计 0）。ent upsert 不支持
	// 跨列表达式，故在 upsert 后用单条 raw UPDATE 折叠：式中 run_count 已是
	// 本批次累加后的新值，旧样本数 = run_count - delta.RunCount。
	if delta.RunCount > 0 {
		if _, uerr := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			r.data.Dialect().RenumberPlaceholders(`
				UPDATE session_metrics
				SET avg_latency_ms = (avg_latency_ms * (run_count - ?) + ?) / run_count,
				    updated_at = ?
				WHERE session_id = ? AND run_count >= ?`),
			delta.RunCount, delta.LatencySumMs, nowRFC3339(), sessionID, delta.RunCount); uerr != nil {
			r.data.lg.Warn("update avg latency failed", loggateway.StepID("data.session_metrics.avg_latency"), loggateway.Err(uerr))
		}
	}
	return nil
}

func (r *sessionMetricsRepo) ApplyMetricsDelta(ctx context.Context, d *session.SessionMetricsDelta) error {
	if d == nil {
		return nil
	}
	sessionID := strings.TrimSpace(d.SessionID)
	if sessionID == "" {
		return nil
	}
	// Delegate to UpsertSessionMetrics which handles both INSERT and UPDATE
	// via ON CONFLICT, avoiding NotFound errors when the row doesn't exist yet.
	return r.UpsertSessionMetrics(ctx, sessionID, d)
}
