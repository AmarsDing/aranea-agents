package server

import (
	a2atrpc "aranea-agents/internal/a2a/trpc"
	"aranea-agents/internal/service"
)

// ServiceRegistry bundles all service references to avoid parameter bloat in
// NewHTTPServer / NewGRPCServer constructors. When a new service is added,
// only this struct and the Wire provider need updating — server constructors
// are unaffected.
type ServiceRegistry struct {
	Admin              *service.AdminService
	Avatar             *service.AvatarService
	Agents             *service.AgentService
	LLM                *service.LlmProviderModelService
	Hook               *service.HookService
	Cron               *service.CronService
	Plugin             *service.PluginService
	MCPServer          *service.MCPServerService
	Skill              *service.SkillService
	Tool               *service.ToolService
	Session            *service.SessionService
	Channel            *service.ChannelService
	Usage              *service.UsageService
	Monitor            *service.MonitorService
	Memory             *service.MemoryService
	SystemSetting      *service.SystemSettingService
	ModelCatalog       *service.ModelCatalogService
	Teams              *service.TeamService
	Chat               *service.ChatService
	Graph              *service.GraphService
	Artifact           *service.ArtifactService
	Knowledge          *service.KnowledgeService
	Eval               *service.EvaluationService
	A2A                *service.A2AService
	A2APublic          *a2atrpc.EndpointRegistry
	Ecosystem          *service.EcosystemService
	Gateway            *service.GatewayService
	ChannelIngress     *service.ChannelIngress
	AIRefine           *service.AIRefineService
	Taxonomy           *service.TaxonomyService
	Organization       *service.OrganizationService
	SkillEvo           *service.SkillEvolutionService
	SkillIntel         *service.SkillIntelligenceService
	SkillDedup         *service.SkillDedupService
	Pack               *service.PackService
	SkillEvoSuggestion *service.SkillEvolutionSuggestionService
	EcosystemPreset    *service.EcosystemPresetService
	// Compat service wrappers: lazily wire trpc-agent-go framework servers.
	AGUICompat    *service.AGUICompatService
	OpenAISession *service.OpenAISessionCompatService
	A2AExtension  *service.A2AExtensionCompatService
	// RuntimeProfileService manages per-agent runtime configuration profiles.
	RuntimeProfile *service.RuntimeProfileService
}

// NewServiceRegistry assembles all services into a single registry for Wire injection.
// This is the single point of change when a new service is added.
func NewServiceRegistry(
	admin *service.AdminService,
	avatar *service.AvatarService,
	agents *service.AgentService,
	llm *service.LlmProviderModelService,
	hookSvc *service.HookService,
	cronSvc *service.CronService,
	pluginSvc *service.PluginService,
	mcpSvc *service.MCPServerService,
	skillSvc *service.SkillService,
	toolSvc *service.ToolService,
	sessionSvc *service.SessionService,
	channelSvc *service.ChannelService,
	usageSvc *service.UsageService,
	monitorSvc *service.MonitorService,
	memorySvc *service.MemoryService,
	systemSettingSvc *service.SystemSettingService,
	modelCatalogSvc *service.ModelCatalogService,
	teams *service.TeamService,
	chatSvc *service.ChatService,
	graphSvc *service.GraphService,
	artifactSvc *service.ArtifactService,
	knowledgeSvc *service.KnowledgeService,
	evalSvc *service.EvaluationService,
	a2aSvc *service.A2AService,
	a2aPublic *a2atrpc.EndpointRegistry,
	ecosystemSvc *service.EcosystemService,
	gatewaySvc *service.GatewayService,
	channelIngress *service.ChannelIngress,
	aiRefine *service.AIRefineService,
	taxonomy *service.TaxonomyService,
	organization *service.OrganizationService,
	skillEvo *service.SkillEvolutionService,
	skillIntel *service.SkillIntelligenceService,
	skillDedup *service.SkillDedupService,
	packSvc *service.PackService,
	skillEvoSuggestion *service.SkillEvolutionSuggestionService,
	ecosystemPresetSvc *service.EcosystemPresetService,
	aguiCompat *service.AGUICompatService,
	openaiSession *service.OpenAISessionCompatService,
	a2aExtension *service.A2AExtensionCompatService,
	runtimeProfile *service.RuntimeProfileService,
) *ServiceRegistry {
	return &ServiceRegistry{
		Admin:              admin,
		Avatar:             avatar,
		Agents:             agents,
		LLM:                llm,
		Hook:               hookSvc,
		Cron:               cronSvc,
		Plugin:             pluginSvc,
		MCPServer:          mcpSvc,
		Skill:              skillSvc,
		Tool:               toolSvc,
		Session:            sessionSvc,
		Channel:            channelSvc,
		Usage:              usageSvc,
		Monitor:            monitorSvc,
		Memory:             memorySvc,
		SystemSetting:      systemSettingSvc,
		ModelCatalog:       modelCatalogSvc,
		Teams:              teams,
		Chat:               chatSvc,
		Graph:              graphSvc,
		Artifact:           artifactSvc,
		Knowledge:          knowledgeSvc,
		Eval:               evalSvc,
		A2A:                a2aSvc,
		A2APublic:          a2aPublic,
		Ecosystem:          ecosystemSvc,
		Gateway:            gatewaySvc,
		ChannelIngress:     channelIngress,
		AIRefine:           aiRefine,
		Taxonomy:           taxonomy,
		Organization:       organization,
		SkillEvo:           skillEvo,
		SkillIntel:         skillIntel,
		SkillDedup:         skillDedup,
		Pack:               packSvc,
		SkillEvoSuggestion: skillEvoSuggestion,
		EcosystemPreset:    ecosystemPresetSvc,
		AGUICompat:         aguiCompat,
		OpenAISession:      openaiSession,
		A2AExtension:       a2aExtension,
		RuntimeProfile:     runtimeProfile,
	}
}
