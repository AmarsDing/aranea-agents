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
	bizcu "aranea-agents/internal/biz/computeruse"
	bizcg "aranea-agents/internal/biz/configgraph"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/biz/diagnostics"
	"aranea-agents/internal/biz/evaluation"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	bizmedia "aranea-agents/internal/biz/media"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/monitor/heal"
	bizsession "aranea-agents/internal/biz/session"
	bizskill "aranea-agents/internal/biz/skill"
	biztool "aranea-agents/internal/biz/tool"
	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/data"
	datacg "aranea-agents/internal/data/configgraph"
	speech "aranea-agents/internal/data/speech"
	"aranea-agents/internal/debug"
	"aranea-agents/internal/event"
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
	"aranea-agents/internal/sandbox"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	araneasession "aranea-agents/internal/session"
	sessiontrpc "aranea-agents/internal/session/trpc"
	"aranea-agents/internal/skill"
	skillevolution "aranea-agents/internal/skill/evolution"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/internal/tools/codingbridge"
	kanbanpkg "aranea-agents/internal/tools/kanban"
	subagenttool "aranea-agents/internal/tools/subagent"
	"aranea-agents/internal/voice"
	"aranea-agents/pkg/appctx"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcevolution "trpc.group/trpc-go/trpc-agent-go/evolution"
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
	monitorUC *biz.MonitorUsecase,
	memWorker *biz.TurnMemoryWorker,
	traceProj *monitor.TraceProjector,
	fileAppender *monitor.FlowFileAppender,
	usage *biz.UsageUsecase,
	logger biz.SessionLogWriter,
	flowLogWriter biz.FlowLogWriter,
) *biz.EventBusSideConsumers {
	var monitorEventBus contract.MonitorBus
	if infra != nil {
		monitorEventBus = infra.MonitorEventBus
	}
	return biz.NewEventBusSideConsumers(eventBus, monitorEventBus, tools, webhooks, sessions, flowLogs, monitorUC, memWorker, traceProj, fileAppender, usage, logger, flowLogWriter)
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

func provideSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, usageRef *biz.UsageUsecaseRef, _ rt.PersistenceSet, lg loggateway.Logger) biz.SessionTitleGenerator {
	if catalog == nil {
		return biz.NewNoopSessionTitleGenerator()
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	return service.NewLLMSessionTitleGenerator(catalog, &provider.RoundTrip{HTTP: httpClient}, usageRef, lg)
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
// by BatchExecuteSpiritTools. IsolationStrategyWorktree on a ToolCall still
// uses a git worktree when the env workspace is a git repo (raw handlers).
// Assembled LLM tools must dispatch via BatchExecuteAssembledTools so they
// do not inherit this isolator. Returns nil when ARANEA_PARALLEL_AUTO is
// disabled so callers fall back to serial execution.
func provideParallelToolExecutor(lg loggateway.Logger) *tools.ParallelToolExecutor {
	if !intent.AllowAutoParallel() {
		return nil
	}
	var opts []tools.ExecutorOption
	if root := tools.LookupGitRoot(tools.WorkspaceRootFromEnv()); root != "" {
		iso, err := tools.NewWorktreeIsolator(root, nil, lg)
		if err != nil {
			lg.Warn("parallel tool worktree isolator skipped",
				loggateway.StepID("tool.parallel.worktree"),
				loggateway.Err(err))
		} else {
			opts = append(opts, tools.WithWorktreeIsolator(iso))
		}
	}
	return tools.NewParallelToolExecutor(nil, lg, opts...)
}

func provideToolUsecaseWithDeps(repo biztool.ToolRepo, sys biztool.SettingRepo, tester biztool.ToolTester, checker biztool.WebResearchReadinessChecker, grants biztool.ToolGrantStore, paramRules biztool.ToolParamRuleStore, lg loggateway.Logger) *biztool.ToolUsecase {
	return biztool.NewToolUsecase(repo, sys, lg, biztool.WithToolTester(tester), biztool.WithWebResearchChecker(checker), biztool.WithToolGrantStore(grants), biztool.WithToolParamRuleStore(paramRules))
}

// provideMCPServerUsecaseWithDeps injects prober and metadata editor via constructor.
// P2: the real-handshake tool discoverer is wired here too (setter keeps the
// wire graph unchanged).
func provideMCPServerUsecaseWithDeps(repo biz.MCPServerRepo, credRepo biz.MCPServerUserCredentialRepo, prober biz.MCPProber, metaEdit biz.MCPMetadataEditor, crypto *biz.CredentialCrypto) *biz.MCPServerUsecase {
	uc := biz.NewMCPServerUsecase(repo, credRepo, prober, metaEdit, crypto)
	uc.SetToolDiscoverer(mcpToolDiscovererAdapter{})
	return uc
}

func provideRunRegistry(lg loggateway.Logger) *rt.RunRegistry {
	return rt.NewRunRegistry().WithLogger(lg)
}

// provideGlobalBuildCache exposes the process-level agent BuildCache singleton
// so it can be registered with the LifecycleManager for orderly shutdown (A3).
func provideGlobalBuildCache() *agent.BuildCache {
	return agent.GetGlobalBuildCache()
}

// provideAgentPolicyResolver initializes the process-level tool-policy resolver
// singleton (P1-2): injects the settings repo and performs the first snapshot
// load. Runtime timeout policy changes then take effect without agent rebuilds.
func provideAgentPolicyResolver(repo biz.AgentRepository, lg loggateway.Logger) *agent.PolicyResolver {
	return agent.InitPolicyResolver(repo, lg)
}

// provideMCPToolSetPool exposes the process-level MCP ToolSet pool singleton
// so it can be registered with the LifecycleManager for orderly shutdown.
func provideMCPToolSetPool() *tools.MCPToolSetPool {
	return tools.GetGlobalMCPToolSetPool()
}

// provideGlobalShardCache exposes the process-level shard build cache
// singleton (P0-2 阶段A) so it can be registered with the LifecycleManager
// for orderly shutdown.
func provideGlobalShardCache() agent.ShardCache {
	return agent.GetGlobalShardCache()
}

// provideLifecycleManager builds the process-level LifecycleManager and
// registers the global build cache for LIFO shutdown (A3). Additional
// process-level resources can be registered here as they are migrated to
// the lifecycle abstraction. The policyResolver parameter is an init-time
// dependency only (P1-2): the resolver is a pure in-memory snapshot with no
// shutdown work, but it must be constructed (and first-loaded) at startup.
func provideLifecycleManager(cache *agent.BuildCache, mcpPool *tools.MCPToolSetPool, shardCache agent.ShardCache, policyResolver *agent.PolicyResolver, monitorBus contract.MonitorBus, evolutionSvc trpcevolution.Service, lg loggateway.Logger) *lifecycle.LifecycleManager {
	cache.SetLogger(lg)
	cache.SetMonitorBus(monitorBus)
	mcpPool.SetLogger(lg)
	shardCache.SetLogger(lg)
	_ = policyResolver // init-time dependency: constructed + first-loaded at startup
	mgr := lifecycle.NewLifecycleManager(lg)
	mgr.Register("global-build-cache", cache)
	mgr.Register("mcp-toolset-pool", mcpPool)
	// P0-2 阶段A：shard cache 最后注册、LIFO 最先关闭。被引用分片仅标记
	// closing，随后 build-cache 关闭经 graveyard 释放分片引用占位符，
	// 分片在最后一次 release 时完成关闭（其内 MCP toolset 池引用随之释放）。
	mgr.Register("global-shard-cache", shardCache)
	// 框架 v1.11 技能演化 worker：进程退出时优雅停止（nil 时 Register 跳过）。
	mgr.Register("skill-evolution-service", evolutionSvc)
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
// file snapshot + Postgres store. The snapshot directory is resolved from (in
// order): PENDING_QUEUE_SNAPSHOT_DIR env var, the loggateway output dir, or
// empty (disables the JSON file). The DB store is always attached when Data
// is available; startup prefers rows in pending_queue_entries over the file.
func providePendingMessageQueue(lg loggateway.Logger, d *data.Data) *rt.PendingMessageQueue {
	dir := strings.TrimSpace(os.Getenv("PENDING_QUEUE_SNAPSHOT_DIR"))
	if dir == "" {
		if gw, ok := lg.(*loggateway.Gateway); ok {
			dir = gw.OutputDir()
		}
	}
	q := rt.NewPendingMessageQueueWithDirAndLogger(dir, lg)
	if store := newDataPendingQueueStore(d); store != nil {
		q.SetStore(store)
	}
	return q
}

func provideCodeExecutorFactory(lg loggateway.Logger, sandboxMgr *sandbox.Manager, sandboxLeases *sandbox.SessionLeases) *localexec.Factory {
	f := localexec.NewFactoryWithLogger(lg)
	f.SetSandboxManager(sandboxMgr, sandboxLeases)
	return f
}

// provideSandboxSessionLeases builds the process-wide shared session-lease
// store (M82 P1-1/P1-2). The SAME instance is bound to the codeexecutor
// sandbox backend and carried into every agent build via ToolBridges.SandboxFS,
// so execute_code and sandbox_fs_write/read share one sandbox per session.
func provideSandboxSessionLeases(sandboxMgr *sandbox.Manager) *sandbox.SessionLeases {
	return sandbox.NewSessionLeases(sandboxMgr)
}

// provideSandboxManager builds the M82 sandbox Manager (P0-8 启动探测降级).
// When the subsystem is disabled or no docker daemon is reachable the engine
// is nil: the Manager stays constructible (wire graph intact) and Available()
// reports false so consumers fall back along sandbox→docker→local (NFR-04).
// The engine is bound ONCE here at startup: although the daemon probe has a
// 30s TTL cache, a daemon started later does NOT get picked up — a process
// restart is required (r2 #7: corrected from the earlier comment that
// claimed cold-create attempts would see it without a restart).
func provideSandboxManager(sbConf *conf.Sandbox, lg loggateway.Logger) *sandbox.Manager {
	cfg := sandbox.ConfigFromProto(sbConf)
	var engine sandbox.Engine
	if cfg.Enabled {
		if sandbox.DockerDaemonAvailable() {
			engine = sandbox.NewDockerEngine()
		} else {
			lg.Warn("sandbox enabled but docker daemon unavailable — pooled backend degraded; code execution falls back to docker/local",
				loggateway.StepID("sandbox.probe"))
		}
	}
	return sandbox.NewManager(cfg, engine, lg)
}

func provideChannelRunEscalationNotifier(channels *biz.ChannelUsecase, sessions *biz.SessionUsecase, lg loggateway.Logger) service.SessionRunEscalationNotifier {
	return service.NewChannelRunEscalationNotifier(channels, sessions, lg)
}

// provideDecisionCollector builds the M80 decision collector (Phase 1).
// repo is nil only when the data layer is absent (CLI mode); Emit stays
// non-blocking and worker flushes degrade to no-ops. newApp owns Start/Stop.
func provideDecisionCollector(repo decision.Repo, lg loggateway.Logger) decision.Lifecycle {
	return decision.NewOutboxCollector(repo, lg)
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

func provideMonitorUsecase(audit biz.MonitorAuditRepo, event biz.MonitorEventRepo, trace biz.MonitorTraceRepo, alert biz.MonitorAlertRepo, runner biz.MonitorRunnerCompletionRepo, notifier biz.AlertNotifier, fsHealth biz.FilesystemHealthReader, spanReader biz.MonitorTraceSpanReader, seq *v2.Sequencer, canary *biz.MemoryCanaryStatus, reg *monitor.AlertMetricRegistry, usageRepo biz.UsageRepo, traceProj *monitor.TraceProjector, fileAppender *monitor.FlowFileAppender, lg loggateway.Logger) *biz.MonitorUsecase {
	rb := monitor.NewMetricRingBuffer()
	uc := biz.NewMonitorUsecase(audit, event, trace, alert, runner, notifier,
		biz.WithTraceSpanReader(spanReader),
		monitor.WithLogger(lg),
		monitor.WithRegistry(reg),
		monitor.WithTraceProjector(traceProj),
		monitor.WithFlowFileAppender(fileAppender),
	)
	w := monitor.NewAlertEvalWorker(uc, rb, lg)
	uc.SetEvalWorker(w)
	// reg is the wire-shared singleton (monitor.WireProviderSet): the
	// SelfCheckScheduler registers self_check.unhealthy_count on the same
	// instance, so all alert metrics live in one registry.
	reg.Register(monitor.NewRunnerErrorRateMetric(event, rb))
	if fsHealth != nil {
		reg.Register(monitor.NewSkillFilesystemMissingMetric(fsHealth))
	}
	if seq != nil {
		// P0-R2a: expose sequencer dead-letter backlog to the alert engine.
		reg.Register(monitor.NewSequencerDeadLetterMetric(seq))
	}
	if canary != nil {
		// P0 canary: expose memory closed-loop failure streak to the alert engine.
		reg.Register(monitor.NewMemoryCanaryMetric(canary))
	}
	// 29-token §9.4: low prompt-cache hit ratio (prefix bust) detection.
	// Narrowed via type assertion to keep the composite usage.Repo untouched.
	if ch, ok := usageRepo.(bizusage.CacheHitRatioStatsRepo); ok && ch != nil {
		reg.Register(monitor.NewCacheHitRatioLowMetric(ch))
	}
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

// provideUsageUsecaseRef builds the late-binding cell for *biz.UsageUsecase
// (P1-2, 2026-08-19). It has zero dependencies so aux-usage recorders that
// sit upstream of UsageUsecase in the DI graph (session-title generator,
// subagent service, evolution review model) can resolve the usecase lazily
// at record time without creating wire cycles.
func provideUsageUsecaseRef() *biz.UsageUsecaseRef {
	return biz.NewUsageUsecaseRef()
}

func provideUsageUsecase(repo biz.UsageRepo, mon *biz.MonitorUsecase, teamUC *biz.TeamUsecase, sessions *biz.SessionUsecase, eventBus biz.EventBus, usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) *biz.UsageUsecase {
	uc := biz.NewUsageUsecase(repo, lg)
	uc.SetAlertNotifier(service.NewMonitorBudgetAlertNotifier(mon))
	uc.SetTeamReader(teamUC)
	uc.SetSessionMetricsAccumulator(&sessionMetricsAdapter{sessions: sessions})
	uc.SetCompletionUsageLinker(&completionLinkerAdapter{mon: mon})
	uc.SetUsageEnvelopePublisher(&envelopePublisherAdapter{eventBus: eventBus})
	// Publish to the late-binding cell so upstream aux recorders can resolve.
	usageRef.Set(uc)
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

// agentToolResultPruneConfig 翻译 runtime 配置为 agent 包消费侧配置
// （79-runtime-governance R2；agent 包不依赖 internal/conf）。
// runtimeConf 为 nil 时 ToolResultPruneConfig() 返回默认开阈值配置（nil-safe）。
func agentToolResultPruneConfig(runtimeConf *conf.Runtime) chatagent.ToolResultPruneConfig {
	c := runtimeConf.ToolResultPruneConfig()
	return chatagent.ToolResultPruneConfig{
		Enabled:     c.Enabled,
		AfterTurns:  c.AfterTurns,
		SizeBytes:   c.SizeBytes,
		ExemptTools: c.ExemptTools,
	}
}

// teamNoProgressAuditorConfig 翻译 runtime 配置为 team 包消费侧配置
// （79-runtime-governance R5；team 包不依赖 internal/conf）。
// runtimeConf 为 nil 时 NoProgressAuditorConfig() 返回默认开阈值配置（nil-safe）。
func teamNoProgressAuditorConfig(runtimeConf *conf.Runtime) team.NoProgressAuditorConfig {
	c := runtimeConf.NoProgressAuditorConfig()
	return team.NoProgressAuditorConfig{
		Enabled:      c.Enabled,
		CorrectAfter: c.CorrectAfter,
		CancelAfter:  c.CancelAfter,
	}
}

func provideRuntimeTooling(
	pluginRT *plugintrpc.Runtime,
	pluginMgr *plugintrpc.Manager,
	skillDBRepo trpcskill.Repository,
	skillHealth *service.SkillHealthMetricsAdapter,
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
	resourceAccess *biz.ResourceAccessUsecase,
	deptMailbox *biz.DeptMailboxUsecase,
	sessionSearch *biz.SessionSearchUsecase,
	clientBridge *clientbridge.Bridge,
	computerUseUC *bizcu.ComputerUseUsecase,
	codingBridgeSvc codingbridge.BridgeService,
	sandboxLeases *sandbox.SessionLeases,
	runtimeConf *conf.Runtime,
) service.RuntimeTooling {
	return service.RuntimeTooling{
		Knowledge: service.KnowledgeTools{
			Retriever:          knowledgeRetriever,
			Router:             knowledgeRouter,
			FederatedRetriever: knowledgeFederatedRetriever,
			Evaluator:          knowledgeEvaluator,
			Usecase:            knowledgeUC,
		},
		Skill: service.SkillRuntime{
			DBRepo:          skillDBRepo,
			Health:          skillHealth,
			CodeExecFactory: codeExecFactory,
		},
		Plugin: service.PluginRuntime{
			RT:      pluginRT,
			Manager: pluginMgr,
		},
		Bridges: service.ToolBridges{
			Kanban:      kanbanBridge,
			ComputerUse: computerUseUC,
			Coding:      codingBridgeSvc,
			Client:      clientBridge,
			SandboxFS:   sandboxLeases,
		},
		Sharing: service.WorkspaceSharing{
			ResourceAccess: resourceAccess,
			DeptMailbox:    deptMailbox,
			SessionSearch:  sessionSearch,
		},
		Extensions: service.TurnExtensions{
			Organization:         orgUC,
			ToolResultGate:       toolResultGate,
			OutboundRouter:       outboundRouter,
			SubAgentService:      subAgentSvc,
			DebugRecorder:        debugRecorder,
			ParallelToolExecutor: parallelExec,
			ToolResultPrune:      agentToolResultPruneConfig(runtimeConf),
		},
	}
}

// ---------------------------------------------------------------------------
// M71: agent resource sharing providers
// ---------------------------------------------------------------------------

func provideResourceAccessUsecase(
	agents biz.AgentReader,
	org biz.OrganizationReader,
	teamLister biz.DeptTeamLister,
	fileReader biz.MemberFileReader,
	dirResolver biz.MemberDirResolver,
	auditor biz.AccessAuditor,
	lg loggateway.Logger,
) *biz.ResourceAccessUsecase {
	return biz.NewResourceAccessUsecase(biz.ResourceAccessUsecaseDeps{
		Agents:      agents,
		Org:         org,
		TeamLister:  teamLister,
		FileReader:  fileReader,
		DirResolver: dirResolver,
		Auditor:     auditor,
		Lg:          lg,
	})
}

func provideDeptMailboxUsecase(
	repo biz.DeptLeadMailboxRepo,
	agents biz.AgentReader,
	org biz.OrganizationReader,
	auditor biz.AccessAuditor,
	waker biz.MailboxWaker,
	lg loggateway.Logger,
) *biz.DeptMailboxUsecase {
	return biz.NewDeptMailboxUsecase(biz.DeptMailboxUsecaseDeps{
		Repo:    repo,
		Agents:  agents,
		Org:     org,
		Auditor: auditor,
		Waker:   waker,
		Lg:      lg,
	})
}

func provideSessionSearchUsecase(
	agents biz.AgentReader,
	sessions biz.SessionReader,
	messages biz.MessageReader,
	searcher biz.GlobalMessageSearcher,
	auditor biz.AccessAuditor,
	lg loggateway.Logger,
) *biz.SessionSearchUsecase {
	return biz.NewSessionSearchUsecase(biz.SessionSearchUsecaseDeps{
		Agents:   agents,
		Sessions: sessions,
		Messages: messages,
		Searcher: searcher,
		Auditor:  auditor,
		Lg:       lg,
	})
}

// provideM71MessageReader exposes the steps_v2-backed MessageReader (same
// adapter used by SessionUsecase) for the sessionaccess usecase.
func provideM71MessageReader(lister bizsession.ActivityLister) biz.MessageReader {
	return bizsession.NewActivityMessageReader(lister)
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
	orgUC *biz.OrganizationUsecase,
	toolResultGate *biz.ToolResultGate,
	outboundRouter *outbound.Router,
	subAgentSvc *subagenttool.Service,
	kanbanBridge kanbanpkg.Bridge,
	computerUseUC *bizcu.ComputerUseUsecase,
	sandboxLeases *sandbox.SessionLeases,
	sandboxMgr *sandbox.Manager,
	a2aUC *biz.A2AUsecase,
	sessions *biz.SessionUsecase,
	skillUC *biz.SkillUsecase,
	agentsUC *biz.AgentUsecase,
	sys biz.SystemSettingRepo,
	v2ProjectorFactory *v2.ProjectorFactory,
	teamUC *biz.TeamUsecase,
	runtimeReplanner graph.RuntimeReplanner,
	decisions decision.Lifecycle,
	runtimeConf *conf.Runtime,
	lg loggateway.Logger,
) team.RunnerConfig {
	cfg := team.RunnerConfig{
		PluginRT:      pluginRT,
		PluginManager: pluginMgr,
		// G2（ADR-F D2）：team 图执行的智能重规划兜底——与 graph run 域共享
		// 同一 RuntimeReplanner 实例（per-execution 计数器隔离）。
		Replanner: runtimeReplanner,
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
		ToolResultPrune: agentToolResultPruneConfig(runtimeConf),
		NoProgressAudit: teamNoProgressAuditorConfig(runtimeConf),
		// M80：token_budget / no_progress 系统闸决策双写（设计 §3.2 row 3）。
		DecisionCollector: decisions,
		OutboundRouter:    outboundRouter,
		SubAgentService: subAgentSvc,
		KanbanBridge:    kanbanBridge,
		ComputerUseUC:   computerUseUC,
		SandboxFSStore:  sandboxLeases,
		SandboxManager:  sandboxMgr,
		A2AEnabled:      a2aUC != nil,
		// SessionChildLookup resolves member agent session IDs for child_session_id
		// in session activities. Uses SessionUsecase.ListChildSessions to look up
		// child sessions by parent (team) session ID and match by MemberAgentKey.
		SessionChildLookup: &sessionChildLookupAdapter{sessions: sessions},
		// MemberCustomTools injects cli_admin_* tools for __system_admin__ members
		// inside teams — the same tool surface as the direct chat path.
		MemberCustomTools: service.NewCLIAdminToolFactory(skillUC, agentsUC, sys),
		// B10 运行时惰性兜底：加载 team 时确保图资产已物化（存量 team
		// linked_graph_id 为空 → 先物化再编译运行）。
		GraphEnsurer: teamUC,
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
	mediaProviders bizmedia.ProviderReader,
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
		MediaProviders:  mediaProviders,
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
	mediaProviders bizmedia.ProviderReader,
	persist rt.PersistenceSet,
	compress biz.NativeTurnCompressor,
	eventBus biz.EventBus,
	monitorEventBus contract.MonitorBus,
	seq *v2.Sequencer,
	learningLoop *biz.LearningLoopUsecase,
	lg loggateway.Logger,
) rt.TurnDeps {
	// LLMHTTP timeout is sourced from TimeoutPolicy.
	// TaskTypeModerate (60min) is the baseline; per-task-type overrides
	// can be applied in the LLM call path via context (see trpc_llm.go).
	timeoutPolicy := provider.NewTimeoutPolicy()
	return rt.TurnDeps{
		ReadDeps:     provideTurnReadDeps(agents, agentsUC, toolRegistry, toolUC, llmCatalog, skillUC, sys, mediaProviders),
		Persist:      persist,
		Pipeline:     rt.EventPipeline{EventBus: eventBus, MonitorEventBus: monitorEventBus, Sequencer: seq},
		LLMHTTP:      &http.Client{Timeout: timeoutPolicy.TimeoutFor(provider.TaskTypeModerate)},
		Sessions:     sessions,
		Compress:     compress,
		RunnerMgr:    rt.NewRunnerManagerFromPersist(persist, lg),
		LearningLoop: learningLoop,
		MsgHistory:   sessions,
		Lg:           lg,
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
	mediaProviders bizmedia.ProviderReader,
	persist rt.PersistenceSet,
	sessionRT *araneasession.Runtime,
	compress biz.NativeTurnCompressor,
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
	skillIntel *biz.SkillIntelligenceUsecase,
	evolution *biz.EvolutionUsecase,
	skillStats biz.SkillInvocationStatsReader,
	outboundRouter *outbound.Router,
	subAgentSvc *subagenttool.Service,
	expAnalytics *biz.ExperienceAnalyticsUsecase,
	turnLifecycle *biz.TurnLifecycleUsecase,
	stepReader biz.StepV2Reader,
	stepWriter biz.StepV2Writer,
	taskV2Repo biz.TaskV2Repo,
	heartbeatEmitter *service.RunHeartbeatEmitter,
	deadLetterQueue *lifecycle.DeadLetterQueue,
	profileResolver *chatagent.ProfileResolver,
	v2ProjectorFactory *v2.ProjectorFactory,
	memberSessions biz.MemberSessionV2Repo,
	memoryConsolidationWriter biz.MemoryConsolidationWriter,
	memoryFactIndexSyncer biz.MemoryFactIndexSyncer,
	skillEmbedder biz.SkillEmbedder,
	memoryConflictDetector biz.MemoryConflictDetector,
	memoryConflictStore biz.L3ConflictStore,
	learningLoop *biz.LearningLoopUsecase,
	voiceDelegation *voice.DelegationRegistry,
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
				ReadDeps:     provideTurnReadDeps(agents, agentsUC, toolRegistry, toolUC, llmCatalog, skillUC, sys, mediaProviders),
				Persist:      persist,
				Pipeline:     rt.EventPipeline{EventBus: eventBus, MonitorEventBus: monitorEventBus},
				LLMHTTP:      &http.Client{Timeout: timeoutPolicy.TimeoutFor(provider.TaskTypeModerate)},
				Sessions:     sessions,
				SessionRT:    sessionRT,
				Compress:     compress,
				AfterTurn:    biz.NoopNativeTurnAfter{},
				RunnerMgr:    rt.NewRunnerManagerFromPersist(persist, lg),
				LearningLoop: learningLoop,
				MsgHistory:   sessions,
				Lg:           lg,
			},
			Runs:         runs,
			PendingQueue: pendingQueue,
			RT:           rtDeps,
			TurnTimeout:  0,
			Admission:    biz.NewTurnAdmissionUsecase(biz.TurnAdmissionUsecaseConfig{Quota: usage, Agents: agents}),
			StepReader:   stepReader,
			StepWriter:   stepWriter,
			TaskV2:       taskV2Repo,
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
			SkillIntel: skillIntel,
			Evolution:  evolution,
		},
		Infra: service.ChatInfraDeps{
			LG:                        lg,
			OrchCache:                 orchCache,
			A2AUC:                     a2aUC,
			MCPServers:                mcpUC,
			OutboundRouter:            outboundRouter,
			SubAgentService:           subAgentSvc,
			TurnLifecycle:             turnLifecycle,
			HeartbeatEmitter:          heartbeatEmitter,
			DeadLetterQueue:           deadLetterQueue,
			ProfileResolver:           profileResolver,
			V2ProjectorFactory:        v2ProjectorFactory,
			MemberSessions:            memberSessions,
			MemoryConsolidationWriter: memoryConsolidationWriter,
			FactIndexSync:             memoryFactIndexSyncer,
			SkillEmbedder:             skillEmbedder,
			MemoryConflictDetector:    memoryConflictDetector,
			MemoryConflictStore:       memoryConflictStore,
			MemoryPreferenceLister:    persist.Memory.PreferenceLister,
			VoiceDelegation:           voiceDelegation,
		},
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
}

// provideTaskResumer binds ChatService as the WS resume_task handler (L3).
func provideTaskResumer(svc *service.ChatService) server.TaskResumer {
	return svc
}

// provideSkillCatalogPusher binds ChatService as the WS skill-catalog push
// hook (design 69 Phase 3).
func provideSkillCatalogPusher(svc *service.ChatService) server.SkillCatalogPusher {
	return svc
}

func provideMemoryService(persist rt.PersistenceSet, cascade *biz.L4CascadeUsecase, sysUC *biz.SystemSettingUsecase, deadLetterRepo biz.MemoryDeadLetterAdminRepo, queue memtrpc.AutoMemoryQueue, queueStats *memtrpc.MemoryJobQueue, workerStats *biz.MemoryWorkerStats, d *data.Data, agentUC *biz.AgentUsecase, lg loggateway.Logger) *service.MemoryService {
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
		Admin:               persist.Memory.AdminUsecase,
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
		AgentUC:             agentUC,
		FactPendingStore:    data.NewMemoryFactPendingRepoFromData(d),
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
	return sessiontrpc.NewTRPCSessionService(pgDSN, lg, sessiontrpc.SummarizerConfig{
		Catalog: catalog,
		RT:      &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}},
		Lg:      lg,
	})
}

// provideEvolutionService 装配框架 v1.11 技能演化 service（hold-all 模式，
// 详见 internal/skill/evolution 包注释）。模型目录/技能 repo 缺失时返回 nil，
// runner 侧自动跳过演化学习。
func provideEvolutionService(catalog *biz.LlmProviderModelUsecase, repo trpcskill.Repository, usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) trpcevolution.Service {
	svc := skillevolution.NewService(skillevolution.Config{
		Catalog:  catalog,
		RT:       &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}},
		Repo:     repo,
		UsageRef: usageRef,
		Lg:       lg,
	})
	if svc == nil {
		return nil
	}
	return svc
}

func provideSessionMemoryResync(admin biz.MemoryAdminDeps) araneasession.MemoryResync {
	if admin == nil {
		return nil
	}
	return admin
}

func provideL1AdminReader(admin biz.MemoryAdminDeps) biz.L1AdminReader {
	if admin == nil {
		return nil
	}
	return admin
}

func provideL1TaskBoardWriter(admin biz.MemoryAdminDeps) biz.L1TaskBoardWriter {
	if admin == nil {
		return nil
	}
	return admin
}

func provideGraphCheckpointSaver(d *data.Data, infra *event.Infra, lg loggateway.Logger) (*graphtrpc.CheckpointSaver, error) {
	rawDB := providePrimaryRawDB(d)
	pgDSN := d.PostgresDSN()
	var monitorBus contract.MonitorBus
	if infra != nil {
		monitorBus = infra.MonitorEventBus
	}
	return rt.NewGraphCheckpointSaver(rawDB, pgDSN, monitorBus, lg)
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
	nodeBreakers *biz.NodeCircuitBreakerRegistry,
	clientBridge *clientbridge.Bridge,
	runtimeConf *conf.Runtime,
	decisions decision.Lifecycle,
	lg loggateway.Logger,
) graphtrpc.GraphNodeResolverSet {
	if catalog == nil || toolUC == nil {
		return graphtrpc.GraphNodeResolverSet{NodeBreakers: nodeBreakers}
	}
	// T3 根修（同 provideTRPCBuilderDeps，两处必须一致）：ToolUC 非 nil ⇒
	// 产品确认门禁机制可用，confirmation_guard runner 插件让位。
	plugintrpc.SetProductConfirmGateActive(true)
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
			// M80：HITL 工具确认决策记录入口（验收3 踩坑——漏装则
			// emitToolConfirmDecisionRecord 遇 nil 静默跳过，hitl_approval 永不落库）。
			DecisionCollector: decisions,
		},
		TRPCMemoryKnowledgeDeps: chatagent.TRPCMemoryKnowledgeDeps{
			HasMemory:               persist.Memory.Available(),
			MemoryService:           persist.Memory.TRPC,
			MemoryLayerPorts:        persist.Memory.MemoryLayerPorts,
			MemoryActionLogWriter:   persist.Memory.ActionLogWriter,
			MemoryL2Recall:          persist.Memory.L2Recall,
			MemoryL3Recall:          persist.Memory.L3Recall,
			MemoryCompositeRecall:   persist.Memory.CompositeRecall,
			AgentCaseRecaller:       persist.Memory.AgentCaseRecaller,
			MemoryPreferenceLister:  persist.Memory.PreferenceLister,
			MemoryProfileCardReader: persist.Memory.ProfileCardReader,
			MemoryFactInjectCounter: persist.Memory.FactInjectCounter,
			KnowledgeRetriever:      knowledgeRetriever,
			KnowledgeUsecase:        knowledgeUC,
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
			ToolResultPrune: agentToolResultPruneConfig(runtimeConf),
			OutboundRouter:  outboundRouter,
			SubAgentService: subAgentSvc,
			A2AEnabled:      a2aUC != nil,
			ClientBridge:    clientBridge,
			// TeamCompletionChecker 将在运行时通过 SetTeamCompletionChecker 注入，避免循环依赖
			LG: lg,
		},
	}
	return graphtrpc.GraphNodeResolverSet{
		Models:       graphadapter.NewCatalogModelResolver(catalog, rtTrip, lg),
		Tools:        graphadapter.NewCatalogToolResolver(toolUC, lg),
		Agents:       graphadapter.NewCatalogAgentResolver(builderDeps, lg, service.NewCLIAdminToolFactory(skillUC, agentUC, sys)),
		Functions:    graphadapter.NewCatalogFunctionResolver(toolUC, lg),
		NodeBreakers: nodeBreakers,
	}
}

