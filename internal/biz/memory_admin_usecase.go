package biz

import "context"

type MemoryAdminUsecase struct {
	admin SessionAdminStore
	vec   *MemoryUsecase
}

func NewMemoryAdminUsecase(admin SessionAdminStore, vec *MemoryUsecase) *MemoryAdminUsecase {
	if admin == nil && vec == nil {
		return nil
	}
	return &MemoryAdminUsecase{admin: admin, vec: vec}
}

func (uc *MemoryAdminUsecase) Vector() *MemoryUsecase { return uc.vec }

func (uc *MemoryAdminUsecase) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.ListL0SnapshotRows(ctx, sessionID, limit)
}

func (uc *MemoryAdminUsecase) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.ListL1TaskRows(ctx, sessionID, agentID, status, includeEnded)
}

func (uc *MemoryAdminUsecase) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.ListL1FieldRows(ctx, taskID, includeInternal)
}

func (uc *MemoryAdminUsecase) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	if uc == nil || uc.admin == nil {
		return nil, 0, 0, 0, nil
	}
	return uc.admin.ListFactRows(ctx, scopeType, scopeID, kind, status, keyword, limit, offset)
}

func (uc *MemoryAdminUsecase) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	if uc == nil || uc.admin == nil {
		return nil, 0, nil
	}
	return uc.admin.ListEntityRows(ctx, scopeType, scopeID, workspaceID, userID, entityType, status, keyword, limit, offset)
}

func (uc *MemoryAdminUsecase) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32) ([]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.NeighborhoodJSON(ctx, centerID, hops, maxNodes)
}

func (uc *MemoryAdminUsecase) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.AgentIdentityJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.AgentStrategyJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.EvolutionProposalRows(ctx, agentID, status, limit)
}

func (uc *MemoryAdminUsecase) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.EvolutionEventRows(ctx, agentID, limit)
}

func (uc *MemoryAdminUsecase) EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.EvolutionMetricsJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.UpsertFactRow(ctx, in)
}

func (uc *MemoryAdminUsecase) InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error) {
	if uc == nil || uc.admin == nil {
		return nil, nil
	}
	return uc.admin.InsertEvolutionEventRow(ctx, in)
}

func (uc *MemoryAdminUsecase) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	if uc == nil || uc.admin == nil {
		return nil
	}
	return uc.admin.DeleteSessionEventEntities(ctx, sessionID)
}
