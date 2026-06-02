package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	a2ahealth "aranea-agents/internal/a2a/health"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/data"
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp/health"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/internal/telemetry"
	"aranea-agents/pkg/auth"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
	"aranea-agents/pkg/safego"

	_ "aranea-agents/internal/channel/all"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"

	"aranea-agents/internal/cronrunner/jobs"

	_ "go.uber.org/automaxprocs"
)

var (
	Name     string
	Version  string
	id, _    = os.Hostname()
	flagconf string
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
}

func newApp(
	logger log.Logger,
	lg loggateway.Logger,
	pipeline logpipeline.Pipeline,
	gs *grpc.Server,
	hs *http.Server,
	wsSrv *server.WSServer,
	consumer *biz.EventBusConsumer,
	sideConsumers *biz.EventBusSideConsumers,
	eventInfra *event.Infra,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
	agentUC *biz.AgentUsecase,
	teamUC *biz.TeamUsecase,
	positionUC *biz.PositionUsecase,
	d *data.Data,
	guard *service.SessionStatusGuard,
	orchCache *biz.OrchestrationCache,
	sessions *biz.SessionUsecase,
) *kratos.App {
	// EP-OBS-03: WSServer implements transport.Server (Start/Stop); register it so
	// kratos.App orchestrates its lifecycle and Stop triggers broadcastShutdown.
	srv := []transport.Server{gs, hs}
	if wsSrv != nil {
		srv = append(srv, wsSrv)
	}

	consumerCtx, consumerCancel := context.WithCancel(context.Background())

	app := kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(srv...),
		kratos.BeforeStart(func(ctx context.Context) error {
			if d != nil {
				if gate := d.Readiness(); gate != nil {
					if err := gate.Wait(ctx); err != nil {
						lg.Warn("BeforeStart: data readiness wait failed", loggateway.StepID("startup.gate"), loggateway.Err(err))
					}
				}
			}
			if err := guard.OnStartup(ctx); err != nil {
				logger.Log(log.LevelWarn, "msg", "session status guard startup failed", "error", err.Error())
			}
			if orchCache != nil {
				orchCache.InitFromRepo(ctx)
			}
			consumer.Start(consumerCtx)
			if sideConsumers != nil {
				sideConsumers.Start(consumerCtx)
			}
			sessions.StartMetricsFlusher(consumerCtx)
			if eventInfra != nil {
				event.BindInfra(eventInfra)
				if pipeline != nil {
					pipeline.AddSink(logpipeline.NewEventBusSink(event.NewLogPipelinePublisher(eventInfra.MonitorBus), "info"))
				}
			}
			logger.Log(log.LevelInfo, "msg", "event infra bound for monitor flow logs")
			return nil
		}),
		kratos.AfterStart(func(startCtx context.Context) error {
			if memoryDataMigration != nil {
				memoryDataMigration.Start(startCtx)
				logger.Log(log.LevelInfo, "msg", "memory data migration worker started")
			}
			safego.Go(startCtx, "seed.industry_agents", func() {
				logger.Log(log.LevelInfo, "msg", "industry agent seed started")
				service.SeedBuiltinIndustryAgents(startCtx, agentUC, teamUC, positionUC, biz.ScenarioDir(), data.NewSeedVersionRepo(d))
				logger.Log(log.LevelInfo, "msg", "industry agent seed completed")
			})
			return nil
		}),
		kratos.AfterStop(func(ctx context.Context) error {
			if err := guard.OnShutdown(ctx); err != nil {
				logger.Log(log.LevelWarn, "msg", "session status guard shutdown failed", "error", err.Error())
			}
			consumerCancel()
			if pipeline != nil {
				pipeline.Close()
			}
			return nil
		}),
	)
	return app
}