// provideTRPCBuilderDeps provides the TRPCBuilderDeps for dependency injection.
// This is a separate provider to allow runtime injection of TeamCompletionChecker.
func provideTRPCBuilderDeps(
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
	clientBridge *clientbridge.Bridge,
	runtimeConf *conf.Runtime,
	decisions decision.Lifecycle,
	lg loggateway.Logger,
) *chatagent.TRPCBuilderDeps {
	rtTrip := &provider.RoundTrip{HTTP: &http.Client{Timeout: 120 * time.Second}}
	// 79-runtime-governance（2026-08-27 三轮审查 T3 根修）：server 形态 ToolUC
	// 非 nil ⇒ 产品确认门禁机制可用，confirmation_guard runner 插件让位（框架
	// 执行序插件先于链，硬拦会遮蔽 param gate 的 deny/ask/allow 语义；插件的
	// ConfirmTools/Patterns 由产品门禁 decide() 插件分支以同一匹配器升级为
	// 交互确认）。CLI 无 DB 形态 ToolUC 为 nil，不置位，插件保留遗产硬拦。
	if toolUC != nil {
		plugintrpc.SetProductConfirmGateActive(true)
	}
	return &chatagent.TRPCBuilderDeps{
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
			// M80：HITL 工具确认决策记录入口（同 provideGraphBuildDeps，两处必须一致）。
			DecisionCollector: decisions,
		},
		TRPCMemoryKnowledgeDeps: chatagent.TRPCMemoryKnowledgeDeps{
			HasMemory:               persist.Memory.Available(),
			MemoryService:           persist.Memory.TRPC,
			MemoryLayerPorts:        persist.Memory.MemoryLayerPorts,
			MemoryActionLogWriter:   persist.Memory.ActionLogWriter,
			MemoryL2Recall:          persist.Memory.L2Recall,
			MemoryL3Recall:          persist.Memory.L3Recall,
			MemoryCompositeRecall:   persist.Memory.CompositeRecall,
			AgentCaseRecaller:       persist.Memory.AgentCaseRecaller,
			MemoryPreferenceLister:  persist.Memory.PreferenceLister,
			MemoryProfileCardReader: persist.Memory.ProfileCardReader,
			MemoryFactInjectCounter: persist.Memory.FactInjectCounter,
			KnowledgeRetriever:      knowledgeRetriever,
			KnowledgeUsecase:        knowledgeUC,
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
			ToolResultPrune: agentToolResultPruneConfig(runtimeConf),
			OutboundRouter:  outboundRouter,
			SubAgentService: subAgentSvc,
			A2AEnabled:      a2aUC != nil,
			ClientBridge:    clientBridge,
			// TeamCompletionChecker 将在运行时通过 SetTeamCompletionChecker 注入，避免循环依赖
			LG: lg,
		},
	}
}

func provideArtifactRuntimeService(uc *biz.ArtifactUsecase, lg loggateway.Logger) trpcartifact.Service {
	if uc == nil {
		return nil
	}
	return artifacttrpc.NewServiceAdapter(uc, lg)
}

func provideArtifactSigner(lg loggateway.Logger) *artifact.Signer {
	return artifact.NewSigner(lg)
}

