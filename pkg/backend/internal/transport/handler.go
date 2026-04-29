package transport

import (
	"net/http"

	caphttp "arenea/backend/internal/capability/adapters/http"
	cataloghttp "arenea/backend/internal/catalog/adapters/http"
	memoryhttp "arenea/backend/internal/memory/adapters/http"
	"arenea/backend/internal/service"
)

// Services 聚合 HTTP 层依赖的全部应用服务。使用单一结构体可在新增服务时
// 保持处理器构造函数稳定，并让调用方显式命名字段而非依赖位置参数。
type Services struct {
	Agent    *service.AgentService
	Team     *service.TeamService
	Session  *service.SessionService
	Chat     *service.ChatService
	Audit    *service.AuditService
	Platform *service.PlatformService
	Usage    *service.UsageService
	Skill    *service.SkillService
	Tool     *service.ToolService
	Plugin   *service.PluginService
	Channel  *service.ChannelService
}

type HTTPHandler struct {
	agentSvc    *service.AgentService
	teamSvc     *service.TeamService
	sessionSvc  *service.SessionService
	chatSvc     *service.ChatService
	auditSvc    *service.AuditService
	platformSvc *service.PlatformService
	usageSvc    *service.UsageService
	skillSvc    *service.SkillService
	toolSvc     *service.ToolService
	pluginSvc   *service.PluginService
	channelSvc  *service.ChannelService
	evolution   *cataloghttp.EvolutionHTTP
	memory      *memoryhttp.MemoryHTTP
	tools       *caphttp.ToolHTTP
}

// NewHTTPHandler 将所有应用路由挂到默认 mux 上。*HTTPHandler 作为 Services 的
// 内部具现，使各处理方法可按名称访问各依赖。
func NewHTTPHandler(svc Services) http.Handler {
	h := &HTTPHandler{
		agentSvc:    svc.Agent,
		teamSvc:     svc.Team,
		sessionSvc:  svc.Session,
		chatSvc:     svc.Chat,
		auditSvc:    svc.Audit,
		platformSvc: svc.Platform,
		usageSvc:    svc.Usage,
		skillSvc:    svc.Skill,
		toolSvc:     svc.Tool,
		pluginSvc:   svc.Plugin,
		channelSvc:  svc.Channel,
	}
	h.evolution = cataloghttp.NewEvolutionHTTP(
		h.evolutionService,
		h.auditSvc,
		writeJSON, writeErr, decodeBody, methodNotAllowed, parsePositiveInt,
	)
	h.tools = caphttp.NewToolHTTP(
		h.toolSvc, h.auditSvc,
		writeJSON, writeErr, decodeBody, methodNotAllowed, pageParams, idFromPath,
	)
	h.memory = memoryhttp.NewMemoryHTTP(
		func() *service.MemoryL0Service {
			if h.chatSvc == nil {
				return nil
			}
			return h.chatSvc.MemoryL0()
		},
		func() *service.MemoryL1Service {
			if h.chatSvc == nil {
				return nil
			}
			return h.chatSvc.MemoryL1()
		},
		func() *service.MemoryL2Service {
			if h.chatSvc == nil {
				return nil
			}
			return h.chatSvc.MemoryL2()
		},
		func() *service.MemoryL3Service {
			if h.chatSvc == nil {
				return nil
			}
			return h.chatSvc.MemoryL3()
		},
		func() *service.MemoryL4Service {
			if h.chatSvc == nil {
				return nil
			}
			return h.chatSvc.MemoryL4()
		},
		h.auditSvc,
		writeJSON, writeErr, decodeBody, methodNotAllowed, parsePositiveInt,
	)
	mux := http.NewServeMux()
	h.registerRoutes(mux)
	return mux
}

