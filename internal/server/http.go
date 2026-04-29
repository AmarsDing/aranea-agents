package server

import (
	adminv1 "aranea-agents/api/kratos/admin/v1"
	agentv1 "aranea-agents/api/kratos/agent/v1"
	agentcategoryv1 "aranea-agents/api/kratos/agent_category/v1"
	avatarv1 "aranea-agents/api/kratos/avatar/v1"
	hookv1 "aranea-agents/api/kratos/hook/v1"
	llmprovidermodelv1 "aranea-agents/api/kratos/llm_provider_model/v1"
	mcpserverv1 "aranea-agents/api/kratos/mcp_server/v1"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/service"
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
	mcpSvc *service.MCPServerService,
) *http.Server {
	var opts = []http.ServerOption{
		http.Filter(
			auth.Middleware(),
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
	mcpserverv1.RegisterMCPServerServiceHTTPServer(srv, mcpSvc)
	return srv
}
