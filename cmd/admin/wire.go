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
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/event"
	graphadapter "aranea-agents/internal/graph/adapter"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/llminspect"
	"aranea-agents/internal/mcp/alert"
	"aranea-agents/internal/mcp/health"
	mcpmetadata "aranea-agents/internal/mcp/metadata"
	mcpprobe "aranea-agents/internal/mcp/probe"
	memtrpc "aranea-agents/internal/memory/trpc"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"
	"aranea-agents/internal/tools/testexec"
	webresearchpkg "aranea-agents/internal/tools/webresearch"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func provideEventBusSideConsumers(
	infra *event.Infra,
	tools *biz.ToolUsecase,
	webhooks *biz.WebhookDispatcher,
	sessions *biz.SessionUsecase,
	flowLogs *biz.FlowLogUsecase,
	monitor *biz.MonitorUsecase,
	memWorker *biz.TurnMemoryWorker,
) *biz.EventBusSideConsumers {
	var sessionBus, monitorBus event.Bus
	if infra != nil {
		sessionBus = infra.SessionBus
		monitorBus = infra.MonitorBus
	}
	return biz.NewEventBusSideConsumers(sessionBus, monitorBus, tools, webhooks, sessions, flowLogs, monitor, memWorker)
}

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

// provideSessionLogWriter moved to service.ProvideSessionLogWriter (Phase 3 decoupling).

func toEventPairs(pairs []biz.LogPair) []event.Pair {
	ep := make([]event.Pair, len(pairs))
	for i, p := range pairs {
		ep[i] = event.P(p.Key, p.Value)
	}
	return ep
}

// mcpProberAdapter wraps internal/mcp/probe to implement biz.MCPProber.
type mcpProberAdapter struct{}

func (mcpProberAdapter) Evaluate(enabled bool, configJSON string) biz.MCPTestResult {
	r := mcpprobe.Evaluate(enabled, configJSON)
	return biz.MCPTestResult{OK: r.OK, Status: r.Status, Message: r.Message, Details: r.Details}
}

func provideMCPProber() biz.MCPProber { return mcpProberAdapter{} }

// mcpMetadataAdapter wraps internal/mcp/metadata to implement biz.MCPMetadataEditor.
type mcpMetadataAdapter struct{}

func (mcpMetadataAdapter) Parse(raw string) map[string]any          { return mcpmetadata.Parse(raw) }
func (mcpMetadataAdapter) Marshal(m map[string]any) (string, error) { return mcpmetadata.Marshal(m) }
func (mcpMetadataAdapter) ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) string {
	return mcpmetadata.ApplyHealth(m, healthStatus, ok, errMsg, at)
}
func (mcpMetadataAdapter) ApplyReconnect(m map[string]any, at time.Time) {
	mcpmetadata.ApplyReconnect(m, at)
}
func (mcpMetadataAdapter) MarkHealthAlert(m map[string]any, at time.Time) {
	mcpmetadata.MarkHealthAlert(m, at)
}

func provideMCPMetadataEditor() biz.MCPMetadataEditor { return mcpMetadataAdapter{} }

// llmInspectorAdapter wraps internal/llminspect to implement biz.LLMInspector.
type llmInspectorAdapter struct{}

func (llmInspectorAdapter) Run(in biz.InspectMerge) (biz.LLMInspectResult, error) {
	r, err := llminspect.Run(llminspect.Input{
		ResourceID:   in.ResourceID,
		ProviderCode: in.ProviderCode,
		ProviderType: in.ProviderType,
		ModelAPIID:   in.ModelAPIID,
		APIBaseURL:   in.APIBaseURL,
		APIKey:       in.APIKey,
		Variant:      in.Variant,
		SecretID:     in.SecretID,
		SecretKey:    in.SecretKey,
		AWSRegion:    in.AWSRegion,
	})
	if err != nil {
		return biz.LLMInspectResult{}, err
	}
	return biz.LLMInspectResult{
		OK:                            r.OK,
		Message:                       r.Message,
		ProviderCode:                  r.ProviderCode,
		ProviderType:                  r.ProviderType,
		ModelAPIID:                    r.ModelAPIID,
		ModelDisplayName:              r.ModelDisplayName,
		ModelSizeLabel:                r.ModelSizeLabel,
		ContextWindowK:                r.ContextWindowK,
		MaxOutputTokens:               r.MaxOutputTokens,
		InputPriceMicroUSDPer1K:       r.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      r.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUSDPer1K: r.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   r.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   r.EmbeddingPriceMicroUSDPer1K,
		Source:                        r.Source,
		RawMetadataJSON:               r.RawMetadataJSON,
		Variant:                       r.Variant,
		EnableTokenTailoring:          r.EnableTokenTailoring,
		SupportsCache:                 r.SupportsCache,
		SupportsThinking:              r.SupportsThinking,
	}, nil
}

