package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// l3FactRepo adapts sessionmemory.Store to biz L3 interfaces.
type l3FactRepo struct {
	store *sessionmemory.Store
}

// Compile-time interface checks.
var (
	_ biz.L3FactReader   = (*l3FactRepo)(nil)
	_ biz.L3FactWriter   = (*l3FactRepo)(nil)
	_ biz.L3ConflictStore = (*l3FactRepo)(nil)
	_ biz.PIIReviewStore  = (*l3FactRepo)(nil)
)

func newL3FactRepo(store *sessionmemory.Store) *l3FactRepo {
	if store == nil {
		return nil
	}
	return &l3FactRepo{store: store}
}

// NewL3FactWriterAdapter creates a biz.L3FactWriter backed by sessionmemory.Store.
// Returns nil if store is nil.
func NewL3FactWriterAdapter(store *sessionmemory.Store) biz.L3FactWriter {
	if store == nil {
		return nil
	}
	return newL3FactRepo(store)
}

// --- L3FactReader ---

func (r *l3FactRepo) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	return r.store.ListFactRows(ctx, scopeType, scopeID, kind, status, keyword, limit, offset)
}

func (r *l3FactRepo) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return r.store.ListFactRowsForUser(ctx, scopeType, scopeID, userID, keyword, limit, offset)
}

func (r *l3FactRepo) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	return r.store.RecallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
}

// --- L3FactWriter ---

func (r *l3FactRepo) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
	return r.store.UpsertFactRow(ctx, factUpsertToStore(in))
}

func (r *l3FactRepo) DeleteFactRow(ctx context.Context, factID string) error {
	return r.store.DeleteFactByID(ctx, factID)
}

func (r *l3FactRepo) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	return r.store.DeleteFactRowsByIDs(ctx, factIDs)
}

// --- L3ConflictStore ---

func (r *l3FactRepo) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	return r.store.IncrementConflictCount(ctx, factID)
}

func (r *l3FactRepo) ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	return r.store.ListConflictingFacts(ctx, scopeType, scopeID, limit, offset)
}

// --- PIIReviewStore ---

func (r *l3FactRepo) ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	return r.store.ListPIIFlaggedFacts(ctx, scopeType, scopeID, limit, offset)
}

func (r *l3FactRepo) ApprovePIIFact(ctx context.Context, factID string) error {
	return r.store.ApprovePIIFact(ctx, factID)
}

func (r *l3FactRepo) RejectPIIFact(ctx context.Context, factID string) error {
	return r.store.RejectPIIFact(ctx, factID)
}

// factUpsertToStore converts biz.FactUpsert to sessionmemory.MemoryFactUpsert.
func factUpsertToStore(in biz.FactUpsert) sessionmemory.MemoryFactUpsert {
	return sessionmemory.MemoryFactUpsert{
		ID:                    in.ID,
		ScopeType:             in.ScopeType,
		ScopeID:               in.ScopeID,
		WorkspaceID:           in.WorkspaceID,
		UserID:                in.UserID,
		TeamID:                in.TeamID,
		AgentID:               in.AgentID,
		Statement:             in.Statement,
		Fingerprint:           in.Fingerprint,
		DetailsMarkdown:       in.DetailsMarkdown,
		FactKind:              in.FactKind,
		TagsJSON:              in.TagsJSON,
		Confidence:            in.Confidence,
		Importance:            in.Importance,
		UseCount:              in.UseCount,
		HitCount:              in.HitCount,
		PositiveFeedbackCount: in.PositiveFeedbackCount,
		NegativeFeedbackCount: in.NegativeFeedbackCount,
		ConflictCount:         in.ConflictCount,
		SourceKind:            in.SourceKind,
		SourceEpisodeID:       in.SourceEpisodeID,
		SourceSessionID:       in.SourceSessionID,
		SourceMessageID:       in.SourceMessageID,
		SourceExternal:        in.SourceExternal,
		Version:               in.Version,
		Status:                in.Status,
		PIIFlag:               in.PIIFlag,
		MetadataJSON:          in.MetadataJSON,
		CreatedAt:             in.CreatedAt,
		UpdatedAt:             in.UpdatedAt,
	}
}
