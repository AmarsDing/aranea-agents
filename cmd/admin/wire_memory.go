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
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
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

func provideMemoryL2Recall(d *data.Data, vec *biz.MemoryUsecase, lg loggateway.Logger) biz.MemoryL2Recaller {
	return biz.NewMemoryL2RecallUsecase(data.NewSessionL2RecallStore(d, d.VectorStore()), vec, lg)
}

func provideMemoryL3Recall(d *data.Data, vec *biz.MemoryUsecase, lg loggateway.Logger) biz.MemoryL3Recaller {
	return biz.NewMemoryL3RecallUsecase(data.NewSessionL3RecallStore(d, d.VectorStore()), data.NewL3ScoredRecallAdapter(d), vec, lg)
}

func provideFeedbackMemoryEnqueuer(q memtrpc.AutoMemoryQueue) biz.FeedbackMemoryEnqueuer {
	return biz.FeedbackMemoryEnqueuerFunc(memtrpc.NewFeedbackMemoryEnqueuer(q))
}

func provideMemoryCompositeRecall(d *data.Data, memSvc trpcmemory.Service) biz.MemoryCompositeRecaller {
	uc := biz.NewMemoryCompositeRecallUsecase(data.NewMemoryCompositeRecallAdapter(d))
	if uc == nil {
		return nil
	}
	// Wire the proactive recaller so the composite usecase can surface
	// memories based on conversation context (P3-11).
	if proactiveRecaller := memtrpc.NewProactiveRecallAdapter(memSvc); proactiveRecaller != nil {
		uc.SetProactiveRecaller(proactiveRecaller)
	}
	return uc
}

// provideMemoryTRPCService builds the trpc-agent-go memory Service backed by
// the SQLite adapter. Centralizing construction here lets both the composite
// recall usecase (P3-11) and the persistence set share the same instance.
func provideMemoryTRPCService(
	d *data.Data,
	agentsUC *biz.AgentUsecase,
	vec *biz.MemoryUsecase,
	factSync biz.MemoryFactIndexSyncer,
	q memtrpc.AutoMemoryQueue,
	lg loggateway.Logger,
) trpcmemory.Service {
	if d == nil {
		return nil
	}
	return memtrpc.NewSQLiteMemoryService(
		data.NewL3FactReaderForUser(d),
		data.NewL3FactWriterAdapter(d, d.VectorStore()),
		factSync,
		q,
		vec,
		memtrpc.NewAgentRuntimeSettingsLoader(agentsUC),
		data.NewFactConsistencyAdapter(d),
		lg,
	)
}

func provideMemoryAdminUsecase(admin biz.MemoryAdminDeps, vec *biz.MemoryUsecase, factSync biz.MemoryFactIndexSyncer, d *data.Data, lg loggateway.Logger) *biz.MemoryAdminUsecase {
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
	memSvc trpcmemory.Service,
	q memtrpc.AutoMemoryQueue,
	policy *biz.MemoryPolicyEngine,
	l2Recall biz.MemoryL2Recaller,
	l3Recall biz.MemoryL3Recaller,
	compositeRecall biz.MemoryCompositeRecaller,
	adminUC *biz.MemoryAdminUsecase,
	lg loggateway.Logger,
	deadLetterRepo *data.MemoryJobDeadLetterRepo,
) rt.PersistenceSet {
	var mem rt.MemorySet
	if d != nil {
		mem = rt.MemorySet{
			TRPC:            memSvc,
			Admin:           data.NewSessionAdminStoreAdapter(d, d.VectorStore()),
			AdminUsecase:    adminUC,
			L2Recall:        l2Recall,
			L3Recall:        l3Recall,
			CompositeRecall: compositeRecall,
		}
		// Connect dead-letter sink so queue overflow is persisted instead of silently dropped.
		if queue, ok := q.(*memtrpc.MemoryJobQueue); ok && deadLetterRepo != nil {
			queue.SetDeadLetterSink(deadLetterRepo)
		}
	}
	var rollback rt.RunnerSessionRollbackStore
	if d != nil {
		rollback = sessiontrpc.NewRunnerRollbackStore(d.RawDB(), lg)
	}
	return rt.PersistenceSet{Session: sess, Memory: mem, AgentMCP: mcp, Artifact: artifact, ArtifactUC: artifactUC, RunnerRollback: rollback}
}
