package main

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	memtrpc "aranea-agents/internal/memory/trpc"
	rt "aranea-agents/internal/runtime"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/loggateway"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func provideAutoMemoryQueue(lg loggateway.Logger) *memtrpc.MemoryJobQueue {
	return memtrpc.NewMemoryJobQueue(256, 30*time.Second, lg)
}

func provideMemoryPolicyEngine(store *sessionmemory.Store, sys biz.SystemSettingRepo) *biz.MemoryPolicyEngine {
	if store == nil {
		return nil
	}
	return biz.NewMemoryPolicyEngine(store, func(ctx context.Context) bool {
		return biz.ResolvePolicyStrict(ctx, sys)
	})
}

func provideFactIndexSync(vec *biz.MemoryUsecase, store *sessionmemory.Store, lg loggateway.Logger) biz.MemoryFactIndexSyncer {
	return data.NewMemoryFactIndexSync(vec, store, lg)
}

func provideEpisodeIndexSync(vec *biz.MemoryUsecase, store *sessionmemory.Store) biz.EpisodeIndexSyncer {
	return data.NewMemoryEpisodeIndexSync(vec, store)
}

func provideMemoryL2Recall(store *sessionmemory.Store, vec *biz.MemoryUsecase) biz.MemoryL2Recaller {
	return biz.NewMemoryL2RecallUsecase(store, vec)
}

func provideMemoryL3Recall(store *sessionmemory.Store, vec *biz.MemoryUsecase) biz.MemoryL3Recaller {
	return biz.NewMemoryL3RecallUsecase(store, data.NewL3ScoredRecallAdapter(store), vec)
}

func provideAutoMemoryEnqueuer(q memtrpc.AutoMemoryQueue) biz.AutoMemoryEnqueuer {
	return biz.AutoMemoryEnqueuerFunc(memtrpc.NewAutoMemoryEnqueuer(q))
}

func provideFeedbackMemoryEnqueuer(q memtrpc.AutoMemoryQueue) biz.FeedbackMemoryEnqueuer {
	return biz.FeedbackMemoryEnqueuerFunc(memtrpc.NewFeedbackMemoryEnqueuer(q))
}

func provideMemoryCompositeRecall(store *sessionmemory.Store) biz.MemoryCompositeRecaller {
	return biz.NewMemoryCompositeRecallUsecase(data.NewMemoryCompositeRecallAdapter(store))
}

func providePersistenceSet(
	d *data.Data,
	store *sessionmemory.Store,
	mcp *biz.AgentMCPTooling,
	sess trpcsession.Service,
	artifact trpcartifact.Service,
	artifactUC *biz.ArtifactUsecase,
	agentsUC *biz.AgentUsecase,
	vec *biz.MemoryUsecase,
	q memtrpc.AutoMemoryQueue,
	policy *biz.MemoryPolicyEngine,
	factSync biz.MemoryFactIndexSyncer,
	l2Recall biz.MemoryL2Recaller,
	l3Recall biz.MemoryL3Recaller,
	compositeRecall biz.MemoryCompositeRecaller,
	lg loggateway.Logger,
) rt.PersistenceSet {
	var mem rt.MemorySet
	if store != nil {
		if policy != nil {
			store.SetPolicyEngine(policy)
		}
		mem = rt.MemorySet{
			TRPC:            memtrpc.NewSQLiteMemoryService(store, factSync, q, vec, memtrpc.NewAgentRuntimeSettingsLoader(agentsUC), lg),
			Admin:           newWireSessionAdminStoreAdapter(store),
			L2Recall:        l2Recall,
			L3Recall:        l3Recall,
			CompositeRecall: compositeRecall,
		}
	}
	var rollback rt.RunnerSessionRollbackStore
	if d != nil {
		rollback = sessiontrpc.NewRunnerRollbackStore(d.RawDB())
	}
	return rt.PersistenceSet{Session: sess, Memory: mem, AgentMCP: mcp, Artifact: artifact, ArtifactUC: artifactUC, RunnerRollback: rollback}
}

