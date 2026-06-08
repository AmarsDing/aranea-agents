package main

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	memtrpc "aranea-agents/internal/memory/trpc"
	rt "aranea-agents/internal/runtime"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/loggateway"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func provideAutoMemoryQueue(runtimeConf *conf.Runtime, lg loggateway.Logger) *memtrpc.MemoryJobQueue {
	return memtrpc.NewMemoryJobQueue(runtimeConf, 256, 30*time.Second, lg)
}

func provideMemoryPolicyEngine(d *data.Data, sys biz.SystemSettingRepo) *biz.MemoryPolicyEngine {
	if d == nil {
		return nil
	}
	return biz.NewMemoryPolicyEngine(data.NewMemoryActionLogWriter(d), func(ctx context.Context) bool {
		return biz.ResolvePolicyStrict(ctx, sys)
	})
}

func provideFactIndexSync(vec *biz.MemoryUsecase, d *data.Data, lg loggateway.Logger) biz.MemoryFactIndexSyncer {
	return data.NewMemoryFactIndexSync(vec, d, lg)
}

func provideEpisodeIndexSync(vec *biz.MemoryUsecase, d *data.Data) biz.EpisodeIndexSyncer {
	return data.NewMemoryEpisodeIndexSync(vec, d)
}

func provideMemoryL2Recall(d *data.Data, vec *biz.MemoryUsecase) biz.MemoryL2Recaller {
	return biz.NewMemoryL2RecallUsecase(data.NewSessionL2RecallStore(d, d.VectorStore()), vec)
}

func provideMemoryL3Recall(d *data.Data, vec *biz.MemoryUsecase) biz.MemoryL3Recaller {
	return biz.NewMemoryL3RecallUsecase(data.NewSessionL3RecallStore(d, d.VectorStore()), data.NewL3ScoredRecallAdapter(d), vec)
}

func provideAutoMemoryEnqueuer(q memtrpc.AutoMemoryQueue) biz.AutoMemoryEnqueuer {
	return biz.AutoMemoryEnqueuerFunc(memtrpc.NewAutoMemoryEnqueuer(q))
}

func provideFeedbackMemoryEnqueuer(q memtrpc.AutoMemoryQueue) biz.FeedbackMemoryEnqueuer {
	return biz.FeedbackMemoryEnqueuerFunc(memtrpc.NewFeedbackMemoryEnqueuer(q))
}

func provideMemoryCompositeRecall(d *data.Data) biz.MemoryCompositeRecaller {
	return biz.NewMemoryCompositeRecallUsecase(data.NewMemoryCompositeRecallAdapter(d))
}

func provideMemoryAdminUsecase(admin biz.SessionAdminStore, vec *biz.MemoryUsecase, factSync biz.MemoryFactIndexSyncer, d *data.Data, lg loggateway.Logger) *biz.MemoryAdminUsecase {
	return biz.NewMemoryAdminUsecase(admin, vec, factSync, data.NewL3FactWriterAdapter(d, d.VectorStore()), lg)
}

// providePathBExtractor creates a PathBExtractor and injects it into MemoryAdminUsecase.
// This breaks the dependency cycle: MemoryAdminUsecase → PathBExtractor → EnhancedTextExtractor → SessionUsecase → … → MemoryAdminUsecase.
func providePathBExtractor(extractor biz.EnhancedTextExtractor, l4 biz.L4EntityWriter, adminUC *biz.MemoryAdminUsecase, d *data.Data, lg loggateway.Logger) *biz.PathBExtractor {
	pe := biz.NewPathBExtractor(extractor, l4, lg)
	if adminUC != nil {
		adminUC.SetPathBExtractor(pe, data.NewRecentMessageLister(d))
	}
	return pe
}

// provideL4EntityWriter provides the L4EntityWriter from Data.
func provideL4EntityWriter(d *data.Data) biz.L4EntityWriter {
	if d == nil {
		return nil
	}
	return data.NewL4GraphRepo(d)
}

func providePersistenceSet(
	d *data.Data,
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
	adminUC *biz.MemoryAdminUsecase,
	lg loggateway.Logger,
) rt.PersistenceSet {
	var mem rt.MemorySet
	if d != nil {
		mem = rt.MemorySet{
			TRPC:            memtrpc.NewSQLiteMemoryService(d, data.NewL3FactWriterAdapter(d, d.VectorStore()), factSync, q, vec, memtrpc.NewAgentRuntimeSettingsLoader(agentsUC), lg),
			Admin:           data.NewSessionAdminStoreAdapter(d, d.VectorStore()),
			AdminUsecase:    adminUC,
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
