package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	a2ahealth "aranea-agents/internal/a2a/health"
	"aranea-agents/internal/biz"
	bizcg "aranea-agents/internal/biz/configgraph"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/event/contract"
	mcphealth "aranea-agents/internal/mcp/health"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// backgroundWorkersConfig holds all optional background workers that main() starts.
type backgroundWorkersConfig struct {
	WatchCtx                    context.Context // separate context for skill watcher
	CronRunner                  *cronrunner.Runner
	SkillWatch                  SkillWatchStarter
	AutoMemory                  BackgroundStarter
	MCPHealthProbe              *mcphealth.Runner
	A2AGatewayHealthProbe       *a2ahealth.Runner
	LearningLoopScanner         BackgroundStarter
	SkillIntelligenceWorker     BackgroundStarter
	CuratorWorker               BackgroundStarter
	EvolutionOrchestratorWorker BackgroundStarter
	SelfImproveObserveWorker    BackgroundStarter
	SelfImproveDriveWorker      BackgroundStarter
	SelfImproveWatchdogWorker   BackgroundStarter
	SelfImproveOutcomeWorker    BackgroundStarter
	ProviderHealthScanner       BackgroundStarter
	ChannelHealthScanner        BackgroundStarter
	ChannelDeliveryScanner      BackgroundStarter
	SessionRunDurableWorker     BackgroundStarter
	RecoveryWorker              BackgroundStarter
	BackgroundJobWorker         BackgroundStarter
	PluginRuntime               PluginRuntimeStarter
	ChannelRuntime              ChannelRuntimeStarter
	ToolAuditCleanup            BackgroundStarter
	FlowLogCleanup              BackgroundStarter
	MonitorEventsCleanup        BackgroundStarter
	AutoHealTTLCleanup          BackgroundStarter
	MonitorAlertEvalWorker      BackgroundStarter
	MonitorTraceBackfillWorker  BackgroundStarter
	SelfCheckScheduler          *monitor.SelfCheckScheduler
	SelfHealObserver            *biz.SelfHealObserver
	MonitorBus                  contract.MonitorBus
	FailurePatternSyncJob       BackgroundStarter
	PredictiveHealJob           BackgroundStarter
	PatternMiningJob            BackgroundStarter
	MemoryL2Decay               BackgroundStarter
	MemoryL1Archive             BackgroundStarter
	ChannelTurnJobSweeper       BackgroundStarter
	MemoryL3Decay               BackgroundStarter
	MemoryL4Decay               BackgroundStarter
	MemoryEbbinghausDecay       BackgroundStarter
	MemoryCanary                BackgroundStarter
	MemoryCitationBackfill      BackgroundStarter
	KnowledgeCitationBackfill   BackgroundStarter
	KnowledgeRelationExtract    BackgroundStarter
	KnowledgeIndexRepair        BackgroundStarter
	KnowledgeCurate             BackgroundStarter
	MemorySleepTime             BackgroundStarter
	MemoryEpisodeBackfill       BackgroundStarter
	MemoryFactIndexReconciler   BackgroundStarter
	MemoryDeadLetterReplayer    BackgroundStarter
	ModelRegistrySyncAgent      any // presence check only
	CronRepo                    biz.CronRepo
	ConfigGraphIndexer          *bizcg.Indexer
}

