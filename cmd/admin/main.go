package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/service"
	"aranea-agents/internal/telemetry"
	"aranea-agents/internal/tools/preview"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/auth"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/env"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"

	_ "go.uber.org/automaxprocs"
)

var (
	Name      string
	Version   string
	Commit    string
	BuildDate string
	id, _     = os.Hostname()
	flagconf  string
	flagver   bool
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../configs", "config path, eg: -conf config.yaml")
	flag.BoolVar(&flagver, "version", false, "print version and exit")
}

func main() {
	// Check --version before flag.Parse to avoid triggering init-time panics (e.g. auth).
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Printf("%s %s (commit: %s, built: %s)\n", Name, Version, Commit, BuildDate)
			os.Exit(0)
		}
	}

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
		panic(redactConfigError("load config", err))
	}

	auth.WarnIfBypassEnabled()

	// Initialize global application-lifecycle context.
	// Background goroutines derive from appctx.Ctx() so they are cancelled on shutdown.
	appctx.Init()

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(redactConfigError("scan config", err))
	}

	lg, pipeline, loggingSinks := initLogging(&bc, logger)

	// Inject logger into auth package (replaces Global() calls)
	auth.SetLogger(lg)

	// Register safego panic hook so recovered panics are counted in Prometheus.
	safego.RegisterPanicHook(metrics.SafegoPanicHook())

	shutdownTelemetry := telemetry.Init(Name, Version, lg)
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	// Initialize service-layer runtime configs before Wire construction.
	service.InitWebhookRateLimitConfig(bc.Runtime)

	out, cleanup, err := wireApp(bc.Server, bc.Data, bc.Runtime, bc.SelfImprovement, nil, logger, lg, pipeline, loggingSinks)
	if err != nil {
		panic(redactConfigError("wire app", err))
	}
	defer cleanup()

	// Re-register the safego panic hook now that the MonitorBus is available.
	registerSafegoPanicFlowHook(appctx.Ctx(), lg, out.MonitorBus)

	cronCtx, cancelCron := context.WithCancel(context.Background())
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	stopBackgroundWorkers := func() {
		cancelCron()
		cancelWatch()
		appctx.Cancel()
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

	installSignalHandler(stopBackgroundWorkers, out.App, logger)

	startBackgroundWorkers(cronCtx, backgroundWorkersConfigFromOutput(watchCtx, &out), logger, lg, waitDataReady)

	if err := out.App.Run(); err != nil {
		panic(err)
	}
}

// redactConfigError wraps a startup error with an operation prefix while
// masking sensitive values. YAML parse/scan errors echo offending source
// scalars (e.g. cannot unmarshal !!str `sk-...` into int) and DI failures may
// embed DSNs with passwords; the message is sanitized before it reaches
// stderr/logs (Grok Build §4.4.2: config errors never leak config content).
func redactConfigError(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s", op, preview.RedactAndTruncate(err.Error(), 500))
}