type wireSessionAdminStoreAdapter struct {
	inner *sessionmemory.Store
}

func newWireSessionAdminStoreAdapter(store *sessionmemory.Store) biz.SessionAdminStore {
	if store == nil {
		return nil
	}
	return &wireSessionAdminStoreAdapter{inner: store}
}

func (a *wireSessionAdminStoreAdapter) ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error) {
	return a.inner.ListL0SnapshotRows(ctx, sessionID, limit)
}

func (a *wireSessionAdminStoreAdapter) GetL0SnapshotRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	return a.inner.GetL0SnapshotRow(ctx, sessionID, id)
}

func (a *wireSessionAdminStoreAdapter) InsertL0AssemblySnapshot(ctx context.Context, in biz.L0AssemblySnapshotInsert) error {
	return a.inner.InsertL0AssemblySnapshot(ctx, in)
}

func (a *wireSessionAdminStoreAdapter) UpdateL0SnapshotActual(ctx context.Context, id string, actualPromptTokens, contextWindowTokens int) error {
	return a.inner.UpdateL0SnapshotActual(ctx, id, actualPromptTokens, contextWindowTokens)
}

func (a *wireSessionAdminStoreAdapter) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	return a.inner.ListL1TaskRows(ctx, sessionID, agentID, status, includeEnded)
}

func (a *wireSessionAdminStoreAdapter) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	return a.inner.ListL1FieldRows(ctx, taskID, includeInternal)
}

func (a *wireSessionAdminStoreAdapter) StartL1Task(ctx context.Context, in biz.L1TaskInsert) ([]byte, error) {
	return a.inner.StartL1Task(ctx, in)
}

func (a *wireSessionAdminStoreAdapter) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	return a.inner.EndL1Task(ctx, sessionID, taskID, status)
}

func (a *wireSessionAdminStoreAdapter) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	return a.inner.GetL1TaskRow(ctx, sessionID, id)
}

func (a *wireSessionAdminStoreAdapter) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) ([]byte, error) {
	return a.inner.UpsertL1Field(ctx, in)
}

func (a *wireSessionAdminStoreAdapter) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	return a.inner.DeleteL1Field(ctx, taskID, fieldPath)
}

func (a *wireSessionAdminStoreAdapter) GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error) {
	return a.inner.GetL1FieldRow(ctx, taskID, fieldPath)
}

func (a *wireSessionAdminStoreAdapter) PatchL1Fields(ctx context.Context, fields []biz.L1FieldInsert) ([][]byte, error) {
	return a.inner.PatchL1Fields(ctx, fields)
}

func (a *wireSessionAdminStoreAdapter) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	return a.inner.ArchiveL1Task(ctx, sessionID, taskID)
}

func (a *wireSessionAdminStoreAdapter) ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error) {
	return a.inner.ListIdleL1Tasks(ctx, cutoffRFC3339)
}

func (a *wireSessionAdminStoreAdapter) InsertL1ArchiveEpisode(ctx context.Context, in biz.L1ArchiveEpisodeInsert) error {
	return a.inner.InsertL1ArchiveEpisode(ctx, in)
}

func (a *wireSessionAdminStoreAdapter) ListPendingConsolidationEpisodes(ctx context.Context, agentID string, limit int) ([][]byte, error) {
	return a.inner.ListPendingConsolidationEpisodes(ctx, agentID, limit)
}

func (a *wireSessionAdminStoreAdapter) MarkEpisodeConsolidated(ctx context.Context, id string, l3Count, l4Count int) error {
	return a.inner.MarkEpisodeConsolidated(ctx, id, l3Count, l4Count)
}

func (a *wireSessionAdminStoreAdapter) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	return a.inner.ListFactRows(ctx, scopeType, scopeID, kind, status, keyword, limit, offset)
}

