package data

// SQL fragments for usage event status bucketing (legacy rows may use ok/error).
const (
	sqlUsageStatusSuccess = `status IN ('success', 'ok')`
	sqlUsageStatusFailed  = `status IN ('failed', 'timeout', 'error')`
)
