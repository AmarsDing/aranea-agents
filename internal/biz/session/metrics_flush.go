package session

import "context"

// AccumulateMetricsDelta delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) AccumulateMetricsDelta(delta SessionMetricsDelta) {
	if uc.metricsUsecase == nil {
		panic("session: AccumulateMetricsDelta called with nil metricsUsecase — this is a programming error")
	}
	uc.metricsUsecase.AccumulateMetricsDelta(delta)
}

// StartMetricsFlusher delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) StartMetricsFlusher(ctx context.Context) {
	if uc.metricsUsecase == nil {
		panic("session: StartMetricsFlusher called with nil metricsUsecase — this is a programming error")
	}
	uc.metricsUsecase.StartMetricsFlusher(ctx)
}

// flushAllMetrics delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) flushAllMetrics(ctx context.Context) {
	if uc.metricsUsecase == nil {
		panic("session: flushAllMetrics called with nil metricsUsecase — this is a programming error")
	}
	uc.metricsUsecase.flushAllMetrics(ctx)
}

// forceFlushSingle delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) forceFlushSingle(sessionID string) {
	if uc.metricsUsecase == nil {
		panic("session: forceFlushSingle called with nil metricsUsecase — this is a programming error")
	}
	uc.metricsUsecase.forceFlushSingle(sessionID)
}
