package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
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
	agentID := strings.TrimSpace(factJSONStr(m, "agent_id"))
	if agentID == "" {
		agentID = strings.TrimSpace(factJSONStr(m, "scope_id"))
	}
	factID := strings.TrimSpace(factJSONStr(m, "id"))
	statement := strings.TrimSpace(factJSONStr(m, "statement"))
	if agentID == "" || factID == "" || statement == "" {
		return nil
	}
	return s.SyncFactIndex(ctx, agentID, factJSONStr(m, "user_id"), factID, statement)
}

func (s *memoryFactIndexSync) syncSQLiteBlob(ctx context.Context, factID string, embedding []float32) error {
	if s == nil || s.store == nil || len(embedding) == 0 {
		return nil
	}
	return s.store.UpsertFactEmbedding(ctx, factID, embedding, "memory_embedder", len(embedding))
}

func factJSONStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
