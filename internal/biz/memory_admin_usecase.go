package biz

import (
	"context"
	"errors"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type L3FactWriter interface {
	DeleteFactRow(ctx context.Context, factID string) error
	DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error)
}

type MemoryAdminUsecase struct {
	admin     SessionAdminStore
	vec       *MemoryUsecase
	indexSync MemoryFactIndexSyncer
	factWriter L3FactWriter
	lg        loggateway.Logger
}

func NewMemoryAdminUsecase(admin SessionAdminStore, vec *MemoryUsecase, indexSync MemoryFactIndexSyncer, factWriter L3FactWriter, lg loggateway.Logger) *MemoryAdminUsecase {
	if admin == nil && vec == nil {
		return nil
	}
	return &MemoryAdminUsecase{admin: admin, vec: vec, indexSync: indexSync, factWriter: factWriter, lg: lg}
}

func (uc *MemoryAdminUsecase) Vector() *MemoryUsecase { return uc.vec }

func (uc *MemoryAdminUsecase) requireAdmin() error {
	if uc == nil || uc.admin == nil {
		return kerrors.InternalServer("MEMORY", "session admin store not wired")
	}
	return nil
}

func (uc *MemoryAdminUsecase) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListL0SnapshotRows(ctx, sessionID, limit)
}

func (uc *MemoryAdminUsecase) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListL1TaskRows(ctx, sessionID, agentID, status, includeEnded)
}

func (uc *MemoryAdminUsecase) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListL1FieldRows(ctx, taskID, includeInternal)
}

func (uc *MemoryAdminUsecase) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, 0, 0, err
	}
	return uc.admin.ListFactRows(ctx, scopeType, scopeID, kind, status, keyword, limit, offset)
}

func (uc *MemoryAdminUsecase) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListEntityRows(ctx, scopeType, scopeID, workspaceID, userID, entityType, status, keyword, limit, offset)
}

func (uc *MemoryAdminUsecase) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.NeighborhoodJSON(ctx, centerID, hops, maxNodes, queryAtRFC3339)
}

func (uc *MemoryAdminUsecase) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.AgentIdentityJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.AgentStrategyJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.EvolutionProposalRows(ctx, agentID, status, limit)
}

func (uc *MemoryAdminUsecase) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.EvolutionEventRows(ctx, agentID, limit)
}

func (uc *MemoryAdminUsecase) EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.EvolutionMetricsJSON(ctx, agentID)
}

func (uc *MemoryAdminUsecase) UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	raw, err := uc.admin.UpsertFactRow(ctx, in)
	if err != nil {
		return nil, err
	}
	uc.syncFactIndexBestEffort(ctx, raw)
	return raw, nil
}

func (uc *MemoryAdminUsecase) syncFactIndexBestEffort(ctx context.Context, raw []byte) {
	if uc == nil || len(raw) == 0 {
		return
	}
	syncer := uc.indexSync
	if syncer == nil {
		syncer = uc.vec
	}
	if syncer == nil {
		return
	}
	if err := syncer.SyncFactIndexFromRow(ctx, raw); err != nil && !errors.Is(err, ErrMemoryUnavailable) {
		uc.lg.Warn("syncFactIndexBestEffort failed", loggateway.StepID("memory.l4_fail"), loggateway.Err(err))
	}
}

func (uc *MemoryAdminUsecase) InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.InsertEvolutionEventRow(ctx, in)
}

func (uc *MemoryAdminUsecase) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.DeleteSessionEventEntities(ctx, sessionID)
}

func (uc *MemoryAdminUsecase) DeleteFactRow(ctx context.Context, factID string) error {
	if uc == nil || uc.factWriter == nil {
		return kerrors.InternalServer("MEMORY", "fact writer not wired")
	}
	return uc.factWriter.DeleteFactRow(ctx, factID)
}

func (uc *MemoryAdminUsecase) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	if uc == nil || uc.factWriter == nil {
		return 0, kerrors.InternalServer("MEMORY", "fact writer not wired")
	}
	return uc.factWriter.DeleteFactRowsByIDs(ctx, factIDs)
}
