package llmcontext

// Context status thresholds — keep in sync with web/src/features/session/contextMetrics.ts.
const (
	ContextStatusExceededThreshold = 0.95
	ContextStatusCriticalThreshold = 0.80
	ContextStatusWarningThreshold  = 0.60
)

// ContextRatio returns prompt usage ratio capped at 1.0.
func ContextRatio(promptTokens, contextWindow int) float64 {
	if contextWindow <= 0 || promptTokens <= 0 {
		return 0
	}
	ratio := float64(promptTokens) / float64(contextWindow)
	if ratio > 1 {
		return 1
	}
	return ratio
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
