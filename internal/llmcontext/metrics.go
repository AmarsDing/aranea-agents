package llmcontext

// Context status thresholds — keep in sync with web/src/features/session/contextMetrics.ts.
const (
	ContextStatusExceededThreshold = 0.95
	ContextStatusCriticalThreshold = 0.80
	ContextStatusWarningThreshold  = 0.60
)

// ContextRatio returns the real prompt usage ratio. Values above 1.0 indicate
// the prompt exceeded the context window and are returned unclamped so callers
// (status mapping, session metrics, frontend display) observe the true state.
func ContextRatio(promptTokens, contextWindow int) float64 {
	if contextWindow <= 0 || promptTokens <= 0 {
		return 0
	}
	return float64(promptTokens) / float64(contextWindow)
}

// ContextStatusForRatio maps usage ratio to sessions.context_status.
func ContextStatusForRatio(ratio float64) string {
	switch {
	case ratio >= ContextStatusExceededThreshold:
		return "exceeded"
	case ratio >= ContextStatusCriticalThreshold:
		return "critical"
	case ratio >= ContextStatusWarningThreshold:
		return "warning"
	default:
		return "normal"
	}
}
