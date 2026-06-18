//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"database/sql"
	"fmt"
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
	"aranea-agents/internal/artifact"
	artifacttrpc "aranea-agents/internal/artifact/trpc"
	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/internal/biz/monitor"
	bizsession "aranea-agents/internal/biz/session"
	bizskill "aranea-agents/internal/biz/skill"
	biztool "aranea-agents/internal/biz/tool"
	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/data"
	"aranea-agents/internal/debug"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
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
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	araneasession "aranea-agents/internal/session"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/internal/skill"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	subagenttool "aranea-agents/internal/tools/subagent"
	"aranea-agents/internal/tools/testexec"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
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
	usage *biz.UsageUsecase,
	logger biz.SessionLogWriter,
) *biz.EventBusSideConsumers {
	var sessionBus, monitorBus event.Bus
	if infra != nil {
		sessionBus = infra.SessionBus
		monitorBus = infra.MonitorBus
	}
	return biz.NewEventBusSideConsumers(sessionBus, monitorBus, tools, webhooks, sessions, flowLogs, monitor, memWorker, traceProj, fileAppender, usage, logger)
}

func provideCronRunnerDeps(
	cron biz.CronRepo,
	session *biz.SessionUsecase,
	teams biz.TeamReader,
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
		watch.SetSyncReporter(r, watch.NewMonitorSyncReporter(mon, eventBus, lg))
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

// provideRefineLLMRoundTrip provides a centralized HTTP client for
// DynamicLLMCaller (PromptRefine / Memory extraction). Uses the same
// pattern as providePromptFileAIEditor / provideSessionTitleGenerator.
func provideRefineLLMRoundTrip() *provider.RoundTrip {
	return &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}}
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
func (mcpMetadataAdapter) ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) (map[string]any, string) {
	return mcpmetadata.ApplyHealth(m, healthStatus, ok, errMsg, at)
}
func (mcpMetadataAdapter) ApplyReconnect(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.ApplyReconnect(m, at)
}
func (mcpMetadataAdapter) MarkHealthAlert(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.MarkHealthAlert(m, at)
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

func provideLlmProviderModelUsecaseWithDeps(repo biz.LlmProviderModelRepo, inspector biz.LLMInspector, crypto *biz.CredentialCrypto, agentRefs biz.AgentReferenceChecker, lg loggateway.Logger) *biz.LlmProviderModelUsecase {
	return biz.NewLlmProviderModelUsecase(repo, repo, repo, repo, inspector, crypto, agentRefs, lg)
}

// webResearchReadinessAdapter wraps internal/tools/webresearch to implement biztool.WebResearchReadinessChecker.
type webResearchReadinessAdapter struct{}

func (webResearchReadinessAdapter) ResolveReady(agentMap map[string]any, platform *biztool.WebResearchPlatformFields) bool {
	return webresearchpkg.ResolveReady(agentMap, bizToolToWebResearchPlatform(platform))
}

func (webResearchReadinessAdapter) IsReady(agentMap map[string]any, platform *biztool.WebResearchPlatformFields) bool {
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

func (bizWebResearchReadinessAdapter) IsReady(agentMap map[string]any, platform *biz.WebResearchPlatformFields) bool {
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

func provideAgentUsecaseWithDeps(repo biz.AgentRepository, tools biz.ToolRegistryReader, sys biz.SystemSettingRepo, checker biz.WebResearchReadinessChecker, providerValidator biz.ProviderModelPairValidator, lg loggateway.Logger) *biz.AgentUsecase {
	return biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader: repo, Writer: repo, Settings: repo, Files: repo,
		Position: repo, Tx: repo, Tools: tools, Sys: sys,
		WebResearchChecker: checker, ProviderValidator: providerValidator, Lg: lg,
	})
}

// toolTesterAdapter wraps internal/tools/testexec to implement biztool.ToolTester.
type toolTesterAdapter struct {
	lg loggateway.Logger
}

func (a toolTesterAdapter) Execute(ctx context.Context, tool biztool.ToolTestInput, argumentsJSON string, timeoutSec int, platform *biztool.WebResearchPlatformFields) (biztool.ToolTestResult, error) {
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
	}, argumentsJSON, timeoutSec, pf, a.lg)
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

func provideToolTester(lg loggateway.Logger) biztool.ToolTester { return toolTesterAdapter{lg: lg} }

func provideToolUsecaseWithDeps(repo biztool.ToolRepo, sys biztool.SettingRepo, tester biztool.ToolTester, checker biztool.WebResearchReadinessChecker, lg loggateway.Logger) *biztool.ToolUsecase {
	return biztool.NewToolUsecase(repo, sys, lg, biztool.WithToolTester(tester), biztool.WithWebResearchChecker(checker))
}

// provideMCPServerUsecaseWithDeps injects prober and metadata editor via constructor.
func provideMCPServerUsecaseWithDeps(repo biz.MCPServerRepo, credRepo biz.MCPServerUserCredentialRepo, prober biz.MCPProber, metaEdit biz.MCPMetadataEditor, crypto *biz.CredentialCrypto) *biz.MCPServerUsecase {
	return biz.NewMCPServerUsecase(repo, credRepo, prober, metaEdit, crypto)
}

func provideRunRegistry(lg loggateway.Logger) *rt.RunRegistry {
	return rt.NewRunRegistry().WithLogger(lg)
}

func providePendingMessageQueue(lg loggateway.Logger) *rt.PendingMessageQueue {
	return rt.NewPendingMessageQueueWithDirAndLogger("", lg)
}

func provideCodeExecutorFactory(lg loggateway.Logger) *localexec.Factory {
	return localexec.NewFactoryWithLogger(lg)
}

func provideChannelRunEscalationNotifier(channels *biz.ChannelUsecase, sessions *biz.SessionUsecase, lg loggateway.Logger) service.SessionRunEscalationNotifier {
	return service.NewChannelRunEscalationNotifier(channels, sessions, lg)
}

func provideSessionRunDurableWorker(sessionRuns *biz.SessionRunUsecase, runCtrl biz.TurnRunControlGateway, resumer biz.DurableResumeGateway, lg loggateway.Logger) *service.SessionRunDurableWorker {
	return service.NewSessionRunDurableWorker(sessionRuns, runCtrl, resumer, lg)
}

func provideMonitorAlertNotifier(channels *biz.ChannelUsecase, eventBus event.Bus, lg loggateway.Logger) biz.AlertNotifier {
	return service.NewMonitorAlertNotifier(channels, eventBus, lg)
}

