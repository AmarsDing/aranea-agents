package session

import "context"

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

// UpdateSessionContextFromLLMUsage refreshes sessions.context_used_ratio after a native LLM turn.
func (uc *SessionUsecase) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error {
	return uc.contextUpdater.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTokens, completionTokens, contextWindow)
}

// UpdateSessionContextAfterCompression refreshes context_used_* from an estimate of the compacted prompt.
func (uc *SessionUsecase) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error {
	return uc.contextUpdater.UpdateSessionContextAfterCompression(ctx, sessionID, estimatedPromptTokens, contextWindow)
}

// InsertSessionSummary appends a rolling summary row.
func (uc *SessionUsecase) InsertSessionSummary(ctx context.Context, row SessionSummary) error {
	return uc.summaryRepo.InsertSessionSummary(ctx, row)
}

// MaxSessionSummaryToTurn returns the largest to_turn stored for the session (0 if none).
func (uc *SessionUsecase) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
	return uc.summaryRepo.MaxSessionSummaryToTurn(ctx, sessionID)
}

// ListSessionSummaries returns summary rows in chronological order.
func (uc *SessionUsecase) ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error) {
	return uc.summaryRepo.ListSessionSummaries(ctx, sessionID)
}

// LatestSessionSummaryTime returns created_at of the newest summary row or empty string.
func (uc *SessionUsecase) LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error) {
	return uc.summaryRepo.LatestSessionSummaryTime(ctx, sessionID)
}

// UpdateSessionListSummary updates sessions.summary (short UI line).
func (uc *SessionUsecase) UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error {
	return uc.summaryRepo.UpdateSessionListSummary(ctx, sessionID, summary)
}

func (uc *SessionUsecase) TryIncrementCompressVersion(ctx context.Context, sessionID string) (int64, error) {
	return uc.compressRepo.TryIncrementCompressVersion(ctx, sessionID)
}

func (uc *SessionUsecase) CompressSessionInTx(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error {
	return uc.compressRepo.CompressSessionInTx(ctx, sessionID, fn)
}

func (uc *SessionUsecase) SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error) {
	return uc.summaryRepo.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
}
