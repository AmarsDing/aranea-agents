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

	// INSERT with zero values; ON CONFLICT adds the delta.
	// This ensures first-time rows start at 0 and deltas accumulate correctly.
	builder := c.SessionMetrics.Create().
		SetID(sessionID).
		SetMessageCount(0).
		SetRunCount(0).
		SetModelCallCount(0).
		SetToolCallCount(0).
		SetSkillCallCount(0).
		SetMcpCallCount(0).
		SetInputTokens(0).
		SetOutputTokens(0).
		SetTotalTokens(0).
		SetTotalCostMicroUsd(0).
		SetAvgLatencyMs(0).
		SetErrorCount(0).
		SetContextUsedTokens(0).
		SetContextUsedRatio(0).
		SetMaxContextUsedRatio(0).
		SetContextStatus("").
		SetLastMessageAt("").
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
	}
	return entErrToBizErr(err, "SESSION_METRICS")
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
