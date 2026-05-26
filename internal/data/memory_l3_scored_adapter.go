package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// L3ScoredRecallAdapter exposes sessionmemory scored L3 recall as biz.RecallHit rows.
type L3ScoredRecallAdapter struct {
	store *sessionmemory.Store
}

func NewL3ScoredRecallAdapter(store *sessionmemory.Store) *L3ScoredRecallAdapter {
	if store == nil {
		return nil
	}
	return &L3ScoredRecallAdapter{store: store}
}

func (a *L3ScoredRecallAdapter) RecallL3Hits(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32) ([]biz.RecallHit, error) {
	if a == nil || a.store == nil {
		return nil, nil
	}
	rows, err := a.store.RecallL3FactsScored(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit)
	if err != nil {
		return nil, err
	}
	out := make([]biz.RecallHit, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.RecallHit{
			Layer:     row.Layer,
			ID:        row.ID,
			Title:     row.Title,
			Summary:   row.Summary,
			Statement: row.Statement,
			Raw:       append([]byte(nil), row.Raw...),
			Scores: biz.RecallScoreBreakdown{
				Keyword:      row.Scores.Keyword,
				Vector:       row.Scores.Vector,
				Importance:   row.Scores.Importance,
				Recency:      row.Scores.Recency,
				CrossEncoder: row.Scores.CrossEncoder,
				Total:        row.Scores.Total,
			},
		})
	}
	return out, nil
}
