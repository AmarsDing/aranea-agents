package data

import (
	"context"
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
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

func (s *memoryFactIndexSync) SyncFactIndex(ctx context.Context, agentID, userID, factID, statement string) error {
	if s == nil || s.vec == nil {
		return biz.ErrMemoryUnavailable
	}
	embedding, err := s.vec.EmbedText(ctx, statement)
	if err != nil {
		return err
	}
	if err := s.vec.UpsertFactVector(ctx, agentID, userID, factID, statement, embedding); err != nil {
		return err
	}
	return s.syncSQLiteBlob(ctx, factID, embedding)
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
