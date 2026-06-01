//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strings"
	"time"

	a2apkg "aranea-agents/internal/a2a"
	a2ahealth "aranea-agents/internal/a2a/health"
	a2atrpc "aranea-agents/internal/a2a/trpc"
	"aranea-agents/internal/agent"
	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	artifacttrpc "aranea-agents/internal/artifact/trpc"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor"
	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	"aranea-agents/internal/debug"
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
	"aranea-agents/internal/modelregistry"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"
	"aranea-agents/internal/tools/testexec"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	loggateway "aranea-agents/pkg/loggateway"

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
	traceProj *monitor.TraceProjector,
	fileAppender *monitor.FlowFileAppender,
) *biz.EventBusSideConsumers {
	var sessionBus, monitorBus event.Bus
	if infra != nil {
		sessionBus = infra.SessionBus
		monitorBus = infra.MonitorBus
	}
	return biz.NewEventBusSideConsumers(sessionBus, monitorBus, tools, webhooks, sessions, flowLogs, monitor, memWorker, traceProj, fileAppender)
}

func provideCronRunnerDeps(
	cron biz.CronRepo,
	session *biz.SessionUsecase,
	teams biz.TeamRepository,
	agents biz.AgentRepository,
	eventBus event.Bus,
	chat *service.ChatService,
	registrySyncAgent cronrunner.CronRegistrySyncAgent,
) cronrunner.Deps {
	return cronrunner.Deps{
		Cron:              cron,
		Session:           session,
		Teams:             teams,
		Agents:            agents,
		EventBus:          eventBus,
		Chat:              chat,
		RegistrySyncAgent: registrySyncAgent,
	}
}

func provideCronRunner(deps cronrunner.Deps, lg loggateway.Logger) *cronrunner.Runner {
	if strings.TrimSpace(os.Getenv("CRON_RUNNER_DISABLED")) == "1" {
		return nil
	}
	return cronrunner.NewRunner(deps, lg)
}

func provideSkillWatchRunner(skillReader watch.SkillReader, skillWriter watch.SkillWriter, sys biz.SystemSettingRepo, eventBus event.Bus, mon *biz.MonitorUsecase, lg loggateway.Logger) *watch.Runner {
	if strings.TrimSpace(os.Getenv("SKILL_WATCH_DISABLED")) == "1" {
		return nil
	}
	r := watch.NewRunnerWithBus(skillReader, skillWriter, sys, eventBus, lg)
	if r != nil {
		watch.SetSyncReporter(r, watch.NewMonitorSyncReporter(mon, eventBus))
		if mon != nil {
			watch.SetAlertEvaluator(r, mon)
		}
	}
	return r
}

func providePromptFileAIEditor(catalog *biz.LlmProviderModelUsecase, _ rt.PersistenceSet, lg loggateway.Logger) *service.PromptFileAIEditor {
	if catalog == nil {
		return nil
	}
	httpClient := &http.Client{Timeout: 90 * time.Second}
	return service.NewPromptFileAIEditor(catalog, &provider.RoundTrip{HTTP: httpClient}, lg)
}

func provideSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, _ rt.PersistenceSet, lg loggateway.Logger) biz.SessionTitleGenerator {
	if catalog == nil {
		return biz.NewNoopSessionTitleGenerator()
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	return service.NewLLMSessionTitleGenerator(catalog, &provider.RoundTrip{HTTP: httpClient}, lg)
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
type mcpProberAdapter struct {
	prober *mcpprobe.Prober
}

func (a mcpProberAdapter) Evaluate(ctx context.Context, enabled bool, configJSON string) biz.MCPTestResult {
	r := a.prober.Evaluate(ctx, enabled, configJSON)
	return biz.MCPTestResult{OK: r.OK, Status: r.Status, Message: r.Message, Details: r.Details}
}

func provideMCPProber() biz.MCPProber {
	return mcpProberAdapter{prober: mcpprobe.NewProber(chatagent.ResolveMCPAuthToken)}
}

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
type llmInspectorAdapter struct {
	lg loggateway.Logger
}

func (a llmInspectorAdapter) Run(in biz.InspectMerge) (biz.LLMInspectResult, error) {
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
	}, a.lg)
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

func provideLLMInspector(lg loggateway.Logger) biz.LLMInspector { return llmInspectorAdapter{lg: lg} }

func provideCredentialCrypto(sys biz.SystemSettingRepo, lg loggateway.Logger) *biz.CredentialCrypto {
	var keyRepo biz.SystemSettingCredentialKeyRepo = sys
	resolver := func(ctx context.Context) ([]byte, error) {
		return biz.ResolveCredentialAESKey(ctx, keyRepo)
	}
	cc := biz.NewCredentialCrypto(resolver, lg)
	if !cc.IsAvailable() {
		lg.Warn("凭据加密密钥未配置，API 密钥将以明文存储。请设置 ARANEA_CREDENTIAL_KEY 环境变量或在系统设置中初始化加密密钥。", loggateway.Str("reason", "credential.encryption"))
	}
	return cc
}

func provideLlmProviderModelUsecaseWithDeps(repo biz.LlmProviderModelRepo, inspector biz.LLMInspector, crypto *biz.CredentialCrypto, lg loggateway.Logger) *biz.LlmProviderModelUsecase {
	return biz.NewLlmProviderModelUsecase(repo, repo, repo, repo, inspector, crypto, lg)
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

func provideAgentUsecaseWithDeps(repo biz.AgentRepository, tools biz.ToolCatalogReader, sys biz.SystemSettingRepo, checker biz.WebResearchReadinessChecker, lg loggateway.Logger) *biz.AgentUsecase {
	uc := biz.NewAgentUsecase(repo, tools, sys, lg)
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

func provideToolUsecaseWithDeps(repo biztool.ToolRepo, sys biztool.SettingRepo, tester biztool.ToolTester, checker biztool.WebResearchReadinessChecker, lg loggateway.Logger) *biztool.ToolUsecase {
	uc := biztool.NewToolUsecase(repo, sys, lg)
	uc.SetToolTester(tester)
	uc.SetWebResearchChecker(checker)
	biztool.SetGlobalWebResearchChecker(checker)
	return uc
}

// provideMCPServerUsecaseWithDeps injects prober and metadata editor after Wire construction.
func provideMCPServerUsecaseWithDeps(repo biz.MCPServerRepo, prober biz.MCPProber, metaEdit biz.MCPMetadataEditor, crypto *biz.CredentialCrypto) *biz.MCPServerUsecase {
	uc := biz.NewMCPServerUsecase(repo, crypto)
	uc.SetProber(prober)
	uc.SetMetadataEditor(metaEdit)
	return uc
}

func provideRunRegistry() *rt.RunRegistry {
	return rt.NewRunRegistry()
}

func providePendingMessageQueue(lg loggateway.Logger) *rt.PendingMessageQueue {
	return rt.NewPendingMessageQueueWithDirAndLogger("", lg)
}

func provideCodeExecutorFactory() *localexec.Factory {
	return localexec.NewFactory()
}

func provideChannelRunEscalationNotifier(channels *biz.ChannelUsecase, sessions *biz.SessionUsecase) service.SessionRunEscalationNotifier {
	return service.NewChannelRunEscalationNotifier(channels, sessions)
}

func provideSessionRunDurableWorker(sessionRuns *biz.SessionRunUsecase, runCtrl biz.TurnRunControlGateway, resumer biz.DurableResumeGateway, lg loggateway.Logger) *service.SessionRunDurableWorker {
	return service.NewSessionRunDurableWorker(sessionRuns, runCtrl, resumer, lg)
}

func provideMonitorAlertNotifier(channels *biz.ChannelUsecase, eventBus event.Bus, lg loggateway.Logger) biz.AlertNotifier {
	return service.NewMonitorAlertNotifier(channels, eventBus, lg)
}

func provideMonitorUsecase(repo biz.MonitorRepo, notifier biz.AlertNotifier, fsHealth biz.FilesystemHealthReader, lg loggateway.Logger) *biz.MonitorUsecase {
	rb := monitor.NewMetricRingBuffer()
	uc := biz.NewMonitorUsecase(repo, notifier,
		biz.WithFilesystemHealthReader(fsHealth),
		biz.WithRingBuffer(rb),
		monitor.WithLogger(lg),
	)
	w := monitor.NewAlertEvalWorker(uc, rb, lg)
	uc.SetEvalWorker(w)
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(monitor.NewRunnerErrorRateMetric(repo, rb))
	if fsHealth != nil {
		reg.Register(monitor.NewSkillFilesystemMissingMetric(fsHealth))
	}
	uc.SetRegistry(reg)
	return uc
}

func provideFilesystemHealthReader(skillUC *biz.SkillUsecase) biz.FilesystemHealthReader {
	if skillUC == nil {
		return nil
	}
	return monitorSkillHealthAdapter{skills: skillUC}
}

func provideUsageUsecase(repo biz.UsageRepo, mon *biz.MonitorUsecase, lg loggateway.Logger) *biz.UsageUsecase {
	uc := biz.NewUsageUsecase(repo, lg)
	uc.SetAlertNotifier(service.NewMonitorBudgetAlertNotifier(mon))
	return uc
}

func provideSystemSettingUsecase(repo biz.SystemSettingRepo, quota biz.UsageQuotaRepo, tester biz.WebResearchTester) *biz.SystemSettingUsecase {
	uc := biz.NewSystemSettingUsecase(repo, quota)
	uc.SetWebResearchTester(tester)
	return uc
}

func provideModelRegistryApplyBackend(llm biz.LlmProviderModelRepo, d *data.Data) modelregistry.ApplyBackend {
	return data.NewModelRegistryApplyBackend(d, llm)
}

func provideModelRegistrySyncAgent(sys biz.SystemSettingRepo, backend modelregistry.ApplyBackend) (*agent.ModelRegistrySyncAgent, error) {
	storeProv := biz.NewModelRegistryStoreProvider(biz.NewSystemSettingRootAdapter(sys))
	return agent.BuildModelRegistrySyncAgent(storeProv, backend)
}

func provideModelRegistryUsecase(sys biz.SystemSettingRepo, backend modelregistry.ApplyBackend) *biz.ModelRegistryUsecase {
	uc := biz.NewModelRegistryUsecase(biz.NewSystemSettingRootAdapter(sys), backend)
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
	knowledgeRouter *knowledge.AdaptiveRouter,
	knowledgeFederatedRetriever *knowledge.FederatedRetriever,
	knowledgeEvaluator *knowledge.RetrievalEvaluator,
	knowledgeUC *biz.KnowledgeUsecase,
	codeExecFactory *localexec.Factory,
	kanbanBridge *service.KanbanToolBridge,
	debugRecorder *debug.RecorderFactory,
) service.RuntimeTooling {
	return service.RuntimeTooling{
		PluginRT:                    pluginRT,
		PluginManager:               pluginMgr,
		SkillDBRepo:                 skillDBRepo,
		KnowledgeRetriever:          knowledgeRetriever,
		KnowledgeRouter:             knowledgeRouter,
		KnowledgeFederatedRetriever: knowledgeFederatedRetriever,
		KnowledgeEvaluator:          knowledgeEvaluator,
		KnowledgeUC:                 knowledgeUC,
		CodeExecFactory:             codeExecFactory,
		KanbanBridge:                kanbanBridge,
		DebugRecorder:               debugRecorder,
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
	toolsCatalog biz.ToolCatalogReader,
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
	lg loggateway.Logger,
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
			RunnerMgr: rt.NewRunnerManagerFromPersist(persist, lg),
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
		LG:           lg,
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
}

func provideMemoryService(persist rt.PersistenceSet, vec *biz.MemoryUsecase, factSync biz.MemoryFactIndexSyncer, cascade *biz.L4CascadeUsecase, sysUC *biz.SystemSettingUsecase, deadLetterRepo biz.MemoryDeadLetterAdminRepo, queue memtrpc.AutoMemoryQueue, queueStats *memtrpc.MemoryJobQueue, memStore *sessionmemory.Store, lg loggateway.Logger) *service.MemoryService {
	enqueue := func(ctx context.Context, id int64) error {
		return deadLetterRepo.ReplayDeadLetterIntoQueue(ctx, id, func(sessionID, appName, userID, feedbackMsgID string, priority biz.MemoryJobPriority) {
			queue.Enqueue(memtrpc.AutoMemoryJobRequest{
				SessionID:         sessionID,
				AppName:           appName,
				UserID:            userID,
				FeedbackMessageID: feedbackMsgID,
				Priority:          priority,
			})
		})
	}
	return service.NewMemoryService(biz.NewMemoryAdminUsecase(persist.Memory.Admin, vec, factSync, nil, lg), cascade, sysUC, deadLetterRepo, data.NewMemoryDebugRecaller(memStore), data.NewMemoryFactIndexCounter(memStore), enqueue, queueStats)
}

func provideL4CascadeUsecase(memStore *sessionmemory.Store, factSync biz.MemoryFactIndexSyncer, lg loggateway.Logger) *biz.L4CascadeUsecase {
	if memStore == nil {
		return nil
	}
	repo := data.NewL4GraphRepo(memStore)
	cgs := data.NewCascadeGraphStore(memStore)
	uc := biz.NewL4CascadeUsecase(cgs, cgs, cgs, cgs, repo, lg)
	if uc != nil {
		uc.SetIndexSync(factSync)
	}
	return uc
}

func provideSQLiteRawDB(d *data.Data) *sql.DB {
	if d == nil {
		return nil
	}
	return d.RawDB()
}

func provideTRPCSessionService(rawDB *sql.DB, lg loggateway.Logger) trpcsession.Service {
	return rt.NewTRPCSessionService(rawDB, lg)
}

func provideSessionMemoryResync(persist rt.PersistenceSet) araneasession.MemoryResync {
	return persist.Memory.Admin
}

func provideGraphCheckpointSaver(rawDB *sql.DB, lg loggateway.Logger) (*graphtrpc.SQLiteCheckpointSaver, error) {
	return rt.NewGraphCheckpointSaver(rawDB, lg)
}

func provideGraphBuildDeps(
	catalog *biz.LlmProviderModelUsecase,
	toolUC *biz.ToolUsecase,
	agentUC *biz.AgentUsecase,
	agents biz.AgentRepository,
	sys biz.SystemSettingRepo,
	lg loggateway.Logger,
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
		Models:    graphadapter.NewCatalogModelResolver(catalog, rtTrip, lg),
		Tools:     graphadapter.NewCatalogToolResolver(toolUC),
		Agents:    graphadapter.NewCatalogAgentResolver(builderDeps, lg),
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
	writer biz.MemoryConsolidationWriter,
	l4 biz.L4GraphWriter,
	factSync biz.MemoryFactIndexSyncer,
	episodeSync biz.EpisodeIndexSyncer,
	extractor biz.MemoryTextExtractor,
	queue memtrpc.AutoMemoryQueue,
	lg loggateway.Logger,
) (*jobs.AutoMemoryWorker, error) {
	return jobs.NewAutoMemoryWorker(0, sessions, agents, writer, factSync, episodeSync, l4, biz.DefaultMemoryConsolidator(extractor), queue, lg)
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

func provideLearningLoopScanner(loop *biz.LearningLoopUsecase, lg loggateway.Logger) *jobs.LearningLoopScanner {
	if strings.TrimSpace(os.Getenv("LEARNING_LOOP_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewLearningLoopScanner(0, loop, lg)
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

func provideChannelDeliveryWorker(channels *biz.ChannelUsecase, ingress *service.ChannelIngress, lg loggateway.Logger) *service.ChannelDeliveryWorker {
	return service.NewChannelDeliveryWorker(channels, ingress, lg)
}

func provideChannelRuntime(channels *biz.ChannelUsecase, ingress *service.ChannelIngress, leases biz.ChannelRuntimeLeaseRepo, lg loggateway.Logger) *service.ChannelRuntime {
	if service.ChannelRuntimeDisabled() {
		return nil
	}
	return service.NewChannelRuntime(channels, ingress, leases, lg)
}

func provideMemoryL2DecayWorker(decayer biz.MemoryEpisodeDecayer, agents *biz.AgentUsecase, lg loggateway.Logger) *jobs.MemoryL2DecayWorker {
	if jobs.MemoryL2DecayDisabled() {
		return nil
	}
	return jobs.NewMemoryL2DecayWorker(0, decayer, agents, lg)
}

func provideMemoryEpisodeBackfillWorker(reader biz.MemoryEpisodeBackfillReader, episodeSync biz.EpisodeIndexSyncer, sys biz.SystemSettingRepo, lg loggateway.Logger) *jobs.MemoryEpisodeBackfillWorker {
	if biz.ResolveEpisodeBackfillDisabled(context.Background(), sys) {
		return nil
	}
	return jobs.NewMemoryEpisodeBackfillWorker(0, reader, episodeSync, sys, lg)
}

func provideMemoryDataMigrationWorker(migrator biz.MemoryLegacyMigrator, lg loggateway.Logger) *jobs.MemoryDataMigrationWorker {
	if jobs.MemoryDataMigrationDisabled() {
		return nil
	}
	return jobs.NewMemoryDataMigrationWorker(migrator, lg)
}

func provideMemoryL3DecayWorker(decayer biz.MemoryFactDecayer, agents *biz.AgentUsecase, lg loggateway.Logger) *jobs.MemoryL3DecayWorker {
	if jobs.MemoryL3DecayDisabled() {
		return nil
	}
	return jobs.NewMemoryL3DecayWorker(0, decayer, agents, lg)
}

func provideMemoryL4DecayWorker(l4 biz.L4GraphWriter, agents *biz.AgentUsecase, lg loggateway.Logger) *jobs.MemoryL4DecayWorker {
	if jobs.MemoryL4DecayDisabled() {
		return nil
	}
	return jobs.NewMemoryL4DecayWorker(0, l4, agents, lg)
}

func provideMemoryFactIndexReconciler(maintainer biz.MemoryFactIndexMaintainer, factSync biz.MemoryFactIndexSyncer, lg loggateway.Logger) *jobs.MemoryFactIndexReconciler {
	if jobs.MemoryIndexReconcileDisabled() {
		return nil
	}
	return jobs.NewMemoryFactIndexReconciler(0, maintainer, factSync, lg)
}

func provideMemoryDeadLetterReplayer(repo biz.MemoryDeadLetterAdminRepo, queue memtrpc.AutoMemoryQueue, logger log.Logger) *jobs.MemoryDeadLetterReplayer {
	if jobs.MemoryDeadLetterReplayDisabled() {
		return nil
	}
	enqueue := func(sessionID, appName, userID, feedbackMsgID string, priority biz.MemoryJobPriority) {
		queue.Enqueue(memtrpc.AutoMemoryJobRequest{
			SessionID:         sessionID,
			AppName:           appName,
			UserID:            userID,
			FeedbackMessageID: feedbackMsgID,
			Priority:          priority,
		})
	}
	return jobs.NewMemoryDeadLetterReplayer(0, repo, enqueue, logger)
}

func provideEventStoreCleanup(store *biz.EventStoreUsecase, lg loggateway.Logger) *jobs.EventStoreCleanup {
	if jobs.EventStoreCleanupDisabled() {
		return nil
	}
	return jobs.NewEventStoreCleanup(0, store, lg)
}

func provideToolAuditCleanup(tools *biz.ToolUsecase, lg loggateway.Logger) *jobs.ToolAuditCleanup {
	if jobs.ToolAuditCleanupDisabled() {
		return nil
	}
	return jobs.NewToolAuditCleanup(0, tools, lg)
}

func provideFlowLogCleanup(flowLogs *biz.FlowLogUsecase, lg loggateway.Logger) *jobs.FlowLogCleanup {
	if jobs.FlowLogCleanupDisabled() {
		return nil
	}
	return jobs.NewFlowLogCleanup(0, flowLogs, lg)
}

func provideMonitorAlertCooldownCleanup(uc *biz.MonitorUsecase, logger log.Logger) *jobs.MonitorAlertCooldownCleanup {
	return jobs.NewMonitorAlertCooldownCleanup(0, 0, uc, logger)
}

func provideMonitorAlertEvalWorker(uc *biz.MonitorUsecase) *monitor.AlertEvalWorker {
	return uc.EvalWorker()
}

func provideTraceProjector(repo biz.MonitorRepo, infra *event.Infra, lg loggateway.Logger) *monitor.TraceProjector {
	var sessionBus, monitorBus event.Bus
	if infra != nil {
		sessionBus = infra.SessionBus
		monitorBus = infra.MonitorBus
	}
	return monitor.NewTraceProjector(repo, lg, sessionBus, monitorBus)
}

func provideFlowFileAppender(lg loggateway.Logger) *monitor.FlowFileAppender {
	dir := strings.TrimSpace(os.Getenv("MONITOR_FLOW_LOG_DIR"))
	if dir == "" {
		if gw, ok := lg.(*loggateway.Gateway); ok {
			dir = gw.OutputDir()
		}
	}
	return monitor.NewFlowFileAppender(dir, lg)
}

func provideMonitorTraceBackfillWorker(repo biz.MonitorRepo, lg loggateway.Logger) *jobs.MonitorTraceBackfillWorker {
	return jobs.NewMonitorTraceBackfillWorker(repo, lg)
}

func provideDiagBundleGenerator(repo biz.MonitorRepo) *biz.DiagBundleGenerator {
	return biz.NewDiagBundleGenerator(repo)
}

func provideChannelDeliveryScanner(worker *service.ChannelDeliveryWorker, logger log.Logger) *jobs.ChannelDeliveryWorker {
	if strings.TrimSpace(os.Getenv("CHANNEL_DELIVERY_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewChannelDeliveryWorker(0, worker, logger)
}

func provideMCPHealthRunnerDeps(mcpRepo biz.MCPServerReader, mcpUC *biz.MCPServerUsecase, bus event.Bus, lg loggateway.Logger) health.Deps {
	return health.Deps{
		MCP:    mcpRepo,
		UC:     mcpUC,
		Alerts: alert.NewPublisher(bus, mcpUC, lg),
	}
}

func provideMCPHealthRunner(deps health.Deps, lg loggateway.Logger) *health.Runner {
	if strings.TrimSpace(os.Getenv("MCP_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return health.NewRunner(deps, lg)
}

func provideA2AGatewayHealthRunnerDeps(a2aUC *biz.A2AUsecase) a2ahealth.Deps {
	return a2ahealth.Deps{A2A: a2aUC}
}

func provideA2AGatewayHealthRunner(deps a2ahealth.Deps, lg loggateway.Logger) *a2ahealth.Runner {
	if strings.TrimSpace(os.Getenv("A2A_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return a2ahealth.NewRunner(deps, lg)
}

func providePluginRuntime(stats plugintrpc.StatsRecorder, usage biz.PluginCostGuardUsageRepo, tools *biz.ToolUsecase, deliveries biz.HookDeliveryRepo, lg loggateway.Logger) *plugintrpc.Runtime {
	rt := plugintrpc.NewRuntime(stats, lg)
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

func providePluginStatsRecorder(repo biz.PluginRepo, runs biz.PluginRunRepo, agents biz.AgentRepository, lg loggateway.Logger) plugintrpc.StatsRecorder {
	rec := plugintrpc.NewRepoStatsRecorder(repo, runs, lg)
	if rec != nil {
		rec.SetAgentKeyResolver(agentKeyToID(agents))
	}
	return rec
}

func providePluginManager(rt *plugintrpc.Runtime, hooks *biz.HookResolver, agents biz.AgentRepository, lg loggateway.Logger) *plugintrpc.Manager {
	m := plugintrpc.NewManager(rt, hooks, lg)
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
	LearningLoopScanner     *jobs.LearningLoopScanner
	ProviderHealthScanner   *jobs.ProviderHealthScanner
	ChannelHealthScanner    *jobs.ChannelHealthScanner
	ChannelDeliveryScanner  *jobs.ChannelDeliveryWorker
	SessionRunDurableWorker *service.SessionRunDurableWorker
	ChannelRuntime          *service.ChannelRuntime
	PluginRuntime               *plugintrpc.Runtime
	EventStoreCleanup           *jobs.EventStoreCleanup
	ToolAuditCleanup            *jobs.ToolAuditCleanup
	FlowLogCleanup              *jobs.FlowLogCleanup
	MonitorAlertCooldownCleanup *jobs.MonitorAlertCooldownCleanup
	MonitorAlertEvalWorker      *monitor.AlertEvalWorker
	MonitorTraceBackfillWorker  *jobs.MonitorTraceBackfillWorker
	MemoryL2Decay               *jobs.MemoryL2DecayWorker
	MemoryL3Decay               *jobs.MemoryL3DecayWorker
	MemoryL4Decay               *jobs.MemoryL4DecayWorker
	MemoryEpisodeBackfill       *jobs.MemoryEpisodeBackfillWorker
	MemoryDataMigration         *jobs.MemoryDataMigrationWorker
	MemoryFactIndexReconciler   *jobs.MemoryFactIndexReconciler
	MemoryDeadLetterReplayer    *jobs.MemoryDeadLetterReplayer
	ModelRegistrySyncAgent      *agent.ModelRegistrySyncAgent
	CronRepo                    biz.CronRepo
}

func provideWireOut(
	app *kratos.App,
	runner *cronrunner.Runner,
	skillWatch *watch.Runner,
	autoMem *jobs.AutoMemoryWorker,
	mcpHealth *health.Runner,
	a2aHealth *a2ahealth.Runner,
	evoScan *jobs.EvolutionScanner,
	learningLoop *jobs.LearningLoopScanner,
	providerHealth *jobs.ProviderHealthScanner,
	channelHealth *jobs.ChannelHealthScanner,
	channelDelivery *jobs.ChannelDeliveryWorker,
	sessionRunDurable *service.SessionRunDurableWorker,
	channelRuntime *service.ChannelRuntime,
	pluginRuntime *plugintrpc.Runtime,
	eventStoreCleanup *jobs.EventStoreCleanup,
	toolAuditCleanup *jobs.ToolAuditCleanup,
	flowLogCleanup *jobs.FlowLogCleanup,
	monitorAlertCooldown *jobs.MonitorAlertCooldownCleanup,
	monitorAlertEvalWorker *monitor.AlertEvalWorker,
	monitorTraceBackfillWorker *jobs.MonitorTraceBackfillWorker,
	memoryL2Decay *jobs.MemoryL2DecayWorker,
	memoryL3Decay *jobs.MemoryL3DecayWorker,
	memoryL4Decay *jobs.MemoryL4DecayWorker,
	memoryEpisodeBackfill *jobs.MemoryEpisodeBackfillWorker,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
	memoryFactIndexReconciler *jobs.MemoryFactIndexReconciler,
	memoryDeadLetterReplayer *jobs.MemoryDeadLetterReplayer,
	modelRegistrySyncAgent *agent.ModelRegistrySyncAgent,
	cronRepo biz.CronRepo,
) wireOut {
	return wireOut{
		App: app, CronRunner: runner, SkillWatch: skillWatch, AutoMemory: autoMem,
		MCPHealthProbe: mcpHealth, A2AGatewayHealthProbe: a2aHealth, EvolutionScanner: evoScan, LearningLoopScanner: learningLoop, ProviderHealthScanner: providerHealth,
		ChannelHealthScanner: channelHealth, ChannelDeliveryScanner: channelDelivery,
		SessionRunDurableWorker: sessionRunDurable,
		ChannelRuntime:          channelRuntime,
		PluginRuntime:           pluginRuntime,
		EventStoreCleanup:       eventStoreCleanup, ToolAuditCleanup: toolAuditCleanup,
		FlowLogCleanup: flowLogCleanup, MonitorAlertCooldownCleanup: monitorAlertCooldown, MonitorAlertEvalWorker: monitorAlertEvalWorker, MonitorTraceBackfillWorker: monitorTraceBackfillWorker, MemoryL2Decay: memoryL2Decay, MemoryL3Decay: memoryL3Decay, MemoryL4Decay: memoryL4Decay,
		MemoryEpisodeBackfill:     memoryEpisodeBackfill,
		MemoryDataMigration:       memoryDataMigration,
		MemoryFactIndexReconciler: memoryFactIndexReconciler,
		MemoryDeadLetterReplayer:  memoryDeadLetterReplayer,
		ModelRegistrySyncAgent:    modelRegistrySyncAgent,
		CronRepo:                  cronRepo,
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

func providePublicBaseURLStore(input a2apkg.PublicBaseURLInput, sys biz.SystemSettingRepo, lg loggateway.Logger) *a2apkg.PublicBaseURLStore {
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
		lg.Warn("A2A public base URL derived; set in System Settings, A2A_PUBLIC_BASE_URL, or server.a2a_public_base_url for production", loggateway.Str("url", result.URL))
	}
	return a2apkg.NewPublicBaseURLStore(result)
}

func provideA2AEndpointRegistry(builder *service.A2AEndpointBuilder, uc *biz.A2AUsecase, store *a2apkg.PublicBaseURLStore, lg loggateway.Logger) *a2atrpc.EndpointRegistry {
	return a2atrpc.NewEndpointRegistry(builder, uc, store, lg)
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
	lg loggateway.Logger,
) *service.A2AService {
	return service.NewA2AService(uc, chat, agents, reg, store, lg)
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, *conf.DebugRecorder, log.Logger, loggateway.Logger) (wireOut, func(), error) {
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
		wire.Bind(new(watch.SkillReader), new(*biz.SkillUsecase)),
		wire.Bind(new(watch.SkillWriter), new(*biz.SkillUsecase)),
		providePromptFileAIEditor,
		provideSessionTitleGenerator,
		provideRunRegistry,
		providePendingMessageQueue,
		provideCodeExecutorFactory,
		provideAutoMemoryQueue,
		wire.Bind(new(memtrpc.AutoMemoryQueue), new(*memtrpc.MemoryJobQueue)),
		wire.Bind(new(biz.MemoryDeadLetterAdminRepo), new(*data.MemoryJobDeadLetterRepo)),
		provideMemoryPolicyEngine,
		provideFactIndexSync,
		provideMemoryL2Recall,
		provideMemoryL3Recall,
		provideMemoryCompositeRecall,
		provideAutoMemoryEnqueuer,
		provideFeedbackMemoryEnqueuer,
		provideMCPProber,
		provideMCPMetadataEditor,
		provideMCPServerUsecaseWithDeps,
		provideLLMInspector,
		provideCredentialCrypto,
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
		provideSQLiteRawDB,
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
		provideLearningLoopScanner,
		provideProviderHealthScanner,
		provideChannelHealthScanner,
		provideChannelDeliveryWorker,
		provideChannelDeliveryScanner,
		provideChannelRuntime,
		provideEventStoreCleanup,
		provideMemoryL2DecayWorker,
		provideMemoryL3DecayWorker,
		provideMemoryL4DecayWorker,
		provideMemoryEpisodeBackfillWorker,
		provideMemoryDataMigrationWorker,
		provideMemoryFactIndexReconciler,
		provideMemoryDeadLetterReplayer,
		provideToolAuditCleanup,
		provideFlowLogCleanup,
		provideMonitorAlertCooldownCleanup,
		provideMonitorAlertEvalWorker,
		provideTraceProjector,
		provideFlowFileAppender,
		provideMonitorTraceBackfillWorker,
		provideDiagBundleGenerator,
		provideMCPHealthRunnerDeps,
		provideMCPHealthRunner,
		provideA2AGatewayHealthRunnerDeps,
		provideA2AGatewayHealthRunner,
		provideMonitorAlertNotifier,
		provideChannelRunEscalationNotifier,
		provideSessionRunDurableWorker,
		provideFilesystemHealthReader,
		provideMonitorUsecase,
		provideUsageUsecase,
		provideSystemSettingUsecase,
		provideModelRegistryApplyBackend,
		provideModelRegistrySyncAgent,
		wire.Bind(new(cronrunner.CronRegistrySyncAgent), new(*agent.ModelRegistrySyncAgent)),
		provideModelRegistryUsecase,
		provideA2APublicBaseInput,
		providePublicBaseURLStore,
		provideA2AEndpointRegistry,
		provideA2APublicBaseReloader,
		provideA2AService,
		provideEventService,
		debug.NewRecorderFactory,
		// PGO-3: DynamicLLMCaller → biz.LLMCaller binding, PromptRefiner.
		chatagent.NewDynamicLLMCaller,
		wire.Bind(new(biz.LLMCaller), new(*chatagent.DynamicLLMCaller)),
		biz.NewPromptRefiner,
		wire.Bind(new(biz.UsageQuotaRepo), new(biz.UsageRepo)),
		wire.Bind(new(biz.ToolCatalogReader), new(biz.ToolRepo)),
		wire.Bind(new(araneasession.AgentKeyLookup), new(biz.AgentRepository)),
		wire.Bind(new(araneasession.CompressorDeps), new(biz.SessionRepo)),
		wire.Bind(new(server.ReadinessProbe), new(*data.Data)),
		wire.Bind(new(biz.TaskGraphResolver), new(*biz.GraphUsecase)),
		wire.Bind(new(importer.SkillImportRepo), new(biz.SkillRepo)),
		wire.Bind(new(biz.MCPServerReader), new(biz.MCPServerRepo)),
		newApp,
		provideWireOut,
	))
}
