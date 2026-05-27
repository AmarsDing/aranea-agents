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
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp/health"
	"aranea-agents/internal/server"
	"aranea-agents/internal/telemetry"
	"aranea-agents/pkg/auth"

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
	gs *grpc.Server,
	hs *http.Server,
	wsSrv *server.WSServer,
	consumer *biz.EventBusConsumer,
	sideConsumers *biz.EventBusSideConsumers,
	eventInfra *event.Infra,
	sessionLogWriter biz.SessionLogWriter,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
) *kratos.App {
	// EP-OBS-03: WSServer implements transport.Server (Start/Stop); register it so
	// kratos.App orchestrates its lifecycle and Stop triggers broadcastShutdown.
	srv := []transport.Server{gs, hs}
	if wsSrv != nil {
		srv = append(srv, wsSrv)
	}

	// Phase 3: inject SessionLogWriter into event bus consumers
	consumer.SetLogger(sessionLogWriter)
	if sideConsumers != nil {
		sideConsumers.SetLogger(sessionLogWriter)
	}

	consumerCtx, consumerCancel := context.WithCancel(context.Background())

	app := kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(srv...),
		kratos.BeforeStart(func(context.Context) error {
			consumer.Start(consumerCtx)
			if sideConsumers != nil {
				sideConsumers.Start(consumerCtx)
			}
			if eventInfra != nil {
				event.BindInfra(eventInfra)
			}
			logger.Log(log.LevelInfo, "msg", "event infra bound for monitor flow logs")
			return nil
		}),
		kratos.AfterStart(func(startCtx context.Context) error {
			if memoryDataMigration != nil {
				memoryDataMigration.Start(startCtx)
				logger.Log(log.LevelInfo, "msg", "memory data migration worker started")
			}
			return nil
		}),
		kratos.AfterStop(func(context.Context) error {
			consumerCancel()
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

	// EP-OBS-02: initialise OTel tracer + meter providers; noop when endpoint not set.
	shutdownTelemetry := telemetry.Init(Name, Version)
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	out, cleanup, err := wireApp(bc.Server, bc.Data, logger)
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
		// Stop the HookDeliveryRetryWorker goroutine started by plugin Runtime (OUT-02 / P0-4).
		if out.PluginRuntime != nil {
			out.PluginRuntime.Close()
		}
	}
	defer stopBackgroundWorkers()

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
		go out.CronRunner.Start(cronCtx, interval)
		logger.Log(log.LevelInfo, "msg", "cron runner started", "interval", interval.String())
	}

	if out.SkillWatch != nil {
		go out.SkillWatch.Start(watchCtx)
		logger.Log(log.LevelInfo, "msg", "skill filesystem watcher started")
	}

	// EP-RT-03: start auto-memory extraction worker.
	if out.AutoMemory != nil {
		go out.AutoMemory.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "auto-memory worker started")
	}

	// MCP health probe: periodically probes all enabled MCP servers.
	if out.MCPHealthProbe != nil {
		mcpInterval := health.DefaultInterval()
		go out.MCPHealthProbe.Start(cronCtx, mcpInterval)
		logger.Log(log.LevelInfo, "msg", "mcp health probe started", "interval", mcpInterval.String())
	}

	if out.A2AGatewayHealthProbe != nil {
		a2aInterval := a2ahealth.DefaultInterval()
		go out.A2AGatewayHealthProbe.Start(cronCtx, a2aInterval)
		logger.Log(log.LevelInfo, "msg", "a2a gateway health probe started", "interval", a2aInterval.String())
	}

	if out.EvolutionScanner != nil {
		go out.EvolutionScanner.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "evolution scanner started", "interval", "30m")
	}

	if out.ProviderHealthScanner != nil {
		go out.ProviderHealthScanner.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "provider health scanner started", "interval", "5m")
	}

	if out.ChannelHealthScanner != nil {
		go out.ChannelHealthScanner.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "channel health scanner started", "interval", "10m")
	}

	if out.ChannelDeliveryScanner != nil {
		go out.ChannelDeliveryScanner.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "channel delivery worker started", "interval", "5s")
	}

	if out.SessionRunDurableWorker != nil {
		out.SessionRunDurableWorker.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "session run durable worker started", "interval", "5s")
	}

	if out.ChannelRuntime != nil {
		out.ChannelRuntime.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "channel runtime manager started")
	}

	if out.EventStoreCleanup != nil {
		go out.EventStoreCleanup.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "event store cleanup started", "interval", "1h")
	}

	if out.ToolAuditCleanup != nil {
		go out.ToolAuditCleanup.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "tool audit cleanup started", "interval", "24h", "retention_days", biz.ToolAuditRetentionDays)
	}

	if out.FlowLogCleanup != nil {
		go out.FlowLogCleanup.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "flow log cleanup started", "interval", "1h")
	}

	if out.MonitorAlertCooldownCleanup != nil {
		go out.MonitorAlertCooldownCleanup.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "monitor alert cooldown cleanup started", "interval", "1h", "maxAge", "24h")
	}

	if out.MemoryL2Decay != nil {
		go out.MemoryL2Decay.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "memory l2 decay worker started", "interval", "24h")
	}

	if out.MemoryL3Decay != nil {
		go out.MemoryL3Decay.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "memory l3 decay worker started", "interval", "24h")
	}

	if out.MemoryL4Decay != nil {
		go out.MemoryL4Decay.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "memory l4 decay worker started", "interval", "24h")
	}

	if out.MemoryEpisodeBackfill != nil {
		go out.MemoryEpisodeBackfill.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "memory episode backfill worker started", "interval", "6h")
	}

	if out.ModelCatalogRunner != nil {
		out.ModelCatalogRunner.Start(cronCtx)
		logger.Log(log.LevelInfo, "msg", "model catalog sync runner started", "interval", "1h")
	}

	if err := out.App.Run(); err != nil {
		panic(err)
	}
}
