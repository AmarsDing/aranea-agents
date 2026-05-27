package service

import (
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/knowledge"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/team"

	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	wire.Bind(new(compress.Compressor), new(*compress.LLMService)),
	wire.Bind(new(biz.NativeTurnGateway), new(*ChatService)),
	wire.Bind(new(biz.TurnGateway), new(*ChatService)),
	wire.Bind(new(biz.TurnControlGateway), new(*ChatService)),
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
	NewAgentCategoryService,
	NewLlmProviderModelService,
	NewHookService,
	NewCronService,
	NewPluginServiceWithBootstrap,
	NewMCPServerService,
	importer.NewEngine,
	NewSkillService,
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
	NewKnowledgeChunker,
	NewKnowledgeEmbedder,
	wire.Bind(new(biz.EmbeddingService), new(*knowledge.Embedder)),
	NewKnowledgeRetriever,
	NewSkillDBRepository,
	NewMemoryLLMExtractor,
	wire.Bind(new(biz.MemoryTextExtractor), new(*MemoryLLMExtractor)),
	// Phase 3 decoupling adapters: biz interfaces → event/webresearch implementations
	ProvideEnvelopeBuffer,
	ProvideSessionLogWriter,
	ProvideSystemLogWriter,
	ProvideWebResearchTester,
)
