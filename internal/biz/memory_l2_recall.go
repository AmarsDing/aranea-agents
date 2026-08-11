package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// L2RecallQuery parameters for episodic memory retrieval.
type L2RecallQuery struct {
	AgentID   string
	SessionID string
	Query     string
	Limit     int32
	// QueryEmbedding / EmbedAttempted (P0-C, 2026-08-11): when the caller has
	// already embedded Query this turn (composite layered path), the vector is
	// passed down so this usecase does not re-embed. EmbedAttempted=true with
	// a nil vector means the upstream embed failed — degrade to non-vector
	// search without retrying.
	QueryEmbedding []float32
	EmbedAttempted bool
}

// MemoryL2Recaller performs query-aware L2 episode recall with rerank.
type MemoryL2Recaller interface {
	RecallEpisodes(ctx context.Context, q L2RecallQuery) ([][]byte, error)
}

// SessionL2RecallStore is the persistence port for L2 recall (implemented by sessionmemory.Store).
type SessionL2RecallStore interface {
	RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error)
}

// MemoryL2RecallUsecase embeds optional query vectors and delegates rerank to the store.
type MemoryL2RecallUsecase struct {
	store    SessionL2RecallStore
	embedder EmbeddingService
	lg       loggateway.Logger
}

func NewMemoryL2RecallUsecase(store SessionL2RecallStore, embedder EmbeddingService, lg loggateway.Logger) *MemoryL2RecallUsecase {
	if store == nil {
		return nil
	}
	return &MemoryL2RecallUsecase{store: store, embedder: embedder, lg: lg}
}

func (uc *MemoryL2RecallUsecase) RecallEpisodes(ctx context.Context, q L2RecallQuery) ([][]byte, error) {
	if uc == nil || uc.store == nil {
		return nil, nil
	}
	var qvec []float32
	query := strings.TrimSpace(q.Query)
	switch {
	case q.EmbedAttempted:
		// P0-C: caller already embedded (or attempted) this turn — reuse.
		qvec = q.QueryEmbedding
	case query != "" && uc.embedder != nil:
		// Short timeout (P0-C): L2 recall is on the LLM critical path.
		// Degrade to non-vector search quickly if embed is slow.
		embedCtx, cancel := context.WithTimeout(ctx, memoryRecallEmbedTimeout)
		vec, err := uc.embedder.Embed(embedCtx, query)
		cancel()
		if err == nil {
			qvec = vec
		} else {
			uc.lg.Warn("L2 recall embed failed, degrading to non-vector search",
				loggateway.StepID("memory.l2_embed_fail"),
				loggateway.Err(err))
		}
	}
	lim := q.Limit
	if lim <= 0 {
		lim = 3
	}
	return uc.store.RecallL2Episodes(ctx, strings.TrimSpace(q.AgentID), strings.TrimSpace(q.SessionID), query, qvec, lim)
}