// provideKnowledgeWriteBackArbiter 装配 M3.2 写回冲突仲裁器：构造后回注 KnowledgeUsecase
// （Set 副作用，与 provideCuratorWorker 的 SetGate 同模式——Usecase 无法构造期持环依赖）。
// KNOWLEDGE_WRITEBACK_ARBITER_DISABLED=1 时返回 nil（写回不仲裁，与 M3 前行为一致）。
func provideKnowledgeWriteBackArbiter(uc *biz.KnowledgeUsecase, caller biz.LLMCaller, sys *biz.SystemSettingUsecase, catalog *biz.LlmProviderModelUsecase, lg loggateway.Logger) *knowledge.WriteBackArbiter {
	if uc == nil || caller == nil || knowledge.WriteBackArbiterDisabled() {
		return nil
	}
	arbiter := knowledge.NewWriteBackArbiter(caller, sys, catalog, lg)
	uc.SetWriteBackArbiter(arbiter)
	return arbiter
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
	monitorBus contract.MonitorBus,
	factPipeline *biz.FactWritePipeline,
	caseExtractor biz.AgentCaseExtractor,
	caseReader biz.AgentCaseReader,
	caseWriter biz.AgentCaseWriter,
	writeBack biz.KnowledgeWriteBack,
	uc *biz.KnowledgeUsecase,
	d *data.Data,
	lg loggateway.Logger,
	// writeBackArbiter 仅作依赖锚点（M3.2：构造时已完成 Set 回注，本函数不直接使用），
	// 保证 wire 图到达 provideKnowledgeWriteBackArbiter。
	writeBackArbiter *knowledge.WriteBackArbiter,
	// distillWiring 仅作依赖锚点（M4 distill：构造时已完成 Set 回注，本函数不直接使用），
	// 保证 wire 图到达 provideKnowledgeDistillWiring。
	distillWiring *knowledgeDistillWiring,
) (*jobs.AutoMemoryWorker, error) {
	if ks, ok := writeBack.(*service.KnowledgeService); ok && uc != nil {
		ks.SetAgentMemoryProjector(bizknowledge.NewAgentMemoryProjector(uc, service.NewL3AgentFactLister(data.NewL3FactReaderForUser(d)), lg))
	}
	var review biz.KnowledgeWriteBackReview
	if q, ok := writeBack.(biz.KnowledgeWriteBackReview); ok {
		review = q
	}
	var proj biz.KnowledgeAgentMemoryProjector
	if p, ok := writeBack.(biz.KnowledgeAgentMemoryProjector); ok {
		proj = p
	}
	return jobs.NewAutoMemoryWorker(jobs.AutoMemoryWorkerConfig{
		RuntimeConf:     runtimeConf,
		Interval:        0,
		Sessions:        sessions,
		Agents:          agents,
		Writer:          writer,
		IndexSync:       factSync,
		EpisodeSync:     episodeSync,
		L4:              l4,
		Consolidator:    biz.DefaultMemoryConsolidator(extractor),
		Queue:           queue,
		DeadLetterSink:  deadLetterSink,
		Stats:           workerStats,
		MonitorBus:      monitorBus,
		FactPipeline:    factPipeline,
		CaseExtractor:   caseExtractor,
		CaseReader:      caseReader,
		CaseWriter:      caseWriter,
		WriteBack:       writeBack,
		ReviewQueue:     review,
		MemoryProjector: proj,
		Logger:          lg,
	})
}

