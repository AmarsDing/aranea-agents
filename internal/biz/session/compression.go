package session

import (
	"context"

	"github.com/google/wire"
)

// SessionCompressionUsecase handles context compression logic,
// extracted from SessionUsecase to reduce God Object scope.
type SessionCompressionUsecase struct {
	compressRepo   CompressRepo
	contextUpdater ContextUpdater
	summaryReader  SummaryReader
	summaryWriter  SummaryWriter
}

// NewSessionCompressionUsecase creates a new SessionCompressionUsecase.
func NewSessionCompressionUsecase(compressRepo CompressRepo, contextUpdater ContextUpdater, summaryReader SummaryReader, summaryWriter SummaryWriter) *SessionCompressionUsecase {
	return &SessionCompressionUsecase{
		compressRepo:   compressRepo,
		contextUpdater: contextUpdater,
		summaryReader:  summaryReader,
		summaryWriter:  summaryWriter,
	}
}

// TryIncrementCompressVersion increments the compress version for a session.
func (uc *SessionCompressionUsecase) TryIncrementCompressVersion(ctx context.Context, sessionID string) (int64, error) {
	return uc.compressRepo.TryIncrementCompressVersion(ctx, sessionID)
}

// CompressSessionInTx runs fn inside a compression transaction.
func (uc *SessionCompressionUsecase) CompressSessionInTx(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error {
	return uc.compressRepo.CompressSessionInTx(ctx, sessionID, fn)
}

// UpdateSessionContextFromLLMUsage refreshes context_used_ratio after a native LLM turn.
func (uc *SessionCompressionUsecase) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error {
	return uc.contextUpdater.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTokens, completionTokens, contextWindow)
}

// UpdateSessionContextAfterCompression refreshes context_used_* from an estimate of the compacted prompt.
func (uc *SessionCompressionUsecase) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error {
	return uc.contextUpdater.UpdateSessionContextAfterCompression(ctx, sessionID, estimatedPromptTokens, contextWindow)
}

// InsertSessionSummary appends a rolling summary row.
func (uc *SessionCompressionUsecase) InsertSessionSummary(ctx context.Context, row SessionSummary) error {
	return uc.summaryWriter.InsertSessionSummary(ctx, row)
}

// DeleteSessionSummaries removes all rolling summary rows for the session (recursive merge).
func (uc *SessionCompressionUsecase) DeleteSessionSummaries(ctx context.Context, sessionID string) error {
	return uc.summaryWriter.DeleteSessionSummaries(ctx, sessionID)
}

// MaxSessionSummaryToTurn returns the largest to_turn stored for the session (0 if none).
func (uc *SessionCompressionUsecase) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
	return uc.summaryReader.MaxSessionSummaryToTurn(ctx, sessionID)
}

// ListSessionSummaries returns summary rows in chronological order.
func (uc *SessionCompressionUsecase) ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error) {
	return uc.summaryReader.ListSessionSummaries(ctx, sessionID)
}

// LatestSessionSummaryTime returns created_at of the newest summary row or empty string.
func (uc *SessionCompressionUsecase) LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error) {
	return uc.summaryReader.LatestSessionSummaryTime(ctx, sessionID)
}

// UpdateSessionListSummary updates sessions.summary (short UI line).
func (uc *SessionCompressionUsecase) UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error {
	return uc.summaryWriter.UpdateSessionListSummary(ctx, sessionID, summary)
}

// SessionSummaryExists checks if a summary for the given turn range already exists.
func (uc *SessionCompressionUsecase) SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error) {
	return uc.summaryWriter.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
}

// UpdateRunnerSnapshotJSON persists the Runner session snapshot.
func (uc *SessionCompressionUsecase) UpdateRunnerSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error {
	return uc.contextUpdater.UpdateRunnerSnapshotJSON(ctx, sessionID, snapshotJSON)
}

// SessionCompressionProviderSet provides Wire bindings for SessionCompressionUsecase.
var SessionCompressionProviderSet = wire.NewSet(NewSessionCompressionUsecase)