func provideMonitorUsecase(audit biz.MonitorAuditRepo, event biz.MonitorEventRepo, trace biz.MonitorTraceRepo, alert biz.MonitorAlertRepo, runner biz.MonitorRunnerCompletionRepo, notifier biz.AlertNotifier, fsHealth biz.FilesystemHealthReader, lg loggateway.Logger) *biz.MonitorUsecase {
	rb := monitor.NewMetricRingBuffer()
	uc := biz.NewMonitorUsecase(audit, event, trace, alert, runner, notifier,
		biz.WithFilesystemHealthReader(fsHealth),
		biz.WithRingBuffer(rb),
		monitor.WithLogger(lg),
	)
	w := monitor.NewAlertEvalWorker(uc, rb, lg)
	uc.SetEvalWorker(w)
	reg := monitor.NewAlertMetricRegistry()
	reg.Register(monitor.NewRunnerErrorRateMetric(event, rb))
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

// provideProcessLogEnabled binds *conf.Server as a ProcessLogEnabledProvider
// so that MonitorService no longer depends on *conf.Server directly.
func provideProcessLogEnabled(server *conf.Server) service.ProcessLogEnabledProvider {
	return server
}

func provideRedisClient(c *conf.Data, lg loggateway.Logger) *data.RedisClient {
	return data.NewRedisClient(c, lg)
}

func provideTurnLifecycleUsecase(sessions *biz.SessionUsecase, lg loggateway.Logger) *biz.TurnLifecycleUsecase {
	return biz.NewTurnLifecycleUsecase(biz.TurnLifecycleUsecaseConfig{
		Sessions: sessions,
		Logger:   lg,
	})
}

func provideUsageUsecase(repo biz.UsageRepo, mon *biz.MonitorUsecase, teamUC *biz.TeamUsecase, sessions *biz.SessionUsecase, bus contract.Bus, lg loggateway.Logger) *biz.UsageUsecase {
	uc := biz.NewUsageUsecase(repo, lg)
	uc.SetAlertNotifier(service.NewMonitorBudgetAlertNotifier(mon))
	uc.SetTeamReader(teamUC)
	uc.SetSessionMetricsAccumulator(&sessionMetricsAdapter{sessions: sessions})
	uc.SetCompletionUsageLinker(&completionLinkerAdapter{mon: mon})
	uc.SetUsageEnvelopePublisher(&envelopePublisherAdapter{bus: bus})
	return uc
}

// sessionMetricsAdapter adapts SessionUsecase to the usage.SessionMetricsWriter interface.
type sessionMetricsAdapter struct {
	sessions *biz.SessionUsecase
}

func (a *sessionMetricsAdapter) AccumulateMetricsDelta(delta bizusage.SessionMetricsDelta) {
	if a.sessions == nil {
		return
	}
	a.sessions.AccumulateMetricsDelta(bizsession.SessionMetricsDelta{
		SessionID:         delta.SessionID,
		MessageCount:      delta.MessageCount,
		ModelCallCount:    delta.ModelCallCount,
		ToolCallCount:     delta.ToolCallCount,
		SkillCallCount:    delta.SkillCallCount,
		McpCallCount:      delta.McpCallCount,
		InputTokens:       delta.InputTokens,
		OutputTokens:      delta.OutputTokens,
		TotalTokens:       delta.TotalTokens,
		TotalCostMicroUsd: delta.TotalCostMicroUsd,
	})
}

// completionLinkerAdapter adapts MonitorUsecase to the usage.CompletionUsageLinker interface.
type completionLinkerAdapter struct {
	mon *biz.MonitorUsecase
}

func (a *completionLinkerAdapter) LinkRunnerCompletionUsage(ctx context.Context, sessionID, runID, usageEventID, traceID string) error {
	return biz.LinkRunnerCompletionUsage(ctx, a.mon, sessionID, runID, usageEventID, traceID)
}

// envelopePublisherAdapter adapts contract.Bus to the usage.UsageEnvelopePublisher interface.
type envelopePublisherAdapter struct {
	bus contract.Bus
}

func (a *envelopePublisherAdapter) PublishTokenUsageEnvelope(ctx context.Context, e bizusage.TokenUsageEvent) {
	biz.PublishTokenUsageEnvelope(ctx, a.bus, e)
}

func provideSystemSettingUsecase(repo biz.SystemSettingRepo, quota biz.UsageQuotaRepo, tester biz.WebResearchTester) *biz.SystemSettingUsecase {
	uc := biz.NewSystemSettingUsecase(repo, quota)
	uc.SetWebResearchTester(tester)
	return uc
}

func provideModelRegistryApplyBackend(llm biz.LlmProviderModelRepo, d *data.Data) modelregistry.ApplyBackend {
	return data.NewModelRegistryApplyBackend(d, llm)
}

func provideModelRegistrySyncAgent(sys biz.SystemSettingRepo, backend modelregistry.ApplyBackend, lg loggateway.Logger) (*agent.ModelRegistrySyncAgent, error) {
	storeProv := biz.NewModelRegistryStoreProvider(biz.NewSystemSettingRootAdapter(sys), lg)
	return agent.BuildModelRegistrySyncAgent(storeProv, backend, lg)
}

func provideModelRegistryUsecase(sys biz.SystemSettingRepo, backend modelregistry.ApplyBackend, lg loggateway.Logger) *biz.ModelRegistryUsecase {
	uc := biz.NewModelRegistryUsecase(biz.NewSystemSettingRootAdapter(sys), backend, lg)
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
	kanbanBridge kanbanpkg.Bridge,
	debugRecorder *debug.RecorderFactory,
	orgUC *biz.OrganizationUsecase,
	toolResultGate *biz.ToolResultGate,
	outboundRouter *outbound.Router,
	subAgentSvc *subagenttool.Service,
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
		OrganizationUC:              orgUC,
		ToolResultGate:              toolResultGate,
		OutboundRouter:              outboundRouter,
		SubAgentService:             subAgentSvc,
	}
}

func provideTeamOrchestrationDeps(
	teamUC *biz.TeamUsecase,
	teamsNative biz.TeamRunnerWirePort,
	graphFactory biz.GraphBuilderFactory,
	graphs *biz.GraphUsecase,
	tasks *biz.TaskUsecase,
	teamGraphCoord biz.TeamGraphCoordPort,
	mediator biz.TeamMediatorPort,
	spiritUC biz.SpiritTeamController,
	taskPlanner biz.TaskPlannerPort,
	agentAllocator biz.AgentAllocatorPort,
) service.TeamOrchestrationDeps {
	return service.TeamOrchestrationDeps{
		TeamUC:         teamUC,
		TeamsNative:    teamsNative,
		GraphFactory:   graphFactory,
		Graphs:         graphs,
		Tasks:          tasks,
		TeamGraphCoord: teamGraphCoord,
		TeamMediator:   mediator,
		SpiritUC:       spiritUC,
		TaskPlanner:    taskPlanner,
		AgentAllocator: agentAllocator,
	}
}

func provideRunnerConfig(
	pluginRT *plugintrpc.Runtime,
	pluginMgr *plugintrpc.Manager,
	knowledgeRetriever *knowledge.Retriever,
	knowledgeRouter *knowledge.AdaptiveRouter,
	knowledgeFederatedRetriever *knowledge.FederatedRetriever,
	knowledgeEvaluator *knowledge.RetrievalEvaluator,
	graphs *biz.GraphUsecase,
	graphFactory biz.GraphBuilderFactory,
	tasks *biz.TaskUsecase,
	runs *rt.RunRegistry,
	tools *biz.ToolUsecase,
	agents biz.AgentRepository,
	sessions biz.SessionTurnExtrasPort,
	activityWriter biz.ActivityWriter,
	eventBus event.Bus,
	lg loggateway.Logger,
) team.RunnerConfig {
	cfg := team.RunnerConfig{
		PluginRT:      pluginRT,
		PluginManager: pluginMgr,
		Knowledge: &team.KnowledgeFacade{
			Retriever:          knowledgeRetriever,
			Router:             knowledgeRouter,
			FederatedRetriever: knowledgeFederatedRetriever,
			Evaluator:          knowledgeEvaluator,
		},
		Runs: runs,
		StreamOptsFactory: &chatactivity.StreamOptsFactoryAdapter{
			Tools: tools, Agents: agents, Sessions: sessions,
			ActivityWriter: activityWriter, EventBus: eventBus, Logger: lg,
		},
		AgentHelper: &chatagent.TeamAgentHelperAdapter{},
	}
	if graphs != nil {
		cfg.GraphLoader = graphadapter.NewLinkedGraphBuildConfigLoader(graphs)
	}
	if graphFactory != nil {
		if builder, ok := graphFactory.(graphadapter.TeamGraphRootBuilder); ok {
			cfg.GraphRoot = builder
		}
	}
	if tasks != nil {
		cfg.TeamGraphTasks = team.NewTaskUsecaseGraphTaskCreator(tasks)
	}
	return cfg
}

// provideTurnReadDeps builds the shared TurnReadDeps used by both Chat and Team.
// Extracted to avoid duplicating the 9-field construction across providers.
func provideTurnReadDeps(
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	toolRegistry biz.ToolRegistryReader,
	toolUC *biz.ToolUsecase,
	llmCatalog *biz.LlmProviderModelUsecase,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
) rt.TurnReadDeps {
	return rt.TurnReadDeps{
		Agents:          agents,
		AgentsUC:        agentsUC,
		CLIAdminAgentUC: agentsUC,
		Tools:           toolRegistry,
		ToolUC:          toolUC,
		LLM:             llmCatalog,
		SkillUC:         skillUC,
		CLIAdminSkillUC: skillUC,
		Settings:        sys,
	}
}

func provideTeamTurnDeps(
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	toolRegistry biz.ToolRegistryReader,
	toolUC *biz.ToolUsecase,
	llmCatalog *biz.LlmProviderModelUsecase,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
	persist rt.PersistenceSet,
	compress biz.NativeTurnCompressor,
	eventBus event.Bus,
	eventBuffer *event.Buffer,
	lg loggateway.Logger,
) rt.TurnDeps {
	return rt.TurnDeps{
		ReadDeps:  provideTurnReadDeps(agents, agentsUC, toolRegistry, toolUC, llmCatalog, skillUC, sys),
		Persist:   persist,
		Pipeline:  rt.EventPipeline{Bus: eventBus, Buffer: eventBuffer},
		LLMHTTP:   &http.Client{Timeout: 300 * time.Second},
		Sessions:  sessions,
		Compress:  compress,
		RunnerMgr: rt.NewRunnerManagerFromPersist(persist, lg),
		Lg:        lg,
	}
}

func provideChannelTurnJobDeps(
	turnJobs *biz.ChannelTurnJobUsecase,
	sessionRuns *biz.SessionRunUsecase,
	channels *biz.ChannelUsecase,
) service.ChannelTurnJobDeps {
	return service.ChannelTurnJobDeps{
		TurnJobs:    turnJobs,
		SessionRuns: sessionRuns,
		Channels:    channels,
	}
}

