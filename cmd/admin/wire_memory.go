package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/memory"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/service"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/pkg/loggateway"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
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

// provideMemoryConflictDetector wires the A2 conflict-governance detector.
// Returns nil when Postgres or the embedding usecase is unavailable — the
// worker then skips conflict governance (best-effort, never blocks writes).
func provideMemoryConflictDetector(d *data.Data, vec *biz.MemoryUsecase) biz.MemoryConflictDetector {
	if d == nil || vec == nil {
		return nil
	}
	return biz.NewMemoryConflictDetector(data.NewMemoryConflictNeighborSearcher(d), vec, data.NewL3FactReaderForUser(d))
}

// provideL3ConflictStore wires the conflict store used to apply supersede /
// conflict-mark decisions.
func provideL3ConflictStore(d *data.Data) biz.L3ConflictStore {
	return data.NewL3ConflictStore(d)
}

// provideFactWriteAdjudicator wires the P1-3 LLM adjudicator for contested
// fact-write candidates. Model resolution mirrors MemoryLLMExtractor
// (MemoryWorker → L0Compress → agent default via ModelCatalog). Returns nil
// when the catalog is unavailable — the pipeline then falls back to
// heuristic ADD for contested candidates.
func provideFactWriteAdjudicator(
	agents *biz.AgentUsecase,
	sessions *biz.SessionUsecase,
	catalog *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
) biz.FactWriteAdjudicator {
	return service.NewMemoryFactWriteAdjudicator(service.MemoryFactWriteAdjudicatorConfig{
		Agents:       agents,
		Sessions:     sessions,
		ModelCatalog: catalog,
		RoundTrip:    &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}},
		LLMDisabled:  false,
		Logger:       lg,
	})
}

// provideFactWritePipeline wires the P1-3 unified write pipeline: all
// automatic fact-write sources (auto_memory worker, sleep-time episode
// consolidator) funnel through gates → neighbor recall → LLM adjudication →
// bi-temporal writes → audit. Returns nil when d is nil (sources then skip
// fact writes gracefully).
func provideFactWritePipeline(
	d *data.Data,
	vec *biz.MemoryUsecase,
	adjudicator biz.FactWriteAdjudicator,
	lg loggateway.Logger,
) *biz.FactWritePipeline {
	if d == nil {
		return nil
	}
	deps := biz.FactWritePipelineDeps{
		Searcher:    data.NewMemoryConflictNeighborSearcher(d),
		Reader:      data.NewL3FactReaderForUser(d),
		Writer:      data.NewL3FactWriterAdapter(d, d.VectorStore()),
		Access:      data.NewL3FactAccessCounter(d),
		Adjudicator: adjudicator,
		ActionLog:   data.NewMemoryActionLogWriter(d),
		LG:          lg,
	}
	// Guard against typed-nil interfaces: a nil *MemoryUsecase wrapped in the
	// EmbeddingService interface would pass the pipeline's nil check and
	// panic on first Embed call.
	if vec != nil {
		deps.Embedder = vec
	}
	return biz.NewFactWritePipeline(deps)
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

func provideMemoryCompositeRecall(d *data.Data, memSvc trpcmemory.Service, l2Recall biz.MemoryL2Recaller, l3Recall biz.MemoryL3Recaller, vec *biz.MemoryUsecase, lg loggateway.Logger) biz.MemoryCompositeRecaller {
	uc := biz.NewMemoryCompositeRecallUsecase(data.NewMemoryCompositeRecallAdapter(d))
	if uc == nil {
		return nil
	}
	// P2-R1: compose the fused L2/L3 recall usecases so the main chat path
	// gets embedding + pgvector/FTS RRF + calibrated scores + recalled_count
	// bumps (the legacy store path had none of these).
	uc.SetLayerRecallers(l2Recall, l3Recall)
	// P0-C: share one query embedding per turn across L2/L3 recallers
	// (previously each embedded the same query independently on the LLM
	// critical path). Guard against typed-nil: vec may be a nil pointer.
	if vec != nil {
		uc.SetEmbedder(vec, lg)
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
	linkEvolver memory.LinkEvolutionService,
	lg loggateway.Logger,
) trpcmemory.Service {
	if d == nil {
		return nil
	}
	return memtrpc.NewMemoryService(
		data.NewL3FactReaderForUser(d),
		data.NewL3FactWriterAdapter(d, d.VectorStore()),
		factSync,
		q,
		vec,
		memtrpc.NewAgentRuntimeSettingsLoader(agentsUC),
		data.NewFactConsistencyAdapter(d),
		linkEvolver,
		lg,
	)
}

// provideLinkEvolutionService builds the LinkEvolutionService used to evolve
// the memory link graph after AddMemory. The LLM is resolved from
// MEMORY_LINK_EVOLUTION_PROVIDER / MEMORY_LINK_EVOLUTION_MODEL env vars; when
// unset, llm is nil and EvolveLinks gracefully degrades to a no-op (warn log
// inside the implementation). The evolution queue is intentionally nil: the
// sqlite adapter triggers EvolveLinks directly via safego.Go, so the
// queue/worker loop is not needed.
func provideLinkEvolutionService(
	d *data.Data,
	catalog *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
) memory.LinkEvolutionService {
	if d == nil {
		return nil
	}
	var llm trpcmodel.Model
	prov := strings.TrimSpace(os.Getenv("MEMORY_LINK_EVOLUTION_PROVIDER"))
	mod := strings.TrimSpace(os.Getenv("MEMORY_LINK_EVOLUTION_MODEL"))
	if prov != "" && mod != "" && catalog != nil {
		rtTrip := &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}}
		if m, err := provider.TRPCModelForProviderModel(context.Background(), catalog, rtTrip, prov, mod, lg); err == nil {
			llm = m
		} else {
			lg.Warn("link evolution: LLM model build failed, EvolveLinks will be no-op",
				loggateway.StepID("memory.link_evolution.wire"),
				loggateway.Str("provider", prov),
				loggateway.Str("model", mod),
				loggateway.Err(err))
		}
	}
	svc := memory.NewLinkEvolutionService(
		llm,
		data.NewL3FactReaderForUser(d),
		data.NewL3FactWriterAdapter(d, d.VectorStore()),
		nil, // queue: not needed — sqlite adapter calls EvolveLinks directly
		d,   // tx: *data.Data implements memory.TxProvider for atomic backlink updates
		lg,
	)
	// Phase 6A-03: wire L4 relation writer + action log for EVOLVED_FROM links.
	svc.SetRelationWriter(data.NewL4GraphRepo(d))
	svc.SetActionLogWriter(data.NewMemoryActionLogWriter(d))
	// T8: per-agent throttle interval (env: MEMORY_LINK_EVOLUTION_THROTTLE).
	// Example: "10s" → at most one LLM call per agent every 10 seconds.
	// Empty or "0" disables throttling (default).
	if raw := strings.TrimSpace(os.Getenv("MEMORY_LINK_EVOLUTION_THROTTLE")); raw != "" && raw != "0" {
		if throttleDur, parseErr := time.ParseDuration(raw); parseErr == nil && throttleDur > 0 {
			svc.SetThrottleInterval(throttleDur)
			lg.Info("link evolution throttle configured",
				loggateway.StepID("memory.link_evolution.wire"),
				loggateway.Str("throttle", throttleDur.String()))
		}
	}
	return svc
}

