package session

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/wire"
)

// SessionMetricsUsecase handles metrics refresh + delta aggregation logic,
// extracted from SessionUsecase to reduce God Object scope.
type SessionMetricsUsecase struct {
	contextUpdater          ContextUpdater
	metricsUpdatedPublisher MetricsUpdatedPublisher
	lg                      loggateway.Logger

	metricsDeltaMu sync.Mutex
	metricsDeltas  map[string]*SessionMetricsDelta
	flushInterval  time.Duration
}

// NewSessionMetricsUsecase creates a new SessionMetricsUsecase.
func NewSessionMetricsUsecase(contextUpdater ContextUpdater, lg loggateway.Logger, metricsUpdatedPublisher MetricsUpdatedPublisher) *SessionMetricsUsecase {
	return &SessionMetricsUsecase{
		contextUpdater:          contextUpdater,
		metricsUpdatedPublisher: metricsUpdatedPublisher,
		lg:                      lg,
		metricsDeltas:           make(map[string]*SessionMetricsDelta),
		flushInterval:           200 * time.Millisecond,
	}
}

// AccumulateMetricsDelta accumulates a metrics delta for batched flush.
func (uc *SessionMetricsUsecase) AccumulateMetricsDelta(delta SessionMetricsDelta) {
	uc.metricsDeltaMu.Lock()
	defer uc.metricsDeltaMu.Unlock()
	if existing, ok := uc.metricsDeltas[delta.SessionID]; ok {
		existing.MessageCount += delta.MessageCount
		existing.RunCount += delta.RunCount
		existing.ModelCallCount += delta.ModelCallCount
		existing.ToolCallCount += delta.ToolCallCount
		existing.SkillCallCount += delta.SkillCallCount
		existing.McpCallCount += delta.McpCallCount
		existing.InputTokens += delta.InputTokens
		existing.OutputTokens += delta.OutputTokens
		existing.TotalTokens += delta.TotalTokens
		existing.TotalCostMicroUsd += delta.TotalCostMicroUsd
		if delta.LastMessageAt != "" && delta.LastMessageAt > existing.LastMessageAt {
			existing.LastMessageAt = delta.LastMessageAt
		}
		// SP-1c：失败回炉的 delta 与窗口内新 delta 合并时，失败计数取 max
		// 保守保留——旧批次已失败 N 次的事实不因并入新数据而清零。
		if delta.FlushFailCount > existing.FlushFailCount {
			existing.FlushFailCount = delta.FlushFailCount
		}
		existing.AccumulatedCount++
		if existing.AccumulatedCount >= MaxDeltaCount || time.Since(existing.FirstAccumulatedAt) > MaxDeltaAge {
			safego.Go(appctx.Ctx(), "metrics-delta-force-flush", func() {
				uc.forceFlushSingle(delta.SessionID)
			})
		}
	} else {
		cp := delta
		cp.FirstAccumulatedAt = time.Now()
		cp.AccumulatedCount = 1
		uc.metricsDeltas[delta.SessionID] = &cp
	}
}

// StartMetricsFlusher starts the periodic metrics flush goroutine.
func (uc *SessionMetricsUsecase) StartMetricsFlusher(ctx context.Context) {
	safego.Go(ctx, "session-metrics-flusher", func() {
		ticker := time.NewTicker(uc.flushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				uc.flushAllMetrics(context.Background())
				return
			case <-ticker.C:
				uc.flushAllMetrics(ctx)
			}
		}
	})
}

func (uc *SessionMetricsUsecase) flushAllMetrics(ctx context.Context) {
	uc.metricsDeltaMu.Lock()
	deltas := uc.metricsDeltas
	uc.metricsDeltas = make(map[string]*SessionMetricsDelta)
	uc.metricsDeltaMu.Unlock()

	for _, d := range deltas {
		if err := uc.contextUpdater.ApplyMetricsDelta(ctx, d); err != nil {
			if uc.lg != nil {
				uc.lg.Error("session_metrics.flush_failed", loggateway.Err(err), loggateway.Str("session_id", d.SessionID),
					loggateway.Int("flush_fail_count", d.FlushFailCount+1))
			}
			uc.reaccumulateAfterFail(d)
		} else if uc.metricsUpdatedPublisher != nil {
			uc.metricsUpdatedPublisher.PublishMetricsUpdated(d.SessionID)
		}
	}
}

// reaccumulateAfterFail 把 flush 失败的 delta 回炉重试；失败次数达到
// MaxFlushFailCount 上限时丢弃并升级告警（SP-1c：防无限重试循环，指标
// 丢失必须带统计量显式告警，而非静默重试到进程退出）。
func (uc *SessionMetricsUsecase) reaccumulateAfterFail(d *SessionMetricsDelta) {
	d.FlushFailCount++
	if d.FlushFailCount >= MaxFlushFailCount {
		if uc.lg != nil {
			uc.lg.Error("session_metrics.flush_dropped",
				loggateway.Str("session_id", d.SessionID),
				loggateway.Int("flush_fail_count", d.FlushFailCount),
				loggateway.Int("message_count", d.MessageCount),
				loggateway.Int("run_count", d.RunCount),
				loggateway.Int64("total_tokens", d.TotalTokens),
			)
		}
		return
	}
	uc.AccumulateMetricsDelta(*d)
}

func (uc *SessionMetricsUsecase) forceFlushSingle(sessionID string) {
	uc.metricsDeltaMu.Lock()
	d, ok := uc.metricsDeltas[sessionID]
	if !ok {
		uc.metricsDeltaMu.Unlock()
		return
	}
	delete(uc.metricsDeltas, sessionID)
	uc.metricsDeltaMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := uc.contextUpdater.ApplyMetricsDelta(ctx, d); err != nil {
		if uc.lg != nil {
			uc.lg.Error("session_metrics.force_flush_failed", loggateway.Err(err), loggateway.Str("session_id", d.SessionID),
				loggateway.Int("flush_fail_count", d.FlushFailCount+1))
		}
		uc.reaccumulateAfterFail(d)
	} else if uc.metricsUpdatedPublisher != nil {
		uc.metricsUpdatedPublisher.PublishMetricsUpdated(d.SessionID)
	}
}

// SessionMetricsProviderSet provides Wire bindings for SessionMetricsUsecase.
var SessionMetricsProviderSet = wire.NewSet(NewSessionMetricsUsecase)
