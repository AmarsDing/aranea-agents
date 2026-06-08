package session

// Query limit constants for the session domain.
// The authoritative centralized copies live in biz/query_limits.go;
// these are kept here because session cannot import biz (circular dependency).
const (
	timelineDefaultInvLimit = 100
	timelineMaxInvLimit     = 500

	MessageListDefaultLimit = 100
	MessageListMaxLimit     = 500
	TimelineMessageMaxFetch = 2000
	CompressMessageMaxRows  = 512
	ActivityCancelScanLimit = 64
)

// Timeline list limit helpers (extracted from session_usecase.go for maintainability).

func timelineMessageFetchLimit(q TimelineQuery) int {
	need := q.Limit + q.Offset
	if need <= 0 || need > TimelineMessageMaxFetch {
		return TimelineMessageMaxFetch
	}
	return need
}

func clampMessageListLimit(limit int) int {
	if limit <= 0 {
		return MessageListDefaultLimit
	}
	if limit > MessageListMaxLimit {
		return MessageListMaxLimit
	}
	return limit
}

// timelineInvocationLimit bounds tool/skill rows loaded before merge-sort (see Timeline).
func timelineInvocationLimit(q TimelineQuery) int {
	limit := q.Limit
	if limit <= 0 {
		limit = timelineDefaultInvLimit
	}
	if limit > timelineMaxInvLimit {
		limit = timelineMaxInvLimit
	}
	return limit
}
