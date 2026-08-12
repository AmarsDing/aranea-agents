package service

import (
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
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
	wire.Bind(new(biz.ChannelTurnGateway), new(*ChatService)),
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
	NewOrganizationService,
	NewLlmProviderModelService,
	NewHookService,
	NewCronService,
	NewPluginServiceWithBootstrap,
	NewMCPServerService,
	importer.ProvideEngine,
	importer.ProvideLLMLister,
	NewSkillService,
	ProvideSkillResolveRootFn,
	storage.NewSkillFilesystem,
	NewSessionService,
	NewSessionV2Service,
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
	ProvideChatService,
	ProvideEvaluationRunner,
	ProvidePublishGate,
	NewGraphExecutionTelemetry,
	NewGraphOrchestrationProjector,
	ProvideGraphUsecase,
	NewGraphService,
	WireGraphTaskRuntime,
	NewKanbanToolBridge,
	ProvideComputerUseUsecase,
	NewComputerUseService,
	NewArtifactService,
	// P1-1: 提供 sessionWorkspaceLookup 适配器，让 wire 自动注入到
	// ArtifactService 做 IDOR 防护。
	ProvideSessionWorkspaceLookup,
	// 「全部产物」：提供 sessionWorkspaceSearcher 适配器，让 wire 自动注入到
	// ArtifactService 做空 session_id 的 workspace 过滤。
	ProvideSessionWorkspaceSearcher,
	NewKnowledgeService,
	NewEvaluationService,
	NewA2AEndpointBuilder,
	NewEcosystemService,
	NewEcosystemPresetService,
	NewGatewayService,
	NewKnowledgeEmbedder,
	NewMemoryEmbeddingAdapter,
	wire.Bind(new(biz.EmbeddingService), new(*MemoryEmbeddingAdapter)),
	wire.Bind(new(biz.SkillEmbedder), new(*knowledge.MultiProviderEmbedder)),
	NewKnowledgeRetriever,
	NewKnowledgeHybridRetriever,
	NewKnowledgeQueryRewriter,
	NewKnowledgeMarkdownOrganizer,
	NewKnowledgeExtractorRegistry,
	NewKnowledgeAssetStore,
	NewKnowledgeAdaptiveRouter,
	NewKnowledgeRetrievalEvaluator,
	NewKnowledgeFederatedRetriever,
	ProvideKnowledgeSearchDeps,
	NewSkillDBRepository,
	// MemoryLLMExtractor and MemoryEnhancedExtractor use Config structs
	// and are provided directly here (no custom wire.go provider needed).
	NewMemoryLLMExtractor,
	NewMemoryEnhancedExtractor,
	wire.Bind(new(biz.MemoryTextExtractor), new(*MemoryLLMExtractor)),
	wire.Bind(new(biz.EnhancedTextExtractor), new(*MemoryEnhancedExtractor)),
	// P3 M2: Agent Case LLM 提取器（复用 MemoryLLMExtractor 的 LLM 通道）。
	NewAgentCaseLLMExtractor,
	wire.Bind(new(biz.AgentCaseExtractor), new(*AgentCaseLLMExtractor)),
	// P3 M4: Case→Skill 蒸馏器（复用 MemoryLLMExtractor 的 LLM 通道）。
	NewAgentCaseSkillDistiller,
	wire.Bind(new(biz.CaseSkillDistiller), new(*AgentCaseSkillDistiller)),
	wire.Bind(new(biz.TeamStarterPort), new(*TeamStarter)),
	// Dependency inversion: bind concrete types to biz ports for TeamService
	wire.Bind(new(biz.TeamTurnRunnerPort), new(*team.Runner)),
	ProvideRunRegistryPort,
	// Dependency inversion: adapt team runtime types to biz ports for ChatOrchestrator
	ProvideTeamRunnerWirePort,
	ProvideTeamMediatorPort,
	ProvideTeamGraphCoordPort,
	// Phase 3 decoupling adapters: biz interfaces → event/webresearch implementations
	ProvideSessionLogWriter,
	ProvideSystemLogWriter,
	ProvideFlowLogWriter,
	ProvideSessionFlowLogWriter,
	ProvideMonitorFlowLogWriter,
	ProvideWebResearchTester,
	// M74 V2-T3: client tool bridge（desktop companion 工具执行协调器）
	ProvideClientToolBridge,
	// PGO-3: AI prompt refinement service.
	NewAIRefineService,
	ProvideSessionStatusPublisher,
	ProvideMetricsUpdatedPublisher,
	NewSessionStatusGuard,
	NewSpiritTeamAssembler,
	NewSpiritSynthesisService,
	NewSynthesisModelAdapter,
	NewTeamStarter,
	NewSkillEvolutionService,
	NewSkillIntelligenceService,
	NewSkillDedupService,
	NewPackService,
	NewSkillCuratorService,
	NewSandboxRunner,
	NewSkillEvolutionSuggestionService,
	// P3 M5: 平台级进化多样性观测（统一建议表聚合视图）。
	NewEvolutionService,
	// Server adapter wrappers: lazily wire trpc-agent-go framework servers
	// (AG-UI, OpenAI session, A2A extension) to per-session Runners built
	// via OpenAIRunnerBuilder (implemented by *ChatService).
	NewAGUICompatService,
	NewOpenAISessionCompatService,
	NewA2AExtensionCompatService,
	NewTwinOpenAPICompatService,
	NewRuntimeProfileService,
	NewLearningLoopService,
	// SelfImprovementService 由 cmd/admin provideSelfImprovementService 显式装配
	// （需要 conf + SystemSettingUsecase 适配 SIRefineLLMReader 窄口，不进 ProviderSet）。
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
