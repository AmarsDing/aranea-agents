package session

// SessionSummary is one persisted rolling-summary row (session_summaries).
type SessionSummary struct {
	ID              string
	SessionID       string
	SummaryMarkdown string
	FromTurn        int
	ToTurn          int
	TokenEstimate   int
	CreatedAt       string
}

// StateDelta represents a key-value state mutation (mirrors biz.DomainStateDelta).
type StateDelta struct {
	Operation string
	Path      string
	ValueJSON string
}
