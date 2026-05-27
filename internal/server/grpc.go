package server

import (
	a2av1 "aranea-agents/api/kratos/a2a/v1"
	adminv1 "aranea-agents/api/kratos/admin/v1"
	agentv1 "aranea-agents/api/kratos/agent/v1"
	agentcategoryv1 "aranea-agents/api/kratos/agent_category/v1"
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
	knowledgev1 "aranea-agents/api/kratos/knowledge/v1"
	llmprovidermodelv1 "aranea-agents/api/kratos/llm_provider_model/v1"
	mcpserverv1 "aranea-agents/api/kratos/mcp_server/v1"
	memoryv1 "aranea-agents/api/kratos/memory/v1"
	modelcatalogv1 "aranea-agents/api/kratos/model_catalog/v1"
	monitorv1 "aranea-agents/api/kratos/monitor/v1"
	pluginv1 "aranea-agents/api/kratos/plugin/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
	skillv1 "aranea-agents/api/kratos/skill/v1"
	systemsettingv1 "aranea-agents/api/kratos/system_setting/v1"
	teamv1 "aranea-agents/api/kratos/team/v1"
	toolv1 "aranea-agents/api/kratos/tool/v1"
	usagev1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/validate"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Server, s *ServiceRegistry) *grpc.Server {
	var opts = []grpc.ServerOption{
		// EP-OBS-02: tracing.Server() spans all gRPC calls when OTel is configured.
		// EP-SEC-04: auth.GRPCMiddleware() validates Bearer JWT from gRPC metadata.
		grpc.Middleware(
			tracing.Server(),
			auth.GRPCMiddleware(),
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
	adminv1.RegisterAdminServiceServer(srv, s.Admin)
	avatarv1.RegisterAvatarServiceServer(srv, s.Avatar)
	agentv1.RegisterAgentServiceServer(srv, s.Agents)
	agentcategoryv1.RegisterAgentCategoryServiceServer(srv, s.AgentCat)
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
	channelv1.RegisterChannelServiceServer(srv, s.Channel)
	usagev1.RegisterUsageServiceServer(srv, s.Usage)
	monitorv1.RegisterMonitorServiceServer(srv, s.Monitor)
	memoryv1.RegisterMemoryServiceServer(srv, s.Memory)
	systemsettingv1.RegisterSystemSettingServiceServer(srv, s.SystemSetting)
	modelcatalogv1.RegisterModelCatalogServiceServer(srv, s.ModelCatalog)
	teamv1.RegisterTeamServiceServer(srv, s.Teams)
	chatv1.RegisterChatServiceServer(srv, s.Chat)
	graphv1.RegisterGraphServiceServer(srv, s.Graph)
	artifactv1.RegisterArtifactServiceServer(srv, s.Artifact)
	knowledgev1.RegisterKnowledgeServiceServer(srv, s.Knowledge)
	evaluationv1.RegisterEvaluationServiceServer(srv, s.Eval)
	a2av1.RegisterA2AServiceServer(srv, s.A2A)
	ecosystemv1.RegisterEcosystemServiceServer(srv, s.Ecosystem)
	eventv1.RegisterEventServiceServer(srv, s.Event)
	gatewayv1.RegisterGatewayServiceServer(srv, s.Gateway)
	return srv
}
