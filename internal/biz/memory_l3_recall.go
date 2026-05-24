package biz

import (
	"context"
	"strings"
)

// RecallScoreBreakdown is the component-wise recall ranking for one hit.
type RecallScoreBreakdown struct {
	Keyword      float64
	Vector       float64
	Importance   float64
	Recency      float64
	SessionBoost float64
	CrossEncoder float64
	Total        float64
}

// RecallHit is one typed recall result (progressive replacement for raw JSON rows).
type RecallHit struct {
	Layer     string
	ID        string
	Title     string
	Summary   string
	Statement string
	Raw       []byte
	Scores    RecallScoreBreakdown
}

// L3RecallQuery parameters for semantic fact retrieval.
type L3RecallQuery struct {
	ScopeType string
	ScopeID   string
	UserID    string
	Query     string
	Limit     int32
}

// MemoryL3Recaller performs query-aware L3 fact recall with rerank.
type MemoryL3Recaller interface {
	RecallFacts(ctx context.Context, q L3RecallQuery) ([][]byte, error)
}

// SessionL3RecallStore is the persistence port for L3 recall (implemented by sessionmemory.Store).
type SessionL3RecallStore interface {
	RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32) ([][]byte, error)
}

// MemoryL3RecallUsecase embeds optional query vectors and delegates rerank to the store.
type MemoryL3RecallUsecase struct {
	store    SessionL3RecallStore
	embedder EmbeddingService
}

func NewMemoryL3RecallUsecase(store SessionL3RecallStore, embedder EmbeddingService) *MemoryL3RecallUsecase {
	if store == nil {
		return nil
	}
	return &MemoryL3RecallUsecase{store: store, embedder: embedder}
}

func (uc *MemoryL3RecallUsecase) RecallFacts(ctx context.Context, q L3RecallQuery) ([][]byte, error) {
	if uc == nil || uc.store == nil {
		return nil, nil
	}
	var qvec []float32
	query := strings.TrimSpace(q.Query)
	if query != "" && uc.embedder != nil {
		if vec, err := uc.embedder.Embed(ctx, query); err == nil {
			qvec = vec
		}
	}
	lim := q.Limit
	if lim <= 0 {
		lim = 12
	}
	return uc.store.RecallL3Facts(ctx, strings.TrimSpace(q.ScopeType), strings.TrimSpace(q.ScopeID), strings.TrimSpace(q.UserID), query, qvec, lim)
}