func main() {
	flag.Parse()

	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", id,
		"service.name", Name,
		"service.version", Version,
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)

	c := config.New(
		config.WithSource(
			file.NewSource(flagconf),
			env.NewSource(),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	auth.WarnIfBypassEnabled()

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	var lg loggateway.Logger = loggateway.NewNoop()
	var pipeline logpipeline.Pipeline
	if bc.Logging != nil {
		lg = loggateway.New(bc.Logging)
		pipeline = logpipeline.NewPipeline(4096)
		if bc.Logging.GetStdoutEnabled() {
			pipeline.AddSink(logpipeline.NewStdoutSink("debug"))
		}
		pipeline.AddSink(logpipeline.NewFileSink(logpipeline.FileSinkConfig{
			OutputDir:  bc.Logging.GetOutputDir(),
			MaxSizeMB:  int(bc.Logging.GetMaxSizeMb()),
			MaxBackups: int(bc.Logging.GetMaxBackups()),
			MaxAgeDays: int(bc.Logging.GetMaxAgeDays()),
			Compress:   bc.Logging.GetCompress(),
		}))
		if gw, ok := lg.(*loggateway.Gateway); ok {
			gw.SetPipeline(pipeline)
		}
	}
	loggateway.SetGlobal(lg.(*loggateway.Gateway))

	shutdownTelemetry := telemetry.Init(Name, Version, lg)
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	out, cleanup, err := wireApp(bc.Server, bc.Data, nil, logger, lg, pipeline)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	cronCtx, cancelCron := context.WithCancel(context.Background())
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	stopBackgroundWorkers := func() {
		cancelCron()
		cancelWatch()
		if out.ChannelRuntime != nil {
			out.ChannelRuntime.Stop()
		}
		if out.PluginRuntime != nil {
			out.PluginRuntime.Close()
		}
	}
	defer stopBackgroundWorkers()

	waitDataReady := func() {
		if out.Data != nil {
			if gate := out.Data.Readiness(); gate != nil {
				if err := gate.Wait(cronCtx); err != nil {
					lg.Warn("background worker: data readiness wait failed", loggateway.StepID("startup.gate"), loggateway.Err(err))
					return
				}
			}
		}
	}

	goAfterReady := func(name string, fn func()) {
		go func() {
			waitDataReady()
			fn()
		}()
	}

	// Windows / IDE terminals sometimes fail to deliver Ctrl+C to kratos App.Run. On first
	// interrupt we stop background workers and call App.Stop explicitly; keep listening so
	// a second interrupt or timeout always terminates the process.
	const shutdownForceExit = 10 * time.Second
	go func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(ch)

		sig := <-ch
		stopBackgroundWorkers()
		logger.Log(log.LevelInfo, "msg", "shutdown signal received", "signal", sig.String())
		if err := out.App.Stop(); err != nil {
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

	if out.CronRunner != nil {
		interval := cronrunner.DefaultInterval()
		goAfterReady("cron", func() { out.CronRunner.Start(cronCtx, interval) })
		logger.Log(log.LevelInfo, "msg", "cron runner scheduled", "interval", interval.String())
	}

	if out.SkillWatch != nil {
		goAfterReady("skill_watch", func() { out.SkillWatch.Start(watchCtx) })
		logger.Log(log.LevelInfo, "msg", "skill filesystem watcher scheduled")
	}

	if out.AutoMemory != nil {
		goAfterReady("auto_memory", func() { out.AutoMemory.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "auto-memory worker scheduled")
	}

	if out.MCPHealthProbe != nil {
		mcpInterval := health.DefaultInterval()
		goAfterReady("mcp_health", func() { out.MCPHealthProbe.Start(cronCtx, mcpInterval) })
		logger.Log(log.LevelInfo, "msg", "mcp health probe scheduled", "interval", mcpInterval.String())
	}

	if out.A2AGatewayHealthProbe != nil {
		a2aInterval := a2ahealth.DefaultInterval()
		goAfterReady("a2a_health", func() { out.A2AGatewayHealthProbe.Start(cronCtx, a2aInterval) })
		logger.Log(log.LevelInfo, "msg", "a2a gateway health probe scheduled", "interval", a2aInterval.String())
	}

	if out.EvolutionScanner != nil {
		goAfterReady("evolution", func() { out.EvolutionScanner.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "evolution scanner scheduled", "interval", "30m")
	}

	if out.LearningLoopScanner != nil {
		goAfterReady("learning_loop", func() { out.LearningLoopScanner.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "learning loop scanner scheduled", "interval", "30m")
	}

	if out.ProviderHealthScanner != nil {
		goAfterReady("provider_health", func() { out.ProviderHealthScanner.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "provider health scanner scheduled", "interval", "5m")
	}

	if out.ChannelHealthScanner != nil {
		goAfterReady("channel_health", func() { out.ChannelHealthScanner.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "channel health scanner scheduled", "interval", "10m")
	}

	if out.ChannelDeliveryScanner != nil {
		goAfterReady("channel_delivery", func() { out.ChannelDeliveryScanner.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "channel delivery worker scheduled", "interval", "5s")
	}

	if out.SessionRunDurableWorker != nil {
		goAfterReady("session_run_durable", func() { out.SessionRunDurableWorker.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "session run durable worker scheduled", "interval", "5s")
	}

	if out.PluginRuntime != nil {
		goAfterReady("plugin_bg", func() { out.PluginRuntime.StartBackgroundWorkers() })
		logger.Log(log.LevelInfo, "msg", "plugin background workers scheduled")
	}

	if out.ChannelRuntime != nil {
		goAfterReady("channel_runtime", func() { out.ChannelRuntime.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "channel runtime manager scheduled")
	}

	if out.EventStoreCleanup != nil {
		goAfterReady("event_store_cleanup", func() { out.EventStoreCleanup.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "event store cleanup scheduled", "interval", "1h")
	}

	if out.ToolAuditCleanup != nil {
		goAfterReady("tool_audit_cleanup", func() { out.ToolAuditCleanup.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "tool audit cleanup scheduled", "interval", "24h", "retention_days", biz.ToolAuditRetentionDays)
	}

	if out.FlowLogCleanup != nil {
		goAfterReady("flow_log_cleanup", func() { out.FlowLogCleanup.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "flow log cleanup scheduled", "interval", "1h")
	}

	if out.MonitorAlertCooldownCleanup != nil {
		goAfterReady("monitor_alert_cooldown", func() { out.MonitorAlertCooldownCleanup.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "monitor alert cooldown cleanup scheduled", "interval", "1h", "maxAge", "24h")
	}

	if out.MonitorAlertEvalWorker != nil {
		goAfterReady("monitor_alert_eval", func() { out.MonitorAlertEvalWorker.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "monitor alert eval worker scheduled", "interval", "30s")
	}

	if out.MonitorTraceBackfillWorker != nil {
		goAfterReady("monitor_trace_backfill", func() { out.MonitorTraceBackfillWorker.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "monitor trace backfill worker scheduled", "interval", "6h")
	}

	if out.MemoryL2Decay != nil {
		goAfterReady("memory_l2_decay", func() { out.MemoryL2Decay.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "memory l2 decay worker scheduled", "interval", "24h")
	}

	if out.MemoryL3Decay != nil {
		goAfterReady("memory_l3_decay", func() { out.MemoryL3Decay.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "memory l3 decay worker scheduled", "interval", "24h")
	}

	if out.MemoryL4Decay != nil {
		goAfterReady("memory_l4_decay", func() { out.MemoryL4Decay.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "memory l4 decay worker scheduled", "interval", "24h")
	}

	if out.MemoryEpisodeBackfill != nil {
		goAfterReady("memory_episode_backfill", func() { out.MemoryEpisodeBackfill.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "memory episode backfill worker scheduled", "interval", "6h")
	}

	if out.MemoryFactIndexReconciler != nil {
		goAfterReady("memory_fact_index", func() { out.MemoryFactIndexReconciler.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "memory fact index reconciler scheduled", "interval", "6h")
	}

	if out.MemoryDeadLetterReplayer != nil {
		goAfterReady("memory_dead_letter", func() { out.MemoryDeadLetterReplayer.Start(cronCtx) })
		logger.Log(log.LevelInfo, "msg", "memory dead letter replayer scheduled", "interval", "30m")
	}

	if out.ModelRegistrySyncAgent != nil {
		safego.Go(cronCtx, "modelregistry.cron_seed", func() {
			if err := biz.SeedModelRegistryCronTask(cronCtx, out.CronRepo); err != nil {
				lg.Warn("Failed to seed model registry cron task", loggateway.StepID("modelregistry.cron_seed"), loggateway.Err(err))
			}
		})
		logger.Log(log.LevelInfo, "msg", "model registry sync agent registered", "schedule", "via CronRunner")
	}

	if err := out.App.Run(); err != nil {
		panic(err)
	}
}