func provideL4GraphWriter(d *data.Data, cascade *biz.L4CascadeUsecase, lg loggateway.Logger) biz.L4GraphWriter {
	if d == nil {
		return nil
	}
	return data.NewL4GraphWriterAdapter(data.NewL4GraphUsecaseFromData(d, cascade, lg))
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

func provideSkillIntelligenceWorker(uc *biz.SkillIntelligenceUsecase, lg loggateway.Logger) *jobs.SkillIntelligenceWorker {
	if strings.TrimSpace(os.Getenv("SKILL_INTELLIGENCE_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewSkillIntelligenceWorker(0, uc, lg)
}

// provideCuratorWorker also wires the Gate verifier into the intelligence
// usecase via SetGate (A7 follow-up): the gate chain
// (GateVerifier → service.SandboxRunner → SkillIntelligenceUsecase) is cyclic
// at the DI level, so the usecase cannot take the gate as a constructor
// dependency. This provider sits below both in the DAG, keeping wiring acyclic.
func provideCuratorWorker(uc *biz.SkillIntelligenceUsecase, gate biz.SkillGateVerifier, skills biz.SkillQueryReader, lg loggateway.Logger) *jobs.CuratorWorker {
	uc.SetGate(gate)
	if strings.TrimSpace(os.Getenv("CURATOR_WORKER_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewCuratorWorker(0, uc, skills, lg)
}

func provideSkillRegistrationPort(skillUC *biz.SkillUsecase) biz.SkillRegistrationPort {
	return service.NewSkillsButlerRegistrationAdapter(skillUC)
}

// provideSkillUsecase wraps biz.NewSkillUsecase to wire the dedup cache
// invalidation hook (A3 fix: skill mutations must invalidate the 10-min dedup
// result cache, otherwise merged/deleted skills keep showing in dedup groups).
// tagRepo 装配标签字典端口（治理重写后全量失效路由 embed 缓存）。
func provideSkillUsecase(repo biz.SkillRepo, embedder *knowledge.MultiProviderEmbedder, dedup *biz.SkillDedupUsecase, tagRepo biz.SkillTagRepo, lg loggateway.Logger) *biz.SkillUsecase {
	u := biz.NewSkillUsecase(repo, embedder, lg)
	u.SetDedupCacheInvalidator(dedup)
	u.SetTagRepo(tagRepo)
	return u
}

// provideKnowledgeVaultFiler 提供共享 VaultFiler 实例（G1-B1/B2）：KnowledgeUsecase
// 树目录扫描/树内写文件与 vault 同步链（applier）必须同一实例——自写标记统一登记，
// watcher 才能过滤 KB 自身写事件（回环防护）。
func provideKnowledgeVaultFiler(lg loggateway.Logger) *bizknowledge.VaultFiler {
	return bizknowledge.NewVaultFiler(lg)
}

// provideKnowledgeEntityPipeline 装配 M2 实体共现轨（LLM 抽实体 → ReplaceDocEntities
// → 共现 → entity 出链；按 docID+contentHash 幂等）。供 vault 同步钩子与写回图谱
// 钩子两处消费；KNOWLEDGE_ENTITY_PIPELINE_DISABLED 置 1 或依赖缺失时返回 nil。
func provideKnowledgeEntityPipeline(
	d *data.Data,
	caller biz.LLMCaller,
	sys *biz.SystemSettingUsecase,
	catalog *biz.LlmProviderModelUsecase,
	uc *biz.KnowledgeUsecase,
	lg loggateway.Logger,
) *knowledge.EntityPipeline {
	if d == nil || caller == nil || uc == nil || knowledge.EntityPipelineDisabled() {
		return nil
	}
	repo := data.NewKnowledgeRepoFromData(d)
	if repo == nil {
		return nil
	}
	state, ok := repo.(bizknowledge.RelationStateRepo)
	if !ok {
		return nil
	}
	p := knowledge.NewEntityPipeline(caller, sys, catalog, uc, uc, state, lg)
	p.SetWikiWriter(uc)
	return p
}

// provideKnowledgeRelationExtractor 装配 M2 typed 关系抽取器（实体清单 → 三元组
// → 谓词归一 → typed 语义边；content_hash 幂等）。供热文档扫描 worker 与写回图谱
// 钩子两处消费；KNOWLEDGE_RELATION_EXTRACT_DISABLED 置 1 或依赖缺失时返回 nil。
func provideKnowledgeRelationExtractor(
	d *data.Data,
	caller biz.LLMCaller,
	sys *biz.SystemSettingUsecase,
	catalog *biz.LlmProviderModelUsecase,
	uc *biz.KnowledgeUsecase,
	lg loggateway.Logger,
) *knowledge.RelationExtractor {
	if d == nil || uc == nil || caller == nil || jobs.KnowledgeRelationExtractDisabled() {
		return nil
	}
	repo := data.NewKnowledgeRepoFromData(d)
	if repo == nil {
		return nil
	}
	links, lok := repo.(bizknowledge.SemanticLinkRepo)
	vocab, vok := repo.(bizknowledge.RelationVocabRepo)
	state, sok := repo.(bizknowledge.RelationStateRepo)
	if !lok || !vok || !sok {
		return nil
	}
	// 宾语实体 → 文档解析键（basename/title/aliases），与 autolink/mention 同源。
	resolver, rok := data.NewKnowledgeBlockRepoFromData(d).(knowledge.RelationObjectResolver)
	if !rok {
		return nil
	}
	return knowledge.NewRelationExtractor(caller, sys, catalog, uc, links, vocab, state, resolver, lg)
}

// provideKnowledgeWriteBackGraphHook 装配写回图谱钩子（2026-08-16）：团队库
// （VaultBackendTeam）无 vault 同步循环，实体钩子唯一载体永不触发，写回词条页
// 在图谱中恒为孤立节点。钩子对 touched 词条页异步触发实体共现 + typed 关系抽取，
// 双抽取器均 content_hash 幂等、safego 不阻塞写回主路径。两器皆 nil 时不接线。
func provideKnowledgeWriteBackGraphHook(
	entity *knowledge.EntityPipeline,
	relation *knowledge.RelationExtractor,
	lg loggateway.Logger,
) bizknowledge.WriteBackGraphFunc {
	if entity == nil && relation == nil {
		return nil
	}
	return func(_ context.Context, col bizknowledge.Collection, entryDocs []bizknowledge.PromoteTouchedDoc) error {
		for _, doc := range entryDocs {
			docID := strings.TrimSpace(doc.DocID)
			if docID == "" {
				continue
			}
			safego.Go(appctx.Ctx(), "knowledge.writeback_graph", func() {
				if entity != nil {
					if _, err := entity.ProcessDoc(appctx.Ctx(), col.ID, docID); err != nil {
						lg.Warn("writeback entity pipeline failed",
							loggateway.Str("collection_id", col.ID),
							loggateway.Str("doc_id", docID),
							loggateway.Err(err),
						)
					}
				}
				if relation != nil {
					if _, err := relation.ExtractDoc(appctx.Ctx(), docID); err != nil {
						lg.Warn("writeback relation extract failed",
							loggateway.Str("collection_id", col.ID),
							loggateway.Str("doc_id", docID),
							loggateway.Err(err),
						)
					}
				}
			})
		}
		return nil
	}
}

// provideVaultSyncSupervisor 装配 vault 同步链（P1-3 生产装配，原遗漏导致新建
// vault 永不同步）：SyncEngine → VaultSyncApplier（共享 filer + 可选 embedder）→
// VaultSyncRunner → Supervisor。embedder 未配置时 buildChunks 按无语义层降级。
// 同时把 applier 回注 usecase（G1-B2：树内新建文档立即索引，不等 45s 轮询）。
// M0：SetCompiler 接入模态路由抽取器（office/图片 → Markdown；nil 时二进制降级 error）。
// M2.1：SetEntityHook 接入实体共现轨（按 docID+contentHash 幂等，safego 异步
// 不阻塞索引主路径；nil 时跳过）。M2.2：SetRelationHook 接入 typed 关系抽取
// （冷文档与上传路径同钩子，不再只等热度工人）。
func provideVaultSyncSupervisor(
	uc *biz.KnowledgeUsecase,
	filer *bizknowledge.VaultFiler,
	embedder knowledge.Embedder,
	entityPipeline *knowledge.EntityPipeline,
	relationExtractor *knowledge.RelationExtractor,
	registry *knowledge.ExtractorRegistry,
	lg loggateway.Logger,
) *knowledge.VaultSyncSupervisor {
	engine := bizknowledge.NewSyncEngine(lg)
	applier := knowledge.NewVaultSyncApplier(uc, filer, embedder, lg)
	applier.SetCompiler(knowledge.NewBodyCompiler(registry))
	if entityPipeline != nil {
		applier.SetEntityHook(func(collectionID, docID string) {
			safego.Go(appctx.Ctx(), "knowledge.entity_pipeline", func() {
				if _, err := entityPipeline.ProcessDoc(appctx.Ctx(), collectionID, docID); err != nil {
					lg.Warn("entity pipeline failed",
						loggateway.Str("collection_id", collectionID),
						loggateway.Str("doc_id", docID),
						loggateway.Err(err),
					)
				}
			})
		})
	}
	if relationExtractor != nil {
		applier.SetRelationHook(func(collectionID, docID string) {
			safego.Go(appctx.Ctx(), "knowledge.relation_extract", func() {
				if _, err := relationExtractor.ExtractDoc(appctx.Ctx(), docID); err != nil {
					lg.Warn("vault relation extract failed",
						loggateway.Str("collection_id", collectionID),
						loggateway.Str("doc_id", docID),
						loggateway.Err(err),
					)
				}
			})
		})
	}
	uc.SetVaultApplier(applier)
	runner := knowledge.NewVaultSyncRunner(engine, applier, uc, lg)
	return knowledge.NewVaultSyncSupervisor(runner, uc, lg)
}

// provideSkillMergeUsecase wraps biz.NewSkillMergeUsecase to wire the dedup
// cache invalidation hook after a successful merge.
func provideSkillMergeUsecase(reader biz.SkillMergeReader, writer biz.SkillMergeWriter, fuser biz.SkillContentFuser, gate biz.SkillGateVerifier, dedup *biz.SkillDedupUsecase, lg loggateway.Logger) *biz.SkillMergeUsecase {
	u := biz.NewSkillMergeUsecase(reader, writer, fuser, gate, lg)
	u.SetDedupCacheInvalidator(dedup)
	return u
}

func provideLearningLoopScanner(loop *biz.LearningLoopUsecase, lg loggateway.Logger) *jobs.LearningLoopScanner {
	if strings.TrimSpace(os.Getenv("LEARNING_LOOP_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewLearningLoopScanner(0, loop, lg)
}

func provideProviderHealthScanner(uc *biz.LlmProviderModelUsecase, lg loggateway.Logger, flowLog biz.FlowLogWriter) *jobs.ProviderHealthScanner {
	if strings.TrimSpace(os.Getenv("PROVIDER_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewProviderHealthScanner(0, uc, lg, flowLog)
}

func provideChannelHealthScanner(uc *biz.ChannelUsecase, lg loggateway.Logger, flowLog biz.FlowLogWriter) *jobs.ChannelHealthScanner {
	if strings.TrimSpace(os.Getenv("CHANNEL_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewChannelHealthScanner(0, uc, lg, flowLog)
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
	eventBus biz.EventBus,
	monitorBus contract.MonitorBus,
	admission *biz.TurnAdmissionUsecase,
	teamCompiler biz.TeamCompiler,
	gateCards *service.ChannelGateCards,
	lg loggateway.Logger,
) *service.ChannelIngress {
	dedupe := biz.NewIngressMessageDedupe(biz.DefaultMessageDedupeTTL)
	debouncer := biz.NewIngressPeerDebouncer(biz.DefaultIngressDebounce, lg)
	registry := biz.NewTurnPreviewRegistry()
	gate := biz.NewChannelConcurrentGate()
	ingress := service.NewChannelIngress(channels, turnJobs, sessions, chat, graphs, cron, eventBus, monitorBus, dedupe, debouncer, registry, gate, admission, teamCompiler, lg)
	ingress.SetGateCards(gateCards)
	return ingress
}

// provideChannelGateCards 构建渠道交互门卡片管理器（确认/澄清卡片的订阅、
// 发送、跟踪与 PATCH）。生命周期 Start 在 app.go readiness 后挂载。
func provideChannelGateCards(
	eventBus biz.EventBus,
	sessions *biz.SessionUsecase,
	channels *biz.ChannelUsecase,
	chat biz.ChannelTurnGateway,
	steps biz.StepV2Reader,
	lg loggateway.Logger,
) *service.ChannelGateCards {
	return service.NewChannelGateCards(eventBus, sessions, channels, chat, steps, lg)
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

func provideSubAgentService(usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) (*subagenttool.Service, error) {
	// stateDir: use ./data as the root for subagent state files.
	// Runner is set later via SetRunner when the first turn creates a runner.
	svc, err := subagenttool.NewService("./data", nil, lg)
	if err != nil {
		return nil, err
	}
	// P1-2 (2026-08-19): record subagent runs' LLM usage as aux_subagent
	// events. Late-bound via ref: the subagent service sits upstream of
	// UsageUsecase in the DI graph (GraphNodeResolverSet → subagent), so
	// direct injection would create a wire cycle.
	svc.SetUsageRecorder(subagenttool.UsageRecorderFunc(func(ctx context.Context, in biz.AuxLLMUsageInput) error {
		if u := usageRef.Get(); u != nil {
			return u.RecordAuxLLMUsage(ctx, in)
		}
		return nil
	}))
	return svc, nil
}

func provideMemoryL2DecayWorker(decayer biz.MemoryEpisodeDecayer, agents *biz.AgentUsecase, lg loggateway.Logger) *jobs.MemoryL2DecayWorker {
	if jobs.MemoryL2DecayDisabled() {
		return nil
	}
	return jobs.NewMemoryL2DecayWorker(0, decayer, agents, lg)
}

func provideMemoryAdminDeps(d *data.Data) biz.MemoryAdminDeps {
	return data.NewSessionAdminStoreAdapter(d, d.VectorStore())
}

func provideMemoryL1ArchiveWorker(admin biz.MemoryAdminDeps, agents *biz.AgentUsecase, flowLog biz.FlowLogWriter, lg loggateway.Logger) *jobs.MemoryL1ArchiveWorker {
	if jobs.MemoryL1ArchiveDisabled() {
		return nil
	}
	return jobs.NewMemoryL1ArchiveWorker(0, admin, admin, admin, agents, lg, flowLog)
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

// provideMemoryCanaryWorker wires the memory closed-loop canary worker. The
// worker periodically proves write → recall → archive works end-to-end via
// the production consolidation upsert path, the production L3 recall path at
// the default minScore, and bi-temporal invalidation. Failures are recorded
// in biz.MemoryCanaryStatus (alert metric) and emitted as flow-log alarms.
// Disabled via MEMORY_CANARY_DISABLED env var.
func provideMemoryCanaryWorker(d *data.Data, status *biz.MemoryCanaryStatus, flowLog biz.FlowLogWriter, lg loggateway.Logger) *jobs.MemoryCanaryWorker {
	if jobs.MemoryCanaryDisabled() {
		return nil
	}
	return jobs.NewMemoryCanaryWorker(0,
		data.NewMemoryConsolidationWriterAdapter(d, lg),
		data.NewL3FactReaderForUser(d),
		data.NewL3FactWriterAdapter(d, d.VectorStore()),
		status, flowLog, lg)
}

// provideMemoryCitationBackfillWorker wires the citation backfill worker
// (FR-12.6: the "cited" stage of the three-stage counters). The worker scans
// recent memory_recalled notices, detects facts explicitly referenced by the
// assistant reply, and increments cited_count via the dedup ledger.
// Disabled via MEMORY_CITATION_BACKFILL_DISABLED env var.
func provideMemoryCitationBackfillWorker(d *data.Data, lg loggateway.Logger) *jobs.MemoryCitationBackfillWorker {
	if d == nil || jobs.MemoryCitationBackfillDisabled() {
		return nil
	}
	return jobs.NewMemoryCitationBackfillWorker(0,
		data.NewMemoryCitationTraceReader(d),
		data.NewL3FactCitationRecorder(d),
		lg)
}

// provideKnowledgeCitationBackfillWorker wires the knowledge-side citation
// backfill worker (29-token P2-2). The worker scans recent knowledge_recalled
// notices (chunks returned by knowledge_search / knowledge_reflect), detects
// chunks explicitly referenced by the assistant reply, and increments
// cited_count via the dedup ledger.
// Disabled via KNOWLEDGE_CITATION_BACKFILL_DISABLED env var.
func provideKnowledgeCitationBackfillWorker(d *data.Data, lg loggateway.Logger) *jobs.KnowledgeCitationBackfillWorker {
	if d == nil || jobs.KnowledgeCitationBackfillDisabled() {
		return nil
	}
	return jobs.NewKnowledgeCitationBackfillWorker(0,
		data.NewKnowledgeCitationTraceReader(d),
		data.NewKnowledgeChunkCitationRecorder(d),
		lg)
}

// provideKnowledgeRelationExtractWorker wires the self-governing graph M2
// semantic relation worker: periodically scans hot documents (knowledge_access_log
// hits >= threshold) per collection and runs the two-step LLM relation extractor
// (entity list → triples → predicate normalization → typed semantic edges).
// Cost gates: hot-doc threshold + content_hash idempotency + per-pass budget.
// Disabled via KNOWLEDGE_RELATION_EXTRACT_DISABLED env var.
// 抽取器本体经 provideKnowledgeRelationExtractor 共享装配（写回图谱钩子同源）。
func provideKnowledgeRelationExtractWorker(
	d *data.Data,
	extractor *knowledge.RelationExtractor,
	uc *biz.KnowledgeUsecase,
	lg loggateway.Logger,
) *jobs.KnowledgeRelationExtractWorker {
	if d == nil || uc == nil || extractor == nil {
		return nil
	}
	repo := data.NewKnowledgeRepoFromData(d)
	if repo == nil {
		return nil
	}
	hot, hok := repo.(bizknowledge.HotDocumentLister)
	if !hok {
		return nil
	}
	return jobs.NewKnowledgeRelationExtractWorker(0, uc, hot, extractor, lg)
}

func provideKnowledgeIndexRepairWorker(
	svc *service.KnowledgeService,
	lg loggateway.Logger,
) *jobs.KnowledgeIndexRepairWorker {
	return jobs.NewKnowledgeIndexRepairWorker(0, svc, lg)
}

func provideKnowledgeCurateWorker(
	uc *biz.KnowledgeUsecase,
	lg loggateway.Logger,
) *jobs.KnowledgeCurateWorker {
	if uc == nil || jobs.KnowledgeCurateDisabled() {
		return nil
	}
	return jobs.NewKnowledgeCurateWorker(0, uc, lg)
}

// knowledgeDistillWiring M4 distill 装配标记（wire 锚点：仅表示 Set 回注已完成）。
type knowledgeDistillWiring struct{}

// knowledgeDistillFactWriter 适配 MemoryAdminUsecase → bizknowledge.DistillFactWriter：
// 蒸馏事实即 L3 semantic 事实，(scope_type, scope_id, fingerprint) 幂等 upsert。
type knowledgeDistillFactWriter struct {
	admin *biz.MemoryAdminUsecase
}

func (w knowledgeDistillFactWriter) UpsertDistilledFact(ctx context.Context, in bizknowledge.DistilledFact) error {
	_, err := w.admin.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:      in.ScopeType,
		ScopeID:        in.ScopeID,
		Statement:      in.Statement,
		Fingerprint:    in.Fingerprint,
		TagsJSON:       in.TagsJSON,
		FactKind:       "semantic",
		SourceKind:     "knowledge_distill",
		SourceExternal: in.SourcePath,
		Status:         "active",
	})
	return err
}

// provideKnowledgeDistillWiring 装配 M4 distill 任务端口（高频词条摘要卡反向蒸馏
// memory_fact）：构造后回注 KnowledgeUsecase（Set 副作用，与
// provideKnowledgeWriteBackArbiter 同模式——Usecase 无法构造期持环依赖）。
// hot 由 knowledgeRepo 断言 HotDocumentLister；writer 适配 MemoryAdminUsecase。
// 任一缺席返回 nil（distill 任务静默跳过，其余治理任务不受影响）。
func provideKnowledgeDistillWiring(uc *biz.KnowledgeUsecase, admin *biz.MemoryAdminUsecase, d *data.Data) *knowledgeDistillWiring {
	if uc == nil || admin == nil || d == nil {
		return nil
	}
	repo := data.NewKnowledgeRepoFromData(d)
	if repo == nil {
		return nil
	}
	hot, hok := repo.(bizknowledge.HotDocumentLister)
	if !hok {
		return nil
	}
	uc.SetDistillRepos(hot, knowledgeDistillFactWriter{admin: admin})
	return &knowledgeDistillWiring{}
}

// memorySleepTimeQueueSize is the buffer size for the in-memory consolidation
// queue consumed by the Sleep-time Agent.
const memorySleepTimeQueueSize = 100

// provideMemorySleepTimeWorker wires the Sleep-time Agent worker. It builds a
// SleepTimeService backed by the shared trpc memory Service, a per-target LLM
// resolver (P1-1: resolved from agent settings via the ModelCatalog — the
// deprecated MEMORY_SLEEP_TIME_PROVIDER/MEMORY_SLEEP_TIME_MODEL env vars have
// been retired), and an in-memory consolidation queue.
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
	factPipeline *biz.FactWritePipeline,
	d *data.Data,
	lg loggateway.Logger,
) *jobs.MemorySleepTimeWorker {
	if jobs.MemorySleepTimeDisabled() {
		return nil
	}
	// P1-1: per-target LLM resolution from agent settings via the ModelCatalog
	// (same precedence as MemoryLLMExtractor: MemoryWorker → L0Compress →
	// agent default). Targets without a resolvable model gracefully degrade
	// to a no-op consolidation pass.
	rtTrip := &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}}
	// Guard against typed-nil interfaces: a nil *LlmProviderModelUsecase
	// wrapped in the TeamModelCatalog interface would pass the resolver's
	// nil check and panic on first use.
	var modelCatalog biz.TeamModelCatalog
	if catalog != nil {
		modelCatalog = catalog
	}
	var agentGetter service.SleepTimeAgentGetter
	if agents != nil {
		agentGetter = agents
	}
	resolver := service.NewSleepTimeLLMResolver(agentGetter, modelCatalog, rtTrip, lg)
	queue := memory.NewConsolidationQueue(memorySleepTimeQueueSize)
	svc := memory.NewSleepTimeService(memSvc, nil, queue, lg)
	svc.SetLLMResolver(resolver)
	// Phase 6A-06: wire EpisodeConsolidator for L2→L3 fact extraction.
	// Uses the same per-target resolver for episode analysis. When d is nil,
	// EpisodeConsolidator gracefully degrades to a no-op.
	if d != nil {
		ec := memory.NewEpisodeConsolidator(
			data.NewL2RecallStore(d, d.VectorStore()),
			data.NewL3FactWriterAdapter(d, d.VectorStore()),
			data.NewMemoryActionLogWriter(d),
			nil,
			lg,
		)
		ec.SetLLMResolver(resolver)
		// P1-3: extracted facts funnel through the unified write pipeline
		// (gates → neighbor recall → adjudication → bi-temporal writes).
		ec.SetFactPipeline(factPipeline)
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
		// FR-12.7 (Sleep-time Phase 3): wire ProfileCardDistiller for resident
		// profile card maintenance. Uses the same per-target resolver; the
		// distiller degrades to a no-op when no model resolves.
		pd := memory.NewProfileCardDistiller(
			data.NewMemoryPreferenceLister(d),
			data.NewMemoryProfileCardStore(d),
			nil,
			lg,
		)
		if pd != nil {
			pd.SetLLMResolver(resolver)
			svc.SetProfileCardDistiller(pd)
		}
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

func provideMonitorEventsCleanup(repo biz.MonitorEventRepo, lg loggateway.Logger) *jobs.MonitorEventsCleanup {
	if jobs.MonitorEventsCleanupDisabled() {
		return nil
	}
	return jobs.NewMonitorEventsCleanup(0, repo, lg)
}

func provideAutoHealTTLCleanup(repo heal.HealRecordRepo, lg loggateway.Logger, flowLog biz.FlowLogWriter) *jobs.AutoHealTTLCleanup {
	return jobs.NewAutoHealTTLCleanup(0, 0, repo, lg, flowLog)
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
func provideTraceProjector(traceRepo biz.MonitorTraceRepo, usageRepo biz.MonitorTraceUsageRepo, infra *event.Infra, lg loggateway.Logger) *monitor.TraceProjector {
	var monitorBus contract.MonitorBus
	if infra != nil {
		monitorBus = infra.MonitorEventBus
	}
	return monitor.NewTraceProjector(traceRepo, lg, usageRepo, monitorBus)
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

func provideMonitorTraceBackfillWorker(traceRepo biz.MonitorTraceRepo, runnerCompletion biz.MonitorRunnerCompletionRepo, usageRepo biz.MonitorTraceUsageRepo, lg loggateway.Logger) *jobs.MonitorTraceBackfillWorker {
	return jobs.NewMonitorTraceBackfillWorker(traceRepo, runnerCompletion, usageRepo, lg)
}

func provideDiagBundleGenerator(eventRepo biz.MonitorEventRepo, traceRepo biz.MonitorTraceRepo, engine *heal.RootCauseEngine) *biz.DiagBundleGenerator {
	return biz.NewDiagBundleGenerator(eventRepo, traceRepo, engine)
}

func provideSelfHealObserver(runtimeConf *conf.Runtime, repo biz.HealRecordRepo, engine *heal.RootCauseEngine, notifier biz.AlertNotifier, lg loggateway.Logger) (*biz.SelfHealObserver, error) {
	return heal.NewSelfHealObserver(runtimeConf, repo, engine, notifier, lg)
}

// provideLLMSkillEvolver assembles the LLM-backed SkillDraftEvolver (P0
// Curator role). Returns nil when no DefaultRefineLLM is configured — the
// usecase then falls back to the rule-based draft template (nil-safe
// degradation, same resolution pattern as provideSkillAutoCreator).
func provideLLMSkillEvolver(caller biz.LLMCaller, sys *biz.SystemSettingUsecase, skills biz.SkillLookupReader, lg loggateway.Logger) biz.SkillDraftEvolver {
	rl, err := sys.GetRefineLLM(context.Background())
	if err != nil || strings.TrimSpace(rl.Provider) == "" || strings.TrimSpace(rl.Model) == "" {
		lg.Warn("llm skill evolver: no DefaultRefineLLM configured, LLM draft generation disabled (rule-based fallback)")
		return nil
	}
	return biz.NewLLMSkillEvolver(caller, skills, rl.Provider, rl.Model, lg)
}

// provideSkillVersionReloader assembles the production SkillReloader (P0
// Reload stage): registers approved evolved drafts as new skill versions,
// anchoring the previous version as parent for rollback.
func provideSkillVersionReloader(writer biz.SkillVersionWriter, queries bizskill.SkillQueryReader, lg loggateway.Logger) biz.SkillReloader {
	return biz.NewSkillVersionReloader(writer, queries, lg)
}

// provideSkillReplayRunner assembles the dataset-replay runner (P1 Solve
// 接线 + P2 F1 AB 对照回放): replays the skill's bound evaluation dataset
// against evolved drafts via the platform DefaultRefineLLM. DI 环检查：本
// provider 依赖 evaluation.Usecase + LLMCaller + SkillRepo，不经
// SkillIntelligenceUsecase，无新环。
func provideSkillReplayRunner(evalUC *evaluation.Usecase, caller biz.LLMCaller, sys *biz.SystemSettingUsecase, skills biz.SkillLookupReader, lg loggateway.Logger) *service.SkillReplayRunner {
	return service.NewSkillReplayRunner(evalUC, caller, sys, skills, lg)
}

// provideSkillTriggerGoldenRunner assembles the trigger golden-set regression
// runner (P2 F4): deterministic frontmatter-trigger accuracy check over the
// {skill.Name|Slug}__trigger evaluation dataset, no LLM. DI 环检查：依赖
// evaluation.Usecase + SkillRepo，不经 SkillIntelligenceUsecase，无新环。
func provideSkillTriggerGoldenRunner(evalUC *evaluation.Usecase, skills biz.SkillLookupReader, lg loggateway.Logger) *service.SkillTriggerGoldenRunner {
	return service.NewSkillTriggerGoldenRunner(evalUC, skills, lg)
}

// provideSkillGateVerifier assembles the Gate verifier for skill merge /
// evolution. Dimensions: sandbox functional check + AB comparison replay
// (P2 F1 棘轮门控, WithABReplayRunner; covers the P1 absolute threshold) +
// harmful-rule effectiveness (计数归因, WithSkillLookup) + drift (P2 F2,
// WithSkillLookup) + trigger-accuracy golden regression (P2 F4,
// WithTriggerGoldenRunner). lintChecker is nil so the style dimension falls
// back to the built-in rule-based checks.
func provideSkillGateVerifier(sandboxRunner biz.SandboxRunner, replayRunner biz.SkillReplayABRunner, goldenRunner biz.SkillTriggerGoldenRunner, skills biz.SkillLookupReader) biz.SkillGateVerifier {
	return biz.NewGateVerifier(sandboxRunner, nil,
		biz.WithABReplayRunner(replayRunner),
		biz.WithSkillLookup(skills),
		biz.WithTriggerGoldenRunner(goldenRunner),
	)
}

func provideSkillIntelligenceUsecase(scorer *biz.SkillScoringUsecase, reporter *biz.SkillReportUsecase, unifiedRepo *data.UnifiedEvolutionRepo, aggregator biz.SkillHealthAggregator, unanalyzedReader biz.SkillInvocationUnanalyzedReader, orch *biz.SkillEvolutionOrchestrator, evolver biz.SkillDraftEvolver, reloader biz.SkillReloader, registrar biz.SkillRegistrationPort, lg loggateway.Logger) *biz.SkillIntelligenceUsecase {
	reporter.SetUnanalyzedReader(unanalyzedReader)
	uc := biz.NewSkillIntelligenceUsecase(scorer, reporter, unifiedRepo, aggregator, lg,
		biz.SkillIntelligenceConfig{
			UnanalyzedReader: unanalyzedReader,
			Orchestrator:     orch,
			Evolver:          evolver,
			Reloader:         reloader,
			Registrar:        registrar,
		},
	)
	return uc
}

// provideSkillEvolutionOrchestrator assembles the unified evolution orchestrator
// (A1): constructs it over the unified store and registers the three triggers.
// Trigger registration is imperative, so this cannot live in biz.ProviderSet.
//
// Dependency notes:
//   - HealthTrigger scores via *biz.SkillScoringUsecase (not the intelligence
//     usecase), which keeps the graph acyclic: orchestrator → scorer → repo.
//   - AgentConfigTrigger carries the ported L3 scan logic (A6, formerly
//     EvolutionUsecase.ScanAgent); both agent-scoped triggers gate on their
//     own opt-in flag via biz.AgentRepository settings.
//   - Platform self-improvement triggers (73-self-iteration-v3) register only
//     when self_improvement.enabled=true; the observe worker (gated the same
//     way) is the sole platform-scan caller, so a disabled pipeline leaves no
//     live signal path.
func provideSkillEvolutionOrchestrator(
	unifiedRepo *data.UnifiedEvolutionRepo,
	agents biz.AgentRepository,
	patterns biz.PatternReader,
	creator biz.SkillAutoCreator,
	registrar biz.SkillRegistrationPort,
	aggregator biz.SkillHealthAggregator,
	scorer *biz.SkillScoringUsecase,
	metricsRepo biz.EvolutionMetricsRepo,
	skills biz.SkillLookupReader,
	caseRecaller biz.AgentCaseRecaller,
	caseDistiller biz.CaseSkillDistiller,
	siConf *conf.SelfImprovement,
	siSignals *data.SelfImprovementSignalRepo,
	siTestRuns biz.TestRunReader,
	siTraces biz.OrchestrationTraceReader,
	cooldownStore biz.SITriggerCooldownStore,
	lg loggateway.Logger,
) *biz.SkillEvolutionOrchestrator {
	orch := biz.NewSkillEvolutionOrchestrator(unifiedRepo, unifiedRepo, lg)
	// per-agent 提议过期：Agent target 读 evo_proposal_ttl_days，其余/异常回退全局默认。
	orch.SetExpirationResolver(func(ctx context.Context, targetType, targetID string) time.Duration {
		if targetType != string(biz.EvolutionTargetAgent) || strings.TrimSpace(targetID) == "" {
			return 0
		}
		s, err := agents.GetAgentRuntimeSettings(ctx, targetID)
		if err != nil || s.EvoProposalTTLDays <= 0 {
			return 0
		}
		return time.Duration(s.EvoProposalTTLDays) * 24 * time.Hour
	})
	if cooldownStore != nil {
		orch.AttachCooldownStore(cooldownStore)
		if err := orch.HydrateTriggerCooldowns(context.Background()); err != nil {
			lg.Warn("orchestrator: cooldown multipliers load failed, starting at 1x",
				loggateway.StepID("evo_orchestrator.cooldown_hydrate"),
				loggateway.Err(err))
		}
	}
	orch.RegisterTrigger(biz.NewPatternTrigger(agents, patterns, creator, registrar, unifiedRepo, lg))
	orch.RegisterTrigger(biz.NewHealthTrigger(aggregator, scorer, lg))
	orch.RegisterTrigger(biz.NewAgentConfigTrigger(agents, metricsRepo, unifiedRepo, lg))
	// P2 F3 成功沉淀：高成功率 skill 固化正向模式（规则块门控在 trigger 内）。
	orch.RegisterTrigger(biz.NewSuccessTrigger(aggregator, skills, lg))
	// P3 M4 case→skill 蒸馏：Agent Case 积累到阈值（5 条）后蒸馏为 SKILL.md
	// 草稿建议；冷却/pending 短路/DB UNIQUE 由 orchestrator 统一兜底。
	orch.RegisterTrigger(biz.NewCaseDistillTrigger(agents, caseRecaller, caseDistiller, lg))
	if siConf.SIEnabled() {
		orch.RegisterTrigger(biz.NewErrorClusterTrigger(siSignals, siConf.SIErrorClusterWindowDays(), siConf.SIErrorClusterMinCount(), lg))
		orch.RegisterTrigger(biz.NewPerfBottleneckTrigger(siSignals, siConf.SIPerfLatencyFactor(), siConf.SIPerfTokenFactor(), lg))
		orch.RegisterTrigger(biz.NewEvalRegressionTrigger(siSignals, siConf.SIEvalRegressionThreshold(), lg))
		orch.RegisterTrigger(biz.NewTestFailureTrigger(siTestRuns, 0, 0, lg))
		// P3-1 编排轨迹 MAST 标注：终态编排 + flow-log 错误聚合 → 失败模式聚类建议。
		orch.RegisterTrigger(biz.NewOrchestrationTraceTrigger(siTraces, 0, 0, lg))
	}
	return orch
}

// provideSelfImprovementTestRunReader adapts the test-run JSON directory to
// the biz.TestRunReader signal port (73-self-iteration-v3). Empty dir → the
// reader stays inert (ListRecentFailures returns nil).
func provideSelfImprovementTestRunReader(siConf *conf.SelfImprovement) biz.TestRunReader {
	return data.NewTestRunFileReader(siConf.SITestRunsDir())
}

// provideSelfImprovementObserveUsecase assembles the Observe-stage usecase
// (73-self-iteration-v3): unified orchestrator + pending-suggestion query +
// run persistence ports.
func provideSelfImprovementObserveUsecase(
	orch *biz.SkillEvolutionOrchestrator,
	unifiedRepo *data.UnifiedEvolutionRepo,
	runReader biz.SelfImprovementRunReader,
	runWriter biz.SelfImprovementRunWriter,
	lg loggateway.Logger,
) *biz.SelfImprovementObserveUsecase {
	return biz.NewSelfImprovementObserveUsecase(orch, unifiedRepo, runReader, runWriter, lg)
}

// provideSelfImprovementObserveWorker gates the Observe-stage scheduler on
// self_improvement.enabled (default off, design §6.2).
func provideSelfImprovementObserveWorker(
	siConf *conf.SelfImprovement,
	uc *biz.SelfImprovementObserveUsecase,
	lg loggateway.Logger,
) *jobs.SelfImproveObserveWorker {
	if !siConf.SIEnabled() {
		return nil
	}
	return jobs.NewSelfImproveObserveWorker(siConf.SIObserveInterval(), uc, lg)
}

// ── Self-improvement Phase 4 chain (73-self-iteration-v3, W6) ───────────────
//
// Every provider in this block gates on self_improvement.enabled (default
// false, design §6.2): a disabled pipeline constructs nothing downstream and
// the three workers (drive/watchdog/outcome) stay nil so workers.go skips
// them. Stage ports are returned as interfaces so a disabled/unconfigured
// stage is a true nil (no nil-interface trap); the pipeline itself reports a
// clear "stages not wired" error when a required stage is absent.

// provideRepoSandboxRunner builds the git-worktree sandbox (biz.RepoSandbox)
// anchored at sandbox.repo_root (fallback: process working directory — the
// admin must then be started from the repository root when self-improvement
// is enabled, gray-rollout feature). Gate timeouts/worktree root come from
// config (D4).
func provideRepoSandboxRunner(siConf *conf.SelfImprovement, lg loggateway.Logger) *service.RepoSandboxRunner {
	if !siConf.SIEnabled() {
		return nil
	}
	repoRoot := siConf.SIRepoRoot()
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			lg.Warn("self-improve sandbox: resolve repo root failed, pipeline disabled",
				loggateway.StepID("si_sandbox.init"), loggateway.Err(err))
			return nil
		}
		repoRoot = wd
	}
	timeouts := siConf.SIGateTimeouts()
	runner, err := service.NewRepoSandboxRunner(repoRoot, lg,
		service.WithWorktreeRoot(siConf.SIWorktreeRoot()),
		service.WithGateTimeout(biz.SandboxGateBuild, timeouts["g1"]),
		service.WithGateTimeout(biz.SandboxGateTest, timeouts["g2"]),
		service.WithGateTimeout(biz.SandboxGateLint, timeouts["g3"]),
	)
	if err != nil {
		lg.Warn("self-improve sandbox init failed, pipeline disabled",
			loggateway.StepID("si_sandbox.init"), loggateway.Err(err))
		return nil
	}
	return runner
}

// provideSIControlPlane builds the user-intervention command plane (T3.6).
func provideSIControlPlane() *biz.SIControlPlane {
	return biz.NewSIControlPlane()
}

// provideSIAnalystStage wires the LLM Analyst stage on the platform
// DefaultRefineLLM (V2 skill_curator pattern). Unconfigured → nil stage;
// the pipeline then fails runs with a clear "stages not wired" error.
func provideSIAnalystStage(siConf *conf.SelfImprovement, caller biz.LLMCaller, sys *biz.SystemSettingUsecase, sandbox *service.RepoSandboxRunner, rca heal.RootCauseAnalyzer, lg loggateway.Logger) biz.SIAnalystStage {
	if !siConf.SIEnabled() {
		return nil
	}
	rl, err := sys.GetRefineLLM(context.Background())
	if err != nil || strings.TrimSpace(rl.Provider) == "" || strings.TrimSpace(rl.Model) == "" {
		lg.Warn("self-improve analyst: no DefaultRefineLLM configured, diagnose stage disabled",
			loggateway.StepID("si_analyst.init"))
		return nil
	}
	var opts []service.SIAnalystOption
	if sandbox != nil {
		if root := sandbox.RepoRoot(); root != "" {
			opts = append(opts, service.WithSIAnalystReadRoot(root))
		}
	}
	if rca != nil {
		opts = append(opts, service.WithSIAnalystRCA(rca))
	}
	return service.NewSIAnalystAgent(caller, rl.Provider, rl.Model, lg, opts...)
}

// provideSIPatcherStage wires the LLM Patcher stage (D10 daily quota default
// 20 inside the agent when dailyMax=0).
func provideSIPatcherStage(siConf *conf.SelfImprovement, caller biz.LLMCaller, sys *biz.SystemSettingUsecase, lg loggateway.Logger) biz.SIPatcherStage {
	if !siConf.SIEnabled() {
		return nil
	}
	rl, err := sys.GetRefineLLM(context.Background())
	if err != nil || strings.TrimSpace(rl.Provider) == "" || strings.TrimSpace(rl.Model) == "" {
		lg.Warn("self-improve patcher: no DefaultRefineLLM configured, patch stage disabled",
			loggateway.StepID("si_patcher.init"))
		return nil
	}
	return service.NewSIPatcherAgent(caller, rl.Provider, rl.Model, 0, lg)
}

// provideSICriticStage wires the Critic G4 stage (D10 daily quota default 10
// inside the agent when dailyMax=0). nil → pipeline degrades G4 (T3.3).
func provideSICriticStage(siConf *conf.SelfImprovement, caller biz.LLMCaller, sys *biz.SystemSettingUsecase, lg loggateway.Logger) biz.SICriticStage {
	if !siConf.SIEnabled() {
		return nil
	}
	rl, err := sys.GetRefineLLM(context.Background())
	if err != nil || strings.TrimSpace(rl.Provider) == "" || strings.TrimSpace(rl.Model) == "" {
		lg.Warn("self-improve critic: no DefaultRefineLLM configured, G4 degraded",
			loggateway.StepID("si_critic.init"))
		return nil
	}
	return service.NewSICriticAgent(caller, rl.Provider, rl.Model, 0, lg)
}

// ── W6 port adapters (service layer): biz ports → platform infrastructure ──

// provideSINotifier wires operator-facing notifications as monitor events.
func provideSINotifier(events biz.MonitorEventRepo, lg loggateway.Logger) biz.SINotifier {
	return service.NewSIMonitorNotifier(events, lg)
}

// provideSIApprovalSink wires manual-approval requests as monitor events
// (idempotent per run, P4 internal path; P5 moves to Proto + console).
func provideSIApprovalSink(events biz.MonitorEventRepo, lg loggateway.Logger) biz.SIApprovalSink {
	return service.NewSIMonitorApprovalSink(events, lg)
}

// provideSIActivitySink wires Meta Team stage activities as monitor events.
func provideSIActivitySink(events biz.MonitorEventRepo, lg loggateway.Logger) biz.SIActivitySink {
	return service.NewSIMonitorActivitySink(events, lg)
}

// provideSINegativePatternSink wires regressed-patch anti-patterns to the
// FailurePattern KB (D8).
func provideSINegativePatternSink(kb *data.FailurePatternReadWriter, lg loggateway.Logger) biz.SINegativePatternSink {
	return service.NewSIKBNegativePatternSink(kb, lg)
}

// provideSITriggerFeedbackSink wires trigger cooldown escalation onto the
// unified evolution orchestrator (D8 adaptive throttle).
func provideSITriggerFeedbackSink(orch *biz.SkillEvolutionOrchestrator) biz.SITriggerFeedbackSink {
	return service.NewSIOrchestratorFeedbackSink(orch)
}

// provideSIApplier wires the git-backed applier on the repo sandbox. nil
// sandbox (disabled/init failed) → nil applier; downstream usecases skip.
func provideSIApplier(sandbox *service.RepoSandboxRunner, lg loggateway.Logger) biz.SIApplier {
	if sandbox == nil {
		return nil
	}
	applier, err := service.NewSIRepoApplier(sandbox, lg)
	if err != nil {
		lg.Warn("self-improve applier init failed, apply chain disabled",
			loggateway.StepID("si_applier.init"), loggateway.Err(err))
		return nil
	}
	return applier
}

// provideSIRiskRules loads the admin-configured risk-classification rules
// (P5 console) once at startup. Raw (un-normalized) rules are returned so
// consumers keep the "zero = inherit code default" semantics; load failures
// degrade to zero rules (= D6/D10 code defaults).
func provideSIRiskRules(siConf *conf.SelfImprovement, repo biz.SIRiskRuleRepo, lg loggateway.Logger) biz.SIRiskRules {
	if !siConf.SIEnabled() || repo == nil {
		return biz.SIRiskRules{}
	}
	rules, err := repo.GetSIRiskRules(context.Background())
	if err != nil {
		lg.Warn("self-improve risk rules load failed, using code defaults",
			loggateway.StepID("si_risk_rules.load"), loggateway.Err(err))
		return biz.SIRiskRules{}
	}
	return rules
}

// provideSelfImprovementPipelineUsecase assembles the Meta Team pipeline
// (T3.2): stages + sandbox + run persistence + activity mount + control
// plane.
func provideSelfImprovementPipelineUsecase(
	siConf *conf.SelfImprovement,
	analyst biz.SIAnalystStage,
	patcher biz.SIPatcherStage,
	critic biz.SICriticStage,
	sandbox *service.RepoSandboxRunner,
	unifiedRepo *data.UnifiedEvolutionRepo,
	runReader biz.SelfImprovementRunReader,
	runWriter biz.SelfImprovementRunWriter,
	activitySink biz.SIActivitySink,
	control *biz.SIControlPlane,
	riskRules biz.SIRiskRules,
	lg loggateway.Logger,
) *biz.SelfImprovementPipelineUsecase {
	if !siConf.SIEnabled() {
		return nil
	}
	// Guard the nil-interface trap: a failed sandbox init yields a nil
	// *RepoSandboxRunner which must stay a nil biz.RepoSandbox.
	var sandboxPort biz.RepoSandbox
	if sandbox != nil {
		sandboxPort = sandbox
	}
	return biz.NewSelfImprovementPipelineUsecase(biz.SelfImprovementPipelineDeps{
		Analyst:      analyst,
		Patcher:      patcher,
		Critic:       critic,
		Sandbox:      sandboxPort,
		Suggestions:  unifiedRepo,
		RunReader:    runReader,
		RunWriter:    runWriter,
		Classifier:   biz.NewSIRiskClassifierWithRules(riskRules),
		ActivitySink: activitySink,
		Control:      control,
		MaxAttempts:  siConf.SIMaxAttempts(),
		MaxDiffLines: siConf.SIMaxDiffLines(),
		Lg:           lg,
	})
}

// provideSelfImprovementApplyUsecase assembles the apply orchestrator (T4.5):
// kind routing + conflict escalation + observing-window admission.
func provideSelfImprovementApplyUsecase(
	siConf *conf.SelfImprovement,
	runReader biz.SelfImprovementRunReader,
	runWriter biz.SelfImprovementRunWriter,
	applier biz.SIApplier,
	approvals biz.SIApprovalSink,
	riskRules biz.SIRiskRules,
	lg loggateway.Logger,
) (*biz.SelfImprovementApplyUsecase, error) {
	if !siConf.SIEnabled() || applier == nil {
		return nil, nil
	}
	return biz.NewSelfImprovementApplyUsecase(biz.SelfImprovementApplyUsecaseDeps{
		RunReader:              runReader,
		RunWriter:              runWriter,
		Applier:                applier,
		Approvals:              approvals,
		MaxConcurrentObserving: siConf.SIMaxConcurrentObserving(),
		ObserveWindow:          siConf.SIObserveWindowDuration(),
		RiskRules:              riskRules,
		Lg:                     lg,
	})
}

// provideSIGovernanceRouter assembles the governance router (T3.5): risk
// channel routing + daily auto-apply quota + approval submission + apply
// driver hook (T4.5).
func provideSIGovernanceRouter(
	siConf *conf.SelfImprovement,
	runReader biz.SelfImprovementRunReader,
	runWriter biz.SelfImprovementRunWriter,
	notifier biz.SINotifier,
	approvals biz.SIApprovalSink,
	apply *biz.SelfImprovementApplyUsecase,
	riskRules biz.SIRiskRules,
	lg loggateway.Logger,
) *biz.SIGovernanceRouter {
	if !siConf.SIEnabled() {
		return nil
	}
	var driver biz.SIApplyDriver
	if apply != nil {
		driver = apply
	}
	// 日配额优先级：DB 管理配置正数（P5）> config.yaml 正数 > 代码默认 0（关闭 auto-apply）。
	quota := int32(siConf.SIDailyAutoApplyQuota())
	if riskRules.DailyAutoQuota > 0 {
		quota = riskRules.DailyAutoQuota
	}
	return biz.NewSIGovernanceRouter(biz.SIGovernanceRouterDeps{
		RunReader:            runReader,
		RunWriter:            runWriter,
		Notifier:             notifier,
		Approvals:            approvals,
		ApplyDriver:          driver,
		AutoApplyQuotaPerDay: quota,
		Lg:                   lg,
	})
}

// provideSelfImprovementDriveUsecase assembles the full-chain driver
// (Phase 4): detected→pipeline / stale mid-pipeline recover / governance
// routing / applying re-drive / applied promotion.
func provideSelfImprovementDriveUsecase(
	siConf *conf.SelfImprovement,
	runReader biz.SelfImprovementRunReader,
	runWriter biz.SelfImprovementRunWriter,
	pipeline *biz.SelfImprovementPipelineUsecase,
	router *biz.SIGovernanceRouter,
	apply *biz.SelfImprovementApplyUsecase,
	lg loggateway.Logger,
) (*biz.SelfImprovementDriveUsecase, error) {
	if !siConf.SIEnabled() || apply == nil {
		return nil, nil
	}
	var exec biz.SIPipelineExecutor
	if pipeline != nil {
		exec = pipeline
	}
	var routePort biz.SIGovernanceRoutePort
	if router != nil {
		routePort = router
	}
	return biz.NewSelfImprovementDriveUsecase(biz.SelfImprovementDriveDeps{
		RunReader:    runReader,
		RunWriter:    runWriter,
		Pipeline:     exec,
		Router:       routePort,
		Applier:      apply,
		StaleTimeout: siConf.SIStaleTimeout(),
		Lg:           lg,
	})
}

// provideSelfImprovementWatchdogUsecase assembles the observing-window
// evaluator (T4.2): baseline vs 1h sliding-window metrics + auto-rollback.
func provideSelfImprovementWatchdogUsecase(
	siConf *conf.SelfImprovement,
	runReader biz.SelfImprovementRunReader,
	runWriter biz.SelfImprovementRunWriter,
	siSignals *data.SelfImprovementSignalRepo,
	applier biz.SIApplier,
	notifier biz.SINotifier,
	lg loggateway.Logger,
) (*biz.SelfImprovementWatchdogUsecase, error) {
	if !siConf.SIEnabled() || applier == nil {
		return nil, nil
	}
	return biz.NewSelfImprovementWatchdogUsecase(biz.SelfImprovementWatchdogDeps{
		RunReader:       runReader,
		RunWriter:       runWriter,
		Metrics:         siSignals,
		Applier:         applier,
		Notifier:        notifier,
		ErrorRateFactor: siConf.SIObserveErrorRateFactor(),
		P95Factor:       siConf.SIObserveP95Factor(),
		MetricsWindow:   time.Hour,
		Lg:              lg,
	})
}

// provideSelfImprovementOutcomeUsecase assembles the Learn-stage attribution
// (T4.4): terminal-run verdicts + KB negative patterns + trigger cooldown
// feedback.
func provideSelfImprovementOutcomeUsecase(
	siConf *conf.SelfImprovement,
	runReader biz.SelfImprovementRunReader,
	runRepo *data.SelfImprovementRunRepo,
	patterns biz.SINegativePatternSink,
	feedback biz.SITriggerFeedbackSink,
	lg loggateway.Logger,
) (*biz.SelfImprovementOutcomeUsecase, error) {
	if !siConf.SIEnabled() {
		return nil, nil
	}
	return biz.NewSelfImprovementOutcomeUsecase(biz.SelfImprovementOutcomeDeps{
		RunReader: runReader,
		Outcomes:  runRepo,
		Patterns:  patterns,
		Feedback:  feedback,
		Lg:        lg,
	})
}

// provideSelfImprovementAdminUsecase assembles the manual admin control
// surface (T4.3) + console query surface (P5: List/Get/OutcomeStats，
// StatsReader 取同一 repo 的 AggregateOutcomeStats）。
func provideSelfImprovementAdminUsecase(
	siConf *conf.SelfImprovement,
	runReader biz.SelfImprovementRunReader,
	runWriter biz.SelfImprovementRunWriter,
	runRepo *data.SelfImprovementRunRepo,
	applier biz.SIApplier,
	apply *biz.SelfImprovementApplyUsecase,
	riskRules biz.SIRiskRuleRepo,
	lg loggateway.Logger,
) (*biz.SelfImprovementAdminUsecase, error) {
	if !siConf.SIEnabled() || applier == nil {
		return nil, nil
	}
	var driver biz.SIApplyDriver
	if apply != nil {
		driver = apply
	}
	return biz.NewSelfImprovementAdminUsecase(biz.SelfImprovementAdminDeps{
		RunReader:   runReader,
		RunWriter:   runWriter,
		Applier:     applier,
		ApplyDriver: driver,
		StatsReader: runRepo,
		RiskRules:   riskRules,
		Lg:          lg,
	})
}

// provideSelfImprovementService ALWAYS constructs the console service (P5.5) —
// even when the feature is disabled — so the HTTP/gRPC routes stay registered
// and the console receives a structured 503 SELF_IMPROVEMENT (rendered as a
// guided empty state) instead of a bare 404. uc is nil when disabled; GetStatus
// answers regardless via cfg + refineLLM.
func provideSelfImprovementService(
	uc *biz.SelfImprovementAdminUsecase,
	siConf *conf.SelfImprovement,
	sys *biz.SystemSettingUsecase,
	control *biz.SIControlPlane,
	lg loggateway.Logger,
) *service.SelfImprovementService {
	return service.NewSelfImprovementService(uc, siConf, sys, control, lg)
}

// provideSelfImproveDriveWorker gates the full-chain drive scheduler on
// self_improvement.enabled (W6).
func provideSelfImproveDriveWorker(
	siConf *conf.SelfImprovement,
	uc *biz.SelfImprovementDriveUsecase,
	lg loggateway.Logger,
) *jobs.SelfImproveDriveWorker {
	if !siConf.SIEnabled() || uc == nil {
		return nil
	}
	return jobs.NewSelfImproveDriveWorker(siConf.SIDriveInterval(), uc, lg)
}

// provideSelfImproveWatchdogWorker gates the observing-window scheduler on
// self_improvement.enabled (T4.2).
func provideSelfImproveWatchdogWorker(
	siConf *conf.SelfImprovement,
	uc *biz.SelfImprovementWatchdogUsecase,
	lg loggateway.Logger,
) *jobs.SelfImproveWatchdogWorker {
	if !siConf.SIEnabled() || uc == nil {
		return nil
	}
	return jobs.NewSelfImproveWatchdogWorker(siConf.SIWatchdogInterval(), uc, lg)
}

// provideSelfImproveOutcomeWorker gates the Learn-stage scheduler on
// self_improvement.enabled (T4.4).
func provideSelfImproveOutcomeWorker(
	siConf *conf.SelfImprovement,
	uc *biz.SelfImprovementOutcomeUsecase,
	lg loggateway.Logger,
) *jobs.SelfImproveOutcomeWorker {
	if !siConf.SIEnabled() || uc == nil {
		return nil
	}
	return jobs.NewSelfImproveOutcomeWorker(siConf.SIOutcomeInterval(), uc, lg)
}

// provideEvolutionUsecase wraps biz.ProvideEvolutionUsecase with the unified
// store (A6). Replaces the bare constructor in biz.ProviderSet.
func provideEvolutionUsecase(
	metricsRepo biz.EvolutionMetricsRepo,
	unifiedRepo *data.UnifiedEvolutionRepo,
	agents biz.AgentRepository,
	tp biz.EvolutionTxProvider,
	lg loggateway.Logger,
) *biz.EvolutionUsecase {
	return biz.ProvideEvolutionUsecase(metricsRepo, unifiedRepo, agents, tp, lg)
}

// provideLearningLoopUsecase wraps biz.NewLearningLoopUsecase to wire the
// unified orchestrator so RegisterKnowledge creates UnifiedEvolutionSuggestions
// through the single pipeline (A1).
func provideLearningLoopUsecase(
	obs biz.ObservationReadWriter,
	pat biz.PatternReadWriter,
	prop biz.ProposalReadWriter,
	agents biz.AgentRepository,
	orch *biz.SkillEvolutionOrchestrator,
	lg loggateway.Logger,
) *biz.LearningLoopUsecase {
	uc := biz.NewLearningLoopUsecase(obs, pat, prop, agents, lg)
	uc.SetOrchestrator(orch)
	return uc
}

// provideEvolutionDrafter wires the EVO-20 LLM draft generator: a post-pass of
// EvolutionOrchestratorWorker that turns notification-only L3 persona/prompt
// suggestions into applicable drafts (apply_payload + diff_preview).
// biz.AgentRepository satisfies both EvolutionDraftAgentReader and
// AgentEvolutionSettingsReader via its composite interfaces.
func provideEvolutionDrafter(
	unifiedRepo *data.UnifiedEvolutionRepo,
	agents biz.AgentRepository,
	caller biz.LLMCaller,
	sys *biz.SystemSettingUsecase,
	lg loggateway.Logger,
) *biz.EvolutionDrafter {
	return biz.NewEvolutionDrafter(unifiedRepo, agents, agents, caller, sys, lg)
}

// provideEvolutionOrchestratorWorker provides the single unified entry point
// for automatic evolution triggering (A1), superseding the trigger half of
// the legacy per-pipeline scanners and CuratorWorker.
func provideEvolutionOrchestratorWorker(
	orch *biz.SkillEvolutionOrchestrator,
	agents biz.AgentRepository,
	skills biz.SkillQueryReader,
	drafter *biz.EvolutionDrafter,
	lg loggateway.Logger,
) *jobs.EvolutionOrchestratorWorker {
	if strings.TrimSpace(os.Getenv("EVOLUTION_ORCHESTRATOR_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewEvolutionOrchestratorWorker(0, orch, agents, skills, drafter, lg)
}

func provideSelfCheckScheduler(
	checkers []monitor.SelfChecker,
	repairers []monitor.SelfCheckRepairer,
	repo monitor.SelfCheckReportRepo,
	registry *monitor.AlertMetricRegistry,
	lg loggateway.Logger,
	flowLog monitor.FlowLogWriter,
) *monitor.SelfCheckScheduler {
	// Wrap repairers in SelfCheckRepairDispatcher to enforce per-checker cooldown
	// (5min) and prevent repeated repairs of the same failing check within the
	// cooldown window. Without the dispatcher, SelfCheckScheduler would call
	// repairers directly on every cycle (every 5min), bypassing cooldown logic.
	dispatcher := monitor.NewSelfCheckRepairDispatcher(repairers, lg)
	scheduler := monitor.NewSelfCheckScheduler(checkers, []monitor.SelfCheckRepairer{dispatcher}, repo, registry, lg, flowLog)
	// Register the unhealthy-count metric so AlertEvalWorker can evaluate
	// alert rules against it. Without this registration, the metric is never
	// polled and self-check degradation goes undetected by the alerting system.
	if registry != nil {
		registry.Register(monitor.NewSelfCheckUnhealthyCountMetric(scheduler))
	}
	return scheduler
}

// monitorBusHealthAdapter adapts the typed MonitorBus to the self-check
// EventBusHealthChecker port. SubscriberCount is exposed structurally by the
// concrete monitorBus (internal/event/monitor_bus.go); IsHealthy is always
// true because the in-process bus is healthy by construction — the real
// health signal is the subscriber count.
type monitorBusHealthAdapter struct {
	counter interface{ SubscriberCount() int }
}

func (a monitorBusHealthAdapter) SubscriberCount(topic string) int {
	return a.counter.SubscriberCount()
}
func (a monitorBusHealthAdapter) IsHealthy(topic string) bool { return true }

func provideEventBusHealthChecker(infra *event.Infra) monitor.EventBusHealthChecker {
	if infra == nil || infra.MonitorEventBus == nil {
		return nil
	}
	counter, ok := infra.MonitorEventBus.(interface{ SubscriberCount() int })
	if !ok {
		return nil
	}
	return monitorBusHealthAdapter{counter: counter}
}

func provideWSConnectionCounter(wsSrv *server.WSServer) monitor.WSConnectionCounter {
	if wsSrv == nil {
		return nil
	}
	return wsSrv
}

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

func provideFailurePatternSyncJob(engine *heal.RootCauseEngine, writer heal.FailurePatternWriter, reader heal.FailurePatternReader, lg loggateway.Logger) *jobs.FailurePatternSyncJob {
	return jobs.NewFailurePatternSyncJob(0, engine, writer, reader, lg)
}

// providePredictiveHealUsecase wires the predictive-heal usecase with the real
// action catalog: retry → provider health refresh (LlmProviderModelUsecase),
// reconnect → MCP health refresh (MCPServerUsecase). The confidence gate is
// metric-driven (base × signal), so actions only fire on real metric signals;
// both executors are idempotent read-only probes. See jobs.PredictiveHealJobEnabled.
func providePredictiveHealUsecase(uc *biz.MonitorUsecase, patternReader heal.FailurePatternReader, healRepo heal.HealRecordRepo, providerUC *biz.LlmProviderModelUsecase, mcpUC *biz.MCPServerUsecase, lg loggateway.Logger) *heal.PredictiveHealUsecase {
	metricsReader := heal.NewMonitorSystemMetricsReader(uc)
	handler := heal.NewCatalogHealActionHandler(lg).
		BindRetry(providerUC).
		BindReconnect(mcpUC)
	return heal.NewPredictiveHealUsecase(metricsReader, patternReader, handler, healRepo, lg)
}

func providePredictiveHealJob(uc *heal.PredictiveHealUsecase, lg loggateway.Logger) *jobs.PredictiveHealJob {
	if !jobs.PredictiveHealJobEnabled() {
		return nil
	}
	return jobs.NewPredictiveHealJob(0, uc, lg)
}

func providePatternMiningUsecase(healRepo heal.HealRecordRepo, patternReader heal.FailurePatternReader, patternWriter heal.FailurePatternWriter, lg loggateway.Logger) *heal.PatternMiningUsecase {
	return heal.NewPatternMiningUsecase(healRepo, patternReader, patternWriter, lg)
}

func providePatternMiningJob(uc *heal.PatternMiningUsecase, lg loggateway.Logger) *jobs.PatternMiningJob {
	return jobs.NewPatternMiningJob(0, uc, lg)
}

func provideVerificationGateExecutor(deptLeadMgr *biz.DeptLeadManager, caller biz.LLMCaller, skillUC *biz.SkillUsecase, lg loggateway.Logger) *biz.VerificationGateExecutor {
	// 2026-07-28 F9 修复：注入 tool_assertion 门的确定性 invoker（白名单
	// cli_admin_skill_get，由 SkillUsecase 直供）。此前生产缺该注入，
	// tool_assertion 门恒 fail-closed 报 "no tool invoker configured"。
	return biz.NewVerificationGateExecutor(deptLeadMgr, caller, lg,
		biz.WithToolAssertionInvoker(biz.NewSkillAssertionInvoker(skillUC)))
}

func provideSpiritTeamUsecase(teamUC *biz.TeamUsecase, sessionUC *biz.SessionUsecase, agentUC *biz.AgentUsecase, transactor biz.SpiritTransactor, orchCache *biz.OrchestrationCache, evolutionUC *biz.EvolutionUsecase, gateExecutor *biz.VerificationGateExecutor, deptLeadMgr *biz.DeptLeadManager, stepReader biz.StepV2Reader, runStatsReader biz.SpiritTeamRunStatsReader, sessionRT *araneasession.Runtime, sysRepo biz.SystemSettingRepo, lg loggateway.Logger) *biz.SpiritTeamUsecase {
	return biz.NewSpiritTeamUsecase(teamUC, sessionUC, agentUC, lg,
		biz.WithSpiritTransactor(transactor),
		biz.WithOrchestrationCache(orchCache),
		biz.WithEvolutionSuggestionCreator(evolutionUC),
		biz.WithVerificationGateExecutor(gateExecutor),
		biz.WithDeptLeadMgr(deptLeadMgr),
		biz.WithSpiritStepReader(stepReader),
		biz.WithSpiritTeamRunStatsReader(runStatsReader),
		biz.WithGraphDeliverableReader(service.NewGraphDeliverableReader(sessionRT)),
		biz.WithTeamInboxFS(service.NewTeamInboxFS(sysRepo)),
	)
}

func provideChannelDeliveryScanner(worker *service.ChannelDeliveryWorker, lg loggateway.Logger, flowLog biz.FlowLogWriter) *jobs.ChannelDeliveryWorker {
	if strings.TrimSpace(os.Getenv("CHANNEL_DELIVERY_DISABLED")) == "1" {
		return nil
	}
	return jobs.NewChannelDeliveryWorker(0, worker, lg, flowLog)
}

func provideMCPHealthRunnerDeps(mcpRepo biz.MCPServerReader, mcpUC *biz.MCPServerUsecase, agentUC *biz.AgentUsecase, monitorBus contract.MonitorBus, lg loggateway.Logger) health.Deps {
	return health.Deps{
		MCP:    mcpRepo,
		UC:     mcpUC,
		Alerts: alert.NewPublisher(monitorBus, mcpUC, lg),
		// P0-3：DOWN→UP 恢复边沿 → 反查引用该 server 的 agent 并失效构建缓存，
		// 下次请求重建时拿到恢复后的健康 toolset（掉线期间装配的旧 toolset 摘掉）。
		OnServerRecovered: func(ctx context.Context, srv biz.MCPServer) {
			serverKey := strings.TrimSpace(srv.Key)
			agentIDs, err := agentUC.AgentIDsReferencingMCPServer(ctx, serverKey)
			if err != nil {
				lg.Warn("MCP 恢复反查受影响 agent 失败",
					loggateway.StepID("mcp.health_recovered"),
					loggateway.Str("server_key", serverKey),
					loggateway.Err(err))
				return
			}
			if len(agentIDs) == 0 {
				return
			}
			for _, id := range agentIDs {
				agent.InvalidateAgentCache(id)
			}
			lg.Info("MCP 恢复已失效关联 agent 构建缓存",
				loggateway.StepID("mcp.health_recovered"),
				loggateway.Str("server_key", serverKey),
				loggateway.Int("invalidated_agents", len(agentIDs)))
		},
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

// ────────────────────────────────────────────────────────────
// M81 配置资产图谱（config graph）providers
//
// 依赖链：raw-SQL repos（datacg）→ Rebuilder（双代重建）→ Indexer（后台组
// 件，P0 仅重建能力）→ ConfigGraphService（HTTP，经 indexer.Rebuilder() 满
// 足窄面 ConfigGraphRebuilderPort）。EffectiveToolsProvider 绑定
// *biz.AgentUsecase（同进程建图，design R1）。
// ────────────────────────────────────────────────────────────

func provideConfigGraphRebuilder(src bizcg.SourceRepo, repo bizcg.Repo, provider bizcg.EffectiveToolsProvider, flowLog monitor.FlowLogWriter, lg loggateway.Logger) *bizcg.Rebuilder {
	return bizcg.NewRebuilder(src, repo, provider, flowLog, lg)
}

func provideConfigGraphIndexer(rebuilder *bizcg.Rebuilder, lg loggateway.Logger) *bizcg.Indexer {
	return bizcg.NewIndexer(rebuilder, lg)
}

func provideConfigGraphService(indexer *bizcg.Indexer, repo bizcg.Repo, lg loggateway.Logger) *service.ConfigGraphService {
	if indexer == nil {
		return nil
	}
	return service.NewConfigGraphService(indexer.Rebuilder(), repo, lg)
}

// ────────────────────────────────────────────────────────────
// 79-runtime-governance R8 运行时体检（diagnostics）providers
//
// 聚合六个检查项的数据源（CS-B7 窄接口，由各域 usecase 直接满足）：
// model_providers=*biz.LlmProviderModelUsecase、mcp_servers=*biz.MCPServerUsecase、
// tool_assembly=*biz.AgentUsecase、memory_stack=biz.MemoryFactPendingStore、
// cache_baseline=bizusage.CacheHitRatioStatsRepo（类型断言收窄，与 monitor
// 的 cache-hit 告警同源）、config_graph=*bizcg.Querier（indexer 缺省时该项
// 缺席，对未启用部署透明）。
// ────────────────────────────────────────────────────────────

func provideDiagnosticsUsecase(
	catalog *biz.LlmProviderModelUsecase,
	mcpUC *biz.MCPServerUsecase,
	agentUC *biz.AgentUsecase,
	d *data.Data,
	usageRepo biz.UsageRepo,
	indexer *bizcg.Indexer,
	cgRepo bizcg.Repo,
	lg loggateway.Logger,
) *diagnostics.Usecase {
	deps := diagnostics.UsecaseDeps{
		ProviderModels: catalog,
		MCPServers:     mcpUC,
		ToolAssembly:   agentUC,
		Lg:             lg,
	}
	if d != nil {
		deps.MemPending = data.NewMemoryFactPendingRepoFromData(d)
	}
	if ch, ok := usageRepo.(bizusage.CacheHitRatioStatsRepo); ok && ch != nil {
		deps.CacheStats = ch
	}
	if indexer != nil {
		deps.ConfigGraph = bizcg.NewQuerier(cgRepo, indexer.Rebuilder().Current)
	}
	return diagnostics.NewUsecase(deps)
}

func provideDiagnosticsService(uc *diagnostics.Usecase, lg loggateway.Logger) *service.DiagnosticsService {
	return service.NewDiagnosticsService(uc, lg)
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
	LearningLoopScanner         *jobs.LearningLoopScanner
	SkillIntelligenceWorker     *jobs.SkillIntelligenceWorker
	CuratorWorker               *jobs.CuratorWorker
	EvolutionOrchestratorWorker *jobs.EvolutionOrchestratorWorker
	SelfImproveObserveWorker    *jobs.SelfImproveObserveWorker
	SelfImproveDriveWorker      *jobs.SelfImproveDriveWorker
	SelfImproveWatchdogWorker   *jobs.SelfImproveWatchdogWorker
	SelfImproveOutcomeWorker    *jobs.SelfImproveOutcomeWorker
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
	MonitorEventsCleanup        *jobs.MonitorEventsCleanup
	AutoHealTTLCleanup          *jobs.AutoHealTTLCleanup
	MonitorAlertEvalWorker      *monitor.AlertEvalWorker
	MonitorTraceBackfillWorker  *jobs.MonitorTraceBackfillWorker
	MemoryL2Decay               *jobs.MemoryL2DecayWorker
	MemoryL1Archive             *jobs.MemoryL1ArchiveWorker
	ChannelTurnJobSweeper       *jobs.ChannelTurnJobSweeper
	MemoryL3Decay               *jobs.MemoryL3DecayWorker
	MemoryL4Decay               *jobs.MemoryL4DecayWorker
	MemoryEbbinghausDecay       *jobs.MemoryEbbinghausDecayWorker
	MemoryCanary                *jobs.MemoryCanaryWorker
	MemoryCitationBackfill      *jobs.MemoryCitationBackfillWorker
	KnowledgeCitationBackfill   *jobs.KnowledgeCitationBackfillWorker
	KnowledgeRelationExtract    *jobs.KnowledgeRelationExtractWorker
	KnowledgeIndexRepair        *jobs.KnowledgeIndexRepairWorker
	KnowledgeCurate             *jobs.KnowledgeCurateWorker
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
	PredictiveHealUsecase       *heal.PredictiveHealUsecase
	PredictiveHealJob           *jobs.PredictiveHealJob
	PatternMiningUsecase        *heal.PatternMiningUsecase
	PatternMiningJob            *jobs.PatternMiningJob
	PathBExtractor              *biz.PathBExtractor
	// WSV2Subscriber forwards v2 Events to WS clients. Owned by wireOut so
	// its lifecycle (Close) is managed alongside the kratos.App shutdown.
	WSV2Subscriber *server.WSV2Subscriber
	// ConfigGraphIndexer is the M81 config-asset graph background component
	// (P0: rebuild capability only; started by startBackgroundWorkers).
	ConfigGraphIndexer *bizcg.Indexer
}

func provideWireOut(
	app *kratos.App,
	dataData *data.Data,
	runner *cronrunner.Runner,
	skillWatch *watch.Runner,
	autoMem *jobs.AutoMemoryWorker,
	mcpHealth *health.Runner,
	a2aHealth *a2ahealth.Runner,
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
	monitorEventsCleanup *jobs.MonitorEventsCleanup,
	autoHealTTLCleanup *jobs.AutoHealTTLCleanup,
	monitorAlertEvalWorker *monitor.AlertEvalWorker,
	monitorTraceBackfillWorker *jobs.MonitorTraceBackfillWorker,
	memoryL2Decay *jobs.MemoryL2DecayWorker,
	memoryL1Archive *jobs.MemoryL1ArchiveWorker,
	channelTurnJobSweeper *jobs.ChannelTurnJobSweeper,
	memoryL3Decay *jobs.MemoryL3DecayWorker,
	memoryL4Decay *jobs.MemoryL4DecayWorker,
	memoryEbbinghausDecay *jobs.MemoryEbbinghausDecayWorker,
	memoryCanary *jobs.MemoryCanaryWorker,
	memoryCitationBackfill *jobs.MemoryCitationBackfillWorker,
	knowledgeCitationBackfill *jobs.KnowledgeCitationBackfillWorker,
	knowledgeRelationExtract *jobs.KnowledgeRelationExtractWorker,
	knowledgeIndexRepair *jobs.KnowledgeIndexRepairWorker,
	knowledgeCurate *jobs.KnowledgeCurateWorker,
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
	skillIntelligenceWorker *jobs.SkillIntelligenceWorker,
	curatorWorker *jobs.CuratorWorker,
	evoOrchWorker *jobs.EvolutionOrchestratorWorker,
	siObserveWorker *jobs.SelfImproveObserveWorker,
	siDriveWorker *jobs.SelfImproveDriveWorker,
	siWatchdogWorker *jobs.SelfImproveWatchdogWorker,
	siOutcomeWorker *jobs.SelfImproveOutcomeWorker,
	failurePatternSyncJob *jobs.FailurePatternSyncJob,
	predictiveHealUsecase *heal.PredictiveHealUsecase,
	predictiveHealJob *jobs.PredictiveHealJob,
	patternMiningUsecase *heal.PatternMiningUsecase,
	patternMiningJob *jobs.PatternMiningJob,
	pathBExtractor *biz.PathBExtractor,
	wsV2Sub *server.WSV2Subscriber,
	configGraphIndexer *bizcg.Indexer,
) wireOut {
	return wireOut{
		App: app, Data: dataData, CronRunner: runner, SkillWatch: skillWatch, AutoMemory: autoMem,
		MCPHealthProbe: mcpHealth, A2AGatewayHealthProbe: a2aHealth, LearningLoopScanner: learningLoop, ProviderHealthScanner: providerHealth,
		ChannelHealthScanner: channelHealth, ChannelDeliveryScanner: channelDelivery,
		SessionRunDurableWorker: sessionRunDurable,
		RecoveryWorker:          recoveryWorker,
		BackgroundJobWorker:     backgroundJobWorker,
		ChannelRuntime:          channelRuntime,
		PluginRuntime:           pluginRuntime,
		ToolAuditCleanup:        toolAuditCleanup,
		FlowLogCleanup:          flowLogCleanup, MonitorEventsCleanup: monitorEventsCleanup, AutoHealTTLCleanup: autoHealTTLCleanup, MonitorAlertEvalWorker: monitorAlertEvalWorker, MonitorTraceBackfillWorker: monitorTraceBackfillWorker, MemoryL2Decay: memoryL2Decay, MemoryL1Archive: memoryL1Archive, ChannelTurnJobSweeper: channelTurnJobSweeper, MemoryL3Decay: memoryL3Decay, MemoryL4Decay: memoryL4Decay,
		MemoryEbbinghausDecay:       memoryEbbinghausDecay,
		MemoryCanary:                memoryCanary,
		MemoryCitationBackfill:      memoryCitationBackfill,
		KnowledgeCitationBackfill:   knowledgeCitationBackfill,
		KnowledgeRelationExtract:    knowledgeRelationExtract,
		KnowledgeIndexRepair:        knowledgeIndexRepair,
		KnowledgeCurate:             knowledgeCurate,
		MemorySleepTime:             memorySleepTime,
		MemoryEpisodeBackfill:       memoryEpisodeBackfill,
		MemoryDataMigration:         memoryDataMigration,
		MemoryFactIndexReconciler:   memoryFactIndexReconciler,
		MemoryDeadLetterReplayer:    memoryDeadLetterReplayer,
		ModelRegistrySyncAgent:      modelRegistrySyncAgent,
		SelfCheckScheduler:          selfCheckScheduler,
		SelfHealObserver:            selfHealObserver,
		MonitorBus:                  eventInfra.MonitorEventBus,
		SelfCheckCleanup:            selfCheckCleanup,
		SelfCheckJob:                selfCheckJob,
		CronRepo:                    cronRepo,
		SkillIntelligence:           skillIntelligence,
		SkillIntelligenceWorker:     skillIntelligenceWorker,
		CuratorWorker:               curatorWorker,
		EvolutionOrchestratorWorker: evoOrchWorker,
		SelfImproveObserveWorker:    siObserveWorker,
		SelfImproveDriveWorker:      siDriveWorker,
		SelfImproveWatchdogWorker:   siWatchdogWorker,
		SelfImproveOutcomeWorker:    siOutcomeWorker,
		FailurePatternSyncJob:       failurePatternSyncJob,
		PredictiveHealUsecase:       predictiveHealUsecase,
		PredictiveHealJob:           predictiveHealJob,
		PatternMiningUsecase:        patternMiningUsecase,
		PatternMiningJob:            patternMiningJob,
		PathBExtractor:              pathBExtractor,
		WSV2Subscriber:              wsV2Sub,
		ConfigGraphIndexer:          configGraphIndexer,
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
// ActivityBridge upsert path removed; typed v2 events persist via RepoSet.
// P1-R2b: durable dead-letter store enables restart-surviving replay of
// permanently failed entity persists.
func provideV2Sequencer(rs v2.RepoSet, bus *event.V2Bus, outbox biz.EventDeliveryOutboxRepo, dlStore biz.EventDeadLetterRepo, lg loggateway.Logger) *v2.Sequencer {
	return v2.NewSequencer(rs, bus, lg, v2.WithEventOutbox(outbox), v2.WithDeadLetterStore(dlStore))
}

// provideWSServer constructs the WS server and attaches the durable outbox for
// last_event_id critical-event replay (B-06).
func provideWSServer(
	c *conf.Server,
	infra *event.Infra,
	canceller server.RunCanceller,
	sender server.ChatSender,
	turnExecutor server.WSTurnExecutor,
	runtimeConf *conf.Runtime,
	lg loggateway.Logger,
	eventBus biz.EventBus,
	sessionAuth server.SessionAuthorizer,
	outbox biz.EventDeliveryOutboxRepo,
	resumer server.TaskResumer,
	catalogPusher server.SkillCatalogPusher,
	clientBridge *clientbridge.Bridge,
) *server.WSServer {
	srv := server.NewWSServerFromInfra(c, infra, canceller, sender, turnExecutor, runtimeConf, lg, eventBus, sessionAuth)
	if srv != nil {
		srv.SetEventOutbox(outbox)
		srv.SetTaskResumer(resumer)
		srv.SetSkillCatalogPusher(catalogPusher)
		srv.SetClientToolBridge(clientBridge)
	}
	return srv
}

// provideSpeechRegistry constructs the speech provider registry (volcengine
// ASR/TTS factories registered by default). M74 voice companion.
func provideSpeechRegistry() *speech.Registry { return speech.NewRegistry() }

// provideSpeechConfigReader constructs the System Settings backed speech config
// reader (V2-T7): DB-first field-level merge with SPEECH_* env fallback. The
// port (biz.SpeechConfigReader) is unchanged from the V1 env implementation.
func provideSpeechConfigReader(repo biz.SystemSettingRepo, lg loggateway.Logger) biz.SpeechConfigReader {
	return speech.NewSystemSpeechConfigReader(repo, lg)
}

// provideVoiceDelegationRegistry 提供语音委派登记表进程级单例（M74 V9，
// 设计 74 §15.4-C）。双向消费：service 层 voiceButlerTools（Register/
// MarkSubmitFailed）+ server 层 VoiceWSServer.SetDelegationRegistry（
// eventLoop 三路分流 BindTask/CompleteTask/SetWatcher）。
func provideVoiceDelegationRegistry(lg loggateway.Logger) *voice.DelegationRegistry {
	return voice.NewDelegationRegistry(lg)
}

// provideVoiceWSServer constructs the /v1/voice WS gateway. Provider 工厂
// 闭包按当前配置懒解析 ASR/TTS Provider（每次 voice.start 重新读配置）。
// V2-T5：注入语音确认 resolver（service 层适配 voice.ConfirmResolver）。
// V2-T6：注入语音留档 archiver（service 层适配 voice.AudioArchiver；
// ASRSessionConfig.Driver 透传给消息元数据 asr_provider）。
func provideVoiceWSServer(
	sessionAuth server.SessionAuthorizer,
	turnExecutor server.WSTurnExecutor,
	canceller server.RunCanceller,
	registry *speech.Registry,
	cfgReader biz.SpeechConfigReader,
	eventBus biz.EventBus,
	infra *event.Infra,
	lg loggateway.Logger,
	chatService *service.ChatService,
	artifactUC *biz.ArtifactUsecase,
	voiceDelegation *voice.DelegationRegistry,
	stepReader biz.StepV2Reader,
	embedder *knowledge.MultiProviderEmbedder,
) *server.VoiceWSServer {
	newASR := func(ctx context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
		cfg, err := cfgReader.ASRConfig(ctx)
		if err != nil {
			return nil, biz.ASRSessionConfig{}, err
		}
		p, err := registry.ASRProvider(cfg, lg)
		if err != nil {
			return nil, biz.ASRSessionConfig{}, err
		}
		return p, biz.ASRSessionConfig{Driver: cfg.Driver, Language: cfg.Language, SampleRate: 16000}, nil
	}
	newTTS := func(ctx context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
		cfg, err := cfgReader.TTSConfig(ctx)
		if err != nil {
			return nil, biz.TTSSessionConfig{}, err
		}
		p, err := registry.TTSProvider(cfg, lg)
		if err != nil {
			return nil, biz.TTSSessionConfig{}, err
		}
		return p, biz.TTSSessionConfig{Voice: cfg.Voice, SpeedRatio: cfg.SpeedRatio, SampleRate: 16000}, nil
	}
	archiver := service.NewVoiceAudioArchiver(artifactUC, cfgReader, lg)
	// V2-T8 差距2：麦克风置灰门控的可用性探测——复用同一 DB-first/env-fallback
	// 配置读取，Validate 通过即视为可用（与 voice.start 的 openASR 判定同源）。
	probe := func(ctx context.Context) (bool, bool) {
		_, asrErr := cfgReader.ASRConfig(ctx)
		_, ttsErr := cfgReader.TTSConfig(ctx)
		return asrErr == nil, ttsErr == nil
	}
	// C1：voice.start 预热 Agent 构建缓存（nil-safe：chatService 异常时返回 nil 即关闭）。
	// C3：并列触发 embedding 冷启动预热（nil-safe 同上）。
	prewarmer := service.NewVoiceTurnPrewarmer(chatService, embedder)
	// C2：ASR partial 稳定 500ms 投机意图（nil-safe 同上）。
	speculator := service.NewVoiceIntentSpeculator(chatService)
	srv := server.NewVoiceWSServer(sessionAuth, turnExecutor, canceller, newASR, newTTS, eventBus, infra, lg, service.NewVoiceConfirmResolver(chatService), archiver, probe)
	srv.SetTurnPrewarmer(prewarmer)
	srv.SetIntentSpeculator(speculator)
	// M74 V9：委派登记表 + 终稿读取（eventLoop 三路分流/播报，设计 74 §15.4-C/D）。
	srv.SetDelegationRegistry(voiceDelegation, stepReader)
	return srv
}

// provideV2ProjectorFactory constructs the v2 ProjectorFactory that produces
// per-turn ActivityProjector instances. Each turn (spirit + each team member)
// gets its own Projector instance, isolating per-turn streaming state.
// The factory shares the singleton Sequencer + SeqAssigner so Seq allocation
// remains globally monotonic per spirit session.
// taskReader 用于 synthesis/cancelled 兜底路径回读父 Task 不可变字段
// （CreatedAt/Seq/UserMessage），避免 task.completed 事件携带零值时间。
func provideV2ProjectorFactory(seq *v2.Sequencer, taskReader biz.TaskV2Reader, lg loggateway.Logger) *v2.ProjectorFactory {
	return v2.NewProjectorFactory(seq, seq.SeqAssigner(), taskReader, lg)
}

// provideWSV2Subscriber constructs the WS subscriber for v2 Events.
// Subscribe is called synchronously in the constructor to avoid missing
// events between construction and goroutine startup.
// Outbox wiring for last_event_id replay is done in provideWSServer.
func provideWSV2Subscriber(bus *event.V2Bus, wsSrv *server.WSServer, lg loggateway.Logger) *server.WSV2Subscriber {
	return server.NewWSV2Subscriber(bus, wsSrv, lg)
}

// providePlanExecutor constructs the v2 forward DAG scheduler.
// The PlanExecutor is injected into TeamStarter via SetPlanExecutor
// (called in ProvideChatService).
// taskPlanRepo 用于 TS9-BUG-1：PlanBoard 生命周期传播到 TaskPlan 状态。
func providePlanExecutor(
	planStep biz.PlanStepV2Repo,
	teamStage biz.TeamStageV2Repo,
	planBoard biz.PlanBoardV2Repo,
	graphStage biz.GraphStageV2Repo,
	graphNode biz.GraphNodeV2Repo,
	orch service.TeamOrchestrator,
	seq *v2.Sequencer,
	taskPlanRepo biz.TaskPlanRepository,
	lg loggateway.Logger,
) *service.PlanExecutor {
	pe := service.NewPlanExecutorFromV2Repos(planStep, teamStage, planBoard, graphStage, graphNode, orch, seq, lg)
	pe.SetTaskPlanUpdater(taskPlanRepo)
	return pe
}

// provideTeamOrchestrator returns the real TeamOrchestrator (Phase 2).
// assembler 和 starter 通过 ProvideChatService 后注入（打破 Wire 循环）。
func provideTeamOrchestrator(lg loggateway.Logger) *service.RealTeamOrchestrator {
	return service.NewRealTeamOrchestrator(lg)
}

func provideA2APublicBaseInput(c *conf.Server) a2apkg.PublicBaseURLInput {
	configURL := ""
	if c != nil {
		configURL = c.GetA2APublicBaseUrl()
	}
	addr := ":8800"
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

// --- Federation (design F.5/F.6) ---

// provideFederationLimiterFactory builds per-policy per-minute limiters with
// the same Redis-vs-memory backend decision as provideA2ALimiter.
func provideFederationLimiterFactory(rdb *data.RedisClient, lg loggateway.Logger) a2abiz.FederationLimiterFactory {
	var client *redis.Client
	if rdb != nil && rdb.IsEnabled() {
		client = rdb.Client
	}
	return func(maxPerMin int) a2abiz.Limiter {
		return a2abiz.NewLimiter(a2abiz.LimiterConfig{
			WindowSize: time.Minute,
			MaxInvokes: maxPerMin,
			KeyPrefix:  "aranea:a2a:fed:",
		}, client, lg)
	}
}

// provideFederationPolicyEngine loads the full policy table at startup; a
// load failure aborts init rather than serving with an empty cache.
func provideFederationPolicyEngine(repo a2abiz.FederationPolicyRepo, lg loggateway.Logger) (*a2abiz.PolicyEngine, error) {
	e := a2abiz.NewPolicyEngine(repo, lg)
	if err := e.Load(context.Background()); err != nil {
		return nil, fmt.Errorf("load federation policies: %w", err)
	}
	return e, nil
}

func provideFederationGovernance(
	policy *a2abiz.PolicyEngine,
	audits a2abiz.FederationAuditRepo,
	factory a2abiz.FederationLimiterFactory,
	lg loggateway.Logger,
) *a2abiz.FederationGovernance {
	return &a2abiz.FederationGovernance{
		Trust:  a2abiz.NewTrustManager(lg),
		Policy: policy,
		Quota:  a2abiz.NewQuotaChecker(policy, audits, factory, lg),
		Audit:  a2abiz.NewAuditLogger(audits, lg),
	}
}

func provideFederationRemoteInvoker(lg loggateway.Logger) a2abiz.RemoteInvokeExecutor {
	return a2apkg.NewFederationRemoteInvoker(lg)
}

func provideFederationUsecase(
	orgs a2abiz.FederationOrgRepo,
	gov *a2abiz.FederationGovernance,
	remotes a2abiz.RemoteAgentLister,
	discoverer a2abiz.RemoteCardDiscoverer,
	cardWriter a2abiz.RemoteAgentCardWriter,
	executor a2abiz.RemoteInvokeExecutor,
	lg loggateway.Logger,
	flowLog biz.FlowLogWriter,
) *a2abiz.FederationUsecase {
	return a2abiz.NewFederationUsecase(orgs, gov,
		a2abiz.NewDirectory(orgs, remotes),
		a2abiz.NewAgentCardSync(remotes, discoverer, cardWriter, lg),
		remotes, executor,
		a2apkg.NewFederationFlowLogWriter(flowLog))
}

func provideFederationService(uc *a2abiz.FederationUsecase) *service.FederationService {
	return service.NewFederationService(uc)
}

func provideTaskPlanner(repo biz.TaskPlanRepository, catalog *biz.LlmProviderModelUsecase, orchCache *biz.OrchestrationCache, eventBus biz.EventBus, lg loggateway.Logger, sysUC *biz.SystemSettingUsecase, seq *v2.Sequencer, agentReader biz.AgentReader, orgReader biz.OrganizationReader, decisions decision.Collector) biz.TaskPlannerPort {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	p := chatagent.NewTaskPlanner(repo, catalog, httpClient, eventBus, orchCache, lg, sysUC, seq, agentReader)
	chatagent.AttachPlannerOrganizationReader(p, orgReader)
	// M80 1.4: planner 双写 — emitPlannerDecision 内增 Emit，4 调用点全覆盖。
	chatagent.AttachPlannerDecisionCollector(p, decisions)
	return p
}

func provideAgentAllocator(
	repo biz.AllocationPlanRepository,
	agentReader biz.AgentReader,
	perfRepo biz.AgentPerformanceRepository,
	orchCache *biz.OrchestrationCache,
	catalog *biz.LlmProviderModelUsecase,
	eventBus biz.EventBus,
	embedder knowledge.Embedder,
	agentFactory biz.AgentFactory,
	lg loggateway.Logger,
	sysUC *biz.SystemSettingUsecase,
	orgReader biz.OrganizationReader,
) biz.AgentAllocatorPort {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	capBuilder := chatagent.NewAgentCapabilityBuilder(agentReader, lg)
	capBuilder.SetOrganizationReader(orgReader)
	return chatagent.NewAgentAllocator(repo, agentReader, perfRepo, orchCache, capBuilder, catalog, httpClient, eventBus, lg, embedder, agentFactory, sysUC)
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
	embedder knowledge.Embedder,
	lg loggateway.Logger,
	orgReader biz.OrganizationReader,
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
	factory := chatagent.NewAgentFactoryImpl(llm, agentWriter, agentReader, templateRepo, eventBus, embedder, lg)
	if impl, ok := factory.(*chatagent.AgentFactoryImpl); ok {
		impl.SetOrganizationReader(orgReader)
	}
	return factory
}

func provideTaskOrchestrator(
	assembler *service.SpiritTeamAssembler,
	repo biz.OrchestrationRepository,
	taskPlanRepo biz.TaskPlanRepository,
	allocPlanRepo biz.AllocationPlanRepository,
	synthesis *service.SpiritSynthesisService,
	checkpointSaver trpcgraph.CheckpointSaver,
	orchCache *biz.OrchestrationCache,
	perfRepo biz.AgentPerformanceRepository,
	evolutionUC *biz.EvolutionUsecase,
	eventBus biz.EventBus,
	lg loggateway.Logger,
) biz.TaskOrchestratorPort {
	// ADR-2（2026-08-20）：Orchestrate 死路径删除后，编排器仅剩 handle 记账 +
	// 在线学习依赖；assembler 作为 SpiritTeamControllerPort 传入（Cancel/CheckProgress）。
	return chatagent.NewTaskOrchestratorImpl(assembler, repo, taskPlanRepo, allocPlanRepo, synthesis, checkpointSaver, orchCache, perfRepo, evolutionUC, eventBus, lg)
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
	activeLister biz.TeamActiveRunLister,
	stepRepo biz.OrchestrationStepRepo,
	deadLetter biz.TaskDeadLetterRepo,
	agentChecker biz.AgentIDExistenceChecker,
	deptLeadMgr *biz.DeptLeadManager,
	graphReader biz.GraphReader,
	graphWriter biz.GraphWriter,
	teamCompiler biz.TeamCompiler,
	graphAssets biz.TeamGraphAssetStore,
	txProvider biz.TeamTxProvider,
	linkedReader biz.TeamLinkedGraphReader,
	agentIDResolver biz.TeamAgentIDResolver,
	channels *biz.ChannelUsecase,
	lg loggateway.Logger,
) biz.TeamUsecaseOpts {
	return biz.TeamUsecaseOpts{
		Reader:       reader,
		Writer:       writer,
		RunReader:    runReader,
		RunWriter:    runWriter,
		ActiveLister: activeLister,
		StepRepo:     stepRepo,
		DeadLetter:   deadLetter,
		AgentChecker: agentChecker,
		DeptLeadMgr:  deptLeadMgr,
		GraphReader:  graphReader,
		GraphWriter:  graphWriter,
		// Team × Graph 一体化（Phase 11）保存钩子
		Compiler:         teamCompiler,
		GraphAssets:      graphAssets,
		TxProvider:       txProvider,
		AgentKeyResolver: biz.TeamAgentKeyResolver(channels.AgentKeyResolver),
		AgentIDResolver:  agentIDResolver,
		LinkedReader:     linkedReader,
		Lg:               lg,
	}
}

// provideTeamAgentIDResolver wires the agent_key → agent_id reverse resolver
// for B6 member sync（Graph 编辑器保存 team-owned 图后回写 members）。
func provideTeamAgentIDResolver(agents biz.AgentRepository) biz.TeamAgentIDResolver {
	return func(ctx context.Context) func(agentKey string) (string, bool) {
		return func(agentKey string) (string, bool) {
			ag, err := agents.GetAgentByAgentKey(ctx, agentKey)
			if err != nil {
				return "", false
			}
			return ag.ID, true
		}
	}
}

// provideTeamUsecase wraps biz.NewTeamUsecase to inject the TeamGraphGuard
// into GraphDefinitionUsecase（B6 反向同步 + B7 删除保护）——跨 usecase 的
// 装配步骤，wire 无法经裸构造函数表达，故 NewTeamUsecase 从 biz.ProviderSet
// 移出在此包装。
func provideTeamUsecase(opts biz.TeamUsecaseOpts, graphs *biz.GraphUsecase) *biz.TeamUsecase {
	uc := biz.NewTeamUsecase(opts)
	graphs.DefUC().SetTeamGraphGuard(uc)
	return uc
}

func provideMemoryLLMExtractorConfig(
	agents *biz.AgentUsecase,
	sessions *biz.SessionUsecase,
	modelCatalog *biz.LlmProviderModelUsecase,
	usageRef *biz.UsageUsecaseRef,
	lg loggateway.Logger,
) service.MemoryLLMExtractorConfig {
	return service.MemoryLLMExtractorConfig{
		Agents:       agents,
		Sessions:     sessions,
		ModelCatalog: modelCatalog,
		RoundTrip:    &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}},
		LLMDisabled:  false,
		UsageRef:     usageRef,
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
func wireApp(*conf.Server, *conf.Data, *conf.Runtime, *conf.SelfImprovement, *conf.Sandbox, *conf.DebugRecorder, log.Logger, loggateway.Logger, logpipeline.Pipeline, []*conf.LoggingSink) (wireOut, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		event.InfraProviderSet,
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
		provideGlobalBuildCache,
		provideAgentPolicyResolver,
		provideVoiceDelegationRegistry,
		provideMCPToolSetPool,
		provideGlobalShardCache,
		provideLifecycleManager,
		provideDeadLetterQueue,
		provideRunHeartbeatEmitter,
		providePendingMessageQueue,
		provideCodeExecutorFactory,
		// M82: sandbox Manager + admin service; the admin port binds onto the
		// Manager (read-only list/metrics surface, ADR-82-3). SessionLeases is
		// the P1-1/P1-2 process-wide shared session-lease store, bound to both
		// the codeexecutor sandbox backend and the sandbox_fs toolset.
		provideSandboxManager,
		provideSandboxSessionLeases,
		service.NewSandboxService,
		wire.Bind(new(biz.SandboxAdminPort), new(*sandbox.Manager)),
		// M80: decision record layer — repo + collector (Lifecycle for
		// newApp Start/Stop; Collector bound for the 1.4 planner adapter).
		data.NewDecisionRepoFromData,
		provideDecisionCollector,
		// M80 Phase 1 查询面：QueryRepo → QueryUsecase → DecisionRecordService。
		data.NewDecisionQueryRepoFromData,
		decision.NewQueryUsecase,
		service.NewDecisionRecordService,
		// T5 四轮审查 IDOR 根修：GetSessionGateStats 会话归属校验的窄接口绑定。
		wire.Bind(new(service.SessionWorkspaceReader), new(*bizsession.SessionUsecase)),
		wire.Bind(new(decision.Collector), new(decision.Lifecycle)),
		// M81: config graph — raw-SQL repos → rebuilder → indexer → HTTP API
		// (P0: rebuild/status/nodes only; querier/health land in P1, event
		// subscription in P2).
		datacg.NewSourceRepo,
		datacg.NewRepo,
		provideConfigGraphRebuilder,
		provideConfigGraphIndexer,
		provideConfigGraphService,
		wire.Bind(new(bizcg.EffectiveToolsProvider), new(*biz.AgentUsecase)),
		// 79-runtime-governance R8: diagnostics doctor API（依赖 M81 indexer，
		// 放在 config graph providers 之后）。
		provideDiagnosticsUsecase,
		provideDiagnosticsService,
		provideAutoMemoryQueue,
		wire.Bind(new(memtrpc.AutoMemoryQueue), new(*memtrpc.MemoryJobQueue)),
		wire.Bind(new(biz.MemoryDeadLetterAdminRepo), new(*data.MemoryJobDeadLetterRepo)),
		wire.Bind(new(biz.MemoryDeadLetterSink), new(*data.MemoryJobDeadLetterRepo)),
		provideMemoryPolicyEngine,
		provideFactIndexSync,
		provideMemoryConflictDetector,
		provideL3ConflictStore,
		provideFactWriteAdjudicator,
		provideFactWritePipeline,
		provideMemoryFactPendingNotifier,
		provideMemoryFactPendingDecider,
		provideMemoryFactSessionGrants,
		provideMemoryL2Recall,
		provideMemoryL3Recall,
		provideMemoryCompositeRecall,
		provideMemoryTRPCService,
		provideLinkEvolutionService,
		provideEvolutionService,
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
		// R1: shared skill health-metrics adapter (singleton) — consumed by
		// provideRuntimeTooling (turn routing) and NewSkillService (preview),
		// backed by biz.SkillHealthAggregator (SkillIntelligenceRepo).
		service.NewSkillHealthMetricsAdapter,
		provideTeamOrchestrationDeps,
		provideRunnerConfig,
		// M71: agent resource sharing (memberfs/deptmail/sessionaccess)
		provideResourceAccessUsecase,
		provideDeptMailboxUsecase,
		provideSessionSearchUsecase,
		provideM71MessageReader,
		service.NewMemberDirResolver,
		service.NewMemberFileReader,
		service.NewMailboxWaker,
		wire.Bind(new(biz.OrganizationReader), new(biz.OrganizationRepo)),
		wire.Bind(new(biz.SessionWriter), new(biz.SessionRepo)),
		// v2 event pipeline providers (Phase 1 收尾)
		provideV2EventBus,
		provideV2RepoSet,
		// v2 reader bindings: SessionV2Service consumes the narrow Reader
		// interfaces; the data layer provides the composite Repo interfaces.
		wire.Bind(new(biz.TaskV2Reader), new(biz.TaskV2Repo)),
		wire.Bind(new(biz.TurnV2Reader), new(biz.TurnV2Repo)),
		wire.Bind(new(biz.StepV2Reader), new(biz.StepV2Repo)),
		wire.Bind(new(biz.StepV2Writer), new(biz.StepV2Repo)),
		wire.Bind(new(biz.TeamStageV2Reader), new(biz.TeamStageV2Repo)),
		wire.Bind(new(biz.TeamStageV2Writer), new(biz.TeamStageV2Repo)),
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
		provideWSServer,
		provideWSV2Subscriber,
		provideSpeechRegistry,
		provideSpeechConfigReader,
		provideVoiceWSServer,
		provideTeamOrchestrator,
		wire.Bind(new(service.TeamOrchestrator), new(*service.RealTeamOrchestrator)),
		providePlanExecutor,
		provideTeamTurnDeps,
		provideChannelTurnJobDeps,
		provideChannelNotifierDeps,
		provideRunCanceller,
		provideChatSender,
		provideTaskResumer,
		provideSkillCatalogPusher,
		provideArtifactRuntimeService,
		provideArtifactSigner,
		provideMemoryService,
		provideTRPCSessionService,
		provideGraphCheckpointSaver,
		wire.Bind(new(trpcgraph.CheckpointSaver), new(*graphtrpc.CheckpointSaver)),
		// P1 fix (2026-06-18): Wire previously-orphan graph components into production.
		provideNL2GraphConverter,
		provideRuntimeReplanner,
		providePersistenceSet,
		provideReconsolidationService,
		provideSessionMemoryResync,
		provideL1AdminReader,
		provideL1TaskBoardWriter,
		provideEpisodeIndexSync,
		providePluginStatsRecorder,
		providePluginManager,
		providePluginRuntime,
		graphtrpc.NewRegistry,
		provideGraphBuildDeps,
		provideTRPCBuilderDeps,
		biz.ProvideNodeCircuitBreakerRegistry,
		graphadapter.NewGraphBuilderFactory,
		provideL4CascadeUsecase,
		provideAutoMemoryWorker,
		provideKnowledgeWriteBackArbiter,
		provideL4GraphWriter,
		provideSkillAutoCreator,
		provideSkillRegistrationPort,
		provideSkillUsecase,
		provideSkillMergeUsecase,
		provideSkillEvolutionOrchestrator,
		provideEvolutionUsecase,
		provideLearningLoopUsecase,
		provideEvolutionDrafter,
		provideEvolutionOrchestratorWorker,
		provideSelfImprovementTestRunReader,
		provideSelfImprovementObserveUsecase,
		provideSelfImprovementObserveWorker,
		// 73-self-iteration-v3 W6: Phase 4 chain (sandbox/applier/stages/
		// adapters/usecases/workers), all gated on self_improvement.enabled.
		provideRepoSandboxRunner,
		provideSIControlPlane,
		provideSIAnalystStage,
		provideSIPatcherStage,
		provideSICriticStage,
		provideSINotifier,
		provideSIApprovalSink,
		provideSIActivitySink,
		provideSINegativePatternSink,
		provideSITriggerFeedbackSink,
		provideSIApplier,
		provideSIRiskRules,
		provideSelfImprovementPipelineUsecase,
		provideSelfImprovementApplyUsecase,
		provideSIGovernanceRouter,
		provideSelfImprovementDriveUsecase,
		provideSelfImprovementWatchdogUsecase,
		provideSelfImprovementOutcomeUsecase,
		provideSelfImprovementAdminUsecase, // P5：SelfImprovementService 消费
		provideSelfImprovementService,      // P5.5：始终注册路由（disabled → 503 SELF_IMPROVEMENT）
		provideSelfImproveDriveWorker,
		provideSelfImproveWatchdogWorker,
		provideSelfImproveOutcomeWorker,
		provideSkillIntelligenceWorker,
		provideCuratorWorker,
		provideLearningLoopScanner,
		provideProviderHealthScanner,
		provideChannelHealthScanner,
		provideTeamCompiler,
		provideChannelIngress,
		provideChannelGateCards,
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
		provideMemoryAdminDeps,
		provideMemoryL1ArchiveWorker,
		provideChannelTurnJobSweeper,
		provideMemoryL3DecayWorker,
		provideMemoryL4DecayWorker,
		provideMemoryEbbinghausDecayWorker,
		provideMemoryCanaryWorker,
		provideMemoryCitationBackfillWorker,
		provideKnowledgeCitationBackfillWorker,
		provideKnowledgeRelationExtractWorker,
		provideKnowledgeIndexRepairWorker,
		provideKnowledgeCurateWorker,
		provideKnowledgeEntityPipeline,
		provideKnowledgeRelationExtractor,
		provideKnowledgeWriteBackGraphHook,
		provideKnowledgeDistillWiring,
		provideMemorySleepTimeWorker,
		provideMemoryEpisodeBackfillWorker,
		provideMemoryDataMigrationWorker,
		provideMemoryFactIndexReconciler,
		provideMemoryDeadLetterReplayer,
		provideToolAuditCleanup,
		provideFlowLogCleanup,
		provideMonitorEventsCleanup,
		provideAutoHealTTLCleanup,
		provideMonitorAlertEvalWorker,
		provideTraceProjector,
		provideFlowFileAppender,
		provideMonitorTraceBackfillWorker,
		provideDiagBundleGenerator,
		provideSelfHealObserver,
		provideSkillIntelligenceUsecase,
		provideLLMSkillEvolver,
		provideSkillVersionReloader,
		provideSkillReplayRunner,
		wire.Bind(new(biz.SkillReplayABRunner), new(*service.SkillReplayRunner)),
		provideSkillTriggerGoldenRunner,
		wire.Bind(new(biz.SkillTriggerGoldenRunner), new(*service.SkillTriggerGoldenRunner)),
		provideSkillGateVerifier,
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
		provideUsageUsecaseRef,
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
		// Federation (design F.5/F.6)
		provideFederationLimiterFactory,
		provideFederationPolicyEngine,
		provideFederationGovernance,
		provideFederationRemoteInvoker,
		provideFederationUsecase,
		provideFederationService,
		wire.Bind(new(a2abiz.FederationOrgRepo), new(*data.A2AFederationRepo)),
		wire.Bind(new(a2abiz.FederationPolicyRepo), new(*data.A2AFederationRepo)),
		wire.Bind(new(a2abiz.FederationAuditRepo), new(*data.A2AFederationRepo)),
		wire.Bind(new(a2abiz.RemoteAgentLister), new(biz.A2ARepo)),
		wire.Bind(new(a2abiz.RemoteCardDiscoverer), new(biz.A2ARepo)),
		provideTaskPlanner,
		provideAgentAllocator,
		provideAgentFactory,
		chatagent.NewProfileResolver,
		provideTaskOrchestrator,
		debug.NewRecorderFactory,
		// PGO-3: DynamicLLMCaller → biz.LLMCaller binding, PromptRefiner.
		provideRefineLLMRoundTrip,
		chatagent.NewDynamicLLMCaller,
		wire.Bind(new(biz.LLMCaller), new(*chatagent.DynamicLLMCaller)),
		wire.Bind(new(biz.SandboxRunner), new(*service.SandboxRunner)),
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
		wire.Bind(new(biz.SkillVersionWriter), new(biz.SkillRepo)),
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
		wire.Bind(new(biz.TeamActiveRunLister), new(*data.TeamRepo)),
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
		wire.Bind(new(heal.FailurePatternReader), new(*data.FailurePatternReadWriter)),
		wire.Bind(new(heal.FailurePatternWriter), new(*data.FailurePatternReadWriter)),
		wire.Bind(new(heal.RootCauseAnalyzer), new(*heal.RootCauseEngine)),
		provideWSTurnExecutor,
		// Kanban bridge binding
		wire.Bind(new(kanbanpkg.Bridge), new(*service.KanbanToolBridge)),
		// Coding agent bridge binding (76-coding-agent-bridge M1-15)
		wire.Bind(new(codingbridge.BridgeService), new(*service.AgentBridgeService)),
		// ToolResultGate bindings
		wire.Bind(new(biz.ToolResultBlobReader), new(*data.ToolResultBlobRepo)),
		wire.Bind(new(biz.ToolResultBlobWriter), new(*data.ToolResultBlobRepo)),
		wire.Bind(new(biz.ToolResultReplacementWriter), new(*data.ToolResultReplacementRepo)),
		wire.Bind(new(biz.ToolResultReplacementReader), new(*data.ToolResultReplacementRepo)),
		// Knowledge embedder bindings
		wire.Bind(new(knowledge.QueryEmbedder), new(*knowledge.MultiProviderEmbedder)),
		wire.Bind(new(knowledge.Embedder), new(*knowledge.MultiProviderEmbedder)),
		// biz.KnowledgeWriteBack = knowledge.SessionWriteBack 别名，生产实现为
		// *service.KnowledgeService（provideAutoMemoryWorker 运行时断言同型）。
		wire.Bind(new(biz.KnowledgeWriteBack), new(*service.KnowledgeService)),
		// AH-04: memory L2/L3 reranker is a knowledge adapter, injected into data.NewData.
		knowledge.NewMemoryReranker,
		// Knowledge vault sync（P1-3 生产装配；M0 编译端口 + M2.1 entity 轨接线）。
		// 注：service.NewKnowledgeExtractorRegistry 由 service.ProviderSet 提供，
		// 此处不再重复列出（重复绑定会导致 wire gen 失败）。
		provideVaultSyncSupervisor,
		provideKnowledgeVaultFiler,
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
		provideTeamUsecase,
		provideTeamAgentIDResolver,
		// SkillSimilarityComparer binding
		wire.Bind(new(biz.SkillSimilarityComparer), new(*biz.SkillSimilarityEngine)),
		// Memory extractor config providers
		provideMemoryLLMExtractorConfig,
		provideMemoryEnhancedExtractorConfig,
		// Bind *ChatService as OpenAIRunnerBuilder for the compat service
		// wrappers (AGUI / OpenAI Session / A2A Extension).
		wire.Bind(new(service.OpenAIRunnerBuilder), new(*service.ChatService)),
		// Bind *ChatService as SessionRunDurableEscalator so SessionStatusGuard
		// can escalate active interactive runs to durable on shutdown (L2).
		wire.Bind(new(service.SessionRunDurableEscalator), new(*service.ChatService)),
		newApp,
		provideWireOut,
	))
}