// backgroundWorkersConfigFromOutput maps the wire output onto the worker
// config (field names align 1:1); WatchCtx is owned by main().
func backgroundWorkersConfigFromOutput(watchCtx context.Context, out *wireOut) *backgroundWorkersConfig {
	return &backgroundWorkersConfig{
		WatchCtx:                    watchCtx,
		CronRunner:                  out.CronRunner,
		SkillWatch:                  out.SkillWatch,
		AutoMemory:                  out.AutoMemory,
		MCPHealthProbe:              out.MCPHealthProbe,
		A2AGatewayHealthProbe:       out.A2AGatewayHealthProbe,
		LearningLoopScanner:         out.LearningLoopScanner,
		SkillIntelligenceWorker:     out.SkillIntelligenceWorker,
		CuratorWorker:               out.CuratorWorker,
		EvolutionOrchestratorWorker: out.EvolutionOrchestratorWorker,
		SelfImproveObserveWorker:    out.SelfImproveObserveWorker,
		SelfImproveDriveWorker:      out.SelfImproveDriveWorker,
		SelfImproveWatchdogWorker:   out.SelfImproveWatchdogWorker,
		SelfImproveOutcomeWorker:    out.SelfImproveOutcomeWorker,
		ProviderHealthScanner:       out.ProviderHealthScanner,
		ChannelHealthScanner:        out.ChannelHealthScanner,
		ChannelDeliveryScanner:      out.ChannelDeliveryScanner,
		SessionRunDurableWorker:     out.SessionRunDurableWorker,
		RecoveryWorker:              out.RecoveryWorker,
		BackgroundJobWorker:         out.BackgroundJobWorker,
		PluginRuntime:               out.PluginRuntime,
		ChannelRuntime:              out.ChannelRuntime,
		ToolAuditCleanup:            out.ToolAuditCleanup,
		FlowLogCleanup:              out.FlowLogCleanup,
		MonitorEventsCleanup:        out.MonitorEventsCleanup,
		AutoHealTTLCleanup:          out.AutoHealTTLCleanup,
		MonitorAlertEvalWorker:      out.MonitorAlertEvalWorker,
		MonitorTraceBackfillWorker:  out.MonitorTraceBackfillWorker,
		SelfCheckScheduler:          out.SelfCheckScheduler,
		SelfHealObserver:            out.SelfHealObserver,
		MonitorBus:                  out.MonitorBus,
		FailurePatternSyncJob:       out.FailurePatternSyncJob,
		PredictiveHealJob:           out.PredictiveHealJob,
		PatternMiningJob:            out.PatternMiningJob,
		MemoryL2Decay:               out.MemoryL2Decay,
		MemoryL1Archive:             out.MemoryL1Archive,
		ChannelTurnJobSweeper:       out.ChannelTurnJobSweeper,
		MemoryL3Decay:               out.MemoryL3Decay,
		MemoryL4Decay:               out.MemoryL4Decay,
		MemoryEbbinghausDecay:       out.MemoryEbbinghausDecay,
		MemoryCanary:                out.MemoryCanary,
		MemoryCitationBackfill:      out.MemoryCitationBackfill,
		KnowledgeCitationBackfill:   out.KnowledgeCitationBackfill,
		KnowledgeRelationExtract:    out.KnowledgeRelationExtract,
		KnowledgeIndexRepair:        out.KnowledgeIndexRepair,
		KnowledgeCurate:             out.KnowledgeCurate,
		MemorySleepTime:             out.MemorySleepTime,
		MemoryEpisodeBackfill:       out.MemoryEpisodeBackfill,
		MemoryFactIndexReconciler:   out.MemoryFactIndexReconciler,
		MemoryDeadLetterReplayer:    out.MemoryDeadLetterReplayer,
		ModelRegistrySyncAgent:      out.ModelRegistrySyncAgent,
		CronRepo:                    out.CronRepo,
		ConfigGraphIndexer:          out.ConfigGraphIndexer,
	}
}

// SkillWatchStarter is the minimal interface for starting the skill file watcher.
type SkillWatchStarter interface{ Start(ctx context.Context) }

// BackgroundStarter is the minimal interface for background workers.
type BackgroundStarter interface{ Start(ctx context.Context) }

// PluginRuntimeStarter groups the two methods main() calls on PluginRuntime.
type PluginRuntimeStarter interface {
	StartBackgroundWorkers()
	Close()
}

// ChannelRuntimeStarter groups the two methods main() calls on ChannelRuntime.
type ChannelRuntimeStarter interface {
	Start(ctx context.Context)
	Stop()
}

