package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
)

// L3ScoredRecallAdapter exposes scored L3 recall as biz.RecallHit rows.
type L3ScoredRecallAdapter struct {
	data *Data
}

func NewL3ScoredRecallAdapter(data *Data) *L3ScoredRecallAdapter {
	if data == nil {
		return nil
	}
	return &L3ScoredRecallAdapter{data: data}
}

func (a *L3ScoredRecallAdapter) RecallL3Hits(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32) ([]biz.RecallHit, error) {
	if a == nil {
		return nil, nil
	}
	l3 := newL3FactRepo(a.data, nil)
	raw, err := l3.RecallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, 0)
	if err != nil {
		return nil, err
	}
	// For scored recall, we return the raw rows as RecallHit objects.
	// The scoring is done internally in RecallL3Facts.
	out := make([]biz.RecallHit, 0, len(raw))
	for _, b := range raw {
		hit := biz.RecallHit{
			Layer: "L3",
			Raw:   b,
		}
		// Extract id and statement from raw JSON
		var m map[string]any
		if err := json.Unmarshal(b, &m); err == nil {
			if id, ok := m["id"].(string); ok {
				hit.ID = id
			}
			if stmt, ok := m["statement"].(string); ok {
				hit.Statement = stmt
			}
		}
		out = append(out, hit)
	}
	return out, nil
}
