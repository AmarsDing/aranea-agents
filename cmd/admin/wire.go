//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	a2apkg "aranea-agents/internal/a2a"
	a2ahealth "aranea-agents/internal/a2a/health"
	a2atrpc "aranea-agents/internal/a2a/trpc"
	"aranea-agents/internal/agent"
	chatagent "aranea-agents/internal/agent"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/agent/intent"
	v2 "aranea-agents/internal/agent/v2"
	"aranea-agents/internal/artifact"
	artifacttrpc "aranea-agents/internal/artifact/trpc"
	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/internal/biz/backgroundjob"
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
	"aranea-agents/internal/event/activityevent"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/graph"
	graphadapter "aranea-agents/internal/graph/adapter"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/mcp/alert"
	"aranea-agents/internal/mcp/health"
	"aranea-agents/internal/memory"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/internal/outbound"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	araneasession "aranea-agents/internal/session"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/internal/skill"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"
	"aranea-agents/internal/tools"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	subagenttool "aranea-agents/internal/tools/subagent"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func provideEventBusSideConsumers(
	infra *event.Infra,
	eventBus biz.EventBus,
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
	var monitorEventBus contract.MonitorBus
	if infra != nil {
		monitorEventBus = infra.MonitorEventBus
	}
	return biz.NewEventBusSideConsumers(eventBus, monitorEventBus, tools, webhooks, sessions, flowLogs, monitor, memWorker, traceProj, fileAppender, usage, logger)
}

func provideCronRunnerDeps(
	cron biz.CronRepo,
	session *biz.SessionUsecase,
	teams biz.TeamReader,
	agents biz.AgentRepository,
	eventBus biz.EventBus,
	monitorBus contract.MonitorBus,
	chat *service.ChatService,
	registrySyncAgent cronrunner.CronRegistrySyncAgent,
) cronrunner.Deps {
	return cronrunner.Deps{
		Cron:              cron,
		Session:           session,
		Teams:             teams,
		Agents:            agents,
		EventBus:          eventBus,
		MonitorBus:        monitorBus,
		Chat:              chat,
		RegistrySyncAgent: registrySyncAgent,
	}
}

func provideCronRunner(deps cronrunner.Deps, d *data.Data, lg loggateway.Logger) *cronrunner.Runner {
	if strings.TrimSpace(os.Getenv("CRON_RUNNER_DISABLED")) == "1" {
		return nil
	}
	// C-26: optional Postgres advisory lease for cross-instance exclusivity.
	deps.DB = providePrimaryRawDB(d)
	return cronrunner.NewRunner(deps, lg)
}

