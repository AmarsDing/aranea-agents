package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

type l4GraphRepo struct {
	store *sessionmemory.Store
}

func NewL4GraphRepo(store *sessionmemory.Store) biz.L4GraphRepo {
	if store == nil {
		return nil
	}
	return &l4GraphRepo{store: store}
}

func (r *l4GraphRepo) UpsertEntity(ctx context.Context, params biz.L4EntityWrite) error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.UpsertEventEntity(ctx, sessionmemory.EventEntityParams{
		ID:               params.ID,
		ScopeType:        params.ScopeType,
		ScopeID:          params.ScopeID,
		UserID:           params.UserID,
		EntityType:       params.EntityType,
		Name:             params.Name,
		NameNormalized:   params.NameNormalized,
		Description:      params.Description,
		Importance:       params.Importance,
		Confidence:       params.Confidence,
		MetadataJSON:     params.MetadataJSON,
	})
}

func (r *l4GraphRepo) UpsertRelation(ctx context.Context, params biz.L4RelationWrite) error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.UpsertRelation(ctx, sessionmemory.RelationParams{
		ScopeType:    params.ScopeType,
		ScopeID:      params.ScopeID,
		SourceID:     params.SourceID,
		TargetID:     params.TargetID,
		RelationType: params.RelationType,
		Weight:       params.Weight,
		Confidence:   params.Confidence,
	})
}

func (r *l4GraphRepo) GetEntityByScopeKey(ctx context.Context, scopeType, scopeID, entityType, nameNormalized string) (biz.L4EntitySnapshot, bool, error) {
	if r == nil || r.store == nil {
		return biz.L4EntitySnapshot{}, false, nil
	}
	snap, ok, err := r.store.GetEntityByScopeKey(ctx, scopeType, scopeID, entityType, nameNormalized)
	if err != nil || !ok {
		return biz.L4EntitySnapshot{}, ok, err
	}
	return biz.L4EntitySnapshot{
		ID:             snap.ID,
		Name:           snap.Name,
		NameNormalized: snap.NameNormalized,
		Confidence:     snap.Confidence,
		MetadataJSON:   snap.MetadataJSON,
	}, true, nil
}

func (r *l4GraphRepo) GetFirstEntityByType(ctx context.Context, scopeType, scopeID, entityType string) (biz.L4EntitySnapshot, bool, error) {
	if r == nil || r.store == nil {
		return biz.L4EntitySnapshot{}, false, nil
	}
	snap, ok, err := r.store.GetFirstEntityByType(ctx, scopeType, scopeID, entityType)
	if err != nil || !ok {
		return biz.L4EntitySnapshot{}, ok, err
	}
	return biz.L4EntitySnapshot{
		ID:             snap.ID,
		Name:           snap.Name,
		NameNormalized: snap.NameNormalized,
		Confidence:     snap.Confidence,
		MetadataJSON:   snap.MetadataJSON,
	}, true, nil
}

func (r *l4GraphRepo) ApplyConfidenceDecay(ctx context.Context, scopeType, scopeID, olderThanRFC3339 string, factor float64) (int64, error) {
	if r == nil || r.store == nil {
		return 0, nil
	}
	return r.store.ApplyConfidenceDecay(ctx, scopeType, scopeID, olderThanRFC3339, factor)
}

type l4GraphWriterAdapter struct {
	uc *biz.L4GraphUsecase
}

func NewL4GraphWriterAdapter(uc *biz.L4GraphUsecase) biz.L4GraphWriter {
	if uc == nil {
		return nil
	}
	return &l4GraphWriterAdapter{uc: uc}
}

func (a *l4GraphWriterAdapter) WriteFromUserText(ctx context.Context, agentID, userID, text string) (int, error) {
	return a.uc.WriteFromUserText(ctx, agentID, userID, text)
}

func NewL4GraphUsecaseFromStore(store *sessionmemory.Store) *biz.L4GraphUsecase {
	repo := NewL4GraphRepo(store)
	return biz.NewL4GraphUsecase(repo)
}
