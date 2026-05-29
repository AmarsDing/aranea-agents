package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

type memoryDebugRecallAdapter struct {
	store *sessionmemory.Store
}

var _ biz.MemoryDebugRecaller = (*memoryDebugRecallAdapter)(nil)

func NewMemoryDebugRecaller(store *sessionmemory.Store) biz.MemoryDebugRecaller {
	if store == nil {
		return nil
	}
	return &memoryDebugRecallAdapter{store: store}
}

func (a *memoryDebugRecallAdapter) RecallL2EpisodesDebug(ctx context.Context, agentID, sessionID, query string, limit int32) ([]biz.RecallDebugRow, error) {
	rows, err := a.store.RecallL2EpisodesDebug(ctx, agentID, sessionID, query, limit)
	if err != nil {
		return nil, err
	}
	return convertRecallDebugRows(rows), nil
}

func (a *memoryDebugRecallAdapter) RecallL3FactsDebug(ctx context.Context, scopeType, scopeID, userID, query string, limit int32) ([]biz.RecallDebugRow, error) {
	rows, err := a.store.RecallL3FactsDebug(ctx, scopeType, scopeID, userID, query, limit)
	if err != nil {
		return nil, err
	}
	return convertRecallDebugRows(rows), nil
}

func (a *memoryDebugRecallAdapter) CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]biz.RecallDebugRow, error) {
	rows, err := a.store.CompositeSearchMemories(ctx, agentID, sessionID, userID, query, limit)
	if err != nil {
		return nil, err
	}
	return convertRecallDebugRows(rows), nil
}

func convertRecallDebugRows(rows []sessionmemory.RecallDebugRow) []biz.RecallDebugRow {
	out := make([]biz.RecallDebugRow, len(rows))
	for i, r := range rows {
		out[i] = biz.RecallDebugRow{
			Layer:     r.Layer,
			ID:        r.ID,
			Title:     r.Title,
			Summary:   r.Summary,
			Statement: r.Statement,
			Scores: biz.RecallScoreBreakdown{
				Keyword:      r.Scores.Keyword,
				Vector:       r.Scores.Vector,
				Importance:   r.Scores.Importance,
				Recency:      r.Scores.Recency,
				CrossEncoder: r.Scores.CrossEncoder,
				Total:        r.Scores.Total,
			},
		}
	}
	return out
}

type memoryFactIndexCounterAdapter struct {
	store *sessionmemory.Store
}

var _ biz.MemoryFactIndexCounter = (*memoryFactIndexCounterAdapter)(nil)

func NewMemoryFactIndexCounter(store *sessionmemory.Store) biz.MemoryFactIndexCounter {
	if store == nil {
		return nil
	}
	return &memoryFactIndexCounterAdapter{store: store}
}

func (a *memoryFactIndexCounterAdapter) CountFactsByIndexStatus(ctx context.Context) (fresh, stale, disabled int64, err error) {
	return a.store.CountFactsByIndexStatus(ctx)
}
