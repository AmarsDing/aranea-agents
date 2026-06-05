package main

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/event"
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
	consumer *biz.EventBusConsumer,
	sideConsumers *biz.EventBusSideConsumers,
	eventInfra *event.Infra,
	memoryDataMigration *jobs.MemoryDataMigrationWorker,
	agentUC *biz.AgentUsecase,
	teamUC *biz.TeamUsecase,
	taxonomyUC *biz.TaxonomyUsecase,
	d *data.Data,
	guard *service.SessionStatusGuard,
	orchCache *biz.OrchestrationCache,
	sessions *biz.SessionUsecase,
	chatSvc *service.ChatService,
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
					if len(loggingSinks) > 0 {
						// Config-driven: create eventbus sinks from config
						for _, s := range loggingSinks {
							cfg := protoSinkToConfig(s)
							if cfg.Type != "eventbus" {
								continue
							}
							sink, err := logpipeline.NewSinkFromConfig(cfg, logpipeline.SinkFactoryDeps{
								EventBusPublisher: event.NewLogPipelinePublisher(eventInfra.MonitorBus),
							})
							if err != nil {
								lg.Warn("failed to create eventbus sink from config", loggateway.Str("sink", cfg.Name), loggateway.Err(err))
								continue
							}
							pipeline.AddSink(sink)
						}
					} else {
						// Default: add eventbus sink with "info" level
						pipeline.AddSink(logpipeline.NewEventBusSink(event.NewLogPipelinePublisher(eventInfra.MonitorBus), "info"))
					}
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
				service.SeedBuiltinIndustryAgents(startCtx, agentUC, teamUC, taxonomyUC, biz.ScenarioDir(), data.NewSeedVersionRepo(d), lg)
				logger.Log(log.LevelInfo, "msg", "industry agent seed completed")
			})
			return nil
		}),
		kratos.AfterStop(func(ctx context.Context) error {
			if err := guard.OnShutdown(ctx); err != nil {
				logger.Log(log.LevelWarn, "msg", "session status guard shutdown failed", "error", err.Error())
			}
			if chatSvc != nil {
				if err := chatSvc.Close(); err != nil {
					logger.Log(log.LevelWarn, "msg", "chat service close failed", "error", err.Error())
				}
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
