package data

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/userembeddingsetting"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"
)

type memoryRepo struct {
	data *Data

	mu    sync.RWMutex
	store map[int]vector.FactVectorStore
}

var _ biz.MemoryRepo = (*memoryRepo)(nil)

type unavailableMemoryRepo struct{}

func NewMemoryRepo(d *Data) biz.MemoryRepo {
	if d == nil || d.Postgres() == nil {
		return unavailableMemoryRepo{}
	}
	return &memoryRepo{
		data:  d,
		store: make(map[int]vector.FactVectorStore),
	}
}

func (unavailableMemoryRepo) Insert(context.Context, *biz.AgentMemory) error {
	return biz.ErrMemoryUnavailable
}

func (unavailableMemoryRepo) FindSimilar(context.Context, string, []float32, int) ([]*biz.AgentMemory, error) {
	return nil, biz.ErrMemoryUnavailable
}

func (unavailableMemoryRepo) FindSimilarWithUser(context.Context, string, string, []float32, int) ([]*biz.AgentMemory, error) {
	return nil, biz.ErrMemoryUnavailable
}

func (unavailableMemoryRepo) UpsertFactVector(context.Context, string, string, string, string, []float32) error {
	return biz.ErrMemoryUnavailable
}

func (r *memoryRepo) dimForEmbedding(ctx context.Context, memoryPartitionUserID string) (int, error) {
	if memoryPartitionUserID == "" {
		return r.data.VectorDim(), nil
	}
	row, err := r.data.RW().Read(ctx).UserEmbeddingSetting.Query().
		Where(userembeddingsetting.UserID(memoryPartitionUserID)).
		Only(ctx)
	switch {
	case dataent.IsNotFound(err):
		return r.data.VectorDim(), nil
	case err != nil:
		return 0, err
	default:
		return row.VectorDimension, nil
	}
}

func (r *memoryRepo) storeFor(ctx context.Context, dim int) (vector.FactVectorStore, error) {
	r.mu.RLock()
	st, ok := r.store[dim]
	r.mu.RUnlock()
	if ok {
		return st, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// double-check
	if st, ok := r.store[dim]; ok {
		return st, nil
	}
	if err := vector.EnsureDimensionTable(ctx, r.data.Postgres(), dim); err != nil {
		r.data.lg.Warn("ensure dimension table failed", loggateway.StepID("memory.pgvector_init_fail"), loggateway.Err(err))
		return nil, err
	}
	s, err := vector.NewPgVectorFactStore(r.data.Postgres(), dim)
	if err != nil {
		return nil, err
	}
	r.store[dim] = s
	return s, nil
}

func (r *memoryRepo) Insert(ctx context.Context, m *biz.AgentMemory) error {
	if m == nil {
		return biz.ErrMemoryUnavailable
	}
	dim, err := r.dimForEmbedding(ctx, m.UserID)
	if err != nil {
		return err
	}
	st, err := r.storeFor(ctx, dim)
	if err != nil {
		return err
	}
	// Use UpsertFact with agentID as the vector ID for L0/L1 memory rows.
	// The id parameter is used as a deduplication key within the fact content prefix protocol.
	return st.UpsertFact(ctx, m.AgentID, m.AgentID, m.UserID, m.Content, float32To64(m.Embedding))
}

func (r *memoryRepo) FindSimilar(ctx context.Context, agentID string, query []float32, topK int) ([]*biz.AgentMemory, error) {
	return r.FindSimilarWithUser(ctx, agentID, "", query, topK)
}

func (r *memoryRepo) FindSimilarWithUser(ctx context.Context, agentID, userID string, query []float32, topK int) ([]*biz.AgentMemory, error) {
	dim, err := r.dimForEmbedding(ctx, userID)
	if err != nil {
		return nil, err
	}
	st, err := r.storeFor(ctx, dim)
	if err != nil {
		return nil, err
	}
	hits, err := st.SearchByAgent(ctx, agentID, userID, float32To64(query), topK, 0)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.AgentMemory, len(hits))
	for i := range hits {
		h := hits[i]
		score := h.Score
		if score < 0 {
			score = 0
		}
		if score > 1.0 {
			score = 1.0
		}
		out[i] = &biz.AgentMemory{
			AgentID: agentID,
			Score:   score,
		}
	}
	return out, nil
}

func (r *memoryRepo) UpsertFactVector(ctx context.Context, agentID, userID, factID, statement string, embedding []float32) error {
	dim, err := r.dimForEmbedding(ctx, userID)
	if err != nil {
		return err
	}
	st, err := r.storeFor(ctx, dim)
	if err != nil {
		return err
	}
	return st.UpsertFact(ctx, factID, agentID, userID, statement, float32To64(embedding))
}

// float32To64 converts a []float32 slice to []float64.
func float32To64(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, f := range v {
		out[i] = float64(f)
	}
	return out
}
