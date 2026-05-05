package server

import (
	adminv1 "aranea-agents/api/kratos/admin/v1"
	agentv1 "aranea-agents/api/kratos/agent/v1"
	agentcategoryv1 "aranea-agents/api/kratos/agent_category/v1"
	avatarv1 "aranea-agents/api/kratos/avatar/v1"
	chatv1 "aranea-agents/api/kratos/chat/v1"
	cronv1 "aranea-agents/api/kratos/cron/v1"
	hookv1 "aranea-agents/api/kratos/hook/v1"
	llmprovidermodelv1 "aranea-agents/api/kratos/llm_provider_model/v1"
	mcpserverv1 "aranea-agents/api/kratos/mcp_server/v1"
	pluginv1 "aranea-agents/api/kratos/plugin/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	skillv1 "aranea-agents/api/kratos/skill/v1"
	teamv1 "aranea-agents/api/kratos/team/v1"
	toolv1 "aranea-agents/api/kratos/tool/v1"
	channelv1 "aranea-agents/api/kratos/channel/v1"
	monitorv1 "aranea-agents/api/kratos/monitor/v1"
	memoryv1 "aranea-agents/api/kratos/memory/v1"
	usagev1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/validate"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Server,
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
	teams *service.TeamService,
	chatSvc *service.ChatService,
) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			validate.Middleware(),
		),
	}
	if c.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	adminv1.RegisterAdminServiceServer(srv, admin)
	avatarv1.RegisterAvatarServiceServer(srv, avatar)
	agentv1.RegisterAgentServiceServer(srv, agents)
	agentcategoryv1.RegisterAgentCategoryServiceServer(srv, agentCat)
	llmprovidermodelv1.RegisterLlmProviderModelServiceServer(srv, llm)
	hookv1.RegisterHookServiceServer(srv, hookSvc)
	cronv1.RegisterCronServiceServer(srv, cronSvc)
	pluginv1.RegisterPluginServiceServer(srv, pluginSvc)
	mcpserverv1.RegisterMCPServerServiceServer(srv, mcpSvc)
	skillv1.RegisterSkillServiceServer(srv, skillSvc)
	toolv1.RegisterToolServiceServer(srv, toolSvc)
	sessionv1.RegisterSessionServiceServer(srv, sessionSvc)
	channelv1.RegisterChannelServiceServer(srv, channelSvc)
	usagev1.RegisterUsageServiceServer(srv, usageSvc)
	monitorv1.RegisterMonitorServiceServer(srv, monitorSvc)
	memoryv1.RegisterMemoryServiceServer(srv, memorySvc)
	teamv1.RegisterTeamServiceServer(srv, teams)
	chatv1.RegisterChatServiceServer(srv, chatSvc)
	return srv
}
