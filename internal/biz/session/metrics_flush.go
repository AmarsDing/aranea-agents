package session

import "context"

// AccumulateMetricsDelta delegates to SessionMetricsUsecase (Facade pattern).
// Graceful degradation: if metricsUsecase is nil (e.g. misconfigured DI), the delta is silently discarded.
func (uc *SessionUsecase) AccumulateMetricsDelta(delta SessionMetricsDelta) {
	if uc.metricsUsecase == nil {
		return
	}
	uc.metricsUsecase.AccumulateMetricsDelta(delta)
}

// StartMetricsFlusher delegates to SessionMetricsUsecase (Facade pattern).
// Graceful degradation: if metricsUsecase is nil, no flusher is started.
func (uc *SessionUsecase) StartMetricsFlusher(ctx context.Context) {
	if uc.metricsUsecase == nil {
		return
	}
	uc.metricsUsecase.StartMetricsFlusher(ctx)
}

// flushAllMetrics delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) flushAllMetrics(ctx context.Context) {
	if uc.metricsUsecase == nil {
		return
	}
	uc.metricsUsecase.flushAllMetrics(ctx)
}

// forceFlushSingle delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) forceFlushSingle(sessionID string) {
	if uc.metricsUsecase == nil {
		return
	}
	uc.metricsUsecase.forceFlushSingle(sessionID)
}
