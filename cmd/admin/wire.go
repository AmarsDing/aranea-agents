//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	a2apkg "aranea-agents/internal/a2a"
	a2ahealth "aranea-agents/internal/a2a/health"
	a2atrpc "aranea-agents/internal/a2a/trpc"
	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	artifacttrpc "aranea-agents/internal/artifact/trpc"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	graphadapter "aranea-agents/internal/graph/adapter"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/mcp/alert"
	"aranea-agents/internal/mcp/health"
	memtrpc "aranea-agents/internal/memory/trpc"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func provideCronRunnerDeps(
	cron biz.CronRepo,
	session *biz.SessionUsecase,
	teams biz.TeamRepository,
	agents biz.AgentRepository,
	eventBus event.Bus,
	chat *service.ChatService,
) cronrunner.Deps {
	return cronrunner.Deps{
		Cron:     cron,
		Session:  session,
		Teams:    teams,
		Agents:   agents,
		EventBus: eventBus,
		Chat:     chat,
	}
}

func provideCronRunner(deps cronrunner.Deps) *cronrunner.Runner {
	if strings.TrimSpace(os.Getenv("CRON_RUNNER_DISABLED")) == "1" {
		return nil
	}
	return cronrunner.NewRunner(deps)
}

func provideSkillWatchRunner(skillUC *biz.SkillUsecase, sys biz.SystemSettingRepo, logger log.Logger) *watch.Runner {
	if strings.TrimSpace(os.Getenv("SKILL_WATCH_DISABLED")) == "1" {
		return nil
	}
	return watch.NewRunner(skillUC, sys, logger)
}

func providePromptFileAIEditor(catalog *biz.LlmProviderModelUsecase, _ rt.PersistenceSet) *service.PromptFileAIEditor {
	if catalog == nil {
		return nil
	}
	httpClient := &http.Client{Timeout: 90 * time.Second}
	return service.NewPromptFileAIEditor(catalog, &provider.RoundTrip{HTTP: httpClient})
}

func provideSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, _ rt.PersistenceSet) biz.SessionTitleGenerator {
	if catalog == nil {
		return biz.NewNoopSessionTitleGenerator()
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	return service.NewLLMSessionTitleGenerator(catalog, &provider.RoundTrip{HTTP: httpClient})
}

func provideRunRegistry() *rt.RunRegistry {
	return rt.NewRunRegistry()
}

func providePendingMessageQueue() *rt.PendingMessageQueue {
	return rt.NewPendingMessageQueue()
}

func provideCodeExecutorFactory() *localexec.Factory {
	return localexec.NewFactory()
}

func provideMonitorAlertNotifier(channels *biz.ChannelUsecase, eventBus event.Bus) biz.AlertNotifier {
	return service.NewMonitorAlertNotifier(channels, eventBus)
}

func provideMonitorUsecase(repo biz.MonitorRepo, notifier biz.AlertNotifier) *biz.MonitorUsecase {
	return biz.NewMonitorUsecase(repo, notifier)
}

func provideUsageUsecase(repo biz.UsageRepo, mon *biz.MonitorUsecase) *biz.UsageUsecase {
	uc := biz.NewUsageUsecase(repo)
	uc.SetAlertNotifier(service.NewMonitorBudgetAlertNotifier(mon))
	return uc
}

func provideSystemSettingUsecase(repo biz.SystemSettingRepo, quota biz.UsageQuotaRepo) *biz.SystemSettingUsecase {
	return biz.NewSystemSettingUsecase(repo, quota)
}

func provideEventService(store *biz.EventStoreUsecase, sessions *biz.SessionUsecase) *service.EventService {
	return service.NewEventService(store, sessions)
}

func provideChatServiceDeps(
	runs *rt.RunRegistry,
	teams biz.TeamRepository,
	teamsNative *team.Runner,
	usage *biz.UsageUsecase,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	toolsCatalog biz.ToolRepo,
	toolUC *biz.ToolUsecase,
	llmCatalog *biz.LlmProviderModelUsecase,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
	persist rt.PersistenceSet,
	compress biz.NativeTurnCompressor,
	eventBus event.Bus,
	eventBuffer *event.Buffer,
	pluginRT *plugintrpc.Runtime,
	pluginMgr *plugintrpc.Manager,
	skillDBRepo trpcskill.Repository,
	a2aUC *biz.A2AUsecase,
	artifacts *biz.ArtifactUsecase,
	mcpUC *biz.MCPServerUsecase,
	knowledgeRetriever *knowledge.Retriever,
	mon *biz.MonitorUsecase,
	codeExecFactory *localexec.Factory,
	pendingQueue *rt.PendingMessageQueue,
	graphFactory biz.GraphBuilderFactory,
	graphs *biz.GraphUsecase,
) service.ChatServiceDeps {
	return service.ChatServiceDeps{
		TurnDeps: rt.TurnDeps{
			Catalog: rt.Catalog{
				Agents:   agents,
				AgentsUC: agentsUC,
				Tools:    toolsCatalog,
				ToolUC:   toolUC,
				LLM:      llmCatalog,
				SkillUC:  skillUC,
				Settings: sys,
			},
			Persist:   persist,
			Pipeline:  rt.EventPipeline{Bus: eventBus, Buffer: eventBuffer},
			LLMHTTP:   &http.Client{Timeout: 300 * time.Second},
			Sessions:  sessions,
			Compress:  compress,
			AfterTurn: biz.NoopNativeTurnAfter{},
			RunnerMgr: rt.NewRunnerManagerFromPersist(persist),
		},
		Runs:               runs,
		Teams:              teams,
		TeamsNative:        teamsNative,
		Usage:              usage,
		Monitor:            mon,
		PluginRT:           pluginRT,
		PluginManager:      pluginMgr,
		SkillDBRepo:        skillDBRepo,
		A2AUC:              a2aUC,
		Artifacts:          artifacts,
		KnowledgeRetriever: knowledgeRetriever,
		CodeExecFactory:    codeExecFactory,
		MCPServers:         mcpUC,
		PendingQueue:       pendingQueue,
		GraphFactory:       graphFactory,
		Graphs:             graphs,
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
}

func provideMemoryService(persist rt.PersistenceSet, vec *biz.MemoryUsecase) *service.MemoryService {
	return service.NewMemoryService(biz.NewMemoryAdminUsecase(persist.Memory.Admin, vec))
}

func provideTRPCSessionService(d *data.Data) trpcsession.Service {
	if d == nil {
		return rt.NewTRPCSessionService(nil)
	}
	return rt.NewTRPCSessionService(d.RawDB())
}

func provideGraphCheckpointSaver(d *data.Data) (*graphtrpc.SQLiteCheckpointSaver, error) {
	if d == nil {
		return nil, fmt.Errorf("data is nil")
	}
	return rt.NewGraphCheckpointSaver(d.RawDB())
}

func provideGraphBuildDeps(
	catalog *biz.LlmProviderModelUsecase,
	toolUC *biz.ToolUsecase,
	agentUC *biz.AgentUsecase,
	agents biz.AgentRepository,
	sys biz.SystemSettingRepo,
) *graphtrpc.BuildDeps {
	if catalog == nil || toolUC == nil {
		return nil
	}
	rtTrip := &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	builderDeps := chatagent.TRPCBuilderDeps{
		Catalog: catalog,
		AgentUC: agentUC,
		Agents:  agents,
		RT:      rtTrip,
		ToolUC:  toolUC,
		Sys:     sys,
	}
	return &graphtrpc.BuildDeps{
		Models: graphadapter.NewCatalogModelResolver(catalog, rtTrip),
		Tools:  graphadapter.NewCatalogToolResolver(toolUC),
		Agents: graphadapter.NewCatalogAgentResolver(builderDeps),
	}
}

func provideArtifactRuntimeService(uc *biz.ArtifactUsecase) trpcartifact.Service {
	if uc == nil {
		return nil
	}
	return artifacttrpc.NewServiceAdapter(uc)
}

// provideAutoMemoryWorker wires the cron auto-memory extraction worker.
// EP-RT-03: injects SessionUsecase + SQLite memory service so extraction writes to session_memory.
func provideAutoMemoryWorker(sessions *biz.SessionUsecase, agents *biz.AgentUsecase, memStore *sessionmemory.Store) *jobs.AutoMemoryWorker {
	var mem trpcmemory.Service
	if memStore != nil {
		mem = memtrpc.NewSQLiteMemoryService(memStore)
	}
	return jobs.NewAutoMemoryWorker(0, sessions, agents, mem, data.NewL4GraphWriterAdapter(data.NewL4GraphUsecaseFromStore(memStore)))
}

func provideEvolutionScanner(evo *biz.EvolutionUsecase, logger log.Logger) *jobs.EvolutionScanner {
	if strings.TrimSpace(os.Getenv("EVOLUTION_SCANNER_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewEvolutionScanner(0, evo, logger)
}

func provideProviderHealthScanner(uc *biz.LlmProviderModelUsecase, logger log.Logger) *jobs.ProviderHealthScanner {
	if strings.TrimSpace(os.Getenv("PROVIDER_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewProviderHealthScanner(0, uc, logger)
}

func provideChannelHealthScanner(uc *biz.ChannelUsecase, logger log.Logger) *jobs.ChannelHealthScanner {
	if strings.TrimSpace(os.Getenv("CHANNEL_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewChannelHealthScanner(0, uc, logger)
}

func provideChannelDeliveryWorker(channels *biz.ChannelUsecase, ingress *service.ChannelIngress, logger log.Logger) *service.ChannelDeliveryWorker {
	return service.NewChannelDeliveryWorker(channels, ingress, logger)
}

func provideChannelRuntime(channels *biz.ChannelUsecase, ingress *service.ChannelIngress) *service.ChannelRuntime {
	if service.ChannelRuntimeDisabled() {
		return nil
	}
	return service.NewChannelRuntime(channels, ingress)
}

func provideEventStoreCleanup(store *biz.EventStoreUsecase, logger log.Logger) *jobs.EventStoreCleanup {
	if jobs.EventStoreCleanupDisabled() {
		return nil
	}
	return jobs.NewEventStoreCleanup(0, store, logger)
}

func provideToolAuditCleanup(tools *biz.ToolUsecase, logger log.Logger) *jobs.ToolAuditCleanup {
	if jobs.ToolAuditCleanupDisabled() {
		return nil
	}
	return jobs.NewToolAuditCleanup(0, tools, logger)
}

func provideFlowLogCleanup(flowLogs *biz.FlowLogUsecase, logger log.Logger) *jobs.FlowLogCleanup {
	if jobs.FlowLogCleanupDisabled() {
		return nil
	}
	return jobs.NewFlowLogCleanup(0, flowLogs, logger)
}

func provideChannelDeliveryScanner(worker *service.ChannelDeliveryWorker, logger log.Logger) *jobs.ChannelDeliveryWorker {
	if strings.TrimSpace(os.Getenv("CHANNEL_DELIVERY_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewChannelDeliveryWorker(0, worker, logger)
}

func provideMCPHealthRunnerDeps(mcpRepo biz.MCPServerRepo, mcpUC *biz.MCPServerUsecase, bus event.Bus) health.Deps {
	return health.Deps{
		MCP:    mcpRepo,
		UC:     mcpUC,
		Alerts: alert.NewPublisher(bus, mcpUC),
	}
}

func provideMCPHealthRunner(deps health.Deps) *health.Runner {
	if strings.TrimSpace(os.Getenv("MCP_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return health.NewRunner(deps)
}

func provideA2AGatewayHealthRunnerDeps(a2aUC *biz.A2AUsecase) a2ahealth.Deps {
	return a2ahealth.Deps{A2A: a2aUC}
}

func provideA2AGatewayHealthRunner(deps a2ahealth.Deps) *a2ahealth.Runner {
	if strings.TrimSpace(os.Getenv("A2A_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return a2ahealth.NewRunner(deps)
}

func providePluginRuntime(stats plugintrpc.StatsRecorder, usage biz.PluginCostGuardUsageRepo, tools *biz.ToolUsecase, deliveries biz.HookDeliveryRepo) *plugintrpc.Runtime {
	rt := plugintrpc.NewRuntime(stats)
	if usage != nil {
		rt.SetCostGuardUsageRepo(usage)
	}
	if deliveries != nil {
		rt.SetHookDeliveryRepo(deliveries)
	}
	if tools != nil {
		rt.SetCatalogConfirmChecker(func(ctx context.Context, agentID, toolName string) bool {
			return tools.RequiresConfirmationForAgent(ctx, agentID, toolName)
		})
	}
	return rt
}

func providePluginStatsRecorder(repo biz.PluginRepo, runs biz.PluginRunRepo, agents biz.AgentRepository) plugintrpc.StatsRecorder {
	rec := plugintrpc.NewRepoStatsRecorder(repo, runs)
	if rec != nil {
		rec.SetAgentKeyResolver(agentKeyToID(agents))
	}
	return rec
}

func providePluginManager(rt *plugintrpc.Runtime, hooks *biz.HookResolver, agents biz.AgentRepository) *plugintrpc.Manager {
	m := plugintrpc.NewManager(rt, hooks)
	m.SetAgentKeyResolver(agentKeyToID(agents))
	return m
}

func agentKeyToID(agents biz.AgentRepository) plugintrpc.AgentKeyResolver {
	if agents == nil {
		return nil
	}
	return func(ctx context.Context, agentKey string) string {
		ag, err := agents.GetAgentByAgentKey(ctx, strings.TrimSpace(agentKey))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(ag.ID)
	}
}

// wireOut is non-cleanup inject outputs (cleanup must be a top-level injector return for Wire).
type wireOut struct {
	App                    *kratos.App
	CronRunner             *cronrunner.Runner
	SkillWatch             *watch.Runner
	AutoMemory             *jobs.AutoMemoryWorker
	MCPHealthProbe         *health.Runner
	A2AGatewayHealthProbe  *a2ahealth.Runner
	EvolutionScanner       *jobs.EvolutionScanner
	ProviderHealthScanner  *jobs.ProviderHealthScanner
	ChannelHealthScanner   *jobs.ChannelHealthScanner
	ChannelDeliveryScanner *jobs.ChannelDeliveryWorker
	ChannelRuntime         *service.ChannelRuntime
	EventStoreCleanup      *jobs.EventStoreCleanup
	ToolAuditCleanup       *jobs.ToolAuditCleanup
	FlowLogCleanup         *jobs.FlowLogCleanup
}

func provideWireOut(
	app *kratos.App,
	runner *cronrunner.Runner,
	skillWatch *watch.Runner,
	autoMem *jobs.AutoMemoryWorker,
	mcpHealth *health.Runner,
	a2aHealth *a2ahealth.Runner,
	evoScan *jobs.EvolutionScanner,
	providerHealth *jobs.ProviderHealthScanner,
	channelHealth *jobs.ChannelHealthScanner,
	channelDelivery *jobs.ChannelDeliveryWorker,
	channelRuntime *service.ChannelRuntime,
	eventStoreCleanup *jobs.EventStoreCleanup,
	toolAuditCleanup *jobs.ToolAuditCleanup,
	flowLogCleanup *jobs.FlowLogCleanup,
) wireOut {
	return wireOut{
		App: app, CronRunner: runner, SkillWatch: skillWatch, AutoMemory: autoMem,
		MCPHealthProbe: mcpHealth, A2AGatewayHealthProbe: a2aHealth, EvolutionScanner: evoScan, ProviderHealthScanner: providerHealth,
		ChannelHealthScanner: channelHealth, ChannelDeliveryScanner: channelDelivery,
		ChannelRuntime: channelRuntime,
		EventStoreCleanup: eventStoreCleanup, ToolAuditCleanup: toolAuditCleanup,
		FlowLogCleanup: flowLogCleanup,
	}
}

func provideA2APublicBaseInput(c *conf.Server) a2apkg.PublicBaseURLInput {
	configURL := ""
	if c != nil {
		configURL = c.GetA2APublicBaseUrl()
	}
	addr := ":8000"
	if c != nil && c.GetHttp() != nil && strings.TrimSpace(c.GetHttp().GetAddr()) != "" {
		addr = strings.TrimSpace(c.GetHttp().GetAddr())
	}
	return a2apkg.PublicBaseURLInput{
		EnvOverride: os.Getenv("A2A_PUBLIC_BASE_URL"),
		ConfigURL:   configURL,
		HTTPAddr:    addr,
		PathPrefix:  strings.TrimSuffix(a2atrpc.PublicPathPrefix, "/"),
	}
}

func providePublicBaseURLStore(input a2apkg.PublicBaseURLInput, sys biz.SystemSettingRepo, logger log.Logger) *a2apkg.PublicBaseURLStore {
	dbURL := ""
	if sys != nil {
		if s, err := sys.Get(context.Background()); err == nil {
			dbURL = s.A2APublicBaseURL
		}
	}
	in := input
	in.DBURL = dbURL
	result := a2apkg.ResolvePublicBaseURL(in)
	if result.Source == a2apkg.PublicBaseSourceDerived {
		log.NewHelper(logger).Warnf(
			"A2A public base URL derived as %q; set in System Settings, A2A_PUBLIC_BASE_URL, or server.a2a_public_base_url for production",
			result.URL,
		)
	}
	return a2apkg.NewPublicBaseURLStore(result)
}

func provideA2AEndpointRegistry(builder *service.A2AEndpointBuilder, uc *biz.A2AUsecase, store *a2apkg.PublicBaseURLStore) *a2atrpc.EndpointRegistry {
	return a2atrpc.NewEndpointRegistry(builder, uc, store)
}

func provideA2APublicBaseReloader(store *a2apkg.PublicBaseURLStore, reg *a2atrpc.EndpointRegistry, input a2apkg.PublicBaseURLInput) *service.A2APublicBaseReloader {
	return service.NewA2APublicBaseReloader(store, reg, input)
}

func provideA2AService(
	uc *biz.A2AUsecase,
	chat *service.ChatService,
	agents biz.AgentRepository,
	reg *a2atrpc.EndpointRegistry,
	store *a2apkg.PublicBaseURLStore,
) *service.A2AService {
	return service.NewA2AService(uc, chat, agents, reg, store)
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, log.Logger) (wireOut, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		event.ProviderSet,
		service.ProviderSet,
		provideCronRunnerDeps,
		provideCronRunner,
		wire.Bind(new(biz.CronTaskTrigger), new(*cronrunner.Runner)),
		provideSkillWatchRunner,
		providePromptFileAIEditor,
		provideSessionTitleGenerator,
		provideRunRegistry,
		providePendingMessageQueue,
		provideCodeExecutorFactory,
		provideChatServiceDeps,
		provideRunCanceller,
		provideChatSender,
		provideArtifactRuntimeService,
		provideMemoryService,
		provideTRPCSessionService,
		provideGraphCheckpointSaver,
		wire.Bind(new(trpcgraph.CheckpointSaver), new(*graphtrpc.SQLiteCheckpointSaver)),
		rt.NewPersistenceSet,
		providePluginStatsRecorder,
		providePluginManager,
		providePluginRuntime,
		graphtrpc.NewRegistry,
		provideGraphBuildDeps,
		graphadapter.NewGraphBuilderFactory,
		provideAutoMemoryWorker,
		provideEvolutionScanner,
		provideProviderHealthScanner,
		provideChannelHealthScanner,
		provideChannelDeliveryWorker,
		provideChannelDeliveryScanner,
		provideChannelRuntime,
		provideEventStoreCleanup,
		provideToolAuditCleanup,
		provideFlowLogCleanup,
		provideMCPHealthRunnerDeps,
		provideMCPHealthRunner,
		provideA2AGatewayHealthRunnerDeps,
		provideA2AGatewayHealthRunner,
		provideMonitorAlertNotifier,
		provideMonitorUsecase,
		provideUsageUsecase,
		provideSystemSettingUsecase,
		provideA2APublicBaseInput,
		providePublicBaseURLStore,
		provideA2AEndpointRegistry,
		provideA2APublicBaseReloader,
		provideA2AService,
		provideEventService,
		wire.Bind(new(biz.UsageQuotaRepo), new(biz.UsageRepo)),
		newApp,
		provideWireOut,
	))
}
