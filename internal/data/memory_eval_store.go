package data

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// evalMemoryStore implements biz.EvalMemoryStore over the existing L3 fact
// store. It is a thin facade: writes reuse UpsertFactRow (idempotent via the
// (scope_type, scope_id, fingerprint) unique key, PII-gated by the repo) and
// recall reuses the hybrid-scored RecallL3Facts. The user scope is the sole
// isolation boundary, per the Agent Memory Challenge rules.
type evalMemoryStore struct {
	facts  *l3FactRepo
	vec    *biz.MemoryUsecase        // nil when no embedder is configured
	syncer biz.MemoryFactIndexSyncer // nil when no embedder is configured
	lg     loggateway.Logger
}

var _ biz.EvalMemoryStore = (*evalMemoryStore)(nil)

// NewEvalMemoryStore assembles the evaluation facade. When emb is nil, writes
// skip vector indexing and recall degrades to the keyword-only brute-force
// path built into l3FactRepo — the Add/Search contract stays functional.
func NewEvalMemoryStore(d *Data, emb biz.EmbeddingService, lg loggateway.Logger) biz.EvalMemoryStore {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	s := &evalMemoryStore{
		facts: newL3FactRepo(d, d.VectorStore()),
		lg:    lg.With(loggateway.Domain("memory_eval")),
	}
	if emb != nil {
		s.vec = biz.NewMemoryUsecase(NewMemoryRepo(d), emb)
		s.syncer = NewMemoryFactIndexSync(s.vec, d, lg)
	}
	return s
}

func (s *evalMemoryStore) AddMessages(ctx context.Context, userID, sessionID string, msgs []biz.EvalMessage) (int, error) {
	stored := 0
	for i, m := range msgs {
		statement := m.Content
		if m.Role != "" {
			statement = m.Role + ": " + m.Content
		}
		raw, err := s.facts.UpsertFactRow(ctx, biz.FactUpsert{
			ScopeType:       "user",
			ScopeID:         userID,
			UserID:          userID,
			Statement:       statement,
			FactKind:        "event",
			Confidence:      1.0,
			Importance:      0.5,
			Status:          "active",
			Version:         1,
			SourceKind:      "agent_memory_challenge",
			SourceSessionID: sessionID,
			SourceMessageID: m.MessageID,
			CreatedAt:       normalizeEvalTimestamp(m.Timestamp),
			MetadataJSON:    evalMetadataJSON(sessionID, i),
		})
		if err != nil {
			return stored, err
		}
		stored++
		// Vector index sync is best-effort: failures mark the row stale
		// inside the syncer and keyword recall keeps working.
		if s.syncer != nil {
			if err := s.syncer.SyncFactIndexFromRow(ctx, raw); err != nil {
				s.lg.Warn("eval fact index sync failed, keyword recall still available",
					loggateway.StepID("memoryeval.index_sync"), loggateway.Err(err))
			}
		}
	}
	return stored, nil
}

func (s *evalMemoryStore) SearchMemories(ctx context.Context, userID, query string, topK int32) ([]biz.EvalMemoryItem, error) {
	var embedding []float32
	if s.vec != nil {
		if v, err := s.vec.EmbedText(ctx, query); err == nil {
			embedding = v
		} else {
			// K3: degrade to keyword-only hybrid recall (empty embedding
			// triggers the brute-force path inside RecallL3Facts).
			s.lg.Warn("eval query embed failed, degrade to keyword recall",
				loggateway.StepID("memoryeval.search_degrade"), loggateway.Err(err))
		}
	}
	// scopeType=user + scopeID=userID (+ userID predicate) hard-scopes the
	// recall to the caller's partition; no cross-user row can surface.
	rows, err := s.facts.RecallL3Facts(ctx, "user", userID, userID, query, embedding, topK, 0)
	if err != nil {
		return nil, err
	}
	items := make([]biz.EvalMemoryItem, 0, len(rows))
	for _, raw := range rows {
		var row struct {
			ID        string `json:"id"`
			Statement string `json:"statement"`
			CreatedAt string `json:"created_at"`
			Scores    struct {
				Total float64 `json:"total"`
			} `json:"scores"`
		}
		if err := json.Unmarshal(raw, &row); err != nil || row.ID == "" {
			continue
		}
		items = append(items, biz.EvalMemoryItem{
			ID:        row.ID,
			Content:   row.Statement,
			Score:     row.Scores.Total,
			Timestamp: row.CreatedAt,
		})
	}
	return items, nil
}

// normalizeEvalTimestamp accepts RFC3339(Nano) strings or epoch seconds/millis
// and returns RFC3339Nano; unparseable input yields "" (repo defaults to now).
func normalizeEvalTimestamp(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
		if n > 1e12 { // epoch millis
			return time.UnixMilli(n).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(n, 0).UTC().Format(time.RFC3339Nano)
	}
	return ""
}

func evalMetadataJSON(sessionID string, seq int) string {
	b, err := json.Marshal(map[string]any{"eval_session": sessionID, "eval_seq": seq})
	if err != nil {
		return "{}"
	}
	return string(b)
}