// startBackgroundWorkers launches all configured background workers after data readiness.
func startBackgroundWorkers(
	ctx context.Context,
	cfg *backgroundWorkersConfig,
	logger log.Logger,
	lg loggateway.Logger,
	waitDataReady func(),
) {
	goAfterReady := func(name string, fn func()) {
		safego.Go(ctx, "worker."+name, func() {
			waitDataReady()
			fn()
		})
	}

	if cfg.CronRunner != nil {
		interval := cronrunner.DefaultInterval()
		goAfterReady("cron", func() { cfg.CronRunner.Start(ctx, interval) })
		logger.Log(log.LevelInfo, "msg", "cron runner scheduled", "interval", interval.String())
	}

	if cfg.SkillWatch != nil {
		watchCtx := cfg.WatchCtx
		goAfterReady("skill_watch", func() { cfg.SkillWatch.Start(watchCtx) })
		logger.Log(log.LevelInfo, "msg", "skill filesystem watcher scheduled")
	}

	if cfg.AutoMemory != nil {
		goAfterReady("auto_memory", func() { cfg.AutoMemory.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "auto-memory worker scheduled")
	}

	if cfg.MCPHealthProbe != nil {
		mcpInterval := mcphealth.DefaultInterval()
		goAfterReady("mcp_health", func() { cfg.MCPHealthProbe.Start(ctx, mcpInterval) })
		logger.Log(log.LevelInfo, "msg", "mcp health probe scheduled", "interval", mcpInterval.String())
	}

	if cfg.A2AGatewayHealthProbe != nil {
		a2aInterval := a2ahealth.DefaultInterval()
		goAfterReady("a2a_health", func() { cfg.A2AGatewayHealthProbe.Start(ctx, a2aInterval) })
		logger.Log(log.LevelInfo, "msg", "a2a gateway health probe scheduled", "interval", a2aInterval.String())
	}

	if cfg.LearningLoopScanner != nil {
		goAfterReady("learning_loop", func() { cfg.LearningLoopScanner.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "learning loop scanner scheduled", "interval", "30m")
	}

	if cfg.SkillIntelligenceWorker != nil {
		goAfterReady("skill_intelligence", func() { cfg.SkillIntelligenceWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "skill intelligence worker scheduled", "interval", "15m")
	}

	if cfg.CuratorWorker != nil {
		goAfterReady("curator_worker", func() { cfg.CuratorWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "curator worker scheduled", "interval", "2h")
	}

	if cfg.EvolutionOrchestratorWorker != nil {
		goAfterReady("evolution_orchestrator", func() { cfg.EvolutionOrchestratorWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "evolution orchestrator worker scheduled", "interval", "2h")
	}

	if cfg.SelfImproveObserveWorker != nil {
		goAfterReady("self_improve_observe", func() { cfg.SelfImproveObserveWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "self-improve observe worker scheduled", "interval", "15m")
	}

	if cfg.SelfImproveDriveWorker != nil {
		goAfterReady("self_improve_drive", func() { cfg.SelfImproveDriveWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "self-improve drive worker scheduled", "interval", "1m")
	}

	if cfg.SelfImproveWatchdogWorker != nil {
		goAfterReady("self_improve_watchdog", func() { cfg.SelfImproveWatchdogWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "self-improve watchdog worker scheduled", "interval", "5m")
	}

	if cfg.SelfImproveOutcomeWorker != nil {
		goAfterReady("self_improve_outcome", func() { cfg.SelfImproveOutcomeWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "self-improve outcome worker scheduled", "interval", "1h")
	}

	if cfg.ProviderHealthScanner != nil {
		goAfterReady("provider_health", func() { cfg.ProviderHealthScanner.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "provider health scanner scheduled", "interval", "5m")
	}

	if cfg.ChannelHealthScanner != nil {
		goAfterReady("channel_health", func() { cfg.ChannelHealthScanner.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "channel health scanner scheduled", "interval", "10m")
	}

	if cfg.ChannelDeliveryScanner != nil {
		goAfterReady("channel_delivery", func() { cfg.ChannelDeliveryScanner.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "channel delivery worker scheduled", "interval", "5s")
	}

	if cfg.SessionRunDurableWorker != nil {
		goAfterReady("session_run_durable", func() { cfg.SessionRunDurableWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "session run durable worker scheduled", "interval", "5s")
	}

	if cfg.RecoveryWorker != nil {
		goAfterReady("recovery_worker", func() { cfg.RecoveryWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "recovery worker scheduled", "interval", "5m")
	}

	if cfg.BackgroundJobWorker != nil {
		goAfterReady("backgroundjob_worker", func() { cfg.BackgroundJobWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "backgroundjob worker scheduled", "interval", "5s")
	}

	if cfg.PluginRuntime != nil {
		goAfterReady("plugin_bg", func() { cfg.PluginRuntime.StartBackgroundWorkers() })
		logger.Log(log.LevelInfo, "msg", "plugin background workers scheduled")
	}

	if cfg.ChannelRuntime != nil {
		goAfterReady("channel_runtime", func() { cfg.ChannelRuntime.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "channel runtime manager scheduled")
	}

	if cfg.ToolAuditCleanup != nil {
		goAfterReady("tool_audit_cleanup", func() { cfg.ToolAuditCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "tool audit cleanup scheduled", "interval", "24h", "retention_days", biz.ToolAuditRetentionDays)
	}

	if cfg.FlowLogCleanup != nil {
		goAfterReady("flow_log_cleanup", func() { cfg.FlowLogCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "flow log cleanup scheduled", "interval", "1h")
	}

	if cfg.MonitorEventsCleanup != nil {
		goAfterReady("monitor_events_cleanup", func() { cfg.MonitorEventsCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "monitor events cleanup scheduled", "interval", "24h", "retention", jobs.MonitorEventsRetention.String())
	}

	if cfg.AutoHealTTLCleanup != nil {
		goAfterReady("auto_heal_ttl_cleanup", func() { cfg.AutoHealTTLCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "auto heal TTL cleanup scheduled", "interval", "1h", "maxAge", "72h")
	}

	if cfg.MonitorAlertEvalWorker != nil {
		goAfterReady("monitor_alert_eval", func() { cfg.MonitorAlertEvalWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "monitor alert eval worker scheduled", "interval", "30s")
	}

	if cfg.MonitorTraceBackfillWorker != nil {
		goAfterReady("monitor_trace_backfill", func() { cfg.MonitorTraceBackfillWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "monitor trace backfill worker scheduled", "interval", "6h")
	}

	if cfg.SelfCheckScheduler != nil {
		goAfterReady("self_check_scheduler", func() { cfg.SelfCheckScheduler.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "self-check scheduler scheduled", "interval", "5m")
	}

	if cfg.SelfHealObserver != nil {
		goAfterReady("self_heal_observer", func() {
			cfg.SelfHealObserver.StartEventDrivenObservation(ctx, cfg.MonitorBus)
		})
		logger.Log(log.LevelInfo, "msg", "self-heal observer scheduled")
	}

	if cfg.FailurePatternSyncJob != nil {
		goAfterReady("failure_pattern_sync", func() { cfg.FailurePatternSyncJob.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "failure pattern sync job scheduled", "interval", "24h")
	}

	if cfg.PredictiveHealJob != nil {
		goAfterReady("predictive_heal", func() { cfg.PredictiveHealJob.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "predictive heal job scheduled", "interval", "5m")
	}

	if cfg.PatternMiningJob != nil {
		goAfterReady("pattern_mining", func() { cfg.PatternMiningJob.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "pattern mining job scheduled", "interval", "24h")
	}

	if cfg.MemoryL2Decay != nil {
		goAfterReady("memory_l2_decay", func() { cfg.MemoryL2Decay.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory l2 decay worker scheduled", "interval", "24h")
	}

	if cfg.MemoryL1Archive != nil {
		goAfterReady("memory_l1_archive", func() { cfg.MemoryL1Archive.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory l1 archive worker scheduled", "interval", "5m")
	}

	if cfg.ChannelTurnJobSweeper != nil {
		goAfterReady("channel_turn_job_sweeper", func() { cfg.ChannelTurnJobSweeper.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "channel turn job sweeper scheduled", "interval", "2m")
	}

	if cfg.MemoryL3Decay != nil {
		goAfterReady("memory_l3_decay", func() { cfg.MemoryL3Decay.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory l3 decay worker scheduled", "interval", "24h")
	}

	if cfg.MemoryL4Decay != nil {
		goAfterReady("memory_l4_decay", func() { cfg.MemoryL4Decay.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory l4 decay worker scheduled", "interval", "24h")
	}

	if cfg.MemoryEbbinghausDecay != nil {
		goAfterReady("memory_ebbinghaus_decay", func() { cfg.MemoryEbbinghausDecay.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory ebbinghaus decay worker scheduled", "interval", "24h")
	}

	if cfg.MemoryCanary != nil {
		goAfterReady("memory_canary", func() { cfg.MemoryCanary.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory canary worker scheduled", "interval", "30m")
	}

	if cfg.MemoryCitationBackfill != nil {
		goAfterReady("memory_citation_backfill", func() { cfg.MemoryCitationBackfill.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory citation backfill worker scheduled", "interval", "10m")
	}

	if cfg.KnowledgeCitationBackfill != nil {
		goAfterReady("knowledge_citation_backfill", func() { cfg.KnowledgeCitationBackfill.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "knowledge citation backfill worker scheduled", "interval", "10m")
	}

	if cfg.KnowledgeRelationExtract != nil {
		goAfterReady("knowledge_relation_extract", func() { cfg.KnowledgeRelationExtract.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "knowledge relation extract worker scheduled", "interval", "30m")
	}

	if cfg.KnowledgeIndexRepair != nil {
		goAfterReady("knowledge_index_repair", func() { cfg.KnowledgeIndexRepair.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "knowledge index repair worker scheduled", "interval", "5m")
	}

	if cfg.KnowledgeCurate != nil {
		goAfterReady("knowledge_curate", func() { cfg.KnowledgeCurate.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "knowledge curate worker scheduled", "interval", "6h")
	}

	if cfg.MemorySleepTime != nil {
		goAfterReady("memory_sleep_time", func() { cfg.MemorySleepTime.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory sleep-time worker scheduled", "interval", "1h")
	}

	if cfg.MemoryEpisodeBackfill != nil {
		goAfterReady("memory_episode_backfill", func() { cfg.MemoryEpisodeBackfill.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory episode backfill worker scheduled", "interval", "6h")
	}

	if cfg.MemoryFactIndexReconciler != nil {
		goAfterReady("memory_fact_index", func() { cfg.MemoryFactIndexReconciler.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory fact index reconciler scheduled", "interval", "6h")
	}

	if cfg.MemoryDeadLetterReplayer != nil {
		goAfterReady("memory_dead_letter", func() { cfg.MemoryDeadLetterReplayer.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory dead letter replayer scheduled", "interval", "30m")
	}

	if cfg.ModelRegistrySyncAgent != nil {
		safego.Go(ctx, "modelregistry.cron_seed", func() {
			if err := biz.SeedModelRegistryCronTask(ctx, cfg.CronRepo); err != nil {
				lg.Warn("Failed to seed model registry cron task", loggateway.StepID("modelregistry.cron_seed"), loggateway.Err(err))
			}
		})
		logger.Log(log.LevelInfo, "msg", "model registry sync agent registered", "schedule", "via CronRunner")
	}

	// M81 config-asset graph indexer (P0: rebuild capability only). The
	// acceptance line `configgraph indexer started` is emitted by the indexer
	// itself (loggateway, design R7); do not duplicate it here.
	if cfg.ConfigGraphIndexer != nil {
		goAfterReady("configgraph_indexer", func() { cfg.ConfigGraphIndexer.Start(ctx) })
	}
}

// installSignalHandler sets up OS signal handling for graceful shutdown.
func installSignalHandler(
	stopBackgroundWorkers func(),
	app interface{ Stop() error },
	logger log.Logger,
) {
	const shutdownForceExit = 10 * time.Second
	go func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(ch)

		sig := <-ch
		stopBackgroundWorkers()
		logger.Log(log.LevelInfo, "msg", "shutdown signal received", "signal", sig.String())
		if err := app.Stop(); err != nil {
			logger.Log(log.LevelWarn, "msg", "app stop error", "error", err.Error())
		}

		forceTimer := time.NewTimer(shutdownForceExit)
		defer forceTimer.Stop()
		for {
			select {
			case sig := <-ch:
				logger.Log(log.LevelWarn, "msg", "interrupt — forcing exit", "signal", sig.String())
				os.Exit(130)
			case <-forceTimer.C:
				logger.Log(log.LevelWarn, "msg", "graceful shutdown timeout — forcing exit", "timeout", shutdownForceExit.String())
				os.Exit(130)
			}
		}
	}()
}