func (a *wireSessionAdminStoreAdapter) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return a.inner.ListFactRowsForUser(ctx, scopeType, scopeID, userID, keyword, limit, offset)
}

func (a *wireSessionAdminStoreAdapter) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	return a.inner.RecallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit, minScore)
}

func (a *wireSessionAdminStoreAdapter) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	return a.inner.IncrementConflictCount(ctx, factID)
}

func (a *wireSessionAdminStoreAdapter) ListConflictingFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	return a.inner.ListConflictingFacts(ctx, scopeType, scopeID, limit, offset)
}

func (a *wireSessionAdminStoreAdapter) ListEpisodeRowsForRecall(ctx context.Context, agentID, sessionID string, limit int32) ([][]byte, error) {
	return a.inner.ListEpisodeRowsForRecall(ctx, agentID, sessionID, limit)
}

func (a *wireSessionAdminStoreAdapter) RecallL2Episodes(ctx context.Context, agentID, sessionID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	return a.inner.RecallL2Episodes(ctx, agentID, sessionID, query, queryEmbedding, limit)
}

func (a *wireSessionAdminStoreAdapter) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	return a.inner.ListEntityRows(ctx, scopeType, scopeID, workspaceID, userID, entityType, status, keyword, limit, offset)
}

func (a *wireSessionAdminStoreAdapter) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	return a.inner.NeighborhoodJSON(ctx, centerID, hops, maxNodes, queryAtRFC3339)
}

func (a *wireSessionAdminStoreAdapter) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	return a.inner.AgentIdentityJSON(ctx, agentID)
}

func (a *wireSessionAdminStoreAdapter) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	return a.inner.AgentStrategyJSON(ctx, agentID)
}

func (a *wireSessionAdminStoreAdapter) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	return a.inner.EvolutionProposalRows(ctx, agentID, status, limit)
}

func (a *wireSessionAdminStoreAdapter) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	return a.inner.EvolutionEventRows(ctx, agentID, limit)
}

func (a *wireSessionAdminStoreAdapter) EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error) {
	return a.inner.EvolutionMetricsJSON(ctx, agentID)
}

func (a *wireSessionAdminStoreAdapter) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
	return a.inner.UpsertFactRow(ctx, wireFactUpsertToStore(in))
}

func (a *wireSessionAdminStoreAdapter) InsertEvolutionEventRow(ctx context.Context, in biz.EvolutionEventInsert) ([]byte, error) {
	return a.inner.InsertEvolutionEventRow(ctx, wireEvolutionEventInsertToStore(in))
}

func (a *wireSessionAdminStoreAdapter) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	return a.inner.DeleteSessionEventEntities(ctx, sessionID)
}

func (a *wireSessionAdminStoreAdapter) ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID string, limit, offset int32) ([][]byte, int32, error) {
	return a.inner.ListPIIFlaggedFacts(ctx, scopeType, scopeID, limit, offset)
}

func (a *wireSessionAdminStoreAdapter) ApprovePIIFact(ctx context.Context, factID string) error {
	return a.inner.ApprovePIIFact(ctx, factID)
}

func (a *wireSessionAdminStoreAdapter) RejectPIIFact(ctx context.Context, factID string) error {
	return a.inner.RejectPIIFact(ctx, factID)
}

func wireFactUpsertToStore(in biz.FactUpsert) sessionmemory.MemoryFactUpsert {
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

func wireEvolutionEventInsertToStore(in biz.EvolutionEventInsert) sessionmemory.EvolutionEventInsert {
	return sessionmemory.EvolutionEventInsert{
		AgentID:       in.AgentID,
		WorkspaceID:   in.WorkspaceID,
		EventKind:     in.EventKind,
		Kind:          in.Kind,
		TargetField:   in.TargetField,
		Reason:        in.Reason,
		TriggerKind:   in.TriggerKind,
		TriggerSource: in.TriggerSource,
		MetadataJSON:  in.MetadataJSON,
	}
}