func provideLLMInspector() biz.LLMInspector { return llmInspectorAdapter{} }

func provideLlmProviderModelUsecaseWithDeps(repo biz.LlmProviderModelRepo, inspector biz.LLMInspector) *biz.LlmProviderModelUsecase {
	uc := biz.NewLlmProviderModelUsecase(repo)
	uc.SetInspector(inspector)
	return uc
}

// webResearchReadinessAdapter wraps internal/tools/webresearch to implement biztool.WebResearchReadinessChecker.
type webResearchReadinessAdapter struct{}

func (webResearchReadinessAdapter) ResolveReady(agentMap map[string]any, platform *biztool.WebResearchPlatformFields) bool {
	return webresearchpkg.ResolveReady(agentMap, bizToolToWebResearchPlatform(platform))
}

func (webResearchReadinessAdapter) CatalogReady(agentMap map[string]any, platform *biztool.WebResearchPlatformFields) bool {
	return webresearchpkg.CatalogReady(agentMap, bizToolToWebResearchPlatform(platform))
}

func bizToolToWebResearchPlatform(p *biztool.WebResearchPlatformFields) *webresearchpkg.PlatformFields {
	if p == nil {
		return nil
	}
	return &webresearchpkg.PlatformFields{
		HasAPIKey:   p.HasAPIKey,
		APIKey:      p.APIKey,
		Provider:    p.Provider,
		MaxResults:  p.MaxResults,
		FetchTop:    p.FetchTop,
		SearchDepth: p.SearchDepth,
		TimeoutSec:  p.TimeoutSec,
		HTTPProxy:   p.HTTPProxy,
	}
}

func provideWebResearchReadinessChecker() biztool.WebResearchReadinessChecker {
	return webResearchReadinessAdapter{}
}

// bizWebResearchReadinessAdapter wraps internal/tools/webresearch to implement biz.WebResearchReadinessChecker.
type bizWebResearchReadinessAdapter struct{}

func (bizWebResearchReadinessAdapter) ResolveReady(agentMap map[string]any, platform *biz.WebResearchPlatformFields) bool {
	return webresearchpkg.ResolveReady(agentMap, bizToWebResearchPlatform(platform))
}

func (bizWebResearchReadinessAdapter) CatalogReady(agentMap map[string]any, platform *biz.WebResearchPlatformFields) bool {
	return webresearchpkg.CatalogReady(agentMap, bizToWebResearchPlatform(platform))
}

func bizToWebResearchPlatform(p *biz.WebResearchPlatformFields) *webresearchpkg.PlatformFields {
	if p == nil {
		return nil
	}
	return &webresearchpkg.PlatformFields{
		HasAPIKey:   p.HasAPIKey,
		APIKey:      p.APIKey,
		Provider:    p.Provider,
		MaxResults:  p.MaxResults,
		FetchTop:    p.FetchTop,
		SearchDepth: p.SearchDepth,
		TimeoutSec:  p.TimeoutSec,
		HTTPProxy:   p.HTTPProxy,
	}
}

func provideBizWebResearchReadinessChecker() biz.WebResearchReadinessChecker {
	return bizWebResearchReadinessAdapter{}
}

func provideAgentUsecaseWithDeps(repo biz.AgentRepository, tools biz.ToolRepo, sys biz.SystemSettingRepo, checker biz.WebResearchReadinessChecker) *biz.AgentUsecase {
	uc := biz.NewAgentUsecase(repo, tools, sys)
	uc.SetWebResearchChecker(checker)
	return uc
}

