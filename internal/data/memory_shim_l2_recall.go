package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
)

// l2RecallRepo implements biz.SessionL2RecallStore using direct Raw SQL.
type l2RecallRepo struct {
	data        *Data
	vectorStore vector.VectorStore
}

func newL2RecallRepo(data *Data, vs vector.VectorStore) *l2RecallRepo {
	if data == nil {
		return nil
	}
	return &l2RecallRepo{data: data, vectorStore: vs}
}

// NewSessionL2RecallStore creates a biz.SessionL2RecallStore backed by data.
func NewSessionL2RecallStore(data *Data, vs vector.VectorStore) biz.SessionL2RecallStore {
	if data == nil {
		return nil
	}
	return newL2RecallRepo(data, vs)
}

// Compile-time interface check.
var _ biz.SessionL2RecallStore = (*l2RecallRepo)(nil)

func (r *l2RecallRepo) RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	l2 := newL2EpisodeRepo(r.data, r.vectorStore)
	return l2.RecallL2Episodes(ctx, agentID, sessionID, query, queryEmbedding, limit)
}
