package server

import (
	"encoding/json"
	nethttp "net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	a2av1 "aranea-agents/api/kratos/a2a/v1"
	adminv1 "aranea-agents/api/kratos/admin/v1"
	agentv1 "aranea-agents/api/kratos/agent/v1"
	airefinev1 "aranea-agents/api/kratos/ai_refine/v1"
	artifactv1 "aranea-agents/api/kratos/artifact/v1"
	avatarv1 "aranea-agents/api/kratos/avatar/v1"
	channelv1 "aranea-agents/api/kratos/channel/v1"
	chatv1 "aranea-agents/api/kratos/chat/v1"
	cronv1 "aranea-agents/api/kratos/cron/v1"
	ecosystemv1 "aranea-agents/api/kratos/ecosystem/v1"
	evaluationv1 "aranea-agents/api/kratos/evaluation/v1"
	eventv1 "aranea-agents/api/kratos/event/v1"
	gatewayv1 "aranea-agents/api/kratos/gateway/v1"
	graphv1 "aranea-agents/api/kratos/graph/v1"
	hookv1 "aranea-agents/api/kratos/hook/v1"
	industrytaxonomyv1 "aranea-agents/api/kratos/industry_taxonomy/v1"
	knowledgev1 "aranea-agents/api/kratos/knowledge/v1"
	llmprovidermodelv1 "aranea-agents/api/kratos/llm_provider_model/v1"
	mcpserverv1 "aranea-agents/api/kratos/mcp_server/v1"
	memoryv1 "aranea-agents/api/kratos/memory/v1"
	modelcatalogv1 "aranea-agents/api/kratos/model_catalog/v1"
	monitorv1 "aranea-agents/api/kratos/monitor/v1"
	packv1 "aranea-agents/api/kratos/pack/v1"
	pluginv1 "aranea-agents/api/kratos/plugin/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	skillv1 "aranea-agents/api/kratos/skill/v1"
	skillintlv1 "aranea-agents/api/kratos/skill_intelligence/v1"
	skilldedupv1 "aranea-agents/api/kratos/skill_dedup/v1"
	skillevov1 "aranea-agents/api/kratos/skill_evolution/v1"
	skillevosuggv1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	systemsettingv1 "aranea-agents/api/kratos/system_setting/v1"
	teamv1 "aranea-agents/api/kratos/team/v1"
	taxonomyv1 "aranea-agents/api/kratos/taxonomy/v1"
	toolv1 "aranea-agents/api/kratos/tool/v1"
	usagev1 "aranea-agents/api/kratos/usage/v1"
	a2atrpc "aranea-agents/internal/a2a/trpc"
	"aranea-agents/internal/conf"
	servermw "aranea-agents/internal/server/middleware"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/validate"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

type ReadinessProbe interface {
	IsReady() bool
	IsFailed() bool
	FailedReason() string
}

func NewHTTPServer(c *conf.Server, s *ServiceRegistry, wsSrv *WSServer, readiness ReadinessProbe, lg loggateway.Logger) *kratoshttp.Server {
	var opts = []kratoshttp.ServerOption{
		kratoshttp.Filter(
			CorsDevFilter(),
			auth.Middleware(lg),
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

	// Custom routes MUST be registered before proto services so that exact
	// paths (e.g. /v1/artifacts/download) take priority over wildcard
	// patterns (e.g. /v1/artifacts/{id}).
	registerCustomRoutes(srv, s.ChannelIngress, s.Skill, s.Artifact, s.A2APublic, s.SystemSetting, s.EcosystemPreset)
	registerProtoServices(srv, s)
	registerInfrastructureRoutes(srv, readiness)
	wsSrv.RegisterOnKratos(srv)

	return srv
}

func registerProtoServices(srv *kratoshttp.Server, s *ServiceRegistry) {
	adminv1.RegisterAdminServiceHTTPServer(srv, s.Admin)
	avatarv1.RegisterAvatarServiceHTTPServer(srv, s.Avatar)
	agentv1.RegisterAgentServiceHTTPServer(srv, s.Agents)
	if s.AIRefine != nil {
		airefinev1.RegisterAIRefineServiceHTTPServer(srv, s.AIRefine)
	}
	llmprovidermodelv1.RegisterLlmProviderModelServiceHTTPServer(srv, s.LLM)
	hookv1.RegisterHookServiceHTTPServer(srv, s.Hook)
	cronv1.RegisterCronServiceHTTPServer(srv, s.Cron)
	pluginv1.RegisterPluginServiceHTTPServer(srv, s.Plugin)
	mcpserverv1.RegisterMCPServerServiceHTTPServer(srv, s.MCPServer)
	skillv1.RegisterSkillServiceHTTPServer(srv, s.Skill)
	toolv1.RegisterToolServiceHTTPServer(srv, s.Tool)
	sessionv1.RegisterSessionServiceHTTPServer(srv, s.Session)
	channelv1.RegisterChannelServiceHTTPServer(srv, s.Channel)
	usagev1.RegisterUsageServiceHTTPServer(srv, s.Usage)
	monitorv1.RegisterMonitorServiceHTTPServer(srv, s.Monitor)
	memoryv1.RegisterMemoryServiceHTTPServer(srv, s.Memory)
	systemsettingv1.RegisterSystemSettingServiceHTTPServer(srv, s.SystemSetting)
	modelcatalogv1.RegisterModelCatalogServiceHTTPServer(srv, s.ModelCatalog)
	teamv1.RegisterTeamServiceHTTPServer(srv, s.Teams)
	chatv1.RegisterChatServiceHTTPServer(srv, s.Chat)
	graphv1.RegisterGraphServiceHTTPServer(srv, s.Graph)
	artifactv1.RegisterArtifactServiceHTTPServer(srv, s.Artifact)
	knowledgev1.RegisterKnowledgeServiceHTTPServer(srv, s.Knowledge)
	evaluationv1.RegisterEvaluationServiceHTTPServer(srv, s.Eval)
	a2av1.RegisterA2AServiceHTTPServer(srv, s.A2A)
	ecosystemv1.RegisterEcosystemServiceHTTPServer(srv, s.Ecosystem)
	eventv1.RegisterEventServiceHTTPServer(srv, s.Event)
	gatewayv1.RegisterGatewayServiceHTTPServer(srv, s.Gateway)
	taxonomyv1.RegisterTaxonomyServiceHTTPServer(srv, s.Taxonomy)
	industrytaxonomyv1.RegisterIndustryTaxonomyServiceHTTPServer(srv, s.IndustryTaxonomy)
	skillevov1.RegisterSkillEvolutionServiceHTTPServer(srv, s.SkillEvo)
	skillintlv1.RegisterSkillIntelligenceServiceHTTPServer(srv, s.SkillIntel)
	skilldedupv1.RegisterSkillDedupServiceHTTPServer(srv, s.SkillDedup)
	skillevosuggv1.RegisterSkillEvolutionSuggestionServiceHTTPServer(srv, s.SkillEvoSuggestion)
	packv1.RegisterPackServiceHTTPServer(srv, s.Pack)
}

// registerCustomRoutes registers cross-cutting operational routes that bypass proto
// registration. These routes have specific requirements that make proto registration
// impractical:
//   - /webhooks/{channel_key}: third-party webhook callbacks with varying path segments
//   - /v1/artifacts/download: signed download with direct response writer access
//   - skill import multipart: file upload handling
//
// All custom routes are explicitly documented here for auditability. New bypass routes
// MUST be added to this centralized block with justification comments.
func registerCustomRoutes(
	srv *kratoshttp.Server,
	channelIngress *service.ChannelIngress,
	skillSvc *service.SkillService,
	artifactSvc *service.ArtifactService,
	a2aPublic *a2atrpc.EndpointRegistry,
	systemSettingSvc *service.SystemSettingService,
	ecosystemPresetSvc *service.EcosystemPresetService,
) {
	// GET /v1/system/info — CLI info endpoint; requires auth (not in noAuthPaths).
	if systemSettingSvc != nil {
		srv.Route("/").GET("/v1/system/info", systemSettingSvc.GetSystemInfoHandler("", "", ""))
	}
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
	if a2aPublic != nil {
		auth.RegisterNoAuthPathPrefix(a2atrpc.PublicPathPrefix)
		srv.HandlePrefix(a2atrpc.PublicPathPrefix, a2aPublic)
	}
	// Ecosystem preset admin routes: load/unload/status for industry presets.
	if ecosystemPresetSvc != nil {
		srv.Route("/").POST("/api/v1/admin/ecosystem/preset/load", ecosystemPresetSvc.HandleLoad())
		srv.Route("/").POST("/api/v1/admin/ecosystem/preset/unload", ecosystemPresetSvc.HandleUnload())
		srv.Route("/").GET("/api/v1/admin/ecosystem/preset/status", ecosystemPresetSvc.HandleStatus())
	}
}

func registerInfrastructureRoutes(srv *kratoshttp.Server, readiness ReadinessProbe) {
	srv.Route("/").GET("/healthz", func(ctx kratoshttp.Context) error {
		w := ctx.Response()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if readiness != nil && readiness.IsFailed() {
			w.WriteHeader(nethttp.StatusServiceUnavailable)
			return json.NewEncoder(w).Encode(map[string]string{
				"status":    "failed",
				"reason":    readiness.FailedReason(),
				"auth_mode": auth.HealthAuthInfo().AuthMode,
			})
		}
		if readiness != nil && !readiness.IsReady() {
			w.WriteHeader(nethttp.StatusServiceUnavailable)
			return json.NewEncoder(w).Encode(map[string]string{
				"status":    "starting",
				"auth_mode": auth.HealthAuthInfo().AuthMode,
			})
		}
		w.WriteHeader(nethttp.StatusOK)
		return json.NewEncoder(w).Encode(auth.HealthAuthInfo())
	})
	srv.Route("/").GET("/metrics", func(ctx kratoshttp.Context) error {
		promhttp.Handler().ServeHTTP(ctx.Response(), ctx.Request())
		return nil
	})
}