func provideChannelNotifierDeps(
	runEscalation service.SessionRunEscalationNotifier,
) service.ChannelNotifierDeps {
	return service.ChannelNotifierDeps{
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
	toolRegistry biz.ToolRegistryReader,
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
	chJobs service.ChannelTurnJobDeps,
	chNotify service.ChannelNotifierDeps,
	a2aUC *biz.A2AUsecase,
	artifacts *biz.ArtifactUsecase,
	mcpUC *biz.MCPServerUsecase,
	mon *biz.MonitorUsecase,
	spiritAssembler *service.SpiritTeamAssembler,
	spiritSynthesis *service.SpiritSynthesisService,
	orchCache *biz.OrchestrationCache,
	teamStarter biz.TeamStarterPort,
	graphExec biz.GraphExecutor,
	taskOrch biz.TaskOrchestratorPort,
	skillEvo *biz.SkillEvolutionUsecase,
	evolution *biz.EvolutionUsecase,
	skillStats biz.SkillInvocationStatsReader,
	outboundRouter *outbound.Router,
	subAgentSvc *subagenttool.Service,
	expAnalytics *biz.ExperienceAnalyticsUsecase,
	turnLifecycle *biz.TurnLifecycleUsecase,
	activityWriter biz.ActivityWriter,
	activityReader biz.ActivityReader,
	lg loggateway.Logger,
) service.ChatOrchestratorDeps {
	// Backfill TaskOrchestrator into teamDeps to break the Wire cycle:
	// TaskOrchestrator → SpiritTeamAssembler → TeamStarterPort → ChatService.
	// provideTeamOrchestrationDeps cannot include TaskOrchestrator directly
	// (it would create a cycle), so we inject it here after Wire resolves it.
	teamDeps.TaskOrchestrator = taskOrch

	return service.ChatOrchestratorDeps{
		Turn: service.ChatTurnDeps{
			TurnDeps: rt.TurnDeps{
				ReadDeps:  provideTurnReadDeps(agents, agentsUC, toolRegistry, toolUC, llmCatalog, skillUC, sys),
				Persist:   persist,
				Pipeline:  rt.EventPipeline{Bus: eventBus, Buffer: eventBuffer},
				LLMHTTP:   &http.Client{Timeout: 300 * time.Second},
				Sessions:  sessions,
				SessionRT: sessionRT,
				Compress:  compress,
				AfterTurn: biz.NoopNativeTurnAfter{},
				RunnerMgr: rt.NewRunnerManagerFromPersist(persist, lg),
				Lg:        lg,
			},
			Runs:           runs,
			PendingQueue:   pendingQueue,
			RT:             rtDeps,
			TurnTimeout:    0,
			Admission:      biz.NewTurnAdmissionUsecase(biz.TurnAdmissionUsecaseConfig{Quota: usage, Agents: agents}),
			ActivityWriter: activityWriter,
			ActivityReader: activityReader,
		},
		Usage: service.ChatUsageDeps{
			Usage:        usage,
			Monitor:      mon,
			Artifacts:    artifacts,
			SkillStats:   skillStats,
			ExpAnalytics: expAnalytics,
		},
		Channel: service.ChatChannelDeps{
			ChJobs:   chJobs,
			ChNotify: chNotify,
		},
		Team: service.ChatTeamDeps{
			Team:            teamDeps,
			TeamStarter:     teamStarter,
			GraphExec:       graphExec,
			SpiritAssembler: spiritAssembler,
			SpiritSynthesis: spiritSynthesis,
		},
		Evolution: service.ChatEvolutionDeps{
			SkillEvo:  skillEvo,
			Evolution: evolution,
		},
		Infra: service.ChatInfraDeps{
			LG:              lg,
			OrchCache:       orchCache,
			A2AUC:           a2aUC,
			MCPServers:      mcpUC,
			OutboundRouter:  outboundRouter,
			SubAgentService: subAgentSvc,
			TurnLifecycle:   turnLifecycle,
		},
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
}

func provideWSTurnExecutor(gateway biz.TurnExecutorGateway, lg loggateway.Logger) server.WSTurnExecutor {
	return &wsTurnExecutorAdapter{gateway: gateway, lg: lg}
}

type wsTurnExecutorAdapter struct {
	gateway biz.TurnExecutorGateway
	lg      loggateway.Logger
}

func (a *wsTurnExecutorAdapter) ExecuteTurn(ctx context.Context, input server.WSTurnInput) error {
	bizInput := biz.TurnInput{
		SessionID: input.SessionID,
		Content:   input.Content,
		AgentKey:  input.AgentKey,
		TeamID:    input.TeamID,
		Options: biz.TurnOptions{
			DialogMode:     input.Options.DialogMode,
			Provider:       input.Options.Provider,
			Model:          input.Options.Model,
			AttachmentIDs:  input.Options.AttachmentIDs,
			KnowledgeBases: input.Options.KnowledgeBases,
		},
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint:  biz.EntryPointWS,
			AllowQueue:  input.AllowQueue,
			AllowStream: input.AllowStream,
		},
	}
	start := time.Now()
	_, err := a.gateway.ExecuteTurn(ctx, bizInput)
	elapsed := time.Since(start)
	a.lg.With(loggateway.SessionID(input.SessionID)).Info("wsTurnExecutorAdapter.ExecuteTurn 完成",
		loggateway.StepID("ws.adapter_turn_done"),
		loggateway.Any("elapsed_ms", elapsed.Milliseconds()),
		loggateway.Any("has_error", err != nil))
	return err
}

func provideMemoryService(persist rt.PersistenceSet, vec *biz.MemoryUsecase, factSync biz.MemoryFactIndexSyncer, cascade *biz.L4CascadeUsecase, sysUC *biz.SystemSettingUsecase, deadLetterRepo biz.MemoryDeadLetterAdminRepo, queue memtrpc.AutoMemoryQueue, queueStats *memtrpc.MemoryJobQueue, workerStats *biz.MemoryWorkerStats, d *data.Data, lg loggateway.Logger) *service.MemoryService {
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
	return service.NewMemoryService(service.MemoryServiceConfig{
		Admin:             biz.NewMemoryAdminUsecase(persist.Memory.Admin, vec, factSync, data.NewL3FactWriterAdapter(d, d.VectorStore()), lg),
		Cascade:           cascade,
		SysUC:             sysUC,
		DeadLetterRepo:    deadLetterRepo,
		DebugRecaller:     data.NewMemoryDebugRecaller(d),
		FactIndexCounter:  data.NewMemoryFactIndexCounter(d),
		WorkerStats:       workerStats,
		DeadLetterEnqueue: enqueue,
		QueueStats:        queueStats,
		Logger:            lg,
	})
}

func provideL4CascadeUsecase(d *data.Data, factSync biz.MemoryFactIndexSyncer, lg loggateway.Logger) *biz.L4CascadeUsecase {
	if d == nil {
		return nil
	}
	repo := data.NewL4GraphRepo(d)
	cascade := data.NewCascadeRepo(d)
	return biz.NewL4CascadeUsecase(biz.L4CascadeDeps{
		Proposals:    cascade,
		Reader:       cascade,
		Mutator:      cascade,
		Saga:         cascade,
		EntityWriter: repo,
		IndexSync:    factSync,
		LG:           lg,
	})
}

func provideSQLiteRawDB(d *data.Data) *sql.DB {
	if d == nil {
		return nil
	}
	return d.RWDB().WriteHandle()
}

// provideEventWAL creates an EventWAL with both SQLite and Postgres DB handles.
// Postgres is preferred when available (Phase 1 migration); SQLite is the fallback.
// This provider is needed because Wire cannot disambiguate two *sql.DB args by type.
func provideEventWAL(d *data.Data, lg loggateway.Logger) *event.EventWAL {
	if d == nil {
		return nil
	}
	sqliteDB := d.RWDB().WriteHandle()
	pgDB := d.Postgres()
	return event.ProvideEventWAL(sqliteDB, pgDB, lg)
}

// providePostgresEventStore creates a Postgres-backed EventStore for cross-process
// event replay (WS reconnect). Returns nil when Postgres is not configured, allowing
// the system to degrade gracefully to in-process event delivery only.
//
// NOTE: Not yet added to wire.Build because no consumer depends on
// *event.PostgresEventStore. Will be wired up when the WS reconnect replay
// handler (Wave 2) consumes it. The function is defined here so it is ready
// to register once a consumer exists.
func providePostgresEventStore(d *data.Data, lg loggateway.Logger) *event.PostgresEventStore {
	if d == nil {
		return nil
	}
	pgDB := d.Postgres()
	if pgDB == nil {
		return nil
	}
	store, err := event.NewPostgresEventStore(pgDB, lg)
	if err != nil {
		if lg != nil {
			lg.Warn("event_store: failed to create Postgres store, cross-process replay disabled",
				loggateway.Err(err),
			)
		}
		return nil
	}
	return store
}

func provideTRPCSessionService(rawDB *sql.DB, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) trpcsession.Service {
	return rt.NewTRPCSessionService(rawDB, lg, sessiontrpc.SummarizerConfig{
		Catalog: catalog,
		RT:      &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}},
		Lg:      lg,
	})
}

func provideSessionMemoryResync(persist rt.PersistenceSet) araneasession.MemoryResync {
	return persist.Memory.Admin
}

