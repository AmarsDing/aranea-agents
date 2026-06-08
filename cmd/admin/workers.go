package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	a2ahealth "aranea-agents/internal/a2a/health"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/cronrunner"
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
	EvolutionScanner            BackgroundStarter
	LearningLoopScanner         BackgroundStarter
	SkillEvolutionScanner       BackgroundStarter
	SkillIntelligenceWorker     BackgroundStarter
	CuratorWorker               BackgroundStarter
	ProviderHealthScanner       BackgroundStarter
	ChannelHealthScanner        BackgroundStarter
	ChannelDeliveryScanner      BackgroundStarter
	SessionRunDurableWorker     BackgroundStarter
	PluginRuntime               PluginRuntimeStarter
	ChannelRuntime              ChannelRuntimeStarter
	EventStoreCleanup           BackgroundStarter
	ToolAuditCleanup            BackgroundStarter
	FlowLogCleanup              BackgroundStarter
	MonitorAlertCooldownCleanup BackgroundStarter
	AutoHealTTLCleanup          BackgroundStarter
	MonitorAlertEvalWorker      BackgroundStarter
	MonitorTraceBackfillWorker  BackgroundStarter
	FailurePatternSyncJob       BackgroundStarter
	PredictiveHealJob           BackgroundStarter
	PatternMiningJob            BackgroundStarter
	MemoryL2Decay               BackgroundStarter
	MemoryL1Archive             BackgroundStarter
	MemoryL3Decay               BackgroundStarter
	MemoryL4Decay               BackgroundStarter
	MemoryEpisodeBackfill       BackgroundStarter
	MemoryFactIndexReconciler   BackgroundStarter
	MemoryDeadLetterReplayer    BackgroundStarter
	ModelRegistrySyncAgent      any // presence check only
	CronRepo                    biz.CronRepo
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
		go func() {
			waitDataReady()
			fn()
		}()
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

	if cfg.EvolutionScanner != nil {
		goAfterReady("evolution", func() { cfg.EvolutionScanner.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "evolution scanner scheduled", "interval", "30m")
	}

	if cfg.LearningLoopScanner != nil {
		goAfterReady("learning_loop", func() { cfg.LearningLoopScanner.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "learning loop scanner scheduled", "interval", "30m")
	}

	if cfg.SkillEvolutionScanner != nil {
		goAfterReady("skill_evolution", func() { cfg.SkillEvolutionScanner.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "skill evolution scanner scheduled", "interval", "60m")
	}

	if cfg.SkillIntelligenceWorker != nil {
		goAfterReady("skill_intelligence", func() { cfg.SkillIntelligenceWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "skill intelligence worker scheduled", "interval", "15m")
	}

	if cfg.CuratorWorker != nil {
		goAfterReady("curator_worker", func() { cfg.CuratorWorker.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "curator worker scheduled", "interval", "2h")
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

	if cfg.PluginRuntime != nil {
		goAfterReady("plugin_bg", func() { cfg.PluginRuntime.StartBackgroundWorkers() })
		logger.Log(log.LevelInfo, "msg", "plugin background workers scheduled")
	}

	if cfg.ChannelRuntime != nil {
		goAfterReady("channel_runtime", func() { cfg.ChannelRuntime.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "channel runtime manager scheduled")
	}

	if cfg.EventStoreCleanup != nil {
		goAfterReady("event_store_cleanup", func() { cfg.EventStoreCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "event store cleanup scheduled", "interval", "1h")
	}

	if cfg.ToolAuditCleanup != nil {
		goAfterReady("tool_audit_cleanup", func() { cfg.ToolAuditCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "tool audit cleanup scheduled", "interval", "24h", "retention_days", biz.ToolAuditRetentionDays)
	}

	if cfg.FlowLogCleanup != nil {
		goAfterReady("flow_log_cleanup", func() { cfg.FlowLogCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "flow log cleanup scheduled", "interval", "1h")
	}

	if cfg.MonitorAlertCooldownCleanup != nil {
		goAfterReady("monitor_alert_cooldown", func() { cfg.MonitorAlertCooldownCleanup.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "monitor alert cooldown cleanup scheduled", "interval", "1h", "maxAge", "24h")
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

	if cfg.MemoryL3Decay != nil {
		goAfterReady("memory_l3_decay", func() { cfg.MemoryL3Decay.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory l3 decay worker scheduled", "interval", "24h")
	}

	if cfg.MemoryL4Decay != nil {
		goAfterReady("memory_l4_decay", func() { cfg.MemoryL4Decay.Start(ctx) })
		logger.Log(log.LevelInfo, "msg", "memory l4 decay worker scheduled", "interval", "24h")
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
