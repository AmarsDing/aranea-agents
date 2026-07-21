package session

import (
	"context"
	"strings"
)

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

// UpdateSessionContextFromLLMUsage delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error {
	return uc.compressionUsecase.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTokens, completionTokens, contextWindow)
}

// UpdateSessionContextAfterCompression delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error {
	return uc.compressionUsecase.UpdateSessionContextAfterCompression(ctx, sessionID, estimatedPromptTokens, contextWindow)
}

// InsertSessionSummary delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) InsertSessionSummary(ctx context.Context, row SessionSummary) error {
	if strings.TrimSpace(row.SessionID) == "" {
		return validationErr("session id is required")
	}
	return uc.compressionUsecase.InsertSessionSummary(ctx, row)
}

// DeleteSessionSummaries delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) DeleteSessionSummaries(ctx context.Context, sessionID string) error {
	return uc.compressionUsecase.DeleteSessionSummaries(ctx, sessionID)
}

// MaxSessionSummaryToTurn delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
	return uc.compressionUsecase.MaxSessionSummaryToTurn(ctx, sessionID)
}

// ListSessionSummaries delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error) {
	return uc.compressionUsecase.ListSessionSummaries(ctx, sessionID)
}

// LatestSessionSummaryTime delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error) {
	return uc.compressionUsecase.LatestSessionSummaryTime(ctx, sessionID)
}

// UpdateSessionListSummary delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error {
	return uc.compressionUsecase.UpdateSessionListSummary(ctx, sessionID, summary)
}

// TryIncrementCompressVersion delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) TryIncrementCompressVersion(ctx context.Context, sessionID string) (int64, error) {
	return uc.compressionUsecase.TryIncrementCompressVersion(ctx, sessionID)
}

// CompressSessionInTx delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) CompressSessionInTx(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error {
	return uc.compressionUsecase.CompressSessionInTx(ctx, sessionID, fn)
}

// SessionSummaryExists delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error) {
	return uc.compressionUsecase.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
}
