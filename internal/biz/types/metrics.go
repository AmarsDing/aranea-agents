package types

import "sort"

// p95Percentile is the percentile used by P95 (95th percentile).
const p95Percentile = 0.95

// IsSuccess determines whether a skill invocation was successful based on
// outcome and status fields. This is a business rule: "success" outcome
// always succeeds; empty outcome with "completed" or "success" status also succeeds.
func IsSuccess(outcome, status string) bool {
	if outcome == "success" {
		return true
	}
	if outcome == "" && (status == "completed" || status == "success") {
		return true
	}
	return false
}

// SafeRate computes success/total as a float64, returning 0 when total <= 0.
func SafeRate(success, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(success) / float64(total)
}

// P95 returns the 95th percentile value from a slice of durations.
func P95(durations []int) int {
	if len(durations) == 0 {
		return 0
	}
	sort.Ints(durations)
	idx := int(float64(len(durations)) * p95Percentile)
	if idx >= len(durations) {
		idx = len(durations) - 1
	}
	return durations[idx]
}

// DayFromCreatedAt extracts the date portion (YYYY-MM-DD) from an RFC3339 timestamp string.
func DayFromCreatedAt(createdAt string) string {
	if len(createdAt) >= 10 {
		return createdAt[:10]
	}
	return createdAt
}
