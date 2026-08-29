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
				uc.lg.Error("session_metrics.flush_failed", loggateway.Err(err), loggateway.Str("session_id", d.SessionID))
			}
			uc.AccumulateMetricsDelta(*d)
		} else if uc.metricsUpdatedPublisher != nil {
			uc.metricsUpdatedPublisher.PublishMetricsUpdated(d.SessionID)
		}
	}
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
			uc.lg.Error("session_metrics.force_flush_failed", loggateway.Err(err), loggateway.Str("session_id", d.SessionID))
		}
		uc.AccumulateMetricsDelta(*d)
	} else if uc.metricsUpdatedPublisher != nil {
		uc.metricsUpdatedPublisher.PublishMetricsUpdated(d.SessionID)
	}
}

// SessionMetricsProviderSet provides Wire bindings for SessionMetricsUsecase.
var SessionMetricsProviderSet = wire.NewSet(NewSessionMetricsUsecase)
