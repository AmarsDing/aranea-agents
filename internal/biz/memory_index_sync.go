package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MemoryFactIndexSyncer projects authoritative memory_facts rows into optional pgvector indexes.
type MemoryFactIndexSyncer interface {
	SyncFactIndex(ctx context.Context, agentID, userID, factID, statement string) error
	SyncFactIndexFromRow(ctx context.Context, raw []byte) error
}

// SyncFactIndex embeds statement and upserts the pgvector read index for one fact.
// Prefer data.NewMemoryFactIndexSync for dual-write to SQLite embedding_blob + pgvector.
func (uc *MemoryUsecase) SyncFactIndex(ctx context.Context, agentID, userID, factID, statement string) error {
	if uc == nil || uc.embedder == nil || uc.repo == nil {
		return ErrMemoryUnavailable
	}
	agentID = strings.TrimSpace(agentID)
	factID = strings.TrimSpace(factID)
	statement = strings.TrimSpace(statement)
	if agentID == "" || factID == "" || statement == "" {
		return nil
	}
	vec, err := uc.embedder.Embed(ctx, statement)
	if err != nil {
		return err
	}
	return uc.UpsertFactVector(ctx, agentID, userID, factID, statement, vec)
}

// UpsertFactVector writes a precomputed embedding to the optional pgvector read index.
func (uc *MemoryUsecase) UpsertFactVector(ctx context.Context, agentID, userID, factID, statement string, embedding []float32) error {
	if uc == nil || uc.repo == nil {
		return ErrMemoryUnavailable
	}
	agentID = strings.TrimSpace(agentID)
	factID = strings.TrimSpace(factID)
	statement = strings.TrimSpace(statement)
	if agentID == "" || factID == "" || statement == "" || len(embedding) == 0 {
		return nil
	}
	return uc.repo.UpsertFactVector(ctx, agentID, userID, factID, statement, embedding)
}

// SyncFactIndexFromRow best-effort sync using a memory_facts JSON row from sessionmemory.
func (uc *MemoryUsecase) SyncFactIndexFromRow(ctx context.Context, raw []byte) error {
	if uc == nil || len(raw) == 0 {
		return ErrMemoryUnavailable
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	agentID := strings.TrimSpace(jsonStr(m, "agent_id"))
	if agentID == "" {
		agentID = strings.TrimSpace(jsonStr(m, "scope_id"))
	}
	return uc.SyncFactIndex(ctx, agentID, jsonStr(m, "user_id"), jsonStr(m, "id"), jsonStr(m, "statement"))
}

// EpisodeIndexSyncer projects episode summaries into memory_episodes.embedding_blob.
type EpisodeIndexSyncer interface {
	SyncEpisodeIndex(ctx context.Context, agentID, episodeID, title, summary string) error
}

func jsonStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}
