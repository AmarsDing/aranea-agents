package server

import (
	"context"
	"sync"
	"time"

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
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	servermw "aranea-agents/internal/server/middleware"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/validate"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Server, s *ServiceRegistry, lg loggateway.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		// EP-OBS-02: tracing.Server() spans all gRPC calls when OTel is configured.
		// EP-SEC-04: auth.GRPCMiddleware() validates Bearer JWT from gRPC metadata.
		grpc.Middleware(
			tracing.Server(),
			auth.GRPCMiddleware(lg),
			recovery.Recovery(),
			servermw.APIToKratos(),
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
	adminv1.RegisterAdminServiceServer(srv, s.Admin)
	avatarv1.RegisterAvatarServiceServer(srv, s.Avatar)
	agentv1.RegisterAgentServiceServer(srv, s.Agents)
	if s.AIRefine != nil {
		airefinev1.RegisterAIRefineServiceServer(srv, s.AIRefine)
	}
	llmprovidermodelv1.RegisterLlmProviderModelServiceServer(srv, s.LLM)
	hookv1.RegisterHookServiceServer(srv, s.Hook)
	cronv1.RegisterCronServiceServer(srv, s.Cron)
	pluginv1.RegisterPluginServiceServer(srv, s.Plugin)
	mcpserverv1.RegisterMCPServerServiceServer(srv, s.MCPServer)
	skillv1.RegisterSkillServiceServer(srv, s.Skill)
	toolv1.RegisterToolServiceServer(srv, s.Tool)
	sessionv1.RegisterSessionServiceServer(srv, s.Session)
	sessionv1.RegisterSessionV2ServiceServer(srv, s.SessionV2)
	channelv1.RegisterChannelServiceServer(srv, s.Channel)
	usagev1.RegisterUsageServiceServer(srv, s.Usage)
	monitorv1.RegisterMonitorServiceServer(srv, s.Monitor)
	memoryv1.RegisterMemoryServiceServer(srv, s.Memory)
	systemsettingv1.RegisterSystemSettingServiceServer(srv, s.SystemSetting)
	modelcatalogv1.RegisterModelCatalogServiceServer(srv, s.ModelCatalog)
	teamv1.RegisterTeamServiceServer(srv, s.Teams)
	chatv1.RegisterChatServiceServer(srv, s.Chat)
	computerusev1.RegisterComputerUseServiceServer(srv, s.ComputerUse)
	agentbridgev1.RegisterAgentBridgeServiceServer(srv, s.AgentBridge)
	graphv1.RegisterGraphServiceServer(srv, s.Graph)
	artifactv1.RegisterArtifactServiceServer(srv, s.Artifact)
	knowledgev1.RegisterKnowledgeServiceServer(srv, s.Knowledge)
	evaluationv1.RegisterEvaluationServiceServer(srv, s.Eval)
	a2av1.RegisterA2AServiceServer(srv, s.A2A)
	a2av1.RegisterFederationServiceServer(srv, s.Federation)
	ecosystemv1.RegisterEcosystemServiceServer(srv, s.Ecosystem)
	gatewayv1.RegisterGatewayServiceServer(srv, s.Gateway)
	taxonomyv1.RegisterTaxonomyServiceServer(srv, s.Taxonomy)
	organizationv1.RegisterOrganizationServiceServer(srv, s.Organization)
	skillintlv1.RegisterSkillIntelligenceServiceServer(srv, s.SkillIntel)
	skilldedupv1.RegisterSkillDedupServiceServer(srv, s.SkillDedup)
	skillevosuggv1.RegisterSkillEvolutionSuggestionServiceServer(srv, s.SkillEvoSuggestion)
	evolutionv1.RegisterEvolutionServiceServer(srv, s.Evolution)
	packv1.RegisterPackServiceServer(srv, s.Pack)
	runtimeprofilev1.RegisterRuntimeProfileServiceServer(srv, s.RuntimeProfile)
	if s.LearningLoop != nil {
		learningloopv1.RegisterLearningLoopServiceServer(srv, s.LearningLoop)
	}
	if s.SelfImprovement != nil {
		selfimprovementv1.RegisterSelfImprovementServiceServer(srv, s.SelfImprovement)
	}
	return srv
}

// grpcUnauthenticatedFlowInterval bounds system.grpc.unauthenticated flow-log
// emission: gRPC is internal-only and most calls carry no token, so the hook
// fires on nearly every request — emit at most one event per minute.
const grpcUnauthenticatedFlowInterval = time.Minute

// RegisterAuthFlowHooks bridges pkg/auth authentication events to flow logs.
// pkg/ libraries cannot import internal/event, so pkg/auth exposes package
// hooks (SetOnGRPCUnauthenticated) and this function registers the emitting
// closures. Called once at startup from cmd/admin/app.go (newApp).
func RegisterAuthFlowHooks(bus contract.MonitorBus, lg loggateway.Logger) {
	if bus == nil {
		return
	}
	var mu sync.Mutex
	var last time.Time
	var suppressed int
	auth.SetOnGRPCUnauthenticated(func(ctx context.Context) {
		mu.Lock()
		now := time.Now()
		if now.Sub(last) < grpcUnauthenticatedFlowInterval {
			suppressed++
			mu.Unlock()
			return
		}
		sup := suppressed
		suppressed = 0
		last = now
		mu.Unlock()
		em := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
			Ctx:    ctx,
			Domain: event.TraceDomainSystem,
			LG:     lg,
			Infra:  event.NewInfraFromBus(bus),
		})
		pairs := []event.Pair{}
		if sup > 0 {
			pairs = append(pairs, event.P("suppressed", sup))
		}
		em.LogWarn("system.grpc.unauthenticated", "", "gRPC 未认证请求（内部网络，M2 前按网络策略放行）", pairs...)
	})
}
