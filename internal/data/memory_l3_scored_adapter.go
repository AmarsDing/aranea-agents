package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

var _ biz.SessionL3ScoredRecallStore = (*L3ScoredRecallAdapter)(nil)

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
	recalledIDs := make([]string, 0, len(raw))
	for _, b := range raw {
		hit := biz.RecallHit{
			Layer: "L3",
			Raw:   b,
		}
		// Extract id, statement, and persisted decay_score from raw JSON.
		// decay_score is the Ebbinghaus reachability R_t ∈ (0,1] written back
		// by the MemoryEbbinghausDecayWorker cron job. Setting it on
		// Scores.Decay enables the fused-recall decay fusion formula in
		// RecallFactsFused (Total *= 0.7 + 0.3*Decay).
		var m map[string]any
		if err := json.Unmarshal(b, &m); err == nil {
			if id, ok := m["id"].(string); ok {
				hit.ID = id
				recalledIDs = append(recalledIDs, id)
			}
			if stmt, ok := m["statement"].(string); ok {
				hit.Statement = stmt
			}
			// Scores computed by the recall path are annotated into the raw
			// JSON under "scores" (see annotateFactScores). Propagate them so
			// fused recall can rank and apply the minScore filter — without
			// this, Total stays 0 and every hit is filtered out (Bug A).
			if sc, ok := m["scores"].(map[string]any); ok {
				hit.Scores.Keyword = anyFloat(sc, "keyword")
				hit.Scores.Vector = anyFloat(sc, "vector")
				hit.Scores.Importance = anyFloat(sc, "importance")
				hit.Scores.Recency = anyFloat(sc, "recency")
				hit.Scores.QualityScore = anyFloat(sc, "quality_score")
				hit.Scores.CrossEncoder = anyFloat(sc, "cross_encoder")
				hit.Scores.Total = anyFloat(sc, "total")
			}
			// decay_score defaults to 1.0 in the DB schema (no forgetting).
			// Only set Scores.Decay when the persisted value is in (0, 1].
			// A value of 0 means "not yet computed by the cron job" — leave
			// Decay at 0 so the fusion formula skips it (treats as no decay).
			if ds, ok := m["decay_score"].(float64); ok && ds > 0 && ds <= 1.0 {
				hit.Scores.Decay = ds
			}
		}
		out = append(out, hit)
	}
	// Increment use_count and update last_used_at for recalled facts so the
	// Ebbinghaus decay worker has accurate access-recency signals. Failures
	// are logged but do not fail the recall (write-on-recall is best-effort).
	if len(recalledIDs) > 0 {
		if err := l3.IncrementFactAccessCount(ctx, recalledIDs); err != nil && a.data.lg != nil {
			a.data.lg.Warn("scored recall: increment access count failed",
				loggateway.StepID("memory.l3_recall_access"),
				loggateway.Err(err))
		}
	}
	return out, nil
}
