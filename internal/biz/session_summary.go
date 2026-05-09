package biz

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
