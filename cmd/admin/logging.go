package main

import (
	"context"
	"fmt"

	"aranea-agents/internal/adapter"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/metrics"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
	"aranea-agents/pkg/safego"

	agentlog "trpc.group/trpc-go/trpc-agent-go/log"

	"github.com/go-kratos/kratos/v2/log"
)

// registerSafegoPanicFlowHook re-registers the safego panic hook once the
// MonitorBus is available: recovered panics are counted in Prometheus AND
// surfaced as flow logs (system.safego.panic). Stack traces stay on stderr
// only — never in extra.
func registerSafegoPanicFlowHook(ctx context.Context, lg loggateway.Logger, bus contract.MonitorBus) {
	metricsPanicHook := metrics.SafegoPanicHook()
	safego.RegisterPanicHook(func(name string, r any, stack []byte) {
		metricsPanicHook(name, r, stack)
		flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
			Ctx:    ctx,
			Domain: event.TraceDomainSystem,
			LG:     lg,
			Infra:  event.NewInfraFromBus(bus),
		})
		flow.LogError("system.safego.panic", "协程 panic 已恢复",
			event.P("where", name),
			event.P("error", fmt.Sprint(r)),
		)
	})
}

// initLogging sets up the loggateway Pipeline and Gateway from bootstrap config.
// Returns the logger, pipeline, and logging sinks for use by Wire and BeforeStart.
func initLogging(bc *conf.Bootstrap, logger log.Logger) (loggateway.Logger, logpipeline.Pipeline, []*conf.LoggingSink) {
	var lg loggateway.Logger = loggateway.NewNoop()
	var pipeline logpipeline.Pipeline
	var loggingSinks []*conf.LoggingSink

	if bc.Logging == nil {
		return lg, pipeline, loggingSinks
	}

	pipeline = logpipeline.NewPipeline(4096)
	loggingSinks = bc.Logging.GetSinks()

	if len(loggingSinks) > 0 {
		// Config-driven sink creation; eventbus sinks are deferred to BeforeStart
		// because they require eventInfra which is not yet available.
		for _, s := range loggingSinks {
			cfg := protoSinkToConfig(s)
			if cfg.Type == "eventbus" {
				continue // handled in BeforeStart
			}
			sink, err := logpipeline.NewSinkFromConfig(cfg, logpipeline.SinkFactoryDeps{})
			if err != nil {
				logger.Log(log.LevelWarn, "msg", "failed to create sink from config", "sink", cfg.Name, "error", err.Error())
				continue
			}
			// Wrap with sanitizing sink to prevent secrets from leaking into logs.
			pipeline.AddSink(logpipeline.NewSanitizingSink(sink))
		}
	} else {
		// Default (backward-compatible) sink setup
		if bc.Logging.GetStdoutEnabled() {
			pipeline.AddSink(logpipeline.NewSanitizingSink(logpipeline.NewStdoutSink("debug")))
		}
		pipeline.AddSink(logpipeline.NewSanitizingSink(logpipeline.NewFileSink(logpipeline.FileSinkConfig{
			OutputDir:  bc.Logging.GetOutputDir(),
			MaxSizeMB:  int(bc.Logging.GetMaxSizeMb()),
			MaxBackups: int(bc.Logging.GetMaxBackups()),
			MaxAgeDays: int(bc.Logging.GetMaxAgeDays()),
			Compress:   bc.Logging.GetCompress(),
		})))
	}

	lg = loggateway.New(loggateway.LoggingConfig{
		Level:      bc.Logging.GetLevel(),
		OutputDir:  bc.Logging.GetOutputDir(),
		MaxSizeMB:  int(bc.Logging.GetMaxSizeMb()),
		MaxBackups: int(bc.Logging.GetMaxBackups()),
		MaxAgeDays: int(bc.Logging.GetMaxAgeDays()),
		Compress:   bc.Logging.GetCompress(),
		Stdout:     bc.Logging.GetStdoutEnabled(),
	}, pipeline)

	// Bridge trpc-agent-go runtime logs to loggateway Pipeline
	if gw, ok := lg.(*loggateway.Gateway); ok {
		rla := adapter.NewRuntimeLogAdapter(gw)
		agentlog.Default = rla
		agentlog.ContextDefault = rla
	}

	return lg, pipeline, loggingSinks
}

// protoSinkToConfig converts a proto LoggingSink to a logpipeline SinkConfig.
func protoSinkToConfig(s *conf.LoggingSink) logpipeline.SinkConfig {
	if s == nil {
		return logpipeline.SinkConfig{}
	}
	var dropPolicy logpipeline.DropPolicy
	switch s.GetDropPolicy() {
	case conf.DropPolicy_DROP_POLICY_BLOCK:
		dropPolicy = logpipeline.DropBlock
	default:
		dropPolicy = logpipeline.DropNewest
	}
	var sinkType string
	switch s.GetType() {
	case conf.SinkType_SINK_TYPE_FILE:
		sinkType = "file"
	case conf.SinkType_SINK_TYPE_STDOUT:
		sinkType = "stdout"
	case conf.SinkType_SINK_TYPE_EVENTBUS:
		sinkType = "eventbus"
	}
	return logpipeline.SinkConfig{
		Name:       s.GetName(),
		Type:       sinkType,
		BufferSize: int(s.GetBufferSize()),
		DropPolicy: dropPolicy,
		Config:     s.GetConfig(),
	}
}