func provideL1AdminReader(admin biz.SessionAdminStore) biz.L1AdminReader {
	if admin == nil {
		return nil
	}
	return admin
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
		TRPCModelCatalogDeps: chatagent.TRPCModelCatalogDeps{
			ModelCatalog: catalog,
			AgentUC:      agentUC,
			Agents:       agents,
			Sys:          sys,
		},
		TRPCModelRouteDeps: chatagent.TRPCModelRouteDeps{
			RT: rtTrip,
		},
		TRPCToolAssemblyDeps: chatagent.TRPCToolAssemblyDeps{
			ToolUC: toolUC,
		},
	}
	return graphtrpc.GraphNodeResolverSet{
		Models:    graphadapter.NewCatalogModelResolver(catalog, rtTrip, lg),
		Tools:     graphadapter.NewCatalogToolResolver(toolUC, lg),
		Agents:    graphadapter.NewCatalogAgentResolver(builderDeps, lg),
		Functions: graphadapter.NewCatalogFunctionResolver(toolUC, lg),
	}
}

func provideArtifactRuntimeService(uc *biz.ArtifactUsecase) trpcartifact.Service {
	if uc == nil {
		return nil
	}
	return artifacttrpc.NewServiceAdapter(uc)
}

func provideArtifactSigner(lg loggateway.Logger) *artifact.Signer {
	return artifact.NewSigner(lg)
}

// provideAutoMemoryWorker wires the cron auto-memory extraction worker.
func provideAutoMemoryWorker(
	runtimeConf *conf.Runtime,
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
	return jobs.NewAutoMemoryWorker(jobs.AutoMemoryWorkerConfig{
		RuntimeConf:  runtimeConf,
		Interval:     0,
		Sessions:     sessions,
		Agents:       agents,
		Writer:       writer,
		IndexSync:    factSync,
		EpisodeSync:  episodeSync,
		L4:           l4,
		Consolidator: biz.DefaultMemoryConsolidator(extractor),
		Queue:        queue,
		Logger:       lg,
	})
}

func provideL4GraphWriter(d *data.Data, cascade *biz.L4CascadeUsecase, lg loggateway.Logger) biz.L4GraphWriter {
	if d == nil {
		return nil
	}
	return data.NewL4GraphWriterAdapter(data.NewL4GraphUsecaseFromData(d, cascade, lg))
}

