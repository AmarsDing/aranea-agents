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
	return biz.NewMemoryPolicyEngine(data.NewMemoryActionLogWriter(store), func(ctx context.Context) bool {
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
	return biz.NewMemoryL2RecallUsecase(data.NewSessionL2RecallStore(store), vec)
}

func provideMemoryL3Recall(store *sessionmemory.Store, vec *biz.MemoryUsecase) biz.MemoryL3Recaller {
	return biz.NewMemoryL3RecallUsecase(data.NewSessionL3RecallStore(store), data.NewL3ScoredRecallAdapter(store), vec)
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

func provideMemoryAdminUsecase(admin biz.SessionAdminStore, vec *biz.MemoryUsecase, factSync biz.MemoryFactIndexSyncer, store *sessionmemory.Store, lg loggateway.Logger) *biz.MemoryAdminUsecase {
	return biz.NewMemoryAdminUsecase(admin, vec, factSync, data.NewL3FactWriterAdapter(store), lg)
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
			Admin:           data.NewSessionAdminStoreAdapter(store),
			L2Recall:        l2Recall,
			L3Recall:        l3Recall,
			CompositeRecall: compositeRecall,
		}
	}
	var rollback rt.RunnerSessionRollbackStore
	if d != nil {
		rollback = sessiontrpc.NewRunnerRollbackStore(d.RawDB(), lg)
	}
	return rt.PersistenceSet{Session: sess, Memory: mem, AgentMCP: mcp, Artifact: artifact, ArtifactUC: artifactUC, RunnerRollback: rollback}
}


