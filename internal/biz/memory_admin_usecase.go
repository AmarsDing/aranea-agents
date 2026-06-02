package biz

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/pkg/jsonutil"
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

func (uc *MemoryAdminUsecase) ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListPIIFlaggedFacts(ctx, scopeType, scopeID, limit, offset)
}

func (uc *MemoryAdminUsecase) ApprovePIIFact(ctx context.Context, factID string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.ApprovePIIFact(ctx, factID)
}

func (uc *MemoryAdminUsecase) RejectPIIFact(ctx context.Context, factID string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.RejectPIIFact(ctx, factID)
}

func (uc *MemoryAdminUsecase) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListL0SnapshotRows(ctx, sessionID, limit)
}

func (uc *MemoryAdminUsecase) GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.GetL0SnapshotRow(ctx, sessionID, id)
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
	// Best-effort conflict detection
	_ = uc.DetectFactConflicts(ctx, in.ScopeType, in.ScopeID, in.Statement)
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

// --- L1 Writer Methods ---

func (uc *MemoryAdminUsecase) StartL1Task(ctx context.Context, in L1TaskInsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.StartL1Task(ctx, in)
}

func (uc *MemoryAdminUsecase) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	raw, err := uc.admin.EndL1Task(ctx, sessionID, taskID, status)
	if err != nil {
		return nil, err
	}
	// Archive the task and create an L2 episode (best-effort, non-blocking).
	uc.archiveAndCreateEpisode(ctx, sessionID, taskID, raw)
	return raw, nil
}

// archiveAndCreateEpisode archives the L1 task and creates an L2 episode from the snapshot.
func (uc *MemoryAdminUsecase) archiveAndCreateEpisode(ctx context.Context, sessionID, taskID string, endTaskRaw []byte) {
	snapshot, err := uc.admin.ArchiveL1Task(ctx, sessionID, taskID)
	if err != nil {
		uc.lg.Warn("L1 archive failed after EndL1Task",
			loggateway.StepID("memory.l1_archive_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(err))
		return
	}
	m, _ := jsonutil.ParseMap(endTaskRaw)
	agentID := jsonutil.IfaceStr(m, "agent_id")
	taskTitle := jsonutil.IfaceStr(m, "task_title")
	status := jsonutil.IfaceStr(m, "status")
	if err := uc.admin.InsertL1ArchiveEpisode(ctx, L1ArchiveEpisodeInsert{
		SessionID:      sessionID,
		AgentID:        agentID,
		TaskID:         taskID,
		TaskTitle:      taskTitle,
		Status:         status,
		L1SnapshotJSON: string(snapshot),
	}); err != nil {
		uc.lg.Warn("L1 archive episode insert failed",
			loggateway.StepID("memory.l1_archive_episode_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(err))
	}
}

func (uc *MemoryAdminUsecase) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.GetL1TaskRow(ctx, sessionID, id)
}

func (uc *MemoryAdminUsecase) UpsertL1Field(ctx context.Context, in L1FieldInsert) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.UpsertL1Field(ctx, in)
}

func (uc *MemoryAdminUsecase) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.DeleteL1Field(ctx, taskID, fieldPath)
}

func (uc *MemoryAdminUsecase) GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.GetL1FieldRow(ctx, taskID, fieldPath)
}

func (uc *MemoryAdminUsecase) PatchL1Fields(ctx context.Context, fields []L1FieldInsert) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.PatchL1Fields(ctx, fields)
}

func (uc *MemoryAdminUsecase) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ArchiveL1Task(ctx, sessionID, taskID)
}

func (uc *MemoryAdminUsecase) ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListIdleL1Tasks(ctx, cutoffRFC3339)
}

func (uc *MemoryAdminUsecase) InsertL1ArchiveEpisode(ctx context.Context, in L1ArchiveEpisodeInsert) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.InsertL1ArchiveEpisode(ctx, in)
}

func (uc *MemoryAdminUsecase) ListPendingConsolidationEpisodes(ctx context.Context, agentID string, limit int) ([][]byte, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	return uc.admin.ListPendingConsolidationEpisodes(ctx, agentID, limit)
}

func (uc *MemoryAdminUsecase) MarkEpisodeConsolidated(ctx context.Context, id string, l3Count, l4Count int) error {
	if err := uc.requireAdmin(); err != nil {
		return err
	}
	return uc.admin.MarkEpisodeConsolidated(ctx, id, l3Count, l4Count)
}

func (uc *MemoryAdminUsecase) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return 0, err
	}
	return uc.admin.IncrementConflictCount(ctx, factID)
}

func (uc *MemoryAdminUsecase) ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	return uc.admin.ListConflictingFacts(ctx, scopeType, scopeID, limit, offset)
}

// DetectFactConflicts checks if a new fact statement conflicts with existing facts in the same scope.
// It uses simple keyword overlap to find potentially conflicting facts and increments their conflict_count.
func (uc *MemoryAdminUsecase) DetectFactConflicts(ctx context.Context, scopeType, scopeID, newStatement string) error {
	if uc.admin == nil {
		return nil
	}
	rows, _, _, _, err := uc.admin.ListFactRows(ctx, scopeType, scopeID, "", "", "", 10, 0)
	if err != nil || len(rows) == 0 {
		return nil
	}
	negationPatterns := []string{"not ", "don't ", "doesn't ", "never ", "no longer ", "不喜欢", "不", "没有"}
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		existing := jsonutil.IfaceStr(m, "statement")
		id := jsonutil.IfaceStr(m, "id")
		if existing == "" || id == "" {
			continue
		}
		newLower := strings.ToLower(newStatement)
		existingLower := strings.ToLower(existing)
		for _, neg := range negationPatterns {
			if strings.Contains(newLower, neg+existingLower) ||
				strings.Contains(existingLower, neg+newLower) {
				if _, err := uc.admin.IncrementConflictCount(ctx, id); err != nil {
					_ = err // best-effort
				}
				break
			}
		}
	}
	return nil
}