func provideEvolutionScanner(evo *biz.EvolutionUsecase, logger log.Logger) *jobs.EvolutionScanner {
	if strings.TrimSpace(os.Getenv("EVOLUTION_SCANNER_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewEvolutionScanner(0, evo, logger)
}

func provideSkillAutoCreator(caller biz.LLMCaller, sys *biz.SystemSettingUsecase, lg loggateway.Logger) biz.SkillAutoCreator {
	rl, err := sys.GetRefineLLM(context.Background())
	if err != nil || strings.TrimSpace(rl.Provider) == "" || strings.TrimSpace(rl.Model) == "" {
		lg.Warn("skill auto creator: no DefaultRefineLLM configured, skill auto-creation disabled")
		return nil
	}
	adapter := skill.NewLLMCallerAdapter(caller, rl.Provider, rl.Model)
	return skill.NewSkillAutoCreator(adapter, lg)
}

func provideSkillEvolutionScanner(skillEvo *biz.SkillEvolutionUsecase, lg loggateway.Logger) *jobs.SkillEvolutionScanner {
	if strings.TrimSpace(os.Getenv("SKILL_EVOLUTION_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewSkillEvolutionScanner(0, skillEvo, lg)
}

func provideSkillIntelligenceWorker(uc *biz.SkillIntelligenceUsecase, lg loggateway.Logger) *jobs.SkillIntelligenceWorker {
	if strings.TrimSpace(os.Getenv("SKILL_INTELLIGENCE_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewSkillIntelligenceWorker(0, uc, lg)
}

func provideCuratorWorker(uc *biz.SkillIntelligenceUsecase, skills biz.SkillQueryReader, lg loggateway.Logger) *jobs.CuratorWorker {
	if strings.TrimSpace(os.Getenv("CURATOR_WORKER_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewCuratorWorker(0, uc, skills, lg)
}

func provideSkillRegistrationPort(skillUC *biz.SkillUsecase) biz.SkillRegistrationPort {
	return service.NewSkillsButlerRegistrationAdapter(skillUC)
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

func provideTeamCompiler(
	channels *biz.ChannelUsecase,
	lg loggateway.Logger,
) biz.TeamCompiler {
	return team.NewTeamCompilerAdapter(
		channels,
		func(ctx context.Context) func(agentID string) string {
			return channels.AgentKeyResolver(ctx)
		},
		lg,
	)
}

func provideChannelIngress(
	channels *biz.ChannelUsecase,
	turnJobs *biz.ChannelTurnJobUsecase,
	sessions *biz.SessionUsecase,
	chat biz.ChannelTurnGateway,
	flowBuffer *event.Buffer,
	graphs biz.GraphExecutor,
	cron biz.CronTriggerGateway,
	eventBus event.Bus,
	admission *biz.TurnAdmissionUsecase,
	teamCompiler biz.TeamCompiler,
	lg loggateway.Logger,
) *service.ChannelIngress {
	dedupe := biz.NewIngressMessageDedupe(biz.DefaultMessageDedupeTTL)
	debouncer := biz.NewIngressPeerDebouncer(biz.DefaultIngressDebounce, lg)
	registry := biz.NewTurnPreviewRegistry()
	gate := biz.NewChannelConcurrentGate()
	return service.NewChannelIngress(channels, turnJobs, sessions, chat, flowBuffer, graphs, cron, eventBus, dedupe, debouncer, registry, gate, admission, teamCompiler, lg)
}

func provideChannelIngressAdmission(
	usage *biz.UsageUsecase,
	agents biz.AgentRepository,
	channels *biz.ChannelUsecase,
) *biz.TurnAdmissionUsecase {
	uc := biz.NewTurnAdmissionUsecase(biz.TurnAdmissionUsecaseConfig{
		Quota:  usage,
		Agents: agents,
		ChannelConfigResolver: biz.ChannelLongTaskConfigResolverFunc(func(ctx context.Context, sess biz.Session) biz.ChannelLongTaskConfig {
			meta, ok := biz.ParseChannelSessionMeta(sess.MetadataJSON)
			if !ok || strings.TrimSpace(meta.ChannelID) == "" {
				return biz.ChannelLongTaskConfig{}
			}
			ch, err := channels.Get(ctx, meta.ChannelID)
			if err != nil {
				return biz.ChannelLongTaskConfig{}
			}
			return biz.ParseChannelLongTaskConfig(ch.ConfigJSON)
		}),
	})
	return uc
}

func provideChannelDeliveryWorker(channels *biz.ChannelUsecase, ingress *service.ChannelIngress, lg loggateway.Logger) *service.ChannelDeliveryWorker {
	return service.NewChannelDeliveryWorker(channels, ingress, lg)
}

func provideChannelRuntime(channels *biz.ChannelUsecase, ingress *service.ChannelIngress, leases biz.ChannelRuntimeLeaseRepo, router *outbound.Router, lg loggateway.Logger) *service.ChannelRuntime {
	if service.ChannelRuntimeDisabled() {
		return nil
	}
	return service.NewChannelRuntime(channels, ingress, leases, router, lg)
}

func provideOutboundRouter(lg loggateway.Logger) *outbound.Router {
	return outbound.NewRouter(lg)
}

func provideSubAgentService(lg loggateway.Logger) (*subagenttool.Service, error) {
	// stateDir: use ./data as the root for subagent state files.
	// Runner is set later via SetRunner when the first turn creates a runner.
	return subagenttool.NewService("./data", nil, lg)
}

func provideMemoryL2DecayWorker(decayer biz.MemoryEpisodeDecayer, agents *biz.AgentUsecase, lg loggateway.Logger) *jobs.MemoryL2DecayWorker {
	if jobs.MemoryL2DecayDisabled() {
		return nil
	}
	return jobs.NewMemoryL2DecayWorker(0, decayer, agents, lg)
}

func provideSessionAdminStore(d *data.Data) biz.SessionAdminStore {
	return data.NewSessionAdminStoreAdapter(d, d.VectorStore())
}

// provideMemoryAdminDeps extracts the narrower MemoryAdminDeps interface from SessionAdminStore.
// SessionAdminStore embeds MemoryAdminDeps, so the cast is always safe.
func provideMemoryAdminDeps(admin biz.SessionAdminStore) biz.MemoryAdminDeps {
	return admin
}

func provideMemoryL1ArchiveWorker(admin biz.SessionAdminStore, agents *biz.AgentUsecase, lg loggateway.Logger) *jobs.MemoryL1ArchiveWorker {
	if jobs.MemoryL1ArchiveDisabled() {
		return nil
	}
	return jobs.NewMemoryL1ArchiveWorker(0, admin, agents, lg)
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

func provideMemoryDeadLetterReplayer(repo biz.MemoryDeadLetterAdminRepo, queue memtrpc.AutoMemoryQueue, logger log.Logger, lg loggateway.Logger) *jobs.MemoryDeadLetterReplayer {
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
	return jobs.NewMemoryDeadLetterReplayer(0, repo, enqueue, logger, lg)
}

func provideEventStoreCleanup(store *biz.EventStoreUsecase, lg loggateway.Logger) *jobs.EventStoreCleanup {
	if jobs.EventStoreCleanupDisabled() {
		return nil
	}
	return jobs.NewEventStoreCleanup(0, store, lg)
}

func provideEventWALCleanup(wal *event.EventWAL, lg loggateway.Logger) *jobs.EventWALCleanup {
	if jobs.EventWALCleanupDisabled() || wal == nil {
		return nil
	}
	return jobs.NewEventWALCleanup(0, 0, wal, lg)
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

func provideAutoHealTTLCleanup(repo monitor.HealRecordRepo, lg loggateway.Logger, logger log.Logger) *jobs.AutoHealTTLCleanup {
	return jobs.NewAutoHealTTLCleanup(0, 0, repo, lg, logger)
}

func provideMonitorAlertEvalWorker(uc *biz.MonitorUsecase) *monitor.AlertEvalWorker {
	return uc.EvalWorker()
}

func provideTraceProjector(traceRepo biz.MonitorTraceRepo, infra *event.Infra, lg loggateway.Logger) *monitor.TraceProjector {
	var sessionBus, monitorBus event.Bus
	if infra != nil {
		sessionBus = infra.SessionBus
		monitorBus = infra.MonitorBus
	}
	return monitor.NewTraceProjector(traceRepo, lg, sessionBus, monitorBus)
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

func provideMonitorTraceBackfillWorker(traceRepo biz.MonitorTraceRepo, runnerCompletion biz.MonitorRunnerCompletionRepo, lg loggateway.Logger) *jobs.MonitorTraceBackfillWorker {
	return jobs.NewMonitorTraceBackfillWorker(traceRepo, runnerCompletion, lg)
}

func provideDiagBundleGenerator(eventRepo biz.MonitorEventRepo, traceRepo biz.MonitorTraceRepo, engine *monitor.RootCauseEngine) *biz.DiagBundleGenerator {
	return biz.NewDiagBundleGenerator(eventRepo, traceRepo, engine)
}

func provideSelfHealUsecase(diag *biz.DiagBundleGenerator, lg loggateway.Logger) *biz.SelfHealUsecase {
	// Deprecated: SelfHealUsecase is being replaced by SelfHealObserver.
	// Provide a nil handler since the runtime now handles healing.
	return biz.NewSelfHealUsecase(diag, nil, lg)
}

func provideSelfHealObserver(runtimeConf *conf.Runtime, repo biz.HealRecordRepo, engine *monitor.RootCauseEngine, notifier biz.AlertNotifier, lg loggateway.Logger) (*biz.SelfHealObserver, error) {
	return monitor.NewSelfHealObserver(runtimeConf, repo, engine, notifier, lg)
}

func provideSkillIntelligenceUsecase(scorer *biz.SkillScoringUsecase, reporter *biz.SkillReportUsecase, suggestionRepo *data.SkillEvolutionSuggestionRepo, unifiedRepo *data.UnifiedEvolutionRepo, aggregator biz.SkillHealthAggregator, unanalyzedReader biz.SkillInvocationUnanalyzedReader, lg loggateway.Logger) *biz.SkillIntelligenceUsecase {
	reporter.SetUnanalyzedReader(unanalyzedReader)
	bridge := data.NewEvolutionStoreBridge(unifiedRepo, suggestionRepo, lg)
	uc := biz.NewSkillIntelligenceUsecase(scorer, reporter, bridge, bridge, aggregator, lg,
		biz.SkillIntelligenceConfig{
			UnanalyzedReader: unanalyzedReader,
		},
	)
	return uc
}

// provideBizRootCauseAdapter bridges monitor.RootCauseAnalyzer to biz.RootCauseAnalyzer.
func provideBizRootCauseAdapter(rca monitor.RootCauseAnalyzer) biz.RootCauseAnalyzer {
	return &skillIntelligenceRCAAdapter{inner: rca}
}

// skillIntelligenceRCAAdapter bridges monitor.RootCauseAnalyzer to biz.RootCauseAnalyzer.
type skillIntelligenceRCAAdapter struct {
	inner monitor.RootCauseAnalyzer
}

func (a *skillIntelligenceRCAAdapter) AnalyzeInvocationFailure(ctx context.Context, inv biz.SkillInvocationWrite) (*biz.RootCauseAnalysisResult, error) {
	report := &monitor.FailureReport{
		Type:      monitor.FailureTypeRuntime,
		Source:    "runtime",
		Job:       "skill",
		ErrorCode: inv.ErrorCode,
		Message:   inv.ErrorMessage,
		Metadata:  make(map[string]string),
	}
	if inv.DurationMS > biz.TimeoutThresholdMS {
		report.Metadata["duration_ms"] = fmt.Sprintf("%d", inv.DurationMS)
	}
	if inv.InputPreview != "" {
		report.Metadata["input_preview_len"] = fmt.Sprintf("%d", len(inv.InputPreview))
	}
	if inv.SkillID != "" {
		report.Metadata["skill_id"] = inv.SkillID
	}

	result, err := a.inner.AnalyzeFromReport(ctx, report)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &biz.RootCauseAnalysisResult{
		RootCause:  result.RootCause,
		FixSuggest: result.FixSuggest,
		Severity:   result.Severity,
		Confidence: result.Confidence,
	}, nil
}

func provideSelfCheckScheduler(
	checkers []monitor.SelfChecker,
	repairers []monitor.SelfCheckRepairer,
	repo monitor.SelfCheckReportRepo,
	registry *monitor.AlertMetricRegistry,
	lg loggateway.Logger,
) *monitor.SelfCheckScheduler {
	return monitor.NewSelfCheckScheduler(checkers, repairers, repo, registry, lg)
}

func provideEventBusHealthChecker() monitor.EventBusHealthChecker { return nil }

func provideWSConnectionCounter() monitor.WSConnectionCounter { return nil }

func provideEventBusResubscriber() monitor.EventBusResubscriber { return nil }

func provideDBPinger(rawDB *sql.DB) monitor.DBPinger {
	return monitor.NewDBPinger(rawDB)
}

func provideSelfCheckCleanup(repo monitor.SelfCheckReportRepo, lg loggateway.Logger) *jobs.SelfCheckCleanup {
	if jobs.SelfCheckCleanupDisabled() {
		return nil
	}
	return jobs.NewSelfCheckCleanup(0, repo, lg)
}

func provideSelfCheckJob(scheduler *monitor.SelfCheckScheduler, lg loggateway.Logger) *jobs.SelfCheckJob {
	if jobs.SelfCheckJobDisabled() {
		return nil
	}
	return jobs.NewSelfCheckJob(0, scheduler, lg)
}

func provideFailurePatternSyncJob(engine *monitor.RootCauseEngine, writer monitor.FailurePatternWriter, reader monitor.FailurePatternReader, lg loggateway.Logger) *jobs.FailurePatternSyncJob {
	return jobs.NewFailurePatternSyncJob(0, engine, writer, reader, lg)
}

func providePredictiveHealUsecase(uc *biz.MonitorUsecase, patternReader monitor.FailurePatternReader, healRepo monitor.HealRecordRepo, lg loggateway.Logger) *monitor.PredictiveHealUsecase {
	metricsReader := monitor.NewMonitorSystemMetricsReader(uc)
	handler := &monitor.NoopHealActionHandler{}
	return monitor.NewPredictiveHealUsecase(metricsReader, patternReader, handler, healRepo, lg)
}

func providePredictiveHealJob(uc *monitor.PredictiveHealUsecase, lg loggateway.Logger) *jobs.PredictiveHealJob {
	return jobs.NewPredictiveHealJob(0, uc, lg)
}

func providePatternMiningUsecase(healRepo monitor.HealRecordRepo, patternReader monitor.FailurePatternReader, patternWriter monitor.FailurePatternWriter, lg loggateway.Logger) *monitor.PatternMiningUsecase {
	return monitor.NewPatternMiningUsecase(healRepo, patternReader, patternWriter, lg)
}

func providePatternMiningJob(uc *monitor.PatternMiningUsecase, lg loggateway.Logger) *jobs.PatternMiningJob {
	return jobs.NewPatternMiningJob(0, uc, lg)
}

func provideVerificationGateExecutor(deptLeadMgr *biz.DeptLeadManager, caller biz.LLMCaller, lg loggateway.Logger) *biz.VerificationGateExecutor {
	return biz.NewVerificationGateExecutor(deptLeadMgr, caller, lg)
}

func provideSpiritTeamUsecase(teamUC *biz.TeamUsecase, sessionUC *biz.SessionUsecase, agentUC *biz.AgentUsecase, transactor biz.SpiritTransactor, orchCache *biz.OrchestrationCache, evolutionSugg biz.EvolutionSuggestionRepo, gateExecutor *biz.VerificationGateExecutor, deptLeadMgr *biz.DeptLeadManager, lg loggateway.Logger) *biz.SpiritTeamUsecase {
	return biz.NewSpiritTeamUsecase(teamUC, sessionUC, agentUC, lg,
		biz.WithSpiritTransactor(transactor),
		biz.WithOrchestrationCache(orchCache),
		biz.WithEvolutionSuggestionRepo(evolutionSugg),
		biz.WithVerificationGateExecutor(gateExecutor),
		biz.WithDeptLeadMgr(deptLeadMgr),
	)
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

func providePluginStatsRecorder(repo biz.PluginRepo, runs biz.PluginRunRepo, agents biz.AgentRepository, runtimeConf *conf.Runtime, lg loggateway.Logger) plugintrpc.StatsRecorder {
	cfg := runtimeConf.PluginConfig()
	rec := plugintrpc.NewRepoStatsRecorder(repo, runs, cfg.PersistSuccessRuns, lg)
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
	App                         *kratos.App
	Data                        *data.Data
	CronRunner                  *cronrunner.Runner
	SkillWatch                  *watch.Runner
	AutoMemory                  *jobs.AutoMemoryWorker
	MCPHealthProbe              *health.Runner
	A2AGatewayHealthProbe       *a2ahealth.Runner
	EvolutionScanner            *jobs.EvolutionScanner
	LearningLoopScanner         *jobs.LearningLoopScanner
	SkillEvolutionScanner       *jobs.SkillEvolutionScanner
	SkillIntelligenceWorker     *jobs.SkillIntelligenceWorker
	CuratorWorker               *jobs.CuratorWorker
	ProviderHealthScanner       *jobs.ProviderHealthScanner
	ChannelHealthScanner        *jobs.ChannelHealthScanner
	ChannelDeliveryScanner      *jobs.ChannelDeliveryWorker
	SessionRunDurableWorker     *service.SessionRunDurableWorker
	ChannelRuntime              *service.ChannelRuntime
	PluginRuntime               *plugintrpc.Runtime
	EventStoreCleanup           *jobs.EventStoreCleanup
	EventWALCleanup             *jobs.EventWALCleanup
	ToolAuditCleanup            *jobs.ToolAuditCleanup
	FlowLogCleanup              *jobs.FlowLogCleanup
	MonitorAlertCooldownCleanup *jobs.MonitorAlertCooldownCleanup
	AutoHealTTLCleanup          *jobs.AutoHealTTLCleanup
	MonitorAlertEvalWorker      *monitor.AlertEvalWorker
	MonitorTraceBackfillWorker  *jobs.MonitorTraceBackfillWorker
	MemoryL2Decay               *jobs.MemoryL2DecayWorker
	MemoryL1Archive             *jobs.MemoryL1ArchiveWorker
	MemoryL3Decay               *jobs.MemoryL3DecayWorker
	MemoryL4Decay               *jobs.MemoryL4DecayWorker
	MemoryEpisodeBackfill       *jobs.MemoryEpisodeBackfillWorker
	MemoryDataMigration         *jobs.MemoryDataMigrationWorker
	MemoryFactIndexReconciler   *jobs.MemoryFactIndexReconciler
	MemoryDeadLetterReplayer    *jobs.MemoryDeadLetterReplayer
	ModelRegistrySyncAgent      *agent.ModelRegistrySyncAgent
	SelfCheckScheduler          *monitor.SelfCheckScheduler
	SelfCheckCleanup            *jobs.SelfCheckCleanup
	SelfCheckJob                *jobs.SelfCheckJob
	CronRepo                    biz.CronRepo
	SkillIntelligence           *biz.SkillIntelligenceUsecase
	FailurePatternSyncJob       *jobs.FailurePatternSyncJob
	PredictiveHealUsecase       *monitor.PredictiveHealUsecase
	PredictiveHealJob           *jobs.PredictiveHealJob
	PatternMiningUsecase        *monitor.PatternMiningUsecase
	PatternMiningJob            *jobs.PatternMiningJob
	PathBExtractor              *biz.PathBExtractor
}

func provideWireOut(
	app *kratos.App,
	dataData *data.Data,
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
	eventWALCleanup *jobs.EventWALCleanup,
	toolAuditCleanup *jobs.ToolAuditCleanup,
	flowLogCleanup *jobs.FlowLogCleanup,
	monitorAlertCooldown *jobs.MonitorAlertCooldownCleanup,
	autoHealTTLCleanup *jobs.AutoHealTTLCleanup,
	monitorAlertEvalWorker *monitor.AlertEvalWorker,
	monitorTraceBackfillWorker *jobs.MonitorTraceBackfillWorker,
	memoryL2Decay *jobs.MemoryL2DecayWorker,
	memoryL1Archive *jobs.MemoryL1ArchiveWorker,
	memoryL3Decay *jobs.MemoryL3DecayWorker,
	memoryL4Decay *jobs.MemoryL4DecayWorker,
	memoryEpisodeBackfill *jobs.MemoryEpisodeBackfillWorker,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
	memoryFactIndexReconciler *jobs.MemoryFactIndexReconciler,
	memoryDeadLetterReplayer *jobs.MemoryDeadLetterReplayer,
	modelRegistrySyncAgent *agent.ModelRegistrySyncAgent,
	selfCheckScheduler *monitor.SelfCheckScheduler,
	selfCheckCleanup *jobs.SelfCheckCleanup,
	selfCheckJob *jobs.SelfCheckJob,
	cronRepo biz.CronRepo,
	skillIntelligence *biz.SkillIntelligenceUsecase,
	skillEvolutionScanner *jobs.SkillEvolutionScanner,
	skillIntelligenceWorker *jobs.SkillIntelligenceWorker,
	curatorWorker *jobs.CuratorWorker,
	failurePatternSyncJob *jobs.FailurePatternSyncJob,
	predictiveHealUsecase *monitor.PredictiveHealUsecase,
	predictiveHealJob *jobs.PredictiveHealJob,
	patternMiningUsecase *monitor.PatternMiningUsecase,
	patternMiningJob *jobs.PatternMiningJob,
	pathBExtractor *biz.PathBExtractor,
) wireOut {
	return wireOut{
		App: app, Data: dataData, CronRunner: runner, SkillWatch: skillWatch, AutoMemory: autoMem,
		MCPHealthProbe: mcpHealth, A2AGatewayHealthProbe: a2aHealth, EvolutionScanner: evoScan, LearningLoopScanner: learningLoop, ProviderHealthScanner: providerHealth,
		ChannelHealthScanner: channelHealth, ChannelDeliveryScanner: channelDelivery,
		SessionRunDurableWorker: sessionRunDurable,
		ChannelRuntime:          channelRuntime,
		PluginRuntime:           pluginRuntime,
		EventStoreCleanup:       eventStoreCleanup, ToolAuditCleanup: toolAuditCleanup,
		EventWALCleanup: eventWALCleanup,
		FlowLogCleanup:  flowLogCleanup, MonitorAlertCooldownCleanup: monitorAlertCooldown, AutoHealTTLCleanup: autoHealTTLCleanup, MonitorAlertEvalWorker: monitorAlertEvalWorker, MonitorTraceBackfillWorker: monitorTraceBackfillWorker, MemoryL2Decay: memoryL2Decay, MemoryL1Archive: memoryL1Archive, MemoryL3Decay: memoryL3Decay, MemoryL4Decay: memoryL4Decay,
		MemoryEpisodeBackfill:     memoryEpisodeBackfill,
		MemoryDataMigration:       memoryDataMigration,
		MemoryFactIndexReconciler: memoryFactIndexReconciler,
		MemoryDeadLetterReplayer:  memoryDeadLetterReplayer,
		ModelRegistrySyncAgent:    modelRegistrySyncAgent,
		SelfCheckScheduler:        selfCheckScheduler,
		SelfCheckCleanup:          selfCheckCleanup,
		SelfCheckJob:              selfCheckJob,
		CronRepo:                  cronRepo,
		SkillIntelligence:         skillIntelligence,
		SkillEvolutionScanner:     skillEvolutionScanner,
		SkillIntelligenceWorker:   skillIntelligenceWorker,
		CuratorWorker:             curatorWorker,
		FailurePatternSyncJob:     failurePatternSyncJob,
		PredictiveHealUsecase:     predictiveHealUsecase,
		PredictiveHealJob:         predictiveHealJob,
		PatternMiningUsecase:      patternMiningUsecase,
		PatternMiningJob:          patternMiningJob,
		PathBExtractor:            pathBExtractor,
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

func provideA2ALimiter(rdb *data.RedisClient, lg loggateway.Logger) a2abiz.Limiter {
	var client *redis.Client
	if rdb != nil && rdb.IsEnabled() {
		client = rdb.Client
	}
	return a2abiz.NewLimiter(a2abiz.DefaultLimiterConfig(), client, lg)
}

func provideA2AService(
	uc *biz.A2AUsecase,
	chat *service.ChatService,
	agents biz.AgentRepository,
	reg *a2atrpc.EndpointRegistry,
	store *a2apkg.PublicBaseURLStore,
	limiter a2abiz.Limiter,
	lg loggateway.Logger,
) *service.A2AService {
	return service.NewA2AService(uc, chat, agents, reg, store, limiter, lg)
}

func provideTaskPlanner(repo biz.TaskPlanRepository, catalog *biz.LlmProviderModelUsecase, orchCache *biz.OrchestrationCache, bus contract.Bus, lg loggateway.Logger) biz.TaskPlannerPort {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	return chatagent.NewTaskPlanner(repo, catalog, httpClient, bus, orchCache, lg)
}

func provideAgentAllocator(
	repo biz.AllocationPlanRepository,
	agentReader biz.AgentReader,
	perfRepo biz.AgentPerformanceRepository,
	catalog *biz.LlmProviderModelUsecase,
	bus contract.Bus,
	embedder knowledge.Embedder,
	agentFactory biz.AgentFactory,
	lg loggateway.Logger,
) biz.AgentAllocatorPort {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	capBuilder := chatagent.NewAgentCapabilityBuilder(agentReader, lg)
	return chatagent.NewAgentAllocator(repo, agentReader, perfRepo, capBuilder, catalog, httpClient, bus, lg, embedder, agentFactory)
}

// provideAgentFactory constructs the AgentFactory (P1-4). The LLM model is
// resolved from ARANEA_PLANNER_PROVIDER/ARANEA_PLANNER_MODEL env vars; when
// unset, llm is nil and EnsureAgent returns an Internal error so callers can
// fall back to other strategies.
func provideAgentFactory(
	agentReader biz.AgentReader,
	agentWriter biz.AgentWriter,
	templateRepo biz.AgentTemplateRepo,
	bus contract.Bus,
	catalog *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
) biz.AgentFactory {
	rt := &provider.RoundTrip{HTTP: &http.Client{Timeout: 60 * time.Second}}
	prov := strings.TrimSpace(os.Getenv("ARANEA_PLANNER_PROVIDER"))
	mod := strings.TrimSpace(os.Getenv("ARANEA_PLANNER_MODEL"))
	var llm trpcmodel.Model
	if prov != "" && mod != "" && catalog != nil {
		if m, err := provider.TRPCModelForProviderModel(context.Background(), catalog, rt, prov, mod, lg); err == nil {
			llm = m
		} else {
			lg.Warn("AgentFactory 模型构建失败，EnsureAgent 将返回错误",
				loggateway.StepID("agent_factory.wire"),
				loggateway.Str("provider", prov),
				loggateway.Str("model", mod),
				loggateway.Err(err))
		}
	}
	return chatagent.NewAgentFactoryImpl(llm, agentWriter, agentReader, templateRepo, bus, lg)
}

func provideTaskOrchestrator(
	spiritUC *biz.SpiritTeamUsecase,
	assembler *service.SpiritTeamAssembler,
	repo biz.OrchestrationRepository,
	matcher biz.AgentMatcherPort,
	catalog *biz.LlmProviderModelUsecase,
	agentUC *biz.AgentUsecase,
	agents biz.AgentRepository,
	toolUC *biz.ToolUsecase,
	sys biz.SystemSettingRepo,
	synthesis *service.SpiritSynthesisService,
	checkpointSaver trpcgraph.CheckpointSaver,
	orchCache *biz.OrchestrationCache,
	perfRepo biz.AgentPerformanceRepository,
	evolutionSugg biz.EvolutionSuggestionRepo,
	bus contract.Bus,
	lg loggateway.Logger,
) biz.TaskOrchestratorPort {
	rtTrip := &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	deps := chatagent.TRPCBuilderDeps{
		TRPCModelCatalogDeps: chatagent.TRPCModelCatalogDeps{
			ModelCatalog: catalog,
			AgentUC:      agentUC,
			Agents:       agents,
			Sys:          sys,
		},
		TRPCModelRouteDeps: chatagent.TRPCModelRouteDeps{
			RT: rtTrip,
		},
		TRPCToolAssemblyDeps: chatagent.TRPCToolAssemblyDeps{
			ToolUC: toolUC,
		},
	}
	compiler := chatagent.NewDAGToGraphCompiler(lg)
	return chatagent.NewTaskOrchestratorImpl(spiritUC, assembler, assembler, compiler, repo, matcher, deps, synthesis, checkpointSaver, orchCache, perfRepo, evolutionSugg, bus, lg)
}

func provideDeptLeadManager(
	orgRepo biz.OrganizationRepo,
	borrowRepo biz.BorrowRequestRepo,
	agentRepo biz.AgentRepository,
	agentUC *biz.AgentUsecase,
	teamGetter biz.DeptLeadTeamGetter,
	bus contract.Bus,
	lg loggateway.Logger,
) *biz.DeptLeadManager {
	return biz.NewDeptLeadManager(biz.DeptLeadManagerOpts{
		OrgRepo:    orgRepo,
		BorrowRepo: borrowRepo,
		AgentRepo:  agentRepo,
		AgentUC:    agentUC,
		TeamGetter: teamGetter,
		EventBus:   bus,
		Logger:     lg,
	})
}

// provideEcosystemPresetScenarioDir provides the scenario directory for EcosystemPresetUsecase.
func provideTeamUsecaseOpts(
	reader biz.TeamReader,
	writer biz.TeamWriter,
	runReader biz.TeamRunReader,
	runWriter biz.TeamRunWriter,
	stepRepo biz.OrchestrationStepRepo,
	deadLetter biz.TaskDeadLetterRepo,
	agentChecker biz.AgentIDExistenceChecker,
	deptLeadMgr *biz.DeptLeadManager,
	graphReader biz.GraphReader,
	graphWriter biz.GraphWriter,
	lg loggateway.Logger,
) biz.TeamUsecaseOpts {
	return biz.TeamUsecaseOpts{
		Reader:       reader,
		Writer:       writer,
		RunReader:    runReader,
		RunWriter:    runWriter,
		StepRepo:     stepRepo,
		DeadLetter:   deadLetter,
		AgentChecker: agentChecker,
		DeptLeadMgr:  deptLeadMgr,
		GraphReader:  graphReader,
		GraphWriter:  graphWriter,
		Lg:           lg,
	}
}

func provideMemoryLLMExtractorConfig(
	agents *biz.AgentUsecase,
	sessions *biz.SessionUsecase,
	modelCatalog *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
) service.MemoryLLMExtractorConfig {
	return service.MemoryLLMExtractorConfig{
		Agents:       agents,
		Sessions:     sessions,
		ModelCatalog: modelCatalog,
		RoundTrip:    &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}},
		LLMDisabled:  false,
		Logger:       lg,
	}
}

func provideMemoryEnhancedExtractorConfig(
	agents *biz.AgentUsecase,
	sessions *biz.SessionUsecase,
	modelCatalog *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
) service.MemoryEnhancedExtractorConfig {
	return service.MemoryEnhancedExtractorConfig{
		Agents:       agents,
		Sessions:     sessions,
		ModelCatalog: modelCatalog,
		RoundTrip:    &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}},
		LLMDisabled:  false,
		Logger:       lg,
	}
}

func provideEcosystemPresetScenarioDir() string {
	return biz.ScenarioDir()
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, *conf.Runtime, *conf.DebugRecorder, log.Logger, loggateway.Logger, logpipeline.Pipeline, []*conf.LoggingSink) (wireOut, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		event.ProviderSet,
		araneasession.ProviderSet,
		service.ProviderSet,
		data.NewPackRepoAdapter,
		wire.Bind(new(service.PackExporterImporterValidator), new(*data.PackRepoAdapter)),
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
		provideRunnerConfig,
		provideTeamTurnDeps,
		provideChannelTurnJobDeps,
		provideChannelNotifierDeps,
		provideRunCanceller,
		provideChatSender,
		provideArtifactRuntimeService,
		provideArtifactSigner,
		provideMemoryService,
		provideSQLiteRawDB,
		provideEventWAL,
		provideTRPCSessionService,
		provideGraphCheckpointSaver,
		wire.Bind(new(trpcgraph.CheckpointSaver), new(*graphtrpc.SQLiteCheckpointSaver)),
		providePersistenceSet,
		provideSessionMemoryResync,
		provideL1AdminReader,
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
		provideSkillAutoCreator,
		provideSkillRegistrationPort,
		provideSkillEvolutionScanner,
		provideSkillIntelligenceWorker,
		provideCuratorWorker,
		provideLearningLoopScanner,
		provideProviderHealthScanner,
		provideChannelHealthScanner,
		provideTeamCompiler,
		provideChannelIngress,
		provideChannelIngressAdmission,
		provideChannelDeliveryWorker,
		provideChannelDeliveryScanner,
		provideChannelRuntime,
		provideOutboundRouter,
		provideSubAgentService,
		provideEventStoreCleanup,
		provideEventWALCleanup,
		provideMemoryL2DecayWorker,
		provideMemoryAdminUsecase,
		providePathBExtractor,
		provideL4EntityWriter,
		provideSessionAdminStore,
		provideMemoryAdminDeps,
		provideMemoryL1ArchiveWorker,
		provideMemoryL3DecayWorker,
		provideMemoryL4DecayWorker,
		provideMemoryEpisodeBackfillWorker,
		provideMemoryDataMigrationWorker,
		provideMemoryFactIndexReconciler,
		provideMemoryDeadLetterReplayer,
		provideToolAuditCleanup,
		provideFlowLogCleanup,
		provideMonitorAlertCooldownCleanup,
		provideAutoHealTTLCleanup,
		provideMonitorAlertEvalWorker,
		provideTraceProjector,
		provideFlowFileAppender,
		provideMonitorTraceBackfillWorker,
		provideDiagBundleGenerator,
		provideSelfHealUsecase,
		provideSelfHealObserver,
		provideSkillIntelligenceUsecase,
		provideBizRootCauseAdapter,
		provideMCPHealthRunnerDeps,
		provideMCPHealthRunner,
		provideA2AGatewayHealthRunnerDeps,
		provideA2AGatewayHealthRunner,
		wire.Bind(new(biz.A2ACardRepo), new(biz.A2ARepo)),
		wire.Bind(new(biz.A2AInvocationRepo), new(biz.A2ARepo)),
		wire.Bind(new(biz.A2AAuditRepo), new(biz.A2ARepo)),
		wire.Bind(new(biz.A2ARemoteAgentRepo), new(biz.A2ARepo)),
		provideMonitorAlertNotifier,
		provideChannelRunEscalationNotifier,
		provideSessionRunDurableWorker,
		provideFilesystemHealthReader,
		provideProcessLogEnabled,
		provideRedisClient,
		provideTurnLifecycleUsecase,
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
		provideA2ALimiter,
		provideA2AService,
		provideEventService,
		provideTaskPlanner,
		provideAgentAllocator,
		provideAgentFactory,
		chatagent.NewAgentMatcher,
		provideTaskOrchestrator,
		debug.NewRecorderFactory,
		// PGO-3: DynamicLLMCaller → biz.LLMCaller binding, PromptRefiner.
		provideRefineLLMRoundTrip,
		chatagent.NewDynamicLLMCaller,
		wire.Bind(new(biz.LLMCaller), new(*chatagent.DynamicLLMCaller)),
		biz.NewPromptRefiner,
		wire.Bind(new(biz.Refiner), new(*biz.PromptRefiner)),
		wire.Bind(new(biz.UsageQuotaRepo), new(biz.UsageRepo)),
		wire.Bind(new(biz.ToolRegistryReader), new(biz.ToolRepo)),
		wire.Bind(new(araneasession.AgentKeyLookup), new(biz.AgentRepository)),
		wire.Bind(new(araneasession.CompressReadDeps), new(biz.SessionRepo)),
		wire.Bind(new(araneasession.CompressWriteDeps), new(biz.SessionRepo)),
		wire.Bind(new(araneasession.CompressTxDeps), new(biz.SessionRepo)),
		wire.Bind(new(server.ReadinessProbe), new(*data.Data)),
		wire.Bind(new(biz.TaskGraphResolver), new(*biz.GraphUsecase)),
		wire.Bind(new(importer.SkillImportRepo), new(biz.SkillRepo)),
		wire.Bind(new(biz.SkillLookupReader), new(biz.SkillRepo)),
		wire.Bind(new(bizskill.SkillQueryReader), new(biz.SkillRepo)),
		wire.Bind(new(biz.ExperienceReportReader), new(*data.SkillIntelligenceRepo)),
		wire.Bind(new(biz.ExperienceReportStatsReader), new(*data.SkillIntelligenceRepo)),
		wire.Bind(new(biz.ExperienceReportWriter), new(*data.SkillIntelligenceRepo)),
		wire.Bind(new(biz.SkillHealthAggregator), new(*data.SkillIntelligenceRepo)),
		wire.Bind(new(biz.SkillInvocationUnanalyzedReader), new(*data.SkillIntelligenceRepo)),
		wire.Bind(new(biz.SkillDedupReader), new(*data.SkillDedupRepo)),
		wire.Bind(new(biz.SkillMergeReader), new(*data.SkillMergeRepo)),
		wire.Bind(new(biz.SkillMergeWriter), new(*data.SkillMergeRepo)),
		wire.Bind(new(biz.SkillContentFuser), new(*biz.RuleBasedContentFuser)),
		wire.Bind(new(bizusage.AnalyticsRepo), new(biz.UsageRepo)),
		wire.Bind(new(biz.SessionReader), new(biz.SessionRepo)),
		wire.Bind(new(bizsession.ContextUpdater), new(biz.SessionRepo)),
		wire.Bind(new(biztool.ToolInvocationReader), new(biz.ToolRepo)),
		wire.Bind(new(biz.MCPServerReader), new(biz.MCPServerRepo)),
		wire.Bind(new(biz.TeamReader), new(*data.TeamRepo)),
		wire.Bind(new(biz.TeamWriter), new(*data.TeamRepo)),
		wire.Bind(new(biz.TeamRunReader), new(*data.TeamRepo)),
		wire.Bind(new(biz.TeamRunWriter), new(*data.TeamRepo)),
		wire.Bind(new(biz.OrchestrationStepRepo), new(*data.TeamRepo)),
		wire.Bind(new(biz.TaskDeadLetterRepo), new(*data.TeamRepo)),
		wire.Bind(new(biz.PatternReader), new(biz.PatternReadWriter)),
		wire.Bind(new(biz.AgentReader), new(biz.AgentRepository)),
		wire.Bind(new(biz.AgentWriter), new(biz.AgentRepository)),
		wire.Bind(new(biz.AgentReferenceChecker), new(biz.AgentRepository)),
		wire.Bind(new(biz.ProviderModelPairValidator), new(*biz.LlmProviderModelUsecase)),
		// Team-layer narrow interface bindings
		wire.Bind(new(biz.TeamUsageQuerier), new(*biz.UsageUsecase)),
		wire.Bind(new(biz.TeamRunStatusTransitioner), new(*biz.TeamUsecase)),
		wire.Bind(new(biz.SessionTurnExtrasPort), new(*biz.SessionUsecase)),
		wire.Bind(new(biz.SpiritTeamController), new(*biz.SpiritTeamUsecase)),
		// Self-check integration
		provideSelfCheckScheduler,
		provideDBPinger,
		provideEventBusHealthChecker,
		provideWSConnectionCounter,
		provideEventBusResubscriber,
		provideSelfCheckCleanup,
		provideSelfCheckJob,
		provideFailurePatternSyncJob,
		providePredictiveHealUsecase,
		providePredictiveHealJob,
		providePatternMiningUsecase,
		providePatternMiningJob,
		provideSpiritTeamUsecase,
		provideVerificationGateExecutor,
		wire.Bind(new(monitor.FailurePatternReader), new(*data.FailurePatternReadWriter)),
		wire.Bind(new(monitor.FailurePatternWriter), new(*data.FailurePatternReadWriter)),
		wire.Bind(new(monitor.RootCauseAnalyzer), new(*monitor.RootCauseEngine)),
		provideWSTurnExecutor,
		// Kanban bridge binding
		wire.Bind(new(kanbanpkg.Bridge), new(*service.KanbanToolBridge)),
		// ToolResultGate bindings
		wire.Bind(new(biz.ToolResultBlobReader), new(*data.ToolResultBlobRepo)),
		wire.Bind(new(biz.ToolResultBlobWriter), new(*data.ToolResultBlobRepo)),
		wire.Bind(new(biz.ToolResultReplacementWriter), new(*data.ToolResultReplacementRepo)),
		wire.Bind(new(biz.ToolResultReplacementReader), new(*data.ToolResultReplacementRepo)),
		// Knowledge embedder bindings
		wire.Bind(new(knowledge.QueryEmbedder), new(*knowledge.MultiProviderEmbedder)),
		wire.Bind(new(knowledge.Embedder), new(*knowledge.MultiProviderEmbedder)),
		// DynamicLLMCaller dependency bindings
		wire.Bind(new(chatagent.LLMCredentialResolver), new(*biz.LlmProviderModelUsecase)),
		wire.Bind(new(chatagent.LLMRefineConfigResolver), new(*biz.SystemSettingUsecase)),
		// Ecosystem preset: bind repo and provide usecase deps
		wire.Bind(new(biz.EcosystemPresetRepo), new(*data.EcosystemPresetRepo)),
		wire.Bind(new(biz.PackSeeder), new(*data.PackSeeder)),
		provideEcosystemPresetScenarioDir,
		wire.Bind(new(biz.DeptLeadTeamGetter), new(*data.TeamRepo)),
		provideDeptLeadManager,
		// TeamUsecaseOpts provider
		provideTeamUsecaseOpts,
		// SkillSimilarityComparer binding
		wire.Bind(new(biz.SkillSimilarityComparer), new(*biz.SkillSimilarityEngine)),
		// Memory extractor config providers
		provideMemoryLLMExtractorConfig,
		provideMemoryEnhancedExtractorConfig,
		// Bind *ChatService as OpenAIRunnerBuilder for the compat service
		// wrappers (AGUI / OpenAI Session / A2A Extension).
		wire.Bind(new(service.OpenAIRunnerBuilder), new(*service.ChatService)),
		newApp,
		provideWireOut,
	))
}