func provideSkillWatchRunner(skillReader watch.SkillReader, skillWriter watch.SkillWriter, sys biz.SystemSettingRepo, monitorBus contract.MonitorBus, mon *biz.MonitorUsecase, lg loggateway.Logger) *watch.Runner {
	if strings.TrimSpace(os.Getenv("SKILL_WATCH_DISABLED")) == "1" {
		return nil
	}
	r := watch.NewRunnerWithBus(skillReader, skillWriter, sys, monitorBus, lg)
	if r != nil {
		watch.SetSyncReporter(r, watch.NewMonitorSyncReporter(mon, monitorBus, lg))
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

func provideLlmProviderModelUsecaseWithDeps(repo biz.LlmProviderModelRepo, inspector biz.LLMInspector, crypto *biz.CredentialCrypto, agentRefs biz.AgentReferenceChecker, statsInjector *biz.ModelStatsInjector, lg loggateway.Logger) *biz.LlmProviderModelUsecase {
	return biz.NewLlmProviderModelUsecase(repo, repo, repo, repo, inspector, crypto, agentRefs, statsInjector, lg)
}

// provideModelStatsInjector 构造统计注入器，绑定到 usage.AnalyticsRepo（已绑定为 biz.UsageRepo）。
// cacheTTL 默认 5 分钟（在 NewModelStatsInjector 内部设定），避免每次 List 都查询 DB。
func provideModelStatsInjector(reader biz.ModelStatsReader, lg loggateway.Logger) *biz.ModelStatsInjector {
	return biz.NewModelStatsInjector(reader, lg)
}

func provideAgentUsecaseWithDeps(repo biz.AgentRepository, tools biz.ToolRegistryReader, sys biz.SystemSettingRepo, checker biz.WebResearchReadinessChecker, providerValidator biz.ProviderModelPairValidator, lg loggateway.Logger) *biz.AgentUsecase {
	return biz.NewAgentUsecase(biz.AgentUsecaseDeps{
		Reader: repo, Writer: repo, Settings: repo, Files: repo,
		Position: repo, Tx: repo, Tools: tools, Sys: sys,
		WebResearchChecker: checker, ProviderValidator: providerValidator, Lg: lg,
	})
}

// provideParallelToolExecutor builds the Wire-bound ParallelToolExecutor used
// by BatchExecuteSpiritTools for batch tool call scenarios (e.g.,
// multi_tool_use.parallel). The handler is nil at construction because tool
// dispatch is agent/session-specific; callers supply the handler at call time
// via BatchExecuteSpiritTools, which reuses this executor's concurrency
// configuration. Returns nil when ARANEA_PARALLEL_AUTO is disabled so callers
// transparently fall back to serial execution.
func provideParallelToolExecutor(lg loggateway.Logger) *tools.ParallelToolExecutor {
	if !intent.AllowAutoParallel() {
		return nil
	}
	return tools.NewParallelToolExecutor(nil, lg)
}

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

// provideGlobalBuildCache exposes the process-level agent BuildCache singleton
// so it can be registered with the LifecycleManager for orderly shutdown (A3).
func provideGlobalBuildCache() *agent.BuildCache {
	return agent.GetGlobalBuildCache()
}

// provideLifecycleManager builds the process-level LifecycleManager and
// registers the global build cache for LIFO shutdown (A3). Additional
// process-level resources can be registered here as they are migrated to
// the lifecycle abstraction.
func provideLifecycleManager(cache *agent.BuildCache, lg loggateway.Logger) *lifecycle.LifecycleManager {
	cache.SetLogger(lg)
	mgr := lifecycle.NewLifecycleManager(lg)
	mgr.Register("global-build-cache", cache)
	return mgr
}

// provideDeadLetterQueue builds the process-level DeadLetterQueue for
// pending-queue failures (A4). The queue is bounded to 1000 entries;
// when full, the oldest message is dropped (logged).
//
// A4 + T3.1: The queue is wired with a persist hook that stores failed
// pending-queue messages to the memory_job_deadletter table (unified dead-letter
// store). This ensures operators can inspect/retry/discard failed messages
// across process restarts, completing the A4 "内存缓冲 + DB 持久化" design.
func provideDeadLetterQueue(sink biz.MemoryDeadLetterSink, lg loggateway.Logger) *lifecycle.DeadLetterQueue {
	q := lifecycle.NewDeadLetterQueue(1000, lg)
	q.SetPersistHook(func(ctx context.Context, msg *lifecycle.DeadLetterMessage) error {
		// Map DeadLetterMessage → MemoryDeadLetterRequest. The Original field
		// holds the pending message content (string); Source identifies the
		// origin (e.g. "pending-queue").
		originalStr := ""
		if s, ok := msg.Original.(string); ok {
			originalStr = s
		}
		sink.WriteMemoryDeadLetter(
			biz.MemoryDeadLetterRequest{
				AppName:  msg.Source,
				Priority: biz.MemoryJobPriorityNormal,
				TenantID: originalStr, // store original content for inspection
			},
			biz.MemoryDeadLetterReasonPendingQueueFailure,
			msg.Error,
		)
		return nil
	})
	return q
}

// provideRunHeartbeatEmitter builds the Wire-bound RunHeartbeatEmitter (P1-7).
// The emit interval is read from the RUN_HEARTBEAT_INTERVAL env var (e.g.
// "10s", "30s"); when unset or invalid, NewRunHeartbeatEmitter applies its
// built-in 10s default (interval <= 0 → defaultHeartbeatInterval).
func provideRunHeartbeatEmitter(eventBus biz.EventBus, lg loggateway.Logger) *service.RunHeartbeatEmitter {
	interval := time.Duration(0)
	if raw := strings.TrimSpace(os.Getenv("RUN_HEARTBEAT_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			interval = d
		}
	}
	return service.NewRunHeartbeatEmitter(interval, eventBus, lg)
}

// providePendingMessageQueue builds the Wire-bound PendingMessageQueue with
// snapshot persistence enabled. The snapshot directory is resolved from (in
// order): PENDING_QUEUE_SNAPSHOT_DIR env var, the loggateway output dir, or
// empty (disables persistence). When persistence is enabled, the queue is
// restored on startup and snapshotted every 10s, so queued messages survive
// process restarts — required by the "no time limit" long-task guarantee.
func providePendingMessageQueue(lg loggateway.Logger) *rt.PendingMessageQueue {
	dir := strings.TrimSpace(os.Getenv("PENDING_QUEUE_SNAPSHOT_DIR"))
	if dir == "" {
		if gw, ok := lg.(*loggateway.Gateway); ok {
			dir = gw.OutputDir()
		}
	}
	return rt.NewPendingMessageQueueWithDirAndLogger(dir, lg)
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

// provideRecoveryWorker builds the P1-8 crash-recovery worker. It reuses the
// shared CheckpointSaver (bound to trpcgraph.CheckpointSaver) and the
// DurableResumeGateway to resume stale durable runs after a process restart.
// Returns nil if any dependency is nil (defensive construction).
func provideRecoveryWorker(sessionRuns *biz.SessionRunUsecase, saver trpcgraph.CheckpointSaver, resumer biz.DurableResumeGateway, lg loggateway.Logger) *service.RecoveryWorker {
	return service.NewRecoveryWorker(sessionRuns, saver, resumer, lg)
}

// provideBackgroundJobRegistry builds the in-memory Runner registry for the
// Unified BackgroundJob subsystem (M56 BLO-5). Runners register themselves
// at construction time; the worker queries the registry to dispatch claimed
// jobs by Kind.
func provideBackgroundJobRegistry() backgroundjob.Registry {
	return backgroundjob.NewRegistry()
}

// provideBackgroundJobWorker builds the worker that polls backgroundjob.Repo
// for queued jobs and dispatches them to registered Runners. Returns nil if
// any dependency is nil (defensive construction). When no runners are
// registered at startup, Start() logs once and exits without polling.
func provideBackgroundJobWorker(repo backgroundjob.Repo, registry backgroundjob.Registry, lg loggateway.Logger) *service.BackgroundJobWorker {
	return service.NewBackgroundJobWorker(repo, registry, lg)
}

func provideMonitorAlertNotifier(channels *biz.ChannelUsecase, monitorBus contract.MonitorBus, lg loggateway.Logger) biz.AlertNotifier {
	return service.NewMonitorAlertNotifier(channels, monitorBus, lg)
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

func provideUsageUsecase(repo biz.UsageRepo, mon *biz.MonitorUsecase, teamUC *biz.TeamUsecase, sessions *biz.SessionUsecase, eventBus biz.EventBus, lg loggateway.Logger) *biz.UsageUsecase {
	uc := biz.NewUsageUsecase(repo, lg)
	uc.SetAlertNotifier(service.NewMonitorBudgetAlertNotifier(mon))
	uc.SetTeamReader(teamUC)
	uc.SetSessionMetricsAccumulator(&sessionMetricsAdapter{sessions: sessions})
	uc.SetCompletionUsageLinker(&completionLinkerAdapter{mon: mon})
	uc.SetUsageEnvelopePublisher(&envelopePublisherAdapter{eventBus: eventBus})
	return uc
}

func provideSystemSettingUsecase(repo biz.SystemSettingRepo, quota biz.UsageQuotaRepo, tester biz.WebResearchTester, tp biz.SystemSettingTxProvider) *biz.SystemSettingUsecase {
	uc := biz.NewSystemSettingUsecase(repo, quota)
	uc.SetWebResearchTester(tester)
	uc.SetTxProvider(tp)
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
	parallelExec *tools.ParallelToolExecutor,
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
		ParallelToolExecutor:        parallelExec,
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
	nl2graph graph.NL2GraphConverter,
	runtimeReplanner graph.RuntimeReplanner,
) service.TeamOrchestrationDeps {
	return service.TeamOrchestrationDeps{
		TeamUC:            teamUC,
		TeamsNative:       teamsNative,
		GraphFactory:      graphFactory,
		Graphs:            graphs,
		Tasks:             tasks,
		TeamGraphCoord:    teamGraphCoord,
		TeamMediator:      mediator,
		SpiritUC:          spiritUC,
		TaskPlanner:       taskPlanner,
		AgentAllocator:    agentAllocator,
		NL2GraphConverter: nl2graph,
		RuntimeReplanner:  runtimeReplanner,
	}
}

func provideRunnerConfig(
	pluginRT *plugintrpc.Runtime,
	pluginMgr *plugintrpc.Manager,
	knowledgeRetriever *knowledge.Retriever,
	knowledgeRouter *knowledge.AdaptiveRouter,
	knowledgeFederatedRetriever *knowledge.FederatedRetriever,
	knowledgeEvaluator *knowledge.RetrievalEvaluator,
	knowledgeUC *biz.KnowledgeUsecase,
	graphs *biz.GraphUsecase,
	graphFactory biz.GraphBuilderFactory,
	tasks *biz.TaskUsecase,
	runs *rt.RunRegistry,
	tools *biz.ToolUsecase,
	agents biz.AgentRepository,
	activityWriter biz.ActivityWriter,
	activityUpserter biz.ActivityUpserter,
	orgUC *biz.OrganizationUsecase,
	toolResultGate *biz.ToolResultGate,
	outboundRouter *outbound.Router,
	subAgentSvc *subagenttool.Service,
	kanbanBridge kanbanpkg.Bridge,
	a2aUC *biz.A2AUsecase,
	sessions *biz.SessionUsecase,
	v2ProjectorFactory *v2.ProjectorFactory,
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
		KnowledgeUsecase: knowledgeUC,
		Runs:             runs,
		StreamOptsFactory: &chatactivity.StreamOptsFactoryAdapter{
			V2ProjectorFactory: v2ProjectorFactory,
		},
		AgentHelper:     &chatagent.TeamAgentHelperAdapter{},
		OrganizationUC:  orgUC,
		ToolResultGate:  toolResultGate,
		OutboundRouter:  outboundRouter,
		SubAgentService: subAgentSvc,
		KanbanBridge:    kanbanBridge,
		A2AEnabled:      a2aUC != nil,
		// SessionChildLookup resolves member agent session IDs for child_session_id
		// in session activities. Uses SessionUsecase.ListChildSessions to look up
		// child sessions by parent (team) session ID and match by MemberAgentKey.
		SessionChildLookup: &sessionChildLookupAdapter{sessions: sessions},
	}
	if graphs != nil {
		cfg.GraphLoader = graphadapter.NewLinkedGraphBuildConfigLoader(graphs)
	}
	if graphFactory != nil {
		if builder, ok := graphFactory.(graphadapter.TeamGraphRootBuilder); ok {
			cfg.GraphRoot = builder
		}
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
	activityBus biz.ActivityEventBus,
	eventBus biz.EventBus,
	monitorEventBus contract.MonitorBus,
	seq *v2.Sequencer,
	lg loggateway.Logger,
) rt.TurnDeps {
	// LLMHTTP timeout is sourced from TimeoutPolicy.
	// TaskTypeModerate (60min) is the baseline; per-task-type overrides
	// can be applied in the LLM call path via context (see trpc_llm.go).
	timeoutPolicy := provider.NewTimeoutPolicy()
	return rt.TurnDeps{
		ReadDeps:  provideTurnReadDeps(agents, agentsUC, toolRegistry, toolUC, llmCatalog, skillUC, sys),
		Persist:   persist,
		Pipeline:  rt.EventPipeline{ActivityBus: activityBus, EventBus: eventBus, MonitorEventBus: monitorEventBus, Sequencer: seq},
		LLMHTTP:   &http.Client{Timeout: timeoutPolicy.TimeoutFor(provider.TaskTypeModerate)},
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
	activityBus biz.ActivityEventBus,
	eventBus biz.EventBus,
	monitorEventBus contract.MonitorBus,
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
	activityUpserter biz.ActivityUpserter,
	activityReader biz.ActivityReader,
	stepReader biz.StepV2Reader,
	heartbeatEmitter *service.RunHeartbeatEmitter,
	deadLetterQueue *lifecycle.DeadLetterQueue,
	profileResolver *chatagent.ProfileResolver,
	v2ProjectorFactory *v2.ProjectorFactory,
	lg loggateway.Logger,
) service.ChatOrchestratorDeps {
	// Backfill TaskOrchestrator into teamDeps to break the Wire cycle:
	// TaskOrchestrator → SpiritTeamAssembler → TeamStarterPort → ChatService.
	// provideTeamOrchestrationDeps cannot include TaskOrchestrator directly
	// (it would create a cycle), so we inject it here after Wire resolves it.
	teamDeps.TaskOrchestrator = taskOrch

	// LLMHTTP timeout is sourced from TimeoutPolicy.
	// TaskTypeModerate (60min) is the baseline; per-task-type overrides
	// can be applied in the LLM call path via context (see trpc_llm.go).
	timeoutPolicy := provider.NewTimeoutPolicy()
	return service.ChatOrchestratorDeps{
		Turn: service.ChatTurnDeps{
			TurnDeps: rt.TurnDeps{
				ReadDeps:  provideTurnReadDeps(agents, agentsUC, toolRegistry, toolUC, llmCatalog, skillUC, sys),
				Persist:   persist,
				Pipeline:  rt.EventPipeline{ActivityBus: activityBus, EventBus: eventBus, MonitorEventBus: monitorEventBus},
				LLMHTTP:   &http.Client{Timeout: timeoutPolicy.TimeoutFor(provider.TaskTypeModerate)},
				Sessions:  sessions,
				SessionRT: sessionRT,
				Compress:  compress,
				AfterTurn: biz.NoopNativeTurnAfter{},
				RunnerMgr: rt.NewRunnerManagerFromPersist(persist, lg),
				Lg:        lg,
			},
			Runs:             runs,
			PendingQueue:     pendingQueue,
			RT:               rtDeps,
			TurnTimeout:      0,
			Admission:        biz.NewTurnAdmissionUsecase(biz.TurnAdmissionUsecaseConfig{Quota: usage, Agents: agents}),
			ActivityWriter:   activityWriter,
			ActivityUpserter: activityUpserter,
			ActivityReader:   activityReader,
			StepReader:       stepReader,
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
			LG:                 lg,
			OrchCache:          orchCache,
			A2AUC:              a2aUC,
			MCPServers:         mcpUC,
			OutboundRouter:     outboundRouter,
			SubAgentService:    subAgentSvc,
			TurnLifecycle:      turnLifecycle,
			HeartbeatEmitter:   heartbeatEmitter,
			DeadLetterQueue:    deadLetterQueue,
			ProfileResolver:    profileResolver,
			V2ProjectorFactory: v2ProjectorFactory,
		},
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
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
		Admin:               biz.NewMemoryAdminUsecase(persist.Memory.Admin, vec, factSync, data.NewL3FactWriterAdapter(d, d.VectorStore()), lg),
		Cascade:             cascade,
		SysUC:               sysUC,
		DeadLetterRepo:      deadLetterRepo,
		DebugRecaller:       data.NewMemoryDebugRecaller(d),
		FactIndexCounter:    data.NewMemoryFactIndexCounter(d),
		WorkerStats:         workerStats,
		SpreadingActivation: memory.NewSpreadingActivationEngine(data.NewL4GraphTraverser(d), lg),
		DeadLetterEnqueue:   enqueue,
		QueueStats:          queueStats,
		Logger:              lg,
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

// providePrimaryRawDB returns the primary database's raw *sql.DB handle.
// After A6, the primary database is Postgres; this name replaces the legacy
// provideSQLiteRawDB to reflect the dialect-agnostic role.
func providePrimaryRawDB(d *data.Data) *sql.DB {
	if d == nil {
		return nil
	}
	return d.RWDB().WriteHandle()
}

func provideTRPCSessionService(d *data.Data, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) trpcsession.Service {
	pgDSN := d.PostgresDSN()
	return rt.NewTRPCSessionService(pgDSN, lg, sessiontrpc.SummarizerConfig{
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

func provideGraphCheckpointSaver(d *data.Data, lg loggateway.Logger) (*graphtrpc.CheckpointSaver, error) {
	rawDB := providePrimaryRawDB(d)
	pgDSN := d.PostgresDSN()
	return rt.NewGraphCheckpointSaver(rawDB, pgDSN, lg)
}

// provideNL2GraphConverter builds the NL2GraphConverter for natural-language →
// graph build config conversion. P1 fix (2026-06-18): previously this component
// was implemented but never wired into production (orphan component). The llm
// parameter is nil for now; callers that need NL2Graph will get an Internal
// error and can fall back to build_orchestration_graph tool. Future work:
// inject a dedicated planner model.
func provideNL2GraphConverter(lg loggateway.Logger) graph.NL2GraphConverter {
	return graph.NewNL2GraphConverter(nil, lg)
}

// provideRuntimeReplanner builds the RuntimeReplanner for graph node failure
// recovery. P1 fix (2026-06-18): previously this component was implemented but
// never wired into production (orphan component). It is now available for
// Graph executors to call OnNodeFailure on node errors.
func provideRuntimeReplanner(eventBus biz.EventBus, lg loggateway.Logger) graph.RuntimeReplanner {
	return graph.NewRuntimeReplanner(eventBus, lg)
}

// provideTopologyEvolver builds the TopologyEvolver for dynamic graph topology
// evolution. The LLM model is resolved from TOPOLOGY_EVOLVER_PROVIDER and
// TOPOLOGY_EVOLVER_MODEL env vars; when unset, the evolver degrades gracefully
// (returns nil edge, no error) because NewTopologyEvolver accepts a nil LLM.
// The evolver is integrated into the Graph executor via AfterNode callbacks
// (B4) and is called when execution insights suggest a new transfer edge.
func provideTopologyEvolver(
	catalog *biz.LlmProviderModelUsecase,
	eventBus biz.EventBus,
	lg loggateway.Logger,
) graph.TopologyEvolver {
	var llm trpcmodel.Model
	prov := strings.TrimSpace(os.Getenv("TOPOLOGY_EVOLVER_PROVIDER"))
	mod := strings.TrimSpace(os.Getenv("TOPOLOGY_EVOLVER_MODEL"))
	if prov != "" && mod != "" && catalog != nil {
		rtTrip := &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}}
		if m, err := provider.TRPCModelForProviderModel(context.Background(), catalog, rtTrip, prov, mod, lg); err == nil {
			llm = m
		} else {
			lg.Warn("topology evolver: LLM model build failed, edge decisions will be no-op",
				loggateway.Str("provider", prov),
				loggateway.Str("model", mod),
				loggateway.Err(err))
		}
	}
	return graph.NewTopologyEvolver(llm, eventBus, lg)
}

func provideGraphBuildDeps(
	catalog *biz.LlmProviderModelUsecase,
	toolUC *biz.ToolUsecase,
	agentUC *biz.AgentUsecase,
	agents biz.AgentRepository,
	sys biz.SystemSettingRepo,
	skillUC *biz.SkillUsecase,
	persist rt.PersistenceSet,
	skillDBRepo trpcskill.Repository,
	codeExecFactory *localexec.Factory,
	knowledgeRetriever *knowledge.Retriever,
	knowledgeUC *biz.KnowledgeUsecase,
	pluginMgr *plugintrpc.Manager,
	orgUC *biz.OrganizationUsecase,
	toolResultGate *biz.ToolResultGate,
	outboundRouter *outbound.Router,
	subAgentSvc *subagenttool.Service,
	a2aUC *biz.A2AUsecase,
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
			ToolUC:     toolUC,
			MCPTooling: persist.AgentMCP,
		},
		TRPCMemoryKnowledgeDeps: chatagent.TRPCMemoryKnowledgeDeps{
			HasMemory:             persist.Memory.Available(),
			MemoryService:         persist.Memory.TRPC,
			MemoryAdmin:           persist.Memory.Admin,
			MemoryActionLogWriter: persist.Memory.ActionLogWriter,
			ContextCompressor:     persist.Memory.ContextCompressor,
			MemoryL2Recall:        persist.Memory.L2Recall,
			MemoryL3Recall:        persist.Memory.L3Recall,
			MemoryCompositeRecall: persist.Memory.CompositeRecall,
			KnowledgeRetriever:    knowledgeRetriever,
			KnowledgeUsecase:      knowledgeUC,
		},
		TRPCPluginDeps: chatagent.TRPCPluginDeps{
			PluginManager: pluginMgr,
		},
		TRPCSkillDeps: chatagent.TRPCSkillDeps{
			SkillUC:         skillUC,
			SkillDBRepo:     skillDBRepo,
			CodeExecFactory: codeExecFactory,
		},
		TRPCExtensionDeps: chatagent.TRPCExtensionDeps{
			Organization:    orgUC,
			ToolResultGate:  toolResultGate,
			OutboundRouter:  outboundRouter,
			SubAgentService: subAgentSvc,
			A2AEnabled:      a2aUC != nil,
			LG:              lg,
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
	deadLetterSink biz.MemoryDeadLetterSink,
	workerStats *biz.MemoryWorkerStats,
	lg loggateway.Logger,
) (*jobs.AutoMemoryWorker, error) {
	return jobs.NewAutoMemoryWorker(jobs.AutoMemoryWorkerConfig{
		RuntimeConf:    runtimeConf,
		Interval:       0,
		Sessions:       sessions,
		Agents:         agents,
		Writer:         writer,
		IndexSync:      factSync,
		EpisodeSync:    episodeSync,
		L4:             l4,
		Consolidator:   biz.DefaultMemoryConsolidator(extractor),
		Queue:          queue,
		DeadLetterSink: deadLetterSink,
		Stats:          workerStats,
		Logger:         lg,
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
	graphs biz.GraphExecutor,
	cron biz.CronTriggerGateway,
	activityBus biz.ActivityEventBus,
	eventBus biz.EventBus,
	admission *biz.TurnAdmissionUsecase,
	teamCompiler biz.TeamCompiler,
	lg loggateway.Logger,
) *service.ChannelIngress {
	dedupe := biz.NewIngressMessageDedupe(biz.DefaultMessageDedupeTTL)
	debouncer := biz.NewIngressPeerDebouncer(biz.DefaultIngressDebounce, lg)
	registry := biz.NewTurnPreviewRegistry()
	gate := biz.NewChannelConcurrentGate()
	return service.NewChannelIngress(channels, turnJobs, sessions, chat, graphs, cron, activityBus, eventBus, dedupe, debouncer, registry, gate, admission, teamCompiler, lg)
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

func provideChannelTurnJobSweeper(
	turnJobRepo biz.ChannelTurnJobRepo,
	graphs *biz.GraphUsecase,
	cron biz.CronTriggerGateway,
	lg loggateway.Logger,
) *jobs.ChannelTurnJobSweeper {
	if jobs.ChannelTurnJobSweeperDisabled() {
		return nil
	}
	return jobs.NewChannelTurnJobSweeper(0, 0, turnJobRepo, graphs, cron, lg)
}

func provideMemoryEpisodeBackfillWorker(reader biz.MemoryEpisodeBackfillReader, episodeSync biz.EpisodeIndexSyncer, sys biz.SystemSettingRepo, stats *biz.MemoryWorkerStats, lg loggateway.Logger) *jobs.MemoryEpisodeBackfillWorker {
	if biz.ResolveEpisodeBackfillDisabled(context.Background(), sys) {
		return nil
	}
	return jobs.NewMemoryEpisodeBackfillWorker(0, reader, episodeSync, sys, stats, lg)
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

// provideMemoryEbbinghausDecayWorker wires the Ebbinghaus exponential decay
// worker. The worker scans memories via L3FactReader (DB read), computes
// per-agent Ebbinghaus reachability R_t, and writes R_t back to the DB via
// DecayScoreWriter so fused recall can down-weight forgotten memories.
// Disabled via MEMORY_EBBINGHAUS_DECAY_DISABLED env var.
func provideMemoryEbbinghausDecayWorker(d *data.Data, agents *biz.AgentUsecase, lg loggateway.Logger) *jobs.MemoryEbbinghausDecayWorker {
	if jobs.MemoryEbbinghausDecayDisabled() {
		return nil
	}
	return jobs.NewMemoryEbbinghausDecayWorker(0, nil, data.NewL3FactReaderForUser(d), data.NewDecayScoreWriter(d), agents, lg)
}

// memorySleepTimeQueueSize is the buffer size for the in-memory consolidation
// queue consumed by the Sleep-time Agent.
const memorySleepTimeQueueSize = 100

// provideMemorySleepTimeWorker wires the Sleep-time Agent worker. It builds a
// SleepTimeService backed by the shared trpc memory Service, an optional LLM
// model (resolved from MEMORY_SLEEP_TIME_PROVIDER/MEMORY_SLEEP_TIME_MODEL env
// vars), and an in-memory consolidation queue.
//
// Target lister selection (in priority order):
//  1. MEMORY_SLEEP_TIME_USER_IDS env var — explicit override for testing/debug.
//  2. SessionRepo-derived lister — production default. Enumerates distinct
//     (agent, user) pairs from sessions active in the last 7 days, filtered to
//     agents with L3 facts enabled.
//  3. nil — queue-only mode (drains the queue but does not proactively enqueue).
//
// A circuit breaker (5 failures → 5min pause → half-open) and a dead-letter
// writer (persists exhausted jobs to memory_job_deadletter) are attached to
// the worker's job runner.
//
// Disabled via MEMORY_SLEEP_TIME_DISABLED env var.
func provideMemorySleepTimeWorker(
	memSvc trpcmemory.Service,
	agents *biz.AgentUsecase,
	catalog *biz.LlmProviderModelUsecase,
	sessionReader biz.SessionReader,
	deadLetterSink biz.MemoryDeadLetterSink,
	d *data.Data,
	lg loggateway.Logger,
) *jobs.MemorySleepTimeWorker {
	if jobs.MemorySleepTimeDisabled() {
		return nil
	}
	// Resolve optional LLM model for consolidation analysis. When unset, the
	// SleepTimeService gracefully degrades to a no-op (llmConsolidate returns
	// an empty result).
	var llm trpcmodel.Model
	prov := strings.TrimSpace(os.Getenv("MEMORY_SLEEP_TIME_PROVIDER"))
	mod := strings.TrimSpace(os.Getenv("MEMORY_SLEEP_TIME_MODEL"))
	if prov != "" && mod != "" && catalog != nil {
		rtTrip := &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}}
		if m, err := provider.TRPCModelForProviderModel(context.Background(), catalog, rtTrip, prov, mod, lg); err == nil {
			llm = m
		} else {
			lg.Warn("sleep-time worker: LLM model build failed, consolidation will be no-op",
				loggateway.Str("provider", prov),
				loggateway.Str("model", mod),
				loggateway.Err(err))
		}
	}
	queue := memory.NewConsolidationQueue(memorySleepTimeQueueSize)
	svc := memory.NewSleepTimeService(memSvc, llm, queue, lg)
	// Phase 6A-06: wire EpisodeConsolidator for L2→L3 fact extraction.
	// Reuses the same LLM (if configured) for episode analysis. When d is nil
	// or LLM is unset, EpisodeConsolidator gracefully degrades to a no-op.
	if d != nil {
		ec := memory.NewEpisodeConsolidator(
			data.NewL2RecallStore(d, d.VectorStore()),
			data.NewL3FactWriterAdapter(d, d.VectorStore()),
			data.NewMemoryActionLogWriter(d),
			llm,
			lg,
		)
		// T8: configurable min importance (env: MEMORY_EPISODE_MIN_IMPORTANCE).
		// Default: 0.3. Set to "0" to disable filtering (keep all extracted facts).
		if raw := strings.TrimSpace(os.Getenv("MEMORY_EPISODE_MIN_IMPORTANCE")); raw != "" {
			if val, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil && val >= 0 {
				ec.SetMinImportance(val)
				lg.Info("episode consolidator: min importance configured",
					loggateway.Str("min_importance", strconv.FormatFloat(val, 'f', 2, 64)))
			}
		}
		svc.SetEpisodeConsolidator(ec)
	}
	// Build target lister. Priority: env-var override → SessionRepo-derived.
	var lister jobs.SleepTimeTargetLister
	if userIDs := parseSleepTimeUserIDsFromEnv(); len(userIDs) > 0 {
		lg.Info("sleep-time worker: using env-var target lister (MEMORY_SLEEP_TIME_USER_IDS)")
		lister = jobs.NewAgentUserKeyLister(agents, userIDs)
	} else if sessionReader != nil {
		lg.Info("sleep-time worker: using SessionRepo-derived target lister (7-day lookback)")
		lister = jobs.NewAgentUserKeyListerFromSession(agents, sessionReader)
	}
	worker := jobs.NewMemorySleepTimeWorker(0, svc, lister, lg)
	// Attach circuit breaker: 5 consecutive failures → 5min cool-down →
	// half-open probe. Prevents retry storms when the memory backend is down.
	worker.WithCircuitBreaker(jobs.NewCircuitBreaker(lg))
	// Attach dead-letter writer: exhausted jobs are persisted to
	// memory_job_deadletter for later replay/analysis.
	worker.WithDeadLetter(jobs.NewDeadLetterSinkAdapter(deadLetterSink))
	return worker
}

// parseSleepTimeUserIDsFromEnv reads the MEMORY_SLEEP_TIME_USER_IDS env var
// (comma-separated) and returns the trimmed, non-empty user ID list.
func parseSleepTimeUserIDsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("MEMORY_SLEEP_TIME_USER_IDS"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func provideMemoryFactIndexReconciler(maintainer biz.MemoryFactIndexMaintainer, factSync biz.MemoryFactIndexSyncer, lg loggateway.Logger) *jobs.MemoryFactIndexReconciler {
	if jobs.MemoryIndexReconcileDisabled() {
		return nil
	}
	return jobs.NewMemoryFactIndexReconciler(0, maintainer, factSync, lg)
}

func provideMemoryDeadLetterReplayer(repo biz.MemoryDeadLetterAdminRepo, queue memtrpc.AutoMemoryQueue, lg loggateway.Logger) *jobs.MemoryDeadLetterReplayer {
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
	return jobs.NewMemoryDeadLetterReplayer(0, repo, enqueue, lg)
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

// provideTraceProjector wires the TraceProjector to the typed MonitorEventBus.
//
// ADR-03 Phase 5 Blocker F: migrated from legacy envelope-based MonitorBus to
// contract.MonitorBus. TraceProjector now subscribes to MonitorEventTypeFlowLog
// via a bus-level Filter, matching the FlowFileAppender / SelfHealObserver
// migration pattern.
func provideTraceProjector(traceRepo biz.MonitorTraceRepo, infra *event.Infra, lg loggateway.Logger) *monitor.TraceProjector {
	var monitorBus contract.MonitorBus
	if infra != nil {
		monitorBus = infra.MonitorEventBus
	}
	return monitor.NewTraceProjector(traceRepo, lg, monitorBus)
}

// provideMonitorBus / ProvideSessionBus removed (ADR-03 Phase 5 Blocker F):
// the legacy envelope-based bus system has been fully retired. All monitor
// subscribers now consume contract.MonitorBus via ProvideMonitorEventBus.

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

func provideSelfCheckScheduler(
	checkers []monitor.SelfChecker,
	repairers []monitor.SelfCheckRepairer,
	repo monitor.SelfCheckReportRepo,
	registry *monitor.AlertMetricRegistry,
	lg loggateway.Logger,
) *monitor.SelfCheckScheduler {
	// Wrap repairers in SelfCheckRepairDispatcher to enforce per-checker cooldown
	// (5min) and prevent repeated repairs of the same failing check within the
	// cooldown window. Without the dispatcher, SelfCheckScheduler would call
	// repairers directly on every cycle (every 5min), bypassing cooldown logic.
	dispatcher := monitor.NewSelfCheckRepairDispatcher(repairers, lg)
	scheduler := monitor.NewSelfCheckScheduler(checkers, []monitor.SelfCheckRepairer{dispatcher}, repo, registry, lg)
	// Register the unhealthy-count metric so AlertEvalWorker can evaluate
	// alert rules against it. Without this registration, the metric is never
	// polled and self-check degradation goes undetected by the alerting system.
	if registry != nil {
		registry.Register(monitor.NewSelfCheckUnhealthyCountMetric(scheduler))
	}
	return scheduler
}

func provideEventBusHealthChecker() monitor.EventBusHealthChecker { return nil }

func provideWSConnectionCounter() monitor.WSConnectionCounter { return nil }

func provideEventBusResubscriber() monitor.EventBusResubscriber { return nil }

func provideDBPinger(d *data.Data) monitor.DBPinger {
	if d == nil {
		return nil
	}
	return monitor.NewDBPinger(d.RWDB().WriteHandle(), d.Dialect().String())
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

func provideMCPHealthRunnerDeps(mcpRepo biz.MCPServerReader, mcpUC *biz.MCPServerUsecase, monitorBus contract.MonitorBus, lg loggateway.Logger) health.Deps {
	return health.Deps{
		MCP:    mcpRepo,
		UC:     mcpUC,
		Alerts: alert.NewPublisher(monitorBus, mcpUC, lg),
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

func providePluginRuntime(stats plugintrpc.StatsRecorder, usage biz.PluginCostGuardUsageRepo, tools *biz.ToolUsecase, deliveries biz.HookDeliveryRepo, monitorBus contract.MonitorBus, lg loggateway.Logger) *plugintrpc.Runtime {
	rt := plugintrpc.NewRuntime(stats, lg)
	rt.SetMonitorBus(monitorBus)
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
	RecoveryWorker              *service.RecoveryWorker
	BackgroundJobWorker         *service.BackgroundJobWorker
	ChannelRuntime              *service.ChannelRuntime
	PluginRuntime               *plugintrpc.Runtime
	ToolAuditCleanup            *jobs.ToolAuditCleanup
	FlowLogCleanup              *jobs.FlowLogCleanup
	MonitorAlertCooldownCleanup *jobs.MonitorAlertCooldownCleanup
	AutoHealTTLCleanup          *jobs.AutoHealTTLCleanup
	MonitorAlertEvalWorker      *monitor.AlertEvalWorker
	MonitorTraceBackfillWorker  *jobs.MonitorTraceBackfillWorker
	MemoryL2Decay               *jobs.MemoryL2DecayWorker
	MemoryL1Archive             *jobs.MemoryL1ArchiveWorker
	ChannelTurnJobSweeper       *jobs.ChannelTurnJobSweeper
	MemoryL3Decay               *jobs.MemoryL3DecayWorker
	MemoryL4Decay               *jobs.MemoryL4DecayWorker
	MemoryEbbinghausDecay       *jobs.MemoryEbbinghausDecayWorker
	MemorySleepTime             *jobs.MemorySleepTimeWorker
	MemoryEpisodeBackfill       *jobs.MemoryEpisodeBackfillWorker
	MemoryDataMigration         *jobs.MemoryDataMigrationWorker
	MemoryFactIndexReconciler   *jobs.MemoryFactIndexReconciler
	MemoryDeadLetterReplayer    *jobs.MemoryDeadLetterReplayer
	ModelRegistrySyncAgent      *agent.ModelRegistrySyncAgent
	SelfCheckScheduler          *monitor.SelfCheckScheduler
	SelfHealObserver            *biz.SelfHealObserver
	MonitorBus                  contract.MonitorBus
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
	// WSV2Subscriber forwards v2 Events to WS clients. Owned by wireOut so
	// its lifecycle (Close) is managed alongside the kratos.App shutdown.
	WSV2Subscriber *server.WSV2Subscriber
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
	recoveryWorker *service.RecoveryWorker,
	backgroundJobWorker *service.BackgroundJobWorker,
	channelRuntime *service.ChannelRuntime,
	pluginRuntime *plugintrpc.Runtime,
	toolAuditCleanup *jobs.ToolAuditCleanup,
	flowLogCleanup *jobs.FlowLogCleanup,
	monitorAlertCooldown *jobs.MonitorAlertCooldownCleanup,
	autoHealTTLCleanup *jobs.AutoHealTTLCleanup,
	monitorAlertEvalWorker *monitor.AlertEvalWorker,
	monitorTraceBackfillWorker *jobs.MonitorTraceBackfillWorker,
	memoryL2Decay *jobs.MemoryL2DecayWorker,
	memoryL1Archive *jobs.MemoryL1ArchiveWorker,
	channelTurnJobSweeper *jobs.ChannelTurnJobSweeper,
	memoryL3Decay *jobs.MemoryL3DecayWorker,
	memoryL4Decay *jobs.MemoryL4DecayWorker,
	memoryEbbinghausDecay *jobs.MemoryEbbinghausDecayWorker,
	memorySleepTime *jobs.MemorySleepTimeWorker,
	memoryEpisodeBackfill *jobs.MemoryEpisodeBackfillWorker,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
	memoryFactIndexReconciler *jobs.MemoryFactIndexReconciler,
	memoryDeadLetterReplayer *jobs.MemoryDeadLetterReplayer,
	modelRegistrySyncAgent *agent.ModelRegistrySyncAgent,
	selfCheckScheduler *monitor.SelfCheckScheduler,
	selfHealObserver *biz.SelfHealObserver,
	eventInfra *event.Infra,
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
	wsV2Sub *server.WSV2Subscriber,
) wireOut {
	return wireOut{
		App: app, Data: dataData, CronRunner: runner, SkillWatch: skillWatch, AutoMemory: autoMem,
		MCPHealthProbe: mcpHealth, A2AGatewayHealthProbe: a2aHealth, EvolutionScanner: evoScan, LearningLoopScanner: learningLoop, ProviderHealthScanner: providerHealth,
		ChannelHealthScanner: channelHealth, ChannelDeliveryScanner: channelDelivery,
		SessionRunDurableWorker: sessionRunDurable,
		RecoveryWorker:          recoveryWorker,
		BackgroundJobWorker:     backgroundJobWorker,
		ChannelRuntime:          channelRuntime,
		PluginRuntime:           pluginRuntime,
		ToolAuditCleanup:        toolAuditCleanup,
		FlowLogCleanup:          flowLogCleanup, MonitorAlertCooldownCleanup: monitorAlertCooldown, AutoHealTTLCleanup: autoHealTTLCleanup, MonitorAlertEvalWorker: monitorAlertEvalWorker, MonitorTraceBackfillWorker: monitorTraceBackfillWorker, MemoryL2Decay: memoryL2Decay, MemoryL1Archive: memoryL1Archive, ChannelTurnJobSweeper: channelTurnJobSweeper, MemoryL3Decay: memoryL3Decay, MemoryL4Decay: memoryL4Decay,
		MemoryEbbinghausDecay:     memoryEbbinghausDecay,
		MemorySleepTime:           memorySleepTime,
		MemoryEpisodeBackfill:     memoryEpisodeBackfill,
		MemoryDataMigration:       memoryDataMigration,
		MemoryFactIndexReconciler: memoryFactIndexReconciler,
		MemoryDeadLetterReplayer:  memoryDeadLetterReplayer,
		ModelRegistrySyncAgent:    modelRegistrySyncAgent,
		SelfCheckScheduler:        selfCheckScheduler,
		SelfHealObserver:          selfHealObserver,
		MonitorBus:                eventInfra.MonitorEventBus,
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
		WSV2Subscriber:            wsV2Sub,
	}
}

// ────────────────────────────────────────────────────────────
// v2 event pipeline providers (Phase 1 收尾)
//
// Wiring chain: trpc event → stream_consumer → v2.Projector →
// v2.Sequencer → RepoSet (persist) + V2Bus → WSV2Subscriber → WS client.
//
// Phase 2 complete: v1 CompatAdapter removed (frontend now consumes v2 events directly).
// ────────────────────────────────────────────────────────────

// provideV2EventBus constructs the in-process fan-out bus for v2 Events.
// B-06: optionally journals critical events to ARANEA_DATA_DIR/critical_events
// (best-effort JSONL). Nil journal path is still valid via empty NewCriticalJournal.
func provideV2EventBus() *event.V2Bus {
	journal := event.NewCriticalJournal(event.DefaultCriticalJournalDir())
	return event.NewV2BusWithJournal(journal)
}

// provideV2RepoSet composes v2 repo interfaces into a single RepoSet
// via v2.NewRepoSetAdapter.
func provideV2RepoSet(
	task biz.TaskV2Repo,
	turn biz.TurnV2Repo,
	step biz.StepV2Repo,
	teamStage biz.TeamStageV2Repo,
	teamRun biz.TeamRunV2Repo,
	memberSession biz.MemberSessionV2Repo,
	planBoard biz.PlanBoardV2Repo,
	planStep biz.PlanStepV2Repo,
	graphStage biz.GraphStageV2Repo,
	graphNode biz.GraphNodeV2Repo,
) v2.RepoSet {
	return v2.NewRepoSetAdapter(task, turn, step, teamStage, teamRun, memberSession, planBoard, planStep, graphStage, graphNode)
}

// provideV2Sequencer constructs the v2 Sequencer.
// Phase 2: injects WithActivityUpserter so ActivityBridgeEvent payloads
// (team package direct-publish) are persisted via the v1 activities table.
func provideV2Sequencer(rs v2.RepoSet, bus *event.V2Bus, au biz.ActivityUpserter, lg loggateway.Logger) *v2.Sequencer {
	return v2.NewSequencer(rs, bus, lg, v2.WithActivityUpserter(au))
}

// provideV2ProjectorFactory constructs the v2 ProjectorFactory that produces
// per-turn ActivityProjector instances. Each turn (spirit + each team member)
// gets its own Projector instance, isolating per-turn streaming state.
// The factory shares the singleton Sequencer + SeqAssigner so Seq allocation
// remains globally monotonic per spirit session.
func provideV2ProjectorFactory(seq *v2.Sequencer, lg loggateway.Logger) *v2.ProjectorFactory {
	return v2.NewProjectorFactory(seq, seq.SeqAssigner(), lg)
}

// provideWSV2Subscriber constructs the WS subscriber for v2 Events.
// Subscribe is called synchronously in the constructor to avoid missing
// events between construction and goroutine startup.
func provideWSV2Subscriber(bus *event.V2Bus, wsSrv *server.WSServer, lg loggateway.Logger) *server.WSV2Subscriber {
	return server.NewWSV2Subscriber(bus, wsSrv, lg)
}

// providePlanExecutor constructs the v2 forward DAG scheduler.
// The PlanExecutor is injected into TeamStarter via SetPlanExecutor
// (called in ProvideChatService).
func providePlanExecutor(
	planStep biz.PlanStepV2Repo,
	teamStage biz.TeamStageV2Repo,
	planBoard biz.PlanBoardV2Repo,
	graphStage biz.GraphStageV2Repo,
	graphNode biz.GraphNodeV2Repo,
	orch service.TeamOrchestrator,
	seq *v2.Sequencer,
	lg loggateway.Logger,
) *service.PlanExecutor {
	return service.NewPlanExecutorFromV2Repos(planStep, teamStage, planBoard, graphStage, graphNode, orch, seq, lg)
}

// provideTeamOrchestrator returns the real TeamOrchestrator (Phase 2).
// assembler 和 starter 通过 ProvideChatService 后注入（打破 Wire 循环）。
func provideTeamOrchestrator(lg loggateway.Logger) *service.RealTeamOrchestrator {
	return service.NewRealTeamOrchestrator(lg)
}

// provideTeamOrchestratorStub returns the Phase 1 no-op TeamOrchestrator.
func provideTeamOrchestratorStub() service.TeamOrchestrator {
	return service.NewStubTeamOrchestrator()
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

func provideTaskPlanner(repo biz.TaskPlanRepository, catalog *biz.LlmProviderModelUsecase, orchCache *biz.OrchestrationCache, activityBus biz.ActivityEventBus, lg loggateway.Logger, sysUC *biz.SystemSettingUsecase, seq *v2.Sequencer) biz.TaskPlannerPort {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	return chatagent.NewTaskPlanner(repo, catalog, httpClient, activityBus, orchCache, lg, sysUC, seq)
}

func provideAgentAllocator(
	repo biz.AllocationPlanRepository,
	agentReader biz.AgentReader,
	perfRepo biz.AgentPerformanceRepository,
	catalog *biz.LlmProviderModelUsecase,
	eventBus biz.EventBus,
	embedder knowledge.Embedder,
	agentFactory biz.AgentFactory,
	lg loggateway.Logger,
	sysUC *biz.SystemSettingUsecase,
) biz.AgentAllocatorPort {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	capBuilder := chatagent.NewAgentCapabilityBuilder(agentReader, lg)
	return chatagent.NewAgentAllocator(repo, agentReader, perfRepo, capBuilder, catalog, httpClient, eventBus, lg, embedder, agentFactory, sysUC)
}

// provideAgentFactory constructs the AgentFactory (P1-4). The LLM model is
// resolved from the planner_model system setting (specify mode) or the first
// enabled catalog model (inherit/fallback). At init time there is no session
// context, so inherit mode falls through to the catalog fallback. When no
// model is available, llm is nil and EnsureAgent returns an Internal error so
// callers can fall back to other strategies.
func provideAgentFactory(
	agentReader biz.AgentReader,
	agentWriter biz.AgentWriter,
	templateRepo biz.AgentTemplateRepo,
	eventBus biz.EventBus,
	catalog *biz.LlmProviderModelUsecase,
	sysUC *biz.SystemSettingUsecase,
	lg loggateway.Logger,
) biz.AgentFactory {
	rt := &provider.RoundTrip{HTTP: &http.Client{Timeout: 60 * time.Second}}

	// Resolve planner model via system setting. At init time the session
	// model is unavailable (empty strings), so inherit mode falls through
	// to the catalog fallback.
	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if sysUC != nil {
		if s, err := sysUC.GetPlannerModel(context.Background()); err == nil {
			setting = s
		}
	}
	prov, mod := chatagent.ResolvePlannerModel(context.Background(), setting, "", "", catalog, lg, "agent_factory.wire", "AgentFactory")

	var llm trpcmodel.Model
	if prov != "" && mod != "" {
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
	return chatagent.NewAgentFactoryImpl(llm, agentWriter, agentReader, templateRepo, eventBus, lg)
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
	eventBus biz.EventBus,
	nl2graph graph.NL2GraphConverter,
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
	return chatagent.NewTaskOrchestratorImpl(spiritUC, assembler, assembler, compiler, repo, matcher, deps, synthesis, checkpointSaver, orchCache, perfRepo, evolutionSugg, eventBus, nl2graph, lg)
}

func provideDeptLeadManager(
	orgRepo biz.OrganizationRepo,
	borrowRepo biz.BorrowRequestRepo,
	agentRepo biz.AgentRepository,
	agentUC *biz.AgentUsecase,
	teamGetter biz.DeptLeadTeamGetter,
	eventBus biz.EventBus,
	lg loggateway.Logger,
) *biz.DeptLeadManager {
	return biz.NewDeptLeadManager(biz.DeptLeadManagerOpts{
		OrgRepo:    orgRepo,
		BorrowRepo: borrowRepo,
		AgentRepo:  agentRepo,
		AgentUC:    agentUC,
		TeamGetter: teamGetter,
		EventBus:   eventBus,
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
		activityevent.New,
		wire.Bind(new(biz.ActivityEventBus), new(*activityevent.Bus)),
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
		provideGlobalBuildCache,
		provideLifecycleManager,
		provideDeadLetterQueue,
		provideRunHeartbeatEmitter,
		providePendingMessageQueue,
		provideCodeExecutorFactory,
		provideAutoMemoryQueue,
		wire.Bind(new(memtrpc.AutoMemoryQueue), new(*memtrpc.MemoryJobQueue)),
		wire.Bind(new(biz.MemoryDeadLetterAdminRepo), new(*data.MemoryJobDeadLetterRepo)),
		wire.Bind(new(biz.MemoryDeadLetterSink), new(*data.MemoryJobDeadLetterRepo)),
		provideMemoryPolicyEngine,
		provideFactIndexSync,
		provideMemoryL2Recall,
		provideMemoryL3Recall,
		provideMemoryCompositeRecall,
		provideMemoryTRPCService,
		provideLinkEvolutionService,
		provideContextCompressor,
		provideFeedbackMemoryEnqueuer,
		provideMCPProber,
		provideMCPMetadataEditor,
		provideMCPServerUsecaseWithDeps,
		provideLLMInspector,
		provideCredentialCrypto,
		provideLlmProviderModelUsecaseWithDeps,
		provideModelStatsInjector,
		provideWebResearchReadinessChecker,
		provideBizWebResearchReadinessChecker,
		provideAgentUsecaseWithDeps,
		provideToolTester,
		provideParallelToolExecutor,
		provideToolUsecaseWithDeps,
		provideChatServiceDeps,
		provideRuntimeTooling,
		provideTeamOrchestrationDeps,
		provideRunnerConfig,
		// v2 event pipeline providers (Phase 1 收尾)
		provideV2EventBus,
		provideV2RepoSet,
		// v2 reader bindings: SessionV2Service consumes the narrow Reader
		// interfaces; the data layer provides the composite Repo interfaces.
		wire.Bind(new(biz.TaskV2Reader), new(biz.TaskV2Repo)),
		wire.Bind(new(biz.TurnV2Reader), new(biz.TurnV2Repo)),
		wire.Bind(new(biz.StepV2Reader), new(biz.StepV2Repo)),
		wire.Bind(new(biz.TeamStageV2Reader), new(biz.TeamStageV2Repo)),
		wire.Bind(new(biz.TeamRunV2Reader), new(biz.TeamRunV2Repo)),
		wire.Bind(new(biz.MemberSessionV2Reader), new(biz.MemberSessionV2Repo)),
		wire.Bind(new(biz.PlanBoardV2Reader), new(biz.PlanBoardV2Repo)),
		wire.Bind(new(biz.PlanStepV2Reader), new(biz.PlanStepV2Repo)),
		wire.Bind(new(biz.GraphStageV2Reader), new(biz.GraphStageV2Repo)),
		wire.Bind(new(biz.GraphNodeV2Reader), new(biz.GraphNodeV2Repo)),
		// Phase 3b-D: bind v2 EventBus interface to *event.V2Bus implementation
		// so Wire can inject biz.EventBus into consumers migrated from v1 ActivityEventBus.
		wire.Bind(new(biz.EventBus), new(*event.V2Bus)),
		// Phase 2: bind v2 EventPublisher interface to *v2.Sequencer so Wire
		// can inject the publish-only Sequencer entry into team package consumers
		// (Runner.Pipeline.Sequencer, TeamGraphRunCoordinator.seq).
		wire.Bind(new(rt.EventPublisher), new(*v2.Sequencer)),
		provideV2Sequencer,
		provideV2ProjectorFactory,
		provideWSV2Subscriber,
		provideTeamOrchestrator,
		wire.Bind(new(service.TeamOrchestrator), new(*service.RealTeamOrchestrator)),
		providePlanExecutor,
		provideTeamTurnDeps,
		provideChannelTurnJobDeps,
		provideChannelNotifierDeps,
		provideRunCanceller,
		provideChatSender,
		provideArtifactRuntimeService,
		provideArtifactSigner,
		provideMemoryService,
		provideTRPCSessionService,
		provideGraphCheckpointSaver,
		wire.Bind(new(trpcgraph.CheckpointSaver), new(*graphtrpc.CheckpointSaver)),
		// P1 fix (2026-06-18): Wire previously-orphan graph components into production.
		provideNL2GraphConverter,
		provideRuntimeReplanner,
		provideTopologyEvolver,
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
		provideMemoryL2DecayWorker,
		provideMemoryAdminUsecase,
		providePathBExtractor,
		providePathBL4Writer,
		provideSessionAdminStore,
		provideMemoryAdminDeps,
		provideMemoryL1ArchiveWorker,
		provideChannelTurnJobSweeper,
		provideMemoryL3DecayWorker,
		provideMemoryL4DecayWorker,
		provideMemoryEbbinghausDecayWorker,
		provideMemorySleepTimeWorker,
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
		provideRecoveryWorker,
		provideBackgroundJobRegistry,
		provideBackgroundJobWorker,
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
		provideTaskPlanner,
		provideAgentAllocator,
		provideAgentFactory,
		chatagent.NewAgentMatcher,
		chatagent.NewProfileResolver,
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
		// Phase 1c-3: CompressReadDeps/CompressWriteDeps now provided by
		// ProvideCompressReadDepsAdapter / ProvideCompressWriteDepsAdapter
		// (in internal/session) because SessionRepo no longer implements
		// MessageReader/MessageWriter (messages table removed).
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
		// biz.ModelStatsReader 是 usage.AnalyticsRepo 的子集（仅 ListTopModelUsageFromDaily）。
		// 通过 AnalyticsRepo 中转绑定，复用 biz.UsageRepo 实现，避免新增 data 层 Repo。
		wire.Bind(new(biz.ModelStatsReader), new(bizusage.AnalyticsRepo)),
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
