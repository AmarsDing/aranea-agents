package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/jsonutil"
)

type memoryFactIndexSync struct {
	vec   *biz.MemoryUsecase
	store *sessionmemory.Store
}

// NewMemoryFactIndexSync dual-writes L3 fact vectors to pgvector (optional) and SQLite embedding_blob.
func NewMemoryFactIndexSync(vec *biz.MemoryUsecase, store *sessionmemory.Store) biz.MemoryFactIndexSyncer {
	if vec == nil {
		return nil
	}
	if store == nil {
		return vec
	}
	return &memoryFactIndexSync{vec: vec, store: store}
}

// SyncFactIndex embeds the statement and writes vectors to pgvector + SQLite.
// On any failure it marks index_status='stale' on the SQLite row (MEM-OPT-01 Phase 1).
// On success it marks index_status='fresh'.
func (s *memoryFactIndexSync) SyncFactIndex(ctx context.Context, agentID, userID, factID, statement string) error {
	if s == nil || s.vec == nil {
		return biz.ErrMemoryUnavailable
	}
	markStale := func(reason error) {
		if s.store == nil {
			return
		}
		if serr := s.store.MarkFactIndexStale(ctx, factID, reason.Error()); serr != nil {
			event.SysLogWarn("system.auto_memory.l4_fail", "failed to mark fact index stale", event.P("fact_id", factID), event.P("error", serr.Error()))
		}
	}
	embedding, err := s.vec.EmbedText(ctx, statement)
	if err != nil {
		markStale(err)
		return err
	}
	if err := s.vec.UpsertFactVector(ctx, agentID, userID, factID, statement, embedding); err != nil {
		markStale(err)
		return err
	}
	if err := s.syncSQLiteBlob(ctx, factID, embedding); err != nil {
		markStale(err)
		return err
	}
	// Mark fresh on full success.
	if s.store != nil {
		if serr := s.store.MarkFactIndexSynced(ctx, factID); serr != nil {
			event.SysLogWarn("system.auto_memory.l4_fail", "failed to mark fact index synced", event.P("fact_id", factID), event.P("error", serr.Error()))
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

func (s *memoryFactIndexSync) syncSQLiteBlob(ctx context.Context, factID string, embedding []float32) error {
	if s == nil || s.store == nil || len(embedding) == 0 {
		return nil
	}
	return s.store.UpsertFactEmbedding(ctx, factID, embedding, "memory_embedder", len(embedding))
}
