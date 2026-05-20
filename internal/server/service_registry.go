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
	Admin          *service.AdminService
	Avatar         *service.AvatarService
	Agents         *service.AgentService
	AgentCat       *service.AgentCategoryService
	LLM            *service.LlmProviderModelService
	Hook           *service.HookService
	Cron           *service.CronService
	Plugin         *service.PluginService
	MCPServer      *service.MCPServerService
	Skill          *service.SkillService
	Tool           *service.ToolService
	Session        *service.SessionService
	Channel        *service.ChannelService
	Usage          *service.UsageService
	Monitor        *service.MonitorService
	Memory         *service.MemoryService
	SystemSetting  *service.SystemSettingService
	Teams          *service.TeamService
	Chat           *service.ChatService
	Graph          *service.GraphService
	Artifact       *service.ArtifactService
	Knowledge      *service.KnowledgeService
	Eval           *service.EvaluationService
	A2A            *service.A2AService
	A2APublic      *a2atrpc.EndpointRegistry
	Ecosystem      *service.EcosystemService
	ChannelIngress *service.ChannelIngress
}

// NewServiceRegistry assembles all services into a single registry for Wire injection.
// This is the single point of change when a new service is added.
func NewServiceRegistry(
	admin *service.AdminService,
	avatar *service.AvatarService,
	agents *service.AgentService,
	agentCat *service.AgentCategoryService,
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
	teams *service.TeamService,
	chatSvc *service.ChatService,
	graphSvc *service.GraphService,
	artifactSvc *service.ArtifactService,
	knowledgeSvc *service.KnowledgeService,
	evalSvc *service.EvaluationService,
	a2aSvc *service.A2AService,
	a2aPublic *a2atrpc.EndpointRegistry,
	ecosystemSvc *service.EcosystemService,
	channelIngress *service.ChannelIngress,
) *ServiceRegistry {
	return &ServiceRegistry{
		Admin:          admin,
		Avatar:         avatar,
		Agents:         agents,
		AgentCat:       agentCat,
		LLM:            llm,
		Hook:           hookSvc,
		Cron:           cronSvc,
		Plugin:         pluginSvc,
		MCPServer:      mcpSvc,
		Skill:          skillSvc,
		Tool:           toolSvc,
		Session:        sessionSvc,
		Channel:        channelSvc,
		Usage:          usageSvc,
		Monitor:        monitorSvc,
		Memory:         memorySvc,
		SystemSetting:  systemSettingSvc,
		Teams:          teams,
		Chat:           chatSvc,
		Graph:          graphSvc,
		Artifact:       artifactSvc,
		Knowledge:      knowledgeSvc,
		Eval:           evalSvc,
		A2A:            a2aSvc,
		A2APublic:      a2aPublic,
		Ecosystem:      ecosystemSvc,
		ChannelIngress: channelIngress,
	}
}
