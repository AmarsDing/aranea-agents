package biz

import "context"

type RecallDebugRow struct {
	Layer     string
	ID        string
	Title     string
	Summary   string
	Statement string
	Scores    RecallScoreBreakdown
}

type MemoryDebugRecaller interface {
	RecallL2EpisodesDebug(ctx context.Context, agentID, sessionID, query string, limit int32) ([]RecallDebugRow, error)
	RecallL3FactsDebug(ctx context.Context, scopeType, scopeID, userID, query string, limit int32) ([]RecallDebugRow, error)
	CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]RecallDebugRow, error)
}

type MemoryFactIndexCounter interface {
	CountFactsByIndexStatus(ctx context.Context) (fresh, stale, disabled int64, err error)
}
