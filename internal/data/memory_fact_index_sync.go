package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
)

type memoryFactIndexSync struct {
	vec  *biz.MemoryUsecase
	data *Data
	lg   loggateway.Logger
}

// NewMemoryFactIndexSync dual-writes L3 fact vectors to pgvector (optional) and the primary DB embedding_blob.
var _ biz.MemoryFactIndexSyncer = (*memoryFactIndexSync)(nil)

func NewMemoryFactIndexSync(vec *biz.MemoryUsecase, data *Data, lg loggateway.Logger) biz.MemoryFactIndexSyncer {
	if vec == nil {
		return nil
	}
	if data == nil {
		return vec
	}
	return &memoryFactIndexSync{vec: vec, data: data, lg: lg}
}

// SyncFactIndex embeds the statement and writes vectors to pgvector + primary DB embedding_blob.
func (s *memoryFactIndexSync) SyncFactIndex(ctx context.Context, agentID, userID, factID, statement string) error {
	if s == nil || s.vec == nil {
		return biz.ErrMemoryUnavailable
	}
	embedding, err := s.vec.EmbedText(ctx, statement)
	if err != nil {
		s.lg.Warn("embed text failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(err))
		s.markStaleOnError(ctx, factID, err)
		return err
	}
	return s.writeFactIndex(ctx, agentID, userID, factID, statement, embedding)
}

// markStaleOnError flips a fact back to stale (and bumps index_attempts) so the
// sweeper retries it later; warn-only by contract.
func (s *memoryFactIndexSync) markStaleOnError(ctx context.Context, factID string, reason error) {
	if s == nil || s.data == nil {
		return
	}
	if serr := s.markFactIndexStale(ctx, factID, reason.Error()); serr != nil {
		s.lg.Warn("failed to mark fact index stale", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(serr))
	}
}

// writeFactIndex persists a precomputed embedding to pgvector + primary DB
// embedding_blob + shared read index, then marks the row fresh. Any failure
// marks the row stale (sweeper retries) and is returned to the caller.
func (s *memoryFactIndexSync) writeFactIndex(ctx context.Context, agentID, userID, factID, statement string, embedding []float32) error {
	if err := s.vec.UpsertFactVector(ctx, agentID, userID, factID, statement, embedding); err != nil {
		s.lg.Warn("upsert fact vector failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(err))
		s.markStaleOnError(ctx, factID, err)
		return err
	}
	if err := s.syncEmbeddingBlob(ctx, factID, embedding); err != nil {
		s.lg.Warn("sync embedding blob failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(err))
		s.markStaleOnError(ctx, factID, err)
		return entErrToBizErr(err, "MEMORY_L3")
	}
	// Dual-write the shared read index (vector_embeddings). L2/L3 recall
	// searches that table via Data.vectorStore, while UpsertFactVector above
	// maintains the per-agent agent_memory_<dim> store used by conflict
	// arbitration and L0/L1 similarity. Historically only the latter was
	// written, leaving the recall read index permanently empty. Fail-soft:
	// the embedding_blob fallback in scoreFactRow still covers reads, and the
	// ARANEA_VECTOR_BACKFILL-gated backfill test repairs existing rows.
	readIndexFailed := false
	if s.data != nil && s.data.vectorStore != nil && len(embedding) > 0 {
		if dim := s.data.VectorDim(); dim > 0 && len(embedding) != dim {
			s.lg.Warn("skip read-index upsert: embedding dim mismatch with vector store",
				loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID))
		} else if uerr := s.data.vectorStore.Upsert(ctx, factID, float32To64(embedding), map[string]string{
			"agent_id": agentID,
			"user_id":  userID,
			"content":  statement,
		}); uerr != nil {
			s.lg.Warn("upsert read-index vector failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(uerr))
			readIndexFailed = true
		}
	}
	// Mark fresh on full success. If the read-index upsert failed, mark stale
	// instead so the reconciler retries: ListStaleIndexFacts only scans
	// stale/failed/pending, and a silently-fresh fact missing from
	// vector_embeddings would never be repaired (2026-08-26 review L1).
	if s.data != nil {
		if readIndexFailed {
			if serr := s.markFactIndexStale(ctx, factID, "read-index upsert failed"); serr != nil {
				s.lg.Warn("failed to mark fact index stale", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(serr))
			}
			return nil
		}
		if serr := s.markFactIndexSynced(ctx, factID); serr != nil {
			s.lg.Warn("failed to mark fact index synced", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(serr))
		}
	}
	return nil
}

// factIndexRowRef is one parsed memory_facts row ready for index sync.
type factIndexRowRef struct {
	agentID   string
	userID    string
	factID    string
	statement string
}

// parseFactIndexRow extracts the fields SyncFactIndex consumes from a raw
// memory_facts JSON row. valid=false means the row is permanently unusable
// (missing agent/fact/statement) — not a transient error, never marked stale.
func parseFactIndexRow(raw []byte) (ref factIndexRowRef, valid bool, err error) {
	var m map[string]any
	if err = json.Unmarshal(raw, &m); err != nil {
		return ref, false, err
	}
	ref.agentID = jsonutil.IfaceStr(m, "agent_id")
	if ref.agentID == "" {
		ref.agentID = jsonutil.IfaceStr(m, "scope_id")
	}
	ref.factID = jsonutil.IfaceStr(m, "id")
	ref.userID = jsonutil.IfaceStr(m, "user_id")
	ref.statement = jsonutil.IfaceStr(m, "statement")
	if ref.agentID == "" || ref.factID == "" || ref.statement == "" {
		return ref, false, nil
	}
	return ref, true, nil
}

// SyncFactIndexBatchFromRows implements biz.MemoryFactIndexBatchSyncer: one
// batched embed call for the whole picked sweep batch, then per-row writes.
// Embed failure marks every parsed row stale and returns the error (sweeper
// backoff paces the retry); per-row write failures only mark that row stale.
func (s *memoryFactIndexSync) SyncFactIndexBatchFromRows(ctx context.Context, raws [][]byte) (int, error) {
	if s == nil || s.vec == nil {
		return 0, biz.ErrMemoryUnavailable
	}
	refs := make([]factIndexRowRef, 0, len(raws))
	texts := make([]string, 0, len(raws))
	for _, raw := range raws {
		ref, valid, perr := parseFactIndexRow(raw)
		if perr != nil {
			s.lg.Warn("fact index sync json parse failed", loggateway.StepID("memory.index_sync_parse_fail"), loggateway.Err(perr))
			continue
		}
		if !valid {
			continue
		}
		refs = append(refs, ref)
		texts = append(texts, ref.statement)
	}
	if len(refs) == 0 {
		return 0, nil
	}
	vecs, err := s.vec.EmbedTexts(ctx, texts)
	if err != nil {
		s.lg.Warn("batch embed texts failed", loggateway.StepID("memory.l4_fail"),
			loggateway.Int("batch", len(refs)), loggateway.Err(err))
		for _, ref := range refs {
			s.markStaleOnError(ctx, ref.factID, err)
		}
		return 0, err
	}
	if len(vecs) != len(refs) {
		cntErr := apierror.Internal(apierror.DomainKnowledge, "embedder batch: expected %d embeddings, got %d", len(refs), len(vecs))
		s.lg.Warn("batch embed count mismatch", loggateway.StepID("memory.l4_fail"),
			loggateway.Int("expected", len(refs)), loggateway.Int("got", len(vecs)))
		for _, ref := range refs {
			s.markStaleOnError(ctx, ref.factID, cntErr)
		}
		return 0, cntErr
	}
	synced := 0
	for i, ref := range refs {
		if len(vecs[i]) == 0 {
			continue
		}
		if werr := s.writeFactIndex(ctx, ref.agentID, ref.userID, ref.factID, ref.statement, vecs[i]); werr != nil {
			continue
		}
		synced++
	}
	return synced, nil
}

func (s *memoryFactIndexSync) SyncFactIndexFromRow(ctx context.Context, raw []byte) error {
	if s == nil || s.vec == nil || len(raw) == 0 {
		return biz.ErrMemoryUnavailable
	}
	ref, valid, err := parseFactIndexRow(raw)
	if err != nil {
		s.lg.Warn("fact index sync json parse failed", loggateway.StepID("memory.index_sync_parse_fail"), loggateway.Err(err))
		return err
	}
	if !valid {
		return nil
	}
	return s.SyncFactIndex(ctx, ref.agentID, ref.userID, ref.factID, ref.statement)
}

func (s *memoryFactIndexSync) syncEmbeddingBlob(ctx context.Context, factID string, embedding []float32) error {
	if s == nil || s.data == nil || len(embedding) == 0 {
		return nil
	}
	blob := encodeFloat32Blob(embedding)
	norm := vectorL2Norm(embedding)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		s.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET embedding_blob = ?, embedding_norm = ?, embedding_dim = ?, embedding_status = 'fresh', embedding_model = 'memory_embedder', index_attempts = 0, updated_at = ? WHERE id = ?`),
		blob, norm, len(embedding), now, factID)
	return entErrToBizErr(err, "MEMORY_L3")
}

func (s *memoryFactIndexSync) markFactIndexStale(ctx context.Context, factID, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		s.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET embedding_status = 'stale', index_attempts = index_attempts + 1, updated_at = ? WHERE id = ?`), now, factID)
	return entErrToBizErr(err, "MEMORY_L3")
}

func (s *memoryFactIndexSync) markFactIndexSynced(ctx context.Context, factID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		s.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET embedding_status = 'fresh', index_attempts = 0, updated_at = ? WHERE id = ?`), now, factID)
	return entErrToBizErr(err, "MEMORY_L3")
}
