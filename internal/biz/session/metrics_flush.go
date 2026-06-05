package session

import "context"

// AccumulateMetricsDelta delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) AccumulateMetricsDelta(delta SessionMetricsDelta) {
	if uc.metricsUsecase != nil {
		uc.metricsUsecase.AccumulateMetricsDelta(delta)
		return
	}
	// Fallback: legacy inline logic (should not happen in production).
	uc.metricsDeltaMu.Lock()
	defer uc.metricsDeltaMu.Unlock()
	if existing, ok := uc.metricsDeltas[delta.SessionID]; ok {
		existing.MessageCount += delta.MessageCount
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
	} else {
		cp := delta
		cp.AccumulatedCount = 1
		uc.metricsDeltas[delta.SessionID] = &cp
	}
}

// StartMetricsFlusher delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) StartMetricsFlusher(ctx context.Context) {
	if uc.metricsUsecase != nil {
		uc.metricsUsecase.StartMetricsFlusher(ctx)
	}
}

// flushAllMetrics delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) flushAllMetrics(ctx context.Context) {
	if uc.metricsUsecase != nil {
		uc.metricsUsecase.flushAllMetrics(ctx)
	}
}

// forceFlushSingle delegates to SessionMetricsUsecase (Facade pattern).
func (uc *SessionUsecase) forceFlushSingle(sessionID string) {
	if uc.metricsUsecase != nil {
		uc.metricsUsecase.forceFlushSingle(sessionID)
	}
}
