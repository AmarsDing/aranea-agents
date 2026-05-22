package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/team"

	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	wire.Bind(new(biz.NativeTurnCompressor), new(*SessionCompressor)),
	wire.Bind(new(compress.Compressor), new(*compress.LLMService)),
	wire.Bind(new(a2apkg.AgentTurnRunner), new(*ChatService)),
	wire.Bind(new(EvalTurnGateway), new(*ChatService)),
	NewCompressHTTPClient,
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
	NewToolService,
	NewChannelService,
	NewUsageService,
	NewMonitorService,
	NewSystemSettingService,
	NewChannelIngress,
	ProvideChatService,
	ProvideEvaluationRunner,
	NewGraphExecutionTelemetry,
	NewGraphOrchestrationProjector,
	ProvideGraphUsecase,
	NewGraphService,
	NewSessionCompressor,
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
)
