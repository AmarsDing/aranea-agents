package main

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
	"aranea-agents/pkg/safego"

	"aranea-agents/internal/cronrunner/jobs"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
)

func newApp(
	logger log.Logger,
	lg loggateway.Logger,
	pipeline logpipeline.Pipeline,
	loggingSinks []*conf.LoggingSink,
	gs *grpc.Server,
	hs *http.Server,
	wsSrv *server.WSServer,
	sideConsumers *biz.EventBusSideConsumers,
	eventInfra *event.Infra,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
	agentUC *biz.AgentUsecase,
	teamUC *biz.TeamUsecase,
	organizationUC *biz.OrganizationUsecase,
	d *data.Data,
	guard *service.SessionStatusGuard,
	orchCache *biz.OrchestrationCache,
	sessions *biz.SessionUsecase,
	chatSvc *service.ChatService,
	spiritUC *biz.SpiritTeamUsecase,
	teamStarter *service.TeamStarter,
	lifecycleMgr *lifecycle.LifecycleManager,
	wsV2Sub *server.WSV2Subscriber,
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
			// Register extra LLM providers (huggingface, bedrock) with the
			// trpc-agent-go provider registry. Was previously in init();
			// moved here for explicit lifecycle control.
			provider.RegisterExtraProviders()

			// Inject service-layer timeout handler into biz layer (breaks circular dep).
			// SetTimeoutHandler is a justified exception like L4GraphUsecase.SetCascade:
			// SpiritTeamUsecase → TimeoutHandler → TeamStarter → SpiritTeamController → SpiritTeamUsecase
			if spiritUC != nil && teamStarter != nil {
				spiritUC.SetTimeoutHandler(teamStarter)
				// Inject completion notifier for the background polling mechanism.
				// When the poller detects all teams done, it notifies the service
				// layer to publish the spirit_teams_all_completed event.
				spiritUC.SetAllTeamsCompletedNotifier(teamStarter)
			}

			// Start readiness-dependent initialization in background.
			// The HTTP server now starts immediately so /healthz can report
			// "starting" (503) while P1 migrations run. A readiness middleware
			// blocks all non-infrastructure routes until ready.
			if d != nil {
				if gate := d.Readiness(); gate != nil {
					safego.Go(ctx, "startup.post_readiness", func() {
						if err := gate.Wait(ctx); err != nil {
							lg.Warn("post-readiness: data readiness wait failed", loggateway.StepID("startup.gate"), loggateway.Err(err))
							return
						}
						lg.Info("post-readiness: data ready, starting dependent services", loggateway.StepID("startup.gate"))
						startReadinessDependentServices(consumerCtx, guard, orchCache, sideConsumers, sessions, eventInfra, pipeline, loggingSinks, spiritUC, lg)
					})
				} else {
					// No readiness gate (unlikely), start immediately.
					startReadinessDependentServices(consumerCtx, guard, orchCache, sideConsumers, sessions, eventInfra, pipeline, loggingSinks, spiritUC, lg)
				}
			} else {
				// No data layer (unlikely), start immediately.
				startReadinessDependentServices(consumerCtx, guard, orchCache, sideConsumers, sessions, eventInfra, pipeline, loggingSinks, spiritUC, lg)
			}
			return nil
		}),
		kratos.AfterStart(func(startCtx context.Context) error {
			if memoryDataMigration != nil {
				memoryDataMigration.Start(startCtx)
				lg.Info("memory data migration worker started", loggateway.StepID("startup.memory_migration"))
			}

			return nil
		}),
		kratos.AfterStop(func(ctx context.Context) error {
			if err := guard.OnShutdown(ctx); err != nil {
				lg.Warn("session status guard shutdown failed", loggateway.StepID("shutdown.guard"), loggateway.Err(err))
			}
			if chatSvc != nil {
				if err := chatSvc.Close(); err != nil {
					lg.Warn("chat service close failed", loggateway.StepID("shutdown.chat"), loggateway.Err(err))
				}
			}
			// Stop the v2 WS subscriber goroutine before the process exits.
			if wsV2Sub != nil {
				if err := wsV2Sub.Close(); err != nil {
					lg.Warn("ws v2 subscriber close failed", loggateway.StepID("shutdown.ws_v2_sub"), loggateway.Err(err))
				}
			}
			// Stop the background team completion poller and cancel all timeout timers.
			if spiritUC != nil {
				spiritUC.Stop()
			}
			// Close process-level resources (build cache, etc.) in LIFO order
			// after the chat service has stopped accepting requests (A3).
			if lifecycleMgr != nil {
				lifecycleMgr.Close()
			}
			consumerCancel()
			if pipeline != nil {
				pipeline.Close()
				if gw, ok := lg.(*loggateway.Gateway); ok {
					gw.SetPipeline(nil)
				}
			}
			return nil
		}),
	)
	return app
}

// startReadinessDependentServices starts all services that require the data layer
// to be fully initialized (DDL migrations, data migrations, seed data).
// Extracted from BeforeStart so it can run in a background goroutine after
// the HTTP server starts listening, allowing /healthz to report "starting".
func startReadinessDependentServices(
	ctx context.Context,
	guard *service.SessionStatusGuard,
	orchCache *biz.OrchestrationCache,
	sideConsumers *biz.EventBusSideConsumers,
	sessions *biz.SessionUsecase,
	eventInfra *event.Infra,
	pipeline logpipeline.Pipeline,
	loggingSinks []*conf.LoggingSink,
	spiritUC *biz.SpiritTeamUsecase,
	lg loggateway.Logger,
) {
	if err := guard.OnStartup(ctx); err != nil {
		lg.Warn("session status guard startup failed", loggateway.StepID("startup.guard"), loggateway.Err(err))
	}
	if orchCache != nil {
		orchCache.InitFromRepo(ctx)
	}
	if sideConsumers != nil {
		sideConsumers.Start(ctx)
	}
	sessions.StartMetricsFlusher(ctx)
	if eventInfra != nil {
		if pipeline != nil {
			if len(loggingSinks) > 0 {
				// Config-driven: create eventbus sinks from config
				for _, s := range loggingSinks {
					cfg := protoSinkToConfig(s)
					if cfg.Type != "eventbus" {
						continue
					}
					sink, err := logpipeline.NewSinkFromConfig(cfg, logpipeline.SinkFactoryDeps{
						EventBusPublisher: event.NewLogPipelinePublisher(eventInfra.MonitorEventBus),
					})
					if err != nil {
						lg.Warn("failed to create eventbus sink from config", loggateway.Str("sink", cfg.Name), loggateway.Err(err))
						continue
					}
					// Wrap with sanitizing sink to prevent secrets from leaking into event bus logs.
				pipeline.AddSink(logpipeline.NewSanitizingSink(sink))
			}
		} else {
			// Default: add eventbus sink with "info" level
			pipeline.AddSink(logpipeline.NewSanitizingSink(logpipeline.NewEventBusSink(event.NewLogPipelinePublisher(eventInfra.MonitorEventBus), "info")))
		}
		}
	}
	lg.Info("event infra bound for monitor flow logs", loggateway.StepID("startup.event_infra"))

	// Start background team completion polling (30s interval).
	// This supplements the event-driven path (HandleTeamTurnResult) with a
	// moderate-frequency backup to catch cases where completion events are
	// missed. Purely backend logic — no frontend-visible activity events.
	if spiritUC != nil {
		spiritUC.StartBackgroundPolling(ctx, 30*time.Second)
	}
}
