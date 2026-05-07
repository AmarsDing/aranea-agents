package server

import (
	"encoding/json"
	nethttp "net/http"

	adminv1 "aranea-agents/api/kratos/admin/v1"
	agentv1 "aranea-agents/api/kratos/agent/v1"
	agentcategoryv1 "aranea-agents/api/kratos/agent_category/v1"
	avatarv1 "aranea-agents/api/kratos/avatar/v1"
	channelv1 "aranea-agents/api/kratos/channel/v1"
	cronv1 "aranea-agents/api/kratos/cron/v1"
	hookv1 "aranea-agents/api/kratos/hook/v1"
	llmprovidermodelv1 "aranea-agents/api/kratos/llm_provider_model/v1"
	mcpserverv1 "aranea-agents/api/kratos/mcp_server/v1"
	memoryv1 "aranea-agents/api/kratos/memory/v1"
	monitorv1 "aranea-agents/api/kratos/monitor/v1"
	systemsettingv1 "aranea-agents/api/kratos/system_setting/v1"
	pluginv1 "aranea-agents/api/kratos/plugin/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	skillv1 "aranea-agents/api/kratos/skill/v1"
	teamv1 "aranea-agents/api/kratos/team/v1"
	toolv1 "aranea-agents/api/kratos/tool/v1"
	usagev1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/service"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/validate"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server,
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
	skillImport *importer.Engine,
) *http.Server {
	var opts = []http.ServerOption{
		http.Filter(
			CorsDevFilter(),
			auth.Middleware(),
			LegacyRESTProxyFilter(),
		),
		http.Middleware(
			recovery.Recovery(),
			validate.Middleware(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, http.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, http.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	adminv1.RegisterAdminServiceHTTPServer(srv, admin)
	avatarv1.RegisterAvatarServiceHTTPServer(srv, avatar)
	agentv1.RegisterAgentServiceHTTPServer(srv, agents)
	agentcategoryv1.RegisterAgentCategoryServiceHTTPServer(srv, agentCat)
	llmprovidermodelv1.RegisterLlmProviderModelServiceHTTPServer(srv, llm)
	hookv1.RegisterHookServiceHTTPServer(srv, hookSvc)
	cronv1.RegisterCronServiceHTTPServer(srv, cronSvc)
	pluginv1.RegisterPluginServiceHTTPServer(srv, pluginSvc)
	mcpserverv1.RegisterMCPServerServiceHTTPServer(srv, mcpSvc)
	skillv1.RegisterSkillServiceHTTPServer(srv, skillSvc)
	toolv1.RegisterToolServiceHTTPServer(srv, toolSvc)
	sessionv1.RegisterSessionServiceHTTPServer(srv, sessionSvc)
	channelv1.RegisterChannelServiceHTTPServer(srv, channelSvc)
	usagev1.RegisterUsageServiceHTTPServer(srv, usageSvc)
	monitorv1.RegisterMonitorServiceHTTPServer(srv, monitorSvc)
	memoryv1.RegisterMemoryServiceHTTPServer(srv, memorySvc)
	systemsettingv1.RegisterSystemSettingServiceHTTPServer(srv, systemSettingSvc)
	teamv1.RegisterTeamServiceHTTPServer(srv, teams)
	RegisterChatIngress(srv, chatSvc)
	RegisterSkillImportHTTPServer(srv, skillImport)
	srv.Route("/").GET("/healthz", func(ctx http.Context) error {
		w := ctx.Response()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(nethttp.StatusOK)
		return json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return srv
}