func provideMemoryAdminUsecase(admin biz.MemoryAdminDeps, vec *biz.MemoryUsecase, factSync biz.MemoryFactIndexSyncer, d *data.Data, lg loggateway.Logger) *biz.MemoryAdminUsecase {
	uc := biz.NewMemoryAdminUsecase(admin, vec, factSync, data.NewL3FactWriterAdapter(d, d.VectorStore()), lg)
	uc.SetMemoryCenterReaders(data.NewL2EpisodeAdminReader(d, d.VectorStore()), data.NewL4RelationAdminReader(d))
	return uc
}

// providePathBExtractor creates a PathBExtractor and injects it into MemoryAdminUsecase.
// This breaks the dependency cycle: MemoryAdminUsecase → PathBExtractor → EnhancedTextExtractor → SessionUsecase → … → MemoryAdminUsecase.
func providePathBExtractor(extractor biz.EnhancedTextExtractor, l4 biz.PathBL4Writer, adminUC *biz.MemoryAdminUsecase, d *data.Data, lg loggateway.Logger) *biz.PathBExtractor {
	pe := biz.NewPathBExtractor(extractor, l4, lg)
	if adminUC != nil {
		adminUC.SetPathBExtractor(pe, data.NewRecentMessageLister(d))
	}
	return pe
}

// providePathBL4Writer provides the narrow PathBL4Writer from Data.
// Used by PathBExtractor for read-then-write entity resolution.
func providePathBL4Writer(d *data.Data) biz.PathBL4Writer {
	if d == nil {
		return nil
	}
	return data.NewL4GraphRepo(d)
}

// provideReconsolidationService wires the L4 memory reconsolidation service
// (design §15.7, FR-10.5): when entities are recalled into the prompt, their
// activation is boosted, use_count incremented, and co-recalled connections
// reinforced via the Hebbian rule. Returns nil when d is nil — the
// before-model hook then skips the trigger.
func provideReconsolidationService(d *data.Data, lg loggateway.Logger) biz.L4Reconsolidator {
	if d == nil {
		return nil
	}
	return memory.NewReconsolidationService(
		data.NewL4ReconsolidationStore(d),
		memory.NewHebbianUpdater(data.NewL4HebbianStore(d), lg),
		lg,
	)
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
	reconsolidator biz.L4Reconsolidator,
	lg loggateway.Logger,
	deadLetterRepo *data.MemoryJobDeadLetterRepo,
) rt.PersistenceSet {
	var mem rt.MemorySet
	if d != nil {
		mem = rt.MemorySet{
			TRPC:              memSvc,
			Admin:             data.NewSessionAdminStoreAdapter(d, d.VectorStore()),
			AdminUsecase:      adminUC,
			ActionLogWriter:   data.NewMemoryActionLogWriter(d),
			L2Recall:          l2Recall,
			L3Recall:          l3Recall,
			CompositeRecall:   compositeRecall,
			PreferenceLister:  data.NewMemoryPreferenceLister(d),
			ProfileCardReader: data.NewMemoryProfileCardStore(d),
			FactInjectCounter: data.NewL3FactInjectCounter(d),
			Reconsolidator:    reconsolidator,
			// P3 M3: Agent Case 召回（任务经验），与 L2/L3 并列注入 recall cue。
			AgentCaseRecaller: data.NewMemoryAgentCaseStore(d),
		}
		// Connect dead-letter sink so queue overflow is persisted instead of silently dropped.
		if queue, ok := q.(*memtrpc.MemoryJobQueue); ok && deadLetterRepo != nil {
			queue.SetDeadLetterSink(deadLetterRepo)
		}
	}
	var rollback rt.RunnerSessionRollbackStore
	if d != nil {
		rollback = sessiontrpc.NewRunnerRollbackStore(d.RWDB(), d.Dialect(), lg)
	}
	return rt.PersistenceSet{Session: sess, Memory: mem, AgentMCP: mcp, Artifact: artifact, ArtifactUC: artifactUC, RunnerRollback: rollback}
}
