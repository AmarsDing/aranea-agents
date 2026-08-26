package server

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	a2av1 "aranea-agents/api/kratos/a2a/v1"
	adminv1 "aranea-agents/api/kratos/admin/v1"
	agentv1 "aranea-agents/api/kratos/agent/v1"
	agentbridgev1 "aranea-agents/api/kratos/agentbridge/v1"
	airefinev1 "aranea-agents/api/kratos/ai_refine/v1"
	artifactv1 "aranea-agents/api/kratos/artifact/v1"
	avatarv1 "aranea-agents/api/kratos/avatar/v1"
	channelv1 "aranea-agents/api/kratos/channel/v1"
	chatv1 "aranea-agents/api/kratos/chat/v1"
	computerusev1 "aranea-agents/api/kratos/computeruse/v1"
	cronv1 "aranea-agents/api/kratos/cron/v1"
	ecosystemv1 "aranea-agents/api/kratos/ecosystem/v1"
	evaluationv1 "aranea-agents/api/kratos/evaluation/v1"
	evolutionv1 "aranea-agents/api/kratos/evolution/v1"
	gatewayv1 "aranea-agents/api/kratos/gateway/v1"
	graphv1 "aranea-agents/api/kratos/graph/v1"
	hookv1 "aranea-agents/api/kratos/hook/v1"
	knowledgev1 "aranea-agents/api/kratos/knowledge/v1"
	learningloopv1 "aranea-agents/api/kratos/learning_loop/v1"
	llmprovidermodelv1 "aranea-agents/api/kratos/llm_provider_model/v1"
	mcpserverv1 "aranea-agents/api/kratos/mcp_server/v1"
	memoryv1 "aranea-agents/api/kratos/memory/v1"
	modelcatalogv1 "aranea-agents/api/kratos/model_catalog/v1"
	monitorv1 "aranea-agents/api/kratos/monitor/v1"
	organizationv1 "aranea-agents/api/kratos/organization/v1"
	packv1 "aranea-agents/api/kratos/pack/v1"
	pluginv1 "aranea-agents/api/kratos/plugin/v1"
	runtimeprofilev1 "aranea-agents/api/kratos/runtime_profile/v1"
	sandboxv1 "aranea-agents/api/kratos/sandbox/v1"
	decisionv1 "aranea-agents/api/kratos/decision/v1"
	selfimprovementv1 "aranea-agents/api/kratos/self_improvement/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	skillv1 "aranea-agents/api/kratos/skill/v1"
	skilldedupv1 "aranea-agents/api/kratos/skill_dedup/v1"
	skillevosuggv1 "aranea-agents/api/kratos/skill_evolution_suggestion/v1"
	skillintlv1 "aranea-agents/api/kratos/skill_intelligence/v1"
	systemsettingv1 "aranea-agents/api/kratos/system_setting/v1"
	taxonomyv1 "aranea-agents/api/kratos/taxonomy/v1"
	teamv1 "aranea-agents/api/kratos/team/v1"
	toolv1 "aranea-agents/api/kratos/tool/v1"
	usagev1 "aranea-agents/api/kratos/usage/v1"
	a2atrpc "aranea-agents/internal/a2a/trpc"
	"aranea-agents/internal/conf"
	servermw "aranea-agents/internal/server/middleware"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/validate"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

type ReadinessProbe interface {
	IsReady() bool
	IsFailed() bool
	FailedReason() string
}

