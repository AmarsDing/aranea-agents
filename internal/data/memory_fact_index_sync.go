package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
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
	markStale := func(reason error) {
		if s.data == nil {
			return
		}
		if serr := s.markFactIndexStale(ctx, factID, reason.Error()); serr != nil {
			s.lg.Warn("failed to mark fact index stale", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(serr))
		}
	}
	embedding, err := s.vec.EmbedText(ctx, statement)
	if err != nil {
		s.lg.Warn("embed text failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(err))
		markStale(err)
		return err
	}
	if err := s.vec.UpsertFactVector(ctx, agentID, userID, factID, statement, embedding); err != nil {
		s.lg.Warn("upsert fact vector failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(err))
		markStale(err)
		return err
	}
	if err := s.syncEmbeddingBlob(ctx, factID, embedding); err != nil {
		s.lg.Warn("sync embedding blob failed", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(err))
		markStale(err)
		return entErrToBizErr(err, "MEMORY_L3")
	}
	// Mark fresh on full success.
	if s.data != nil {
		if serr := s.markFactIndexSynced(ctx, factID); serr != nil {
			s.lg.Warn("failed to mark fact index synced", loggateway.StepID("memory.l4_fail"), loggateway.Str("fact_id", factID), loggateway.Err(serr))
		}
	}
	return nil
}

func (s *memoryFactIndexSync) SyncFactIndexFromRow(ctx context.Context, raw []byte) error {
	if s == nil || s.vec == nil || len(raw) == 0 {
		return biz.ErrMemoryUnavailable
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		s.lg.Warn("fact index sync json parse failed", loggateway.StepID("memory.index_sync_parse_fail"), loggateway.Err(err))
		return err
	}
	agentID := jsonutil.IfaceStr(m, "agent_id")
	if agentID == "" {
		agentID = jsonutil.IfaceStr(m, "scope_id")
	}
	factID := jsonutil.IfaceStr(m, "id")
	statement := jsonutil.IfaceStr(m, "statement")
	if agentID == "" || factID == "" || statement == "" {
		return nil
	}
	return s.SyncFactIndex(ctx, agentID, jsonutil.IfaceStr(m, "user_id"), factID, statement)
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
