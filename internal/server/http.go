package server

import (
	"encoding/json"
	nethttp "net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	a2av1 "aranea-agents/api/kratos/a2a/v1"
	adminv1 "aranea-agents/api/kratos/admin/v1"
	agentv1 "aranea-agents/api/kratos/agent/v1"
	agentcategoryv1 "aranea-agents/api/kratos/agent_category/v1"
	artifactv1 "aranea-agents/api/kratos/artifact/v1"
	avatarv1 "aranea-agents/api/kratos/avatar/v1"
	channelv1 "aranea-agents/api/kratos/channel/v1"
	chatv1 "aranea-agents/api/kratos/chat/v1"
	cronv1 "aranea-agents/api/kratos/cron/v1"
	evaluationv1 "aranea-agents/api/kratos/evaluation/v1"
	graphv1 "aranea-agents/api/kratos/graph/v1"
	hookv1 "aranea-agents/api/kratos/hook/v1"
	knowledgev1 "aranea-agents/api/kratos/knowledge/v1"
	llmprovidermodelv1 "aranea-agents/api/kratos/llm_provider_model/v1"
	mcpserverv1 "aranea-agents/api/kratos/mcp_server/v1"
	memoryv1 "aranea-agents/api/kratos/memory/v1"
	monitorv1 "aranea-agents/api/kratos/monitor/v1"
	pluginv1 "aranea-agents/api/kratos/plugin/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	skillv1 "aranea-agents/api/kratos/skill/v1"
	systemsettingv1 "aranea-agents/api/kratos/system_setting/v1"
	teamv1 "aranea-agents/api/kratos/team/v1"
	toolv1 "aranea-agents/api/kratos/tool/v1"
	usagev1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/conf"
	servermw "aranea-agents/internal/server/middleware"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/validate"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

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
	graphSvc *service.GraphService,
	artifactSvc *service.ArtifactService,
	knowledgeSvc *service.KnowledgeService,
	evalSvc *service.EvaluationService,
	a2aSvc *service.A2AService,
	channelIngress *service.ChannelIngress,
	wsSrv *WSServer,
) *kratoshttp.Server {
	var opts = []kratoshttp.ServerOption{
		kratoshttp.Filter(
			CorsDevFilter(),
			auth.Middleware(),
			servermw.WorkspaceFilter(),
		),
		kratoshttp.Middleware(
			tracing.Server(),
			recovery.Recovery(),
			validate.Middleware(),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, kratoshttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratoshttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, kratoshttp.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := kratoshttp.NewServer(opts...)

	registerProtoServices(srv, admin, avatar, agents, agentCat, llm, hookSvc, cronSvc,
		pluginSvc, mcpSvc, skillSvc, toolSvc, sessionSvc, channelSvc, usageSvc,
		monitorSvc, memorySvc, systemSettingSvc, teams, chatSvc, graphSvc,
		artifactSvc, knowledgeSvc, evalSvc, a2aSvc)
	registerCustomRoutes(srv, channelIngress, skillSvc, artifactSvc)
	registerInfrastructureRoutes(srv)
	wsSrv.RegisterOnKratos(srv)

	return srv
}

func registerProtoServices(
	srv *kratoshttp.Server,
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
) {
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
	chatv1.RegisterChatServiceHTTPServer(srv, chatSvc)
	graphv1.RegisterGraphServiceHTTPServer(srv, graphSvc)
	artifactv1.RegisterArtifactServiceHTTPServer(srv, artifactSvc)
	knowledgev1.RegisterKnowledgeServiceHTTPServer(srv, knowledgeSvc)
	evaluationv1.RegisterEvaluationServiceHTTPServer(srv, evalSvc)
	a2av1.RegisterA2AServiceHTTPServer(srv, a2aSvc)
}

func registerCustomRoutes(
	srv *kratoshttp.Server,
	channelIngress *service.ChannelIngress,
	skillSvc *service.SkillService,
	artifactSvc *service.ArtifactService,
) {
	if channelIngress != nil {
		auth.RegisterWebhookPath("/webhooks/")
		srv.Route("/").POST("/webhooks/{channel_key}", channelIngress.FeishuWebhookHTTP())
	}
	if skillSvc != nil {
		skillSvc.RegisterSkillImportMultipart(srv)
	}
	if artifactSvc != nil {
		auth.RegisterNoAuthPath("/v1/artifacts/download")
		srv.Route("/").GET("/v1/artifacts/download", func(ctx kratoshttp.Context) error {
			artifactSvc.ServeSignedDownload(ctx.Response(), ctx.Request())
			return nil
		})
	}
}

func registerInfrastructureRoutes(srv *kratoshttp.Server) {
	srv.Route("/").GET("/healthz", func(ctx kratoshttp.Context) error {
		w := ctx.Response()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(nethttp.StatusOK)
		return json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	srv.Route("/").GET("/metrics", func(ctx kratoshttp.Context) error {
		promhttp.Handler().ServeHTTP(ctx.Response(), ctx.Request())
		return nil
	})
}
