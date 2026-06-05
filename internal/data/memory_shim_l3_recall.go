package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
)

// l3RecallRepo implements biz.SessionL3RecallStore using direct Raw SQL.
type l3RecallRepo struct {
	data        *Data
	vectorStore vector.VectorStore
}

func newL3RecallRepo(data *Data, vs vector.VectorStore) *l3RecallRepo {
	if data == nil {
		return nil
	}
	return &l3RecallRepo{data: data, vectorStore: vs}
}

// NewSessionL3RecallStore creates a biz.SessionL3RecallStore backed by data.
func NewSessionL3RecallStore(data *Data, vs vector.VectorStore) biz.SessionL3RecallStore {
	if data == nil {
		return nil
	}
	return newL3RecallRepo(data, vs)
}

// Compile-time interface check.
var _ biz.SessionL3RecallStore = (*l3RecallRepo)(nil)

func (r *l3RecallRepo) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	l3 := newL3FactRepo(r.data, r.vectorStore)
	return l3.RecallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
}
