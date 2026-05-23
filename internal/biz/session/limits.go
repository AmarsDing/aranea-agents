package session

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
