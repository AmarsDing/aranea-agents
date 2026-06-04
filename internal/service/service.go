package service

import (
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/data"
	"aranea-agents/internal/knowledge"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/storage"
	"aranea-agents/internal/team"
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"

	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	ProvideCompressor,
	wire.Bind(new(biz.NativeTurnGateway), new(*ChatService)),
	wire.Bind(new(biz.TurnExecutorGateway), new(*ChatService)),
	wire.Bind(new(biz.TurnRunControlGateway), new(*ChatService)),
	wire.Bind(new(biz.TurnGateway), new(*ChatService)),
	wire.Bind(new(biz.TurnControlGateway), new(*ChatService)),
	wire.Bind(new(biz.DurableResumeGateway), new(*ChatService)),
	wire.Bind(new(biz.A2ARunnerFactory), new(*ChatService)),
	NewCronTriggerGatewayAdapter,
	wire.Bind(new(biz.TurnExecutor), new(*ChatOrchestrator)),
	ProvideChatOrchestrator,
	wire.Bind(new(biz.GraphExecutor), new(*GraphService)),
	wire.Bind(new(a2apkg.AgentTurnRunner), new(*ChatService)),
	wire.Bind(new(EvalTurnGateway), new(*ChatService)),
	NewPendingMessageGatewayAdapter,
	araneasession.NewCompressHTTPClient,
	compress.NewLLMService,
	team.ProviderSet,
	NewAdminService,
	NewAvatarService,
	NewAgentService,
	NewTeamService,
	NewTaxonomyService,
	NewIndustryTaxonomyService,
	NewLlmProviderModelService,
	NewHookService,
	NewCronService,
	NewPluginServiceWithBootstrap,
	NewMCPServerService,
	importer.NewEngine,
	importer.ProvideLLMLister,
	NewSkillService,
	ProvideSkillResolveRootFn,
	storage.NewSkillFilesystem,
	NewSessionService,
	NewSessionProjectionAdapter,
	wire.Bind(new(biz.SessionProjection), new(*SessionProjectionAdapter)),
	NewToolService,
	NewChannelService,
	NewUsageService,
	NewFlowLogService,
	NewCodeExecutorService,
	NewMonitorService,
	NewSystemSettingService,
	NewModelCatalogService,
	NewChannelIngress,
	ProvideChatService,
	ProvideEvaluationRunner,
	NewGraphExecutionTelemetry,
	NewGraphOrchestrationProjector,
	ProvideGraphUsecase,
	NewGraphService,
	WireGraphTaskRuntime,
	NewKanbanToolBridge,
	NewArtifactService,
	NewKnowledgeService,
	NewEvaluationService,
	NewA2AEndpointBuilder,
	NewEcosystemService,
	NewGatewayService,
	NewKnowledgeEmbedder,
	NewMemoryEmbeddingAdapter,
	wire.Bind(new(biz.EmbeddingService), new(*MemoryEmbeddingAdapter)),
	wire.Bind(new(biz.SkillEmbedder), new(*knowledge.Embedder)),
	NewKnowledgeRetriever,
	NewKnowledgeHybridRetriever,
	NewKnowledgeQueryRewriter,
	NewKnowledgeAdaptiveRouter,
	NewKnowledgeRetrievalEvaluator,
	NewKnowledgeFederatedRetriever,
	ProvideKnowledgeSearchDeps,
	NewSkillDBRepository,
	NewMemoryLLMExtractor,
	wire.Bind(new(biz.MemoryTextExtractor), new(*MemoryLLMExtractor)),
	wire.Bind(new(biz.TeamStarterPort), new(*TeamStarter)),
	// Phase 3 decoupling adapters: biz interfaces → event/webresearch implementations
	ProvideEnvelopeBuffer,
	ProvideSessionLogWriter,
	ProvideSystemLogWriter,
	ProvideWebResearchTester,
	// PGO-3: AI prompt refinement service.
	NewAIRefineService,
	WireSessionStatusPublisher,
	NewSpiritTeamAssembler,
	NewSpiritSynthesisService,
	NewTeamStarter,
	NewSkillEvolutionService,
	NewPackService,
	data.NewPackRepoAdapter,
	wire.Bind(new(packExporterImporterValidator), new(*data.PackRepoAdapter)),
)

func ProvideSkillResolveRootFn(sys biz.SystemSettingRepo) func(ctx context.Context) string {
	return func(ctx context.Context) string {
		st, err := sys.Get(ctx)
		if err == nil && strings.TrimSpace(st.RootDirectory) != "" {
			return storage.ResolveRootWithPlatform(st.RootDirectory)
		}
		return storage.ResolveRoot()
	}
}

func ProvideCompressor(svc *compress.LLMService, lg loggateway.Logger) compress.Compressor {
	cache := compress.NewCompressCache(256, 10*time.Minute, lg)
	return compress.NewCachingCompressor(svc, cache, lg)
}
