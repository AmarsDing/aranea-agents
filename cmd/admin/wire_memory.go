package main

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	memtrpc "aranea-agents/internal/memory/trpc"
	rt "aranea-agents/internal/runtime"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func provideAutoMemoryQueue() *memtrpc.MemoryJobQueue {
	return memtrpc.NewMemoryJobQueue(256, 30*time.Second)
}

func provideMemoryPolicyEngine(store *sessionmemory.Store, sys biz.SystemSettingRepo) *biz.MemoryPolicyEngine {
	if store == nil {
		return nil
	}
	return biz.NewMemoryPolicyEngine(store, func(ctx context.Context) bool {
		return biz.ResolvePolicyStrict(ctx, sys)
	})
}

func provideFactIndexSync(vec *biz.MemoryUsecase, store *sessionmemory.Store) biz.MemoryFactIndexSyncer {
	return data.NewMemoryFactIndexSync(vec, store)
}

func provideEpisodeIndexSync(vec *biz.MemoryUsecase, store *sessionmemory.Store) biz.EpisodeIndexSyncer {
	return data.NewMemoryEpisodeIndexSync(vec, store)
}

func provideMemoryL2Recall(store *sessionmemory.Store, vec *biz.MemoryUsecase) biz.MemoryL2Recaller {
	return biz.NewMemoryL2RecallUsecase(store, vec)
}

func provideMemoryL3Recall(store *sessionmemory.Store, vec *biz.MemoryUsecase) biz.MemoryL3Recaller {
	return biz.NewMemoryL3RecallUsecase(store, vec)
}

func provideAutoMemoryEnqueuer(q memtrpc.AutoMemoryQueue) biz.AutoMemoryEnqueuer {
	return biz.AutoMemoryEnqueuerFunc(memtrpc.NewAutoMemoryEnqueuer(q))
}

func provideFeedbackMemoryEnqueuer(q memtrpc.AutoMemoryQueue) biz.FeedbackMemoryEnqueuer {
	return biz.FeedbackMemoryEnqueuerFunc(memtrpc.NewFeedbackMemoryEnqueuer(q))
}

func providePersistenceSet(
	store *sessionmemory.Store,
	mcp *biz.AgentMCPTooling,
	sess trpcsession.Service,
	artifact trpcartifact.Service,
	vec *biz.MemoryUsecase,
	q memtrpc.AutoMemoryQueue,
	policy *biz.MemoryPolicyEngine,
	factSync biz.MemoryFactIndexSyncer,
	l2Recall biz.MemoryL2Recaller,
	l3Recall biz.MemoryL3Recaller,
) rt.PersistenceSet {
	var mem rt.MemorySet
	if store != nil {
		if policy != nil {
			store.SetPolicyEngine(policy)
		}
		mem = rt.MemorySet{
			TRPC:     memtrpc.NewSQLiteMemoryService(store, factSync, q, vec),
			Admin:    newWireSessionAdminStoreAdapter(store),
			L2Recall: l2Recall,
			L3Recall: l3Recall,
		}
	}
	return rt.PersistenceSet{Session: sess, Memory: mem, AgentMCP: mcp, Artifact: artifact}
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

func (a *wireSessionAdminStoreAdapter) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	return a.inner.ListL1TaskRows(ctx, sessionID, agentID, status, includeEnded)
}

func (a *wireSessionAdminStoreAdapter) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	return a.inner.ListL1FieldRows(ctx, taskID, includeInternal)
}

func (a *wireSessionAdminStoreAdapter) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	return a.inner.ListFactRows(ctx, scopeType, scopeID, kind, status, keyword, limit, offset)
}

func (a *wireSessionAdminStoreAdapter) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return a.inner.ListFactRowsForUser(ctx, scopeType, scopeID, userID, keyword, limit, offset)
}

func (a *wireSessionAdminStoreAdapter) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32) ([][]byte, error) {
	return a.inner.RecallL3Facts(ctx, scopeType, scopeID, userID, query, queryEmbedding, limit)
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