// toolTesterAdapter wraps internal/tools/testexec to implement biztool.ToolTester.
type toolTesterAdapter struct{}

func (toolTesterAdapter) Execute(ctx context.Context, tool biztool.ToolTestInput, argumentsJSON string, timeoutSec int, platform *biztool.WebResearchPlatformFields) (biztool.ToolTestResult, error) {
	var pf *webresearchpkg.PlatformFields
	if platform != nil {
		pf = &webresearchpkg.PlatformFields{
			HasAPIKey:   platform.HasAPIKey,
			APIKey:      platform.APIKey,
			Provider:    platform.Provider,
			MaxResults:  platform.MaxResults,
			FetchTop:    platform.FetchTop,
			SearchDepth: platform.SearchDepth,
			TimeoutSec:  platform.TimeoutSec,
			HTTPProxy:   platform.HTTPProxy,
		}
	}
	res, err := testexec.Execute(ctx, testexec.CatalogTool{
		Key:               tool.Key,
		Source:            tool.Source,
		ConfigJSON:        tool.ConfigJSON,
		DefaultConfigJSON: tool.DefaultConfigJSON,
		MetadataJSON:      tool.MetadataJSON,
	}, argumentsJSON, timeoutSec, pf)
	if err != nil {
		return biztool.ToolTestResult{}, err
	}
	return biztool.ToolTestResult{
		Status:        res.Status,
		ResultPreview: res.ResultPreview,
		ErrorMessage:  res.ErrorMessage,
		DurationMS:    res.DurationMS,
	}, nil
}

func provideToolTester() biztool.ToolTester { return toolTesterAdapter{} }

func provideToolUsecaseWithDeps(repo biztool.ToolRepo, sys biztool.SettingRepo, tester biztool.ToolTester, checker biztool.WebResearchReadinessChecker) *biztool.ToolUsecase {
	uc := biztool.NewToolUsecase(repo, sys)
	uc.SetToolTester(tester)
	uc.SetWebResearchChecker(checker)
	biztool.SetGlobalWebResearchChecker(checker)
	return uc
}

