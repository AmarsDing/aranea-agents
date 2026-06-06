package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/telemetry"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/auth"
	loggateway "aranea-agents/pkg/loggateway"

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
		panic(err)
	}

	auth.WarnIfBypassEnabled()

	// Initialize global application-lifecycle context.
	// Background goroutines derive from appctx.Ctx() so they are cancelled on shutdown.
	appctx.Init()

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	lg, pipeline, loggingSinks := initLogging(&bc, logger)

	// Inject logger into auth package (replaces Global() calls)
	auth.SetLogger(lg)

	shutdownTelemetry := telemetry.Init(Name, Version, lg)
	defer func() { _ = shutdownTelemetry(context.Background()) }()

	out, cleanup, err := wireApp(bc.Server, bc.Data, nil, logger, lg, pipeline, loggingSinks)
	if err != nil {
		panic(err)
	}
	defer cleanup()

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

	startBackgroundWorkers(cronCtx, &backgroundWorkersConfig{
		WatchCtx:                    watchCtx,
		CronRunner:                  out.CronRunner,
		SkillWatch:                  out.SkillWatch,
		AutoMemory:                  out.AutoMemory,
		MCPHealthProbe:              out.MCPHealthProbe,
		A2AGatewayHealthProbe:       out.A2AGatewayHealthProbe,
		EvolutionScanner:            out.EvolutionScanner,
		LearningLoopScanner:         out.LearningLoopScanner,
		SkillEvolutionScanner:       out.SkillEvolutionScanner,
		SkillIntelligenceWorker:     out.SkillIntelligenceWorker,
		CuratorWorker:               out.CuratorWorker,
		ProviderHealthScanner:       out.ProviderHealthScanner,
		ChannelHealthScanner:        out.ChannelHealthScanner,
		ChannelDeliveryScanner:      out.ChannelDeliveryScanner,
		SessionRunDurableWorker:     out.SessionRunDurableWorker,
		PluginRuntime:               out.PluginRuntime,
		ChannelRuntime:              out.ChannelRuntime,
		EventStoreCleanup:           out.EventStoreCleanup,
		ToolAuditCleanup:            out.ToolAuditCleanup,
		FlowLogCleanup:              out.FlowLogCleanup,
		MonitorAlertCooldownCleanup: out.MonitorAlertCooldownCleanup,
		AutoHealTTLCleanup:          out.AutoHealTTLCleanup,
		MonitorAlertEvalWorker:      out.MonitorAlertEvalWorker,
		MonitorTraceBackfillWorker:  out.MonitorTraceBackfillWorker,
		FailurePatternSyncJob:       out.FailurePatternSyncJob,
		PredictiveHealJob:           out.PredictiveHealJob,
		PatternMiningJob:            out.PatternMiningJob,
		MemoryL2Decay:               out.MemoryL2Decay,
		MemoryL2Consolidate:         out.MemoryL2Consolidate,
		MemoryL1Archive:             out.MemoryL1Archive,
		MemoryL3Decay:               out.MemoryL3Decay,
		MemoryL4Decay:               out.MemoryL4Decay,
		MemoryEpisodeBackfill:       out.MemoryEpisodeBackfill,
		MemoryFactIndexReconciler:   out.MemoryFactIndexReconciler,
		MemoryDeadLetterReplayer:    out.MemoryDeadLetterReplayer,
		ModelRegistrySyncAgent:      out.ModelRegistrySyncAgent,
		CronRepo:                    out.CronRepo,
	}, logger, lg, waitDataReady)

	if err := out.App.Run(); err != nil {
		panic(err)
	}
}