func NewHTTPServer(c *conf.Server, s *ServiceRegistry, wsSrv *WSServer, voiceSrv *VoiceWSServer, readiness ReadinessProbe, lg loggateway.Logger) *kratoshttp.Server {
	var opts = []kratoshttp.ServerOption{
		kratoshttp.Filter(
			CorsDevFilter(),
			readinessFilter(readiness),
			loginRateLimitFilter(newLoginLimiter(time.Now), lg),
			auth.Middleware(lg),
			servermw.WorkspaceFilter(),
		),
		kratoshttp.Middleware(
			tracing.Server(),
			recovery.Recovery(),
			servermw.APIToKratos(),
			validate.Middleware(),
		),
		// Skill ZIP import accepts JSON bytes or legacy multipart/form-data.
		kratoshttp.RequestDecoder(service.DecodeSkillImportRequest),
		// Ensure all JSON responses declare charset=utf-8 to prevent
		// encoding misinterpretation on Windows (e.g. GBK default).
		kratoshttp.ResponseEncoder(utf8ResponseEncoder),
		kratoshttp.ErrorEncoder(utf8ErrorEncoder),
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
	registerCustomRoutes(srv, s.ChannelIngress, s.Artifact, s.Knowledge, s.A2APublic, s.SystemSetting, s.EcosystemPreset, s.AGUICompat, s.OpenAISession, s.A2AExtension, s.TwinOpenAPI, s.ConfigGraph, s.Diagnostics)
	registerProtoServices(srv, s)
	registerCompatibilityRedirects(srv)
	registerInfrastructureRoutes(srv, readiness)
	wsSrv.RegisterOnKratos(srv)
	if voiceSrv != nil {
		voiceSrv.RegisterOnKratos(srv)
	}

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
	sessionv1.RegisterSessionV2ServiceHTTPServer(srv, s.SessionV2)
	channelv1.RegisterChannelServiceHTTPServer(srv, s.Channel)
	usagev1.RegisterUsageServiceHTTPServer(srv, s.Usage)
	monitorv1.RegisterMonitorServiceHTTPServer(srv, s.Monitor)
	memoryv1.RegisterMemoryServiceHTTPServer(srv, s.Memory)
	systemsettingv1.RegisterSystemSettingServiceHTTPServer(srv, s.SystemSetting)
	modelcatalogv1.RegisterModelCatalogServiceHTTPServer(srv, s.ModelCatalog)
	teamv1.RegisterTeamServiceHTTPServer(srv, s.Teams)
	chatv1.RegisterChatServiceHTTPServer(srv, s.Chat)
	computerusev1.RegisterComputerUseServiceHTTPServer(srv, s.ComputerUse)
	agentbridgev1.RegisterAgentBridgeServiceHTTPServer(srv, s.AgentBridge)
	graphv1.RegisterGraphServiceHTTPServer(srv, s.Graph)
	artifactv1.RegisterArtifactServiceHTTPServer(srv, s.Artifact)
	knowledgev1.RegisterKnowledgeServiceHTTPServer(srv, s.Knowledge)
	evaluationv1.RegisterEvaluationServiceHTTPServer(srv, s.Eval)
	a2av1.RegisterA2AServiceHTTPServer(srv, s.A2A)
	a2av1.RegisterFederationServiceHTTPServer(srv, s.Federation)
	ecosystemv1.RegisterEcosystemServiceHTTPServer(srv, s.Ecosystem)
	gatewayv1.RegisterGatewayServiceHTTPServer(srv, s.Gateway)
	taxonomyv1.RegisterTaxonomyServiceHTTPServer(srv, s.Taxonomy)
	organizationv1.RegisterOrganizationServiceHTTPServer(srv, s.Organization)
	skillintlv1.RegisterSkillIntelligenceServiceHTTPServer(srv, s.SkillIntel)
	skilldedupv1.RegisterSkillDedupServiceHTTPServer(srv, s.SkillDedup)
	skillevosuggv1.RegisterSkillEvolutionSuggestionServiceHTTPServer(srv, s.SkillEvoSuggestion)
	evolutionv1.RegisterEvolutionServiceHTTPServer(srv, s.Evolution)
	packv1.RegisterPackServiceHTTPServer(srv, s.Pack)
	runtimeprofilev1.RegisterRuntimeProfileServiceHTTPServer(srv, s.RuntimeProfile)
	sandboxv1.RegisterSandboxServiceHTTPServer(srv, s.Sandbox)
	if s.DecisionRecord != nil {
		decisionv1.RegisterDecisionRecordServiceHTTPServer(srv, s.DecisionRecord)
	}
	if s.LearningLoop != nil {
		learningloopv1.RegisterLearningLoopServiceHTTPServer(srv, s.LearningLoop)
	}
	if s.SelfImprovement != nil {
		selfimprovementv1.RegisterSelfImprovementServiceHTTPServer(srv, s.SelfImprovement)
	}
}

// registerCustomRoutes registers cross-cutting operational routes that bypass proto
// registration. These routes have specific requirements that make proto registration
// impractical:
//   - /webhooks/{channel_key}: third-party webhook callbacks with varying path segments
//   - /v1/artifacts/download: signed download with direct response writer access
//   - /v1/knowledge/documents/{id}/asset: raw file streaming (G2-B6, Range/inline media)
//   - /v1/knowledge/documents/{id}/autolink-preview|autolink: confirm-then-apply outgoing wikilinks
//   - /v1/knowledge/collections/{id}/health|experts|writeback-pending: SP7 G7/G8 + pending gate
//   - /api/v1/config-graph/rebuild|status|nodes|nodes/{type}/{ref}/impact|dependencies|edges|health:
//     M81 config-asset graph (handwritten JSON, generation-scoped reads;
//     async rebuild trigger must outlive the request ctx)
//   - /api/v1/admin/diagnostics: 79-runtime-governance R8 doctor API（同
//     ecosystem preset 先例：/api/v1 子树被 twinOpenAPI 前缀接管，proto
//     注册次序在其后会被遮蔽，故走 custom route）
//
// All custom routes are explicitly documented here for auditability. New bypass routes
// MUST be added to this centralized block with justification comments.
func registerCustomRoutes(
	srv *kratoshttp.Server,
	channelIngress *service.ChannelIngress,
	artifactSvc *service.ArtifactService,
	knowledgeSvc *service.KnowledgeService,
	a2aPublic *a2atrpc.EndpointRegistry,
	systemSettingSvc *service.SystemSettingService,
	ecosystemPresetSvc *service.EcosystemPresetService,
	aguiCompat *service.AGUICompatService,
	openaiSession *service.OpenAISessionCompatService,
	a2aExtension *service.A2AExtensionCompatService,
	twinOpenAPI *service.TwinOpenAPICompatService,
	configGraphSvc *service.ConfigGraphService,
	diagnosticsSvc *service.DiagnosticsService,
) {
	// GET /v1/system/info — CLI info endpoint; requires auth (not in noAuthPaths).
	if systemSettingSvc != nil {
		srv.Route("/").GET("/v1/system/info", systemSettingSvc.GetSystemInfoHandler("", "", ""))
	}
	if channelIngress != nil {
		auth.RegisterWebhookPath("/webhooks/")
		srv.Route("/").POST("/webhooks/{channel_key}", channelIngress.FeishuWebhookHTTP())
	}
	if artifactSvc != nil {
		auth.RegisterNoAuthPath("/v1/artifacts/download")
		srv.Route("/").GET("/v1/artifacts/download", func(ctx kratoshttp.Context) error {
			artifactSvc.ServeSignedDownload(ctx.Response(), ctx.Request())
			return nil
		})
		// POST /v1/system/reveal — M27 Phase 5 本地打开文件夹；默认关闭
		// （FEATURES_LOCAL_REVEAL_ENABLED），仅本地单机部署启用。未开启时
		// 路由不注册（404），前端经 GET /v1/system/info features.local_reveal 探知。
		if conf.LocalRevealEnabled() {
			srv.Route("/").POST("/v1/system/reveal", func(ctx kratoshttp.Context) error {
				artifactSvc.ServeRevealLocal(ctx.Response(), ctx.Request())
				return nil
			})
		}
	}
	if knowledgeSvc != nil {
		// G2-B6：原始文件流式输出（图片/音频/视频 inline，word 下载，Range 拖动）。
		// 鉴权走标准 auth 过滤器（前端 fetch 带 JWT → blob/object URL 渲染）。
		srv.Route("/").GET("/v1/knowledge/documents/{id}/asset", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeDocumentAsset(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		// US-39：出链成链预览/确认（custom JSON，避免 make api）。
		srv.Route("/").GET("/v1/knowledge/documents/{id}/autolink-preview", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeAutolinkPreview(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		srv.Route("/").POST("/v1/knowledge/documents/{id}/autolink", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeAutolinkApply(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		srv.Route("/").GET("/v1/knowledge/documents/{id}/visibility", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeDocumentVisibility(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		srv.Route("/").POST("/v1/knowledge/documents/{id}/visibility", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeDocumentVisibility(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		// US-43 / US-42 / US-44：健康度、专家、待确认写回。
		srv.Route("/").GET("/v1/knowledge/collections/{id}/health", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeCollectionHealth(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		srv.Route("/").GET("/v1/knowledge/collections/{id}/experts", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeCollectionExperts(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		srv.Route("/").GET("/v1/knowledge/collections/{id}/writeback-pending", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeWriteBackPending(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		srv.Route("/").POST("/v1/knowledge/collections/{id}/writeback-pending/apply", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeWriteBackPendingApply(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		// US-45：显式成链回填（会改写 Markdown）；US-46：写回落点只读解析。
		srv.Route("/").POST("/v1/knowledge/collections/{id}/autolink-backfill", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeAutolinkBackfill(ctx.Response(), ctx.Request(), ctx.Vars().Get("id"))
			return nil
		})
		srv.Route("/").GET("/v1/knowledge/writeback-home", func(ctx kratoshttp.Context) error {
			knowledgeSvc.ServeWriteBackHome(ctx.Response(), ctx.Request())
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
	// M81 config-asset graph (design §6): handwritten JSON admin API. Registered
	// before the twinOpenAPI /api/v1 prefix so exact paths win; JWT-protected
	// (not in noAuthPaths), admin role enforced inside the handlers.
	if configGraphSvc != nil {
		srv.Route("/").POST("/api/v1/config-graph/rebuild", func(ctx kratoshttp.Context) error {
			configGraphSvc.ServeRebuild(ctx.Response(), ctx.Request())
			return nil
		})
		srv.Route("/").GET("/api/v1/config-graph/status", func(ctx kratoshttp.Context) error {
			configGraphSvc.ServeStatus(ctx.Response(), ctx.Request())
			return nil
		})
		srv.Route("/").GET("/api/v1/config-graph/nodes", func(ctx kratoshttp.Context) error {
			configGraphSvc.ServeNodes(ctx.Response(), ctx.Request())
			return nil
		})
		// P1 查询端点（design §5/§6）：{type}/{ref} 双解（ref_id 或 node_key）。
		srv.Route("/").GET("/api/v1/config-graph/nodes/{type}/{ref}/impact", func(ctx kratoshttp.Context) error {
			configGraphSvc.ServeImpact(ctx.Response(), ctx.Request(), ctx.Vars().Get("type"), ctx.Vars().Get("ref"))
			return nil
		})
		srv.Route("/").GET("/api/v1/config-graph/nodes/{type}/{ref}/dependencies", func(ctx kratoshttp.Context) error {
			configGraphSvc.ServeDependencies(ctx.Response(), ctx.Request(), ctx.Vars().Get("type"), ctx.Vars().Get("ref"))
			return nil
		})
		srv.Route("/").GET("/api/v1/config-graph/nodes/{type}/{ref}/edges", func(ctx kratoshttp.Context) error {
			configGraphSvc.ServeNodeEdges(ctx.Response(), ctx.Request(), ctx.Vars().Get("type"), ctx.Vars().Get("ref"))
			return nil
		})
		srv.Route("/").GET("/api/v1/config-graph/health", func(ctx kratoshttp.Context) error {
			configGraphSvc.ServeHealth(ctx.Response(), ctx.Request())
			return nil
		})
	}
	// 79-runtime-governance R8: doctor API。同 config-graph 先例——注册在
	// twinOpenAPI /api/v1 前缀之前，JWT 保护 + handler 内 admin 校验。
	if diagnosticsSvc != nil {
		srv.Route("/").GET("/api/v1/admin/diagnostics", func(ctx kratoshttp.Context) error {
			diagnosticsSvc.ServeDiagnostics(ctx.Response(), ctx.Request())
			return nil
		})
		// audit.py 服务侧单源取数口（复算下线，design §9.1 ADR C2）。
		srv.Route("/").GET("/api/v1/admin/tool-assembly/reconcile", func(ctx kratoshttp.Context) error {
			diagnosticsSvc.ServeToolAssemblyReconcile(ctx.Response(), ctx.Request())
			return nil
		})
	}
	// Compat server adapter routes. Each service lazily initializes its
	// underlying framework server on the first request via Handler(ctx).
	if aguiCompat != nil && aguiCompat.Enabled() {
		srv.HandlePrefix(aguiCompat.Path(), lazyCompatHandler(aguiCompat))
	}
	if openaiSession != nil && openaiSession.Enabled() {
		srv.Handle(openaiSession.Path(), lazyCompatHandler(openaiSession))
	}
	if a2aExtension != nil && a2aExtension.Enabled() {
		srv.HandlePrefix(a2aExtension.Path(), lazyCompatHandler(a2aExtension))
	}
	// TwinOpenAPI 门面（twinmonitor 机器对接）：自校验 Bearer token
	// （ARANEA_TWINOPENAPI_TOKEN），门面子树注册为 noAuth 以跳过 JWT 过滤器。
	// 注意：/api/v1/admin/* 不在豁免清单内，仍走 JWT 保护；且 mux 按注册
	// 次序匹配，上方 admin 精确路由优先于此前缀。
	if twinOpenAPI != nil && twinOpenAPI.Enabled() {
		auth.RegisterNoAuthPath("/api/v1/health")
		auth.RegisterNoAuthPathPrefix("/api/v1/agents")
		auth.RegisterNoAuthPathPrefix("/api/v1/graphs")
		auth.RegisterNoAuthPathPrefix("/api/v1/runs")
		auth.RegisterNoAuthPathPrefix("/api/v1/memory/")
		auth.RegisterNoAuthPathPrefix("/api/v1/quota/")
		auth.RegisterNoAuthPathPrefix("/api/v1/metrics/")
		srv.HandlePrefix(twinOpenAPI.Path(), lazyCompatHandler(twinOpenAPI))
	}
}

// compatHandler is the minimal interface satisfied by all three compat
// service wrappers (AGUICompatService, OpenAISessionCompatService,
// A2AExtensionCompatService). It allows lazyCompatHandler to wrap any of
// them uniformly.
type compatHandler interface {
	Handler(ctx context.Context) (nethttp.Handler, error)
}

// lazyCompatHandler returns an http.Handler that lazily resolves the
// underlying framework handler on the first request. This preserves the
// double-check locking lazy-init pattern used by the compat services.
func lazyCompatHandler(svc compatHandler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		h, err := svc.Handler(r.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(nethttp.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":500,"reason":"COMPAT_INIT","message":"failed to initialize compat server"}`))
			return
		}
		if h == nil {
			w.WriteHeader(nethttp.StatusServiceUnavailable)
			return
		}
		h.ServeHTTP(w, r)
	})
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

// utf8ResponseEncoder wraps Kratos DefaultResponseEncoder to ensure
// Content-Type includes charset=utf-8. Without this, Kratos sets
// "application/json" without charset, which can cause Windows HTTP clients
// to misinterpret UTF-8 bytes as GBK/GB2312, rendering Chinese as "????".
func utf8ResponseEncoder(w nethttp.ResponseWriter, r *nethttp.Request, v interface{}) error {
	codec, _ := kratoshttp.CodecForRequest(r, "Accept")
	data, err := codec.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentTypeWithCharset(codec.Name()))
	_, err = w.Write(data)
	return err
}

// utf8ErrorEncoder wraps Kratos DefaultErrorEncoder to ensure
// Content-Type includes charset=utf-8 in error responses.
func utf8ErrorEncoder(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	se := errors.FromError(err)
	codec, _ := kratoshttp.CodecForRequest(r, "Accept")
	body, marshalErr := codec.Marshal(se)
	if marshalErr != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(nethttp.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"reason":"CODEC","message":"internal marshal error"}`))
		return
	}
	w.Header().Set("Content-Type", contentTypeWithCharset(codec.Name()))
	w.WriteHeader(int(se.Code))
	_, _ = w.Write(body)
}

// contentTypeWithCharset returns "application/<subtype>; charset=utf-8"
// for known text-based subtypes, or "application/<subtype>" otherwise.
func contentTypeWithCharset(subtype string) string {
	base := "application/" + subtype
	switch subtype {
	case "json", "xml", "x-www-form-urlencoded":
		return base + "; charset=utf-8"
	default:
		return base
	}
}

// registerCompatibilityRedirects adds HTTP 307 redirects from legacy
// /v1/taxonomy/* paths to /v1/organization/* so that old client code
// is transparently forwarded to the new API. These are registered after
// proto services so the actual taxonomy handlers take priority when
// clients still call the old endpoints directly.
func registerCompatibilityRedirects(srv *kratoshttp.Server) {
	redirects := map[string]string{
		"/v1/taxonomy/legacy/list": "/v1/organization",
		"/v1/taxonomy/legacy/tree": "/v1/organization/tree",
	}
	for from, to := range redirects {
		srv.Route("/").GET(from, func(ctx kratoshttp.Context) error {
			ctx.Response().Header().Set("Location", to)
			ctx.Response().WriteHeader(nethttp.StatusTemporaryRedirect)
			return nil
		})
	}
}

// readinessFilter returns an HTTP filter that rejects all requests except
// infrastructure routes (/healthz, /metrics) when the server is not ready.
// This allows the HTTP server to start listening immediately while P1
// migrations run in the background, so /healthz can properly report
// "starting" (503) instead of the connection being refused entirely.
func readinessFilter(readiness ReadinessProbe) func(next nethttp.Handler) nethttp.Handler {
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			if readiness == nil || readiness.IsReady() {
				next.ServeHTTP(w, r)
				return
			}
			// Allow infrastructure routes through so /healthz reports accurate status.
			switch r.URL.Path {
			case "/healthz", "/metrics":
				next.ServeHTTP(w, r)
				return
			}
			// Reject everything else with 503.
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(nethttp.StatusServiceUnavailable)
			status := "starting"
			reason := ""
			if readiness.IsFailed() {
				status = "failed"
				reason = readiness.FailedReason()
			}
			resp := map[string]string{"status": status, "auth_mode": auth.HealthAuthInfo().AuthMode}
			if reason != "" {
				resp["reason"] = reason
			}
			_ = json.NewEncoder(w).Encode(resp)
		})
	}
}