// registerRoutes 注册全部 API 路由。按聚合根分组，使相关端点在视觉上相邻、便于调整顺序。
func (h *HTTPHandler) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.healthz)

	// 智能体与团队。
	mux.HandleFunc("/api/v1/agents/validate-model", h.handleValidateModel)
	mux.HandleFunc("/api/v1/agents/", h.handleAgentByID)
	mux.HandleFunc("/api/v1/agents", h.handleAgents)
	mux.HandleFunc("/api/v1/teams/", h.handleTeamByID)
	mux.HandleFunc("/api/v1/teams", h.handleTeams)
	mux.HandleFunc("/api/v1/team-runs/", h.handleTeamRunByID)
	mux.HandleFunc("/api/v1/team-runs", h.handleTeamRuns)
	mux.HandleFunc("/api/v1/team-run-events", h.handleTeamRunEvents)

	// 平台资源。
	mux.HandleFunc("/api/v1/agent-categories/tree", h.handlePlatformTree("agent-categories"))
	mux.HandleFunc("/api/v1/agent-categories", h.handlePlatformCollection("agent-categories"))
	mux.HandleFunc("/api/v1/agent-categories/", h.handlePlatformItem("agent-categories", "/api/v1/agent-categories/"))
	mux.HandleFunc("/api/v1/llm-provider-models/inspect", h.handleInspectProviderModel)
	mux.HandleFunc("/api/v1/llm-provider-models", h.handlePlatformCollection("llm-provider-models"))
	mux.HandleFunc("/api/v1/llm-provider-models/", h.handlePlatformItem("llm-provider-models", "/api/v1/llm-provider-models/"))
	mux.HandleFunc("/api/v1/avatar-assets", h.handleAvatarAssets)
	mux.HandleFunc("/api/v1/avatar-assets/", h.handleAvatarAssetByID)
	mux.HandleFunc("/api/v1/hooks", h.handlePlatformCollection("hooks"))
	mux.HandleFunc("/api/v1/hooks/", h.handlePlatformItem("hooks", "/api/v1/hooks/"))
	mux.HandleFunc("/api/v1/mcp-servers", h.handlePlatformCollection("mcp-servers"))
	mux.HandleFunc("/api/v1/mcp-servers/", h.handlePlatformItem("mcp-servers", "/api/v1/mcp-servers/"))

	// 频道。
	mux.HandleFunc("/api/v1/channels/catalog", h.handleChannelCatalog)
	mux.HandleFunc("/api/v1/channels", h.handleChannels)
	mux.HandleFunc("/api/v1/channels/", h.handleChannelByID)

	// 技能与工具。
	mux.HandleFunc("/api/v1/skills/import", h.handleSkillImport)
	mux.HandleFunc("/api/v1/skills/import/", h.handleSkillImportByID)
	mux.HandleFunc("/api/v1/skills", h.handleSkills)
	mux.HandleFunc("/api/v1/skills/", h.handleSkillByID)
	mux.HandleFunc("/api/v1/skill-runs", h.handleSkillRuns)
	h.tools.Register(mux)

	// 插件与定时任务。
	mux.HandleFunc("/api/v1/plugins", h.handlePlugins)
	mux.HandleFunc("/api/v1/plugins/", h.handlePluginByID)
	mux.HandleFunc("/api/v1/cron-tasks", h.handlePlatformCollection("cron-tasks"))
	mux.HandleFunc("/api/v1/cron-tasks/", h.handlePlatformItem("cron-tasks", "/api/v1/cron-tasks/"))
	mux.HandleFunc("/api/v1/cron-task-runs", h.handleCronTaskRuns)

	// 会话与聊天。
	mux.HandleFunc("/api/v1/sessions", h.handleSessions)
	mux.HandleFunc("/api/v1/sessions/", h.handleSessionByID)
	mux.HandleFunc("/api/v1/chat/messages/stream", h.handleChatMessagesStream)
	mux.HandleFunc("/api/v1/chat/messages", h.handleChatMessages)
	mux.HandleFunc("/api/v1/chat/options", h.handleChatOptions)

	// 记忆 L0~L4。会话下 L0/L1/L2 在 sessions.go 经 memory 适配器分发。
	h.memory.Register(mux)
	h.evolution.Register(mux)

	// 模型用量分析。
	mux.HandleFunc("/api/v1/model-usage/overview", h.handleModelUsageOverview)
	mux.HandleFunc("/api/v1/model-usage/trends", h.handleModelUsageTrends)
	mux.HandleFunc("/api/v1/model-usage/top-models", h.handleModelUsageTopModels)
	mux.HandleFunc("/api/v1/model-usage/top-agents", h.handleModelUsageTopAgents)
	mux.HandleFunc("/api/v1/model-usage/events", h.handleModelUsageEvents)

	// 监控 / 可观测性。
	mux.HandleFunc("/api/v1/monitor/logs/stream", h.handleMonitorLogStream)
	mux.HandleFunc("/api/v1/monitor/logs", h.handleMonitorLogs)
	mux.HandleFunc("/api/v1/monitor/events", h.handleMonitorEvents)
	mux.HandleFunc("/api/v1/monitor/events/", h.handleMonitorEventByID)
	mux.HandleFunc("/api/v1/monitor/traces", h.handleMonitorTraces)
	mux.HandleFunc("/api/v1/monitor/traces/", h.handleMonitorTraceByID)
	mux.HandleFunc("/api/v1/monitor/audit", h.handleMonitorAudit)
}

func (h *HTTPHandler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// evolutionService 与 l3Service / l4Service 类似：未注入时返回 nil，调用方可返回 503。
func (h *HTTPHandler) evolutionService() *service.AgentEvolutionService {
	if h.chatSvc == nil {
		return nil
	}
	return h.chatSvc.AgentEvolution()
}