// provideMCPServerUsecaseWithDeps injects prober and metadata editor after Wire construction.
func provideMCPServerUsecaseWithDeps(repo biz.MCPServerRepo, prober biz.MCPProber, metaEdit biz.MCPMetadataEditor) *biz.MCPServerUsecase {
	uc := biz.NewMCPServerUsecase(repo)
	uc.SetProber(prober)
	uc.SetMetadataEditor(metaEdit)
	return uc
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

func provideChannelRunEscalationNotifier(channels *biz.ChannelUsecase, sessions *biz.SessionUsecase) service.SessionRunEscalationNotifier {
	return service.NewChannelRunEscalationNotifier(channels, sessions)
}

func provideSessionRunDurableWorker(sessionRuns *biz.SessionRunUsecase, chat *service.ChatService) *service.SessionRunDurableWorker {
	return service.NewSessionRunDurableWorker(sessionRuns, chat)
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

func provideSystemSettingUsecase(repo biz.SystemSettingRepo, quota biz.UsageQuotaRepo, tester biz.WebResearchTester) *biz.SystemSettingUsecase {
	uc := biz.NewSystemSettingUsecase(repo, quota)
	uc.SetWebResearchTester(tester)
	return uc
}

func provideEventService(store *biz.EventStoreUsecase, sessions *biz.SessionUsecase) *service.EventService {
	return service.NewEventService(store, sessions)
}

func provideRuntimeTooling(
	pluginRT *plugintrpc.Runtime,
	pluginMgr *plugintrpc.Manager,
	skillDBRepo trpcskill.Repository,
	knowledgeRetriever *knowledge.Retriever,
	codeExecFactory *localexec.Factory,
) service.RuntimeTooling {
	return service.RuntimeTooling{
		PluginRT:           pluginRT,
		PluginManager:      pluginMgr,
		SkillDBRepo:        skillDBRepo,
		KnowledgeRetriever: knowledgeRetriever,
		CodeExecFactory:    codeExecFactory,
	}
}

func provideTeamOrchestrationDeps(
	teams biz.TeamRepository,
	teamsNative *team.Runner,
	graphFactory biz.GraphBuilderFactory,
	graphs *biz.GraphUsecase,
	tasks *biz.TaskUsecase,
	teamGraphCoord *team.TeamGraphRunCoordinator,
) service.TeamOrchestrationDeps {
	return service.TeamOrchestrationDeps{
		Teams:          teams,
		TeamsNative:    teamsNative,
		GraphFactory:   graphFactory,
		Graphs:         graphs,
		Tasks:          tasks,
		TeamGraphCoord: teamGraphCoord,
	}
}

func provideChannelTurnDeps(
	turnJobs *biz.ChannelTurnJobUsecase,
	sessionRuns *biz.SessionRunUsecase,
	channels *biz.ChannelUsecase,
	runEscalation service.SessionRunEscalationNotifier,
) service.ChannelTurnDeps {
	return service.ChannelTurnDeps{
		TurnJobs:      turnJobs,
		SessionRuns:   sessionRuns,
		Channels:      channels,
		RunEscalation: runEscalation,
	}
}

func provideChatServiceDeps(
	runs *rt.RunRegistry,
	pendingQueue *rt.PendingMessageQueue,
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
	sessionRT *araneasession.Runtime,
	compress biz.NativeTurnCompressor,
	eventBus event.Bus,
	eventBuffer *event.Buffer,
	rtDeps service.RuntimeTooling,
	teamDeps service.TeamOrchestrationDeps,
	chTurn service.ChannelTurnDeps,
	a2aUC *biz.A2AUsecase,
	artifacts *biz.ArtifactUsecase,
	mcpUC *biz.MCPServerUsecase,
	mon *biz.MonitorUsecase,
) service.ChatOrchestratorDeps {
	return service.ChatOrchestratorDeps{
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
			SessionRT: sessionRT,
			Compress:  compress,
			AfterTurn: biz.NoopNativeTurnAfter{},
			RunnerMgr: rt.NewRunnerManagerFromPersist(persist),
		},
		Runs:         runs,
		PendingQueue: pendingQueue,
		RT:           rtDeps,
		Team:         teamDeps,
		ChTurn:       chTurn,
		Usage:        usage,
		Monitor:      mon,
		Artifacts:    artifacts,
		A2AUC:        a2aUC,
		MCPServers:   mcpUC,
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
}

func provideMemoryService(persist rt.PersistenceSet, vec *biz.MemoryUsecase, factSync biz.MemoryFactIndexSyncer, cascade *biz.L4CascadeUsecase, memStore *sessionmemory.Store, sysUC *biz.SystemSettingUsecase) *service.MemoryService {
	return service.NewMemoryService(biz.NewMemoryAdminUsecase(persist.Memory.Admin, vec, factSync), cascade, memStore, sysUC)
}

func provideL4CascadeUsecase(memStore *sessionmemory.Store, factSync biz.MemoryFactIndexSyncer) *biz.L4CascadeUsecase {
	if memStore == nil {
		return nil
	}
	repo := data.NewL4GraphRepo(memStore)
	uc := biz.NewL4CascadeUsecase(data.NewCascadeGraphStore(memStore), repo)
	if uc != nil {
		uc.SetIndexSync(factSync)
	}
	return uc
}

func provideTRPCSessionService(d *data.Data) trpcsession.Service {
	if d == nil {
		return rt.NewTRPCSessionService(nil)
	}
	return rt.NewTRPCSessionService(d.RawDB())
}

func provideSessionMemoryResync(persist rt.PersistenceSet) araneasession.MemoryResync {
	return persist.Memory.Admin
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
) graphtrpc.GraphNodeResolverSet {
	if catalog == nil || toolUC == nil {
		return graphtrpc.GraphNodeResolverSet{}
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
	return graphtrpc.GraphNodeResolverSet{
		Models:    graphadapter.NewCatalogModelResolver(catalog, rtTrip),
		Tools:     graphadapter.NewCatalogToolResolver(toolUC),
		Agents:    graphadapter.NewCatalogAgentResolver(builderDeps),
		Functions: graphadapter.NewCatalogFunctionResolver(toolUC),
	}
}

func provideArtifactRuntimeService(uc *biz.ArtifactUsecase) trpcartifact.Service {
	if uc == nil {
		return nil
	}
	return artifacttrpc.NewServiceAdapter(uc)
}

// provideAutoMemoryWorker wires the cron auto-memory extraction worker.
func provideAutoMemoryWorker(
	sessions *biz.SessionUsecase,
	agents *biz.AgentUsecase,
	memStore *sessionmemory.Store,
	l4 biz.L4GraphWriter,
	factSync biz.MemoryFactIndexSyncer,
	episodeSync biz.EpisodeIndexSyncer,
	extractor biz.MemoryTextExtractor,
	queue memtrpc.AutoMemoryQueue,
) *jobs.AutoMemoryWorker {
	return jobs.NewAutoMemoryWorker(0, sessions, agents, memStore, factSync, episodeSync, l4, biz.DefaultMemoryConsolidator(extractor), queue)
}

func provideL4GraphWriter(memStore *sessionmemory.Store, cascade *biz.L4CascadeUsecase) biz.L4GraphWriter {
	if memStore == nil {
		return nil
	}
	return data.NewL4GraphWriterAdapter(data.NewL4GraphUsecaseFromStore(memStore, cascade))
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

func provideMemoryL2DecayWorker(store *sessionmemory.Store, logger log.Logger) *jobs.MemoryL2DecayWorker {
	if jobs.MemoryL2DecayDisabled() {
		return nil
	}
	return jobs.NewMemoryL2DecayWorker(0, store, logger)
}

func provideMemoryEpisodeBackfillWorker(store *sessionmemory.Store, episodeSync biz.EpisodeIndexSyncer, sys biz.SystemSettingRepo, logger log.Logger) *jobs.MemoryEpisodeBackfillWorker {
	if biz.ResolveEpisodeBackfillDisabled(context.Background(), sys) {
		return nil
	}
	return jobs.NewMemoryEpisodeBackfillWorker(0, store, episodeSync, sys, logger)
}

func provideMemoryDataMigrationWorker(store *sessionmemory.Store, logger log.Logger) *jobs.MemoryDataMigrationWorker {
	if jobs.MemoryDataMigrationDisabled() {
		return nil
	}
	return jobs.NewMemoryDataMigrationWorker(store, logger)
}

func provideMemoryL3DecayWorker(store *sessionmemory.Store, logger log.Logger) *jobs.MemoryL3DecayWorker {
	if jobs.MemoryL3DecayDisabled() {
		return nil
	}
	return jobs.NewMemoryL3DecayWorker(0, store, logger)
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
	App                     *kratos.App
	CronRunner              *cronrunner.Runner
	SkillWatch              *watch.Runner
	AutoMemory              *jobs.AutoMemoryWorker
	MCPHealthProbe          *health.Runner
	A2AGatewayHealthProbe   *a2ahealth.Runner
	EvolutionScanner        *jobs.EvolutionScanner
	ProviderHealthScanner   *jobs.ProviderHealthScanner
	ChannelHealthScanner    *jobs.ChannelHealthScanner
	ChannelDeliveryScanner  *jobs.ChannelDeliveryWorker
	SessionRunDurableWorker *service.SessionRunDurableWorker
	ChannelRuntime          *service.ChannelRuntime
	EventStoreCleanup       *jobs.EventStoreCleanup
	ToolAuditCleanup        *jobs.ToolAuditCleanup
	FlowLogCleanup          *jobs.FlowLogCleanup
	MemoryL2Decay           *jobs.MemoryL2DecayWorker
	MemoryL3Decay           *jobs.MemoryL3DecayWorker
	MemoryEpisodeBackfill   *jobs.MemoryEpisodeBackfillWorker
	MemoryDataMigration     *jobs.MemoryDataMigrationWorker
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
	sessionRunDurable *service.SessionRunDurableWorker,
	channelRuntime *service.ChannelRuntime,
	eventStoreCleanup *jobs.EventStoreCleanup,
	toolAuditCleanup *jobs.ToolAuditCleanup,
	flowLogCleanup *jobs.FlowLogCleanup,
	memoryL2Decay *jobs.MemoryL2DecayWorker,
	memoryL3Decay *jobs.MemoryL3DecayWorker,
	memoryEpisodeBackfill *jobs.MemoryEpisodeBackfillWorker,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
) wireOut {
	return wireOut{
		App: app, CronRunner: runner, SkillWatch: skillWatch, AutoMemory: autoMem,
		MCPHealthProbe: mcpHealth, A2AGatewayHealthProbe: a2aHealth, EvolutionScanner: evoScan, ProviderHealthScanner: providerHealth,
		ChannelHealthScanner: channelHealth, ChannelDeliveryScanner: channelDelivery,
		SessionRunDurableWorker: sessionRunDurable,
		ChannelRuntime:          channelRuntime,
		EventStoreCleanup:       eventStoreCleanup, ToolAuditCleanup: toolAuditCleanup,
		FlowLogCleanup: flowLogCleanup, MemoryL2Decay: memoryL2Decay, MemoryL3Decay: memoryL3Decay,
		MemoryEpisodeBackfill: memoryEpisodeBackfill,
		MemoryDataMigration:   memoryDataMigration,
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
		araneasession.ProviderSet,
		service.ProviderSet,
		provideEventBusSideConsumers,
		provideCronRunnerDeps,
		provideCronRunner,
		wire.Bind(new(biz.CronTaskTrigger), new(*cronrunner.Runner)),
		provideSkillWatchRunner,
		providePromptFileAIEditor,
		provideSessionTitleGenerator,
		provideRunRegistry,
		providePendingMessageQueue,
		provideCodeExecutorFactory,
		provideAutoMemoryQueue,
		wire.Bind(new(memtrpc.AutoMemoryQueue), new(*memtrpc.MemoryJobQueue)),
		provideMemoryPolicyEngine,
		provideFactIndexSync,
		provideMemoryL2Recall,
		provideMemoryL3Recall,
		provideAutoMemoryEnqueuer,
		provideFeedbackMemoryEnqueuer,
		provideMCPProber,
		provideMCPMetadataEditor,
		provideMCPServerUsecaseWithDeps,
		provideLLMInspector,
		provideLlmProviderModelUsecaseWithDeps,
		provideWebResearchReadinessChecker,
		provideBizWebResearchReadinessChecker,
		provideAgentUsecaseWithDeps,
		provideToolTester,
		provideToolUsecaseWithDeps,
		provideChatServiceDeps,
		provideRuntimeTooling,
		provideTeamOrchestrationDeps,
		provideChannelTurnDeps,
		provideRunCanceller,
		provideChatSender,
		provideArtifactRuntimeService,
		provideMemoryService,
		provideTRPCSessionService,
		provideGraphCheckpointSaver,
		wire.Bind(new(trpcgraph.CheckpointSaver), new(*graphtrpc.SQLiteCheckpointSaver)),
		providePersistenceSet,
		provideSessionMemoryResync,
		provideEpisodeIndexSync,
		providePluginStatsRecorder,
		providePluginManager,
		providePluginRuntime,
		graphtrpc.NewRegistry,
		provideGraphBuildDeps,
		graphadapter.NewGraphBuilderFactory,
		provideL4CascadeUsecase,
		provideAutoMemoryWorker,
		provideL4GraphWriter,
		provideEvolutionScanner,
		provideProviderHealthScanner,
		provideChannelHealthScanner,
		provideChannelDeliveryWorker,
		provideChannelDeliveryScanner,
		provideChannelRuntime,
		provideEventStoreCleanup,
		provideMemoryL2DecayWorker,
		provideMemoryL3DecayWorker,
		provideMemoryEpisodeBackfillWorker,
		provideMemoryDataMigrationWorker,
		provideToolAuditCleanup,
		provideFlowLogCleanup,
		provideMCPHealthRunnerDeps,
		provideMCPHealthRunner,
		provideA2AGatewayHealthRunnerDeps,
		provideA2AGatewayHealthRunner,
		provideMonitorAlertNotifier,
		provideChannelRunEscalationNotifier,
		provideSessionRunDurableWorker,
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
