package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/event"
	"aranea-agents/internal/mcp/health"
	"aranea-agents/internal/server"
	"aranea-agents/internal/telemetry"
	"aranea-agents/pkg/auth"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"

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
	eventBus event.Bus,
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
		kratos.BeforeStart(func(context.Context) error {
			consumer.Start(consumerCtx)
			event.SetGlobalBus(eventBus)
			logger.Log(log.LevelInfo, "msg", "flow log v2 global bus wired")
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
	}
	defer stopBackgroundWorkers()

	// Windows / IDE terminals sometimes fail to deliver Ctrl+C to go run; cancel workers on
	// first interrupt so kratos Stop can finish. Second interrupt forces exit.
	go func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		sig := <-ch
		stopBackgroundWorkers()
		logger.Log(log.LevelInfo, "msg", "shutdown signal received", "signal", sig.String())
		select {
		case <-ch:
			logger.Log(log.LevelWarn, "msg", "second interrupt — forcing exit")
			os.Exit(130)
		case <-time.After(15 * time.Second):
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

	if err := out.App.Run(); err != nil {
		panic(err)
	}
}
