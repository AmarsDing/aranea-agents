package data

import (
	"context"
	"sync"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/userembeddingsetting"
	"aranea-agents/internal/data/pgvector"
	"aranea-agents/pkg/loggateway"
)

type memoryRepo struct {
	data *Data

	mu    sync.RWMutex
	store map[int]*pgvector.Store
}

var _ biz.MemoryRepo = (*memoryRepo)(nil)

type unavailableMemoryRepo struct{}

func NewMemoryRepo(d *Data) biz.MemoryRepo {
	if d == nil || d.Postgres() == nil {
		return unavailableMemoryRepo{}
	}
	return &memoryRepo{
		data:  d,
		store: make(map[int]*pgvector.Store),
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
	row, err := r.data.ReadEnt().UserEmbeddingSetting.Query().
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

func (r *memoryRepo) storeFor(ctx context.Context, dim int) (*pgvector.Store, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.store[dim]; s != nil {
		return s, nil
	}
	if err := pgvector.EnsureDimensionTable(ctx, r.data.Postgres(), dim); err != nil {
		r.data.lg.Warn("ensure dimension table failed", loggateway.StepID("memory.pgvector_init_fail"), loggateway.Err(err))
		return nil, err
	}
	s := pgvector.NewStore(r.data.Postgres(), dim)
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
	return st.Insert(ctx, m.AgentID, m.UserID, m.Content, m.Embedding)
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
	rows, err := st.SearchNearest(ctx, agentID, userID, query, topK)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.AgentMemory, len(rows))
	for i := range rows {
		row := rows[i]
		// Convert cosine distance to cosine similarity (1 - distance)
		score := 1.0 - row.Distance
		// Clamp score to [0, 1] range
		if score < 0 {
			score = 0
		}
		if score > 1.0 {
			score = 1.0
		}
		out[i] = &biz.AgentMemory{
			ID:        row.ID,
			AgentID:   row.AgentID,
			UserID:    row.UserID,
			Content:   row.Content,
			Score:     score,
			CreatedAt: row.CreatedAt,
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
	return st.UpsertFactVector(ctx, agentID, userID, factID, statement, embedding)
}
