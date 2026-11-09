package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

var _ biz.SessionL3ScoredRecallStore = (*L3ScoredRecallAdapter)(nil)

// skipUserScopePIIHit 判定 user 域召回命中是否因 PII 门被丢弃：仅
// scope_type="user" 且 raw JSON 的 pii_flag 为 true 时跳过。其它域
//（agent/team/workspace/global）不受影响——PII 门是 user 域召回的前置门
//（spec §6.3 召回接线，过渡期按 pii_flag 保守处理）。
func skipUserScopePIIHit(scopeType string, raw []byte) bool {
	if !strings.EqualFold(strings.TrimSpace(scopeType), "user") {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	flag, _ := m["pii_flag"].(bool)
	return flag
}

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
	// P2-3: pass the shared VectorStore so the pgvector+FTS RRF fusion path
	// activates on the main chat recall path (previously nil → fusion never
	// ran here; brute-force/recency paths still apply for small fact sets).
	l3 := newL3FactRepo(a.data, a.data.VectorStore())
	raw, err := l3.RecallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, 0)
	if err != nil {
		return nil, err
	}
	// For scored recall, we return the raw rows as RecallHit objects.
	// The scoring is done internally in RecallL3Facts.
	out := make([]biz.RecallHit, 0, len(raw))
	recalledIDs := make([]string, 0, len(raw))
	for _, b := range raw {
		// D3 user 域召回 PII 门（过渡期保守口径，spec §6.3）：user 域召回
		// 命中 pii_flag=1 的行不进 prompt——写路径 PII 门已把 statement 裁成
		// 脱敏载体，但召回侧在 L-4 密级列落位前一律保守丢弃（也不计
		// recalled_count，避免污染艾宾浩斯访问信号）。
		if skipUserScopePIIHit(scopeType, b) {
			continue
		}
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
	// Increment recalled_count and update last_used_at for recalled facts so the
	// Ebbinghaus decay worker has accurate access-recency signals. Failures
	// are logged but do not fail the recall (write-on-recall is best-effort).
	// FR-12.6: this is the "recalled" stage of the three-stage counters.
	if len(recalledIDs) > 0 {
		if err := l3.IncrementFactRecalledCount(ctx, recalledIDs); err != nil && a.data.lg != nil {
			a.data.lg.Warn("scored recall: increment recalled count failed",
				loggateway.StepID("memory.l3_recall_access"),
				loggateway.Err(err))
		}
	}
	return out, nil
}
