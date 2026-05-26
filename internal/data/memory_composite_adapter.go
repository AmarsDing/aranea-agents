package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// MemoryCompositeRecallAdapter maps sessionmemory composite recall to biz ports.
type MemoryCompositeRecallAdapter struct {
	store *sessionmemory.Store
}

func NewMemoryCompositeRecallAdapter(store *sessionmemory.Store) biz.SessionCompositeRecallStore {
	if store == nil {
		return nil
	}
	return &MemoryCompositeRecallAdapter{store: store}
}

func (a *MemoryCompositeRecallAdapter) CompositeSearchMemories(ctx context.Context, agentID, sessionID, userID, query string, limit int32) ([]biz.CompositeRecallStoreRow, error) {
	if a == nil || a.store == nil {
		return nil, nil
	}
	rows, err := a.store.CompositeSearchMemories(ctx, agentID, sessionID, userID, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]biz.CompositeRecallStoreRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.CompositeRecallStoreRow{
			Layer:     row.Layer,
			Title:     row.Title,
			Summary:   row.Summary,
			Statement: row.Statement,
			Score:     row.Scores.Total,
		})
	}
	return out, nil
}
