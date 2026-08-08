package main

import (
	"context"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
	"aranea-agents/pkg/safego"

	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/telemetry"
	"aranea-agents/pkg/auth"

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
	graphBuildDeps *chatagent.TRPCBuilderDeps,
	knowledgeSvc *service.KnowledgeService,
	vaultSyncSup *knowledge.VaultSyncSupervisor,
) *kratos.App {
	// startupBegin approximates the start of the P1 migration window: NewData
	// (which launches the P1 goroutine) runs immediately before newApp inside
	// wireApp, so this timestamp is close enough for a coarse duration.
	startupBegin := time.Now()

	// Register pkg/auth flow-log hooks (gRPC unauthenticated requests) backed
	// by the monitor bus. pkg/ libraries cannot import internal/event, so the
	// hook implementation lives in internal/server and is wired here.
	if eventInfra != nil {
		server.RegisterAuthFlowHooks(eventInfra.MonitorEventBus, lg)
	}

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

			// Inject TeamCompletionChecker into graph builder deps to prevent LLM polling.
			// This breaks the circular dependency: SpiritTeamUsecase → GraphBuildDeps → SpiritTeamUsecase
			if spiritUC != nil && graphBuildDeps != nil {
				graphBuildDeps.SetTeamCompletionChecker(biz.NewTeamCompletionCheckerAdapter(spiritUC))
			}

			// Inject vault sync controller into knowledge service (P1-3 production wiring).
			if knowledgeSvc != nil && vaultSyncSup != nil {
				knowledgeSvc.SetVaultSyncController(vaultSyncSup)
			}

			// Start readiness-dependent initialization in background.
			// The HTTP server now starts immediately so /healthz can report
			// "starting" (503) while P1 migrations run. A readiness middleware
			// blocks all non-infrastructure routes until ready.
			runPostReadiness := func() {
				// B10：team × graph 存量物化迁移（幂等，单队失败 warn 继续，
				// 不阻塞启动）。依赖 TeamCompiler 无法放入 data 层 L3 迁移
				// 注册表，故在 readiness 门控后以后台任务执行。
				if teamUC != nil {
					safego.Go(consumerCtx, "startup.team_graph_migration", func() {
						teamUC.MigrateLegacyEmbeddedGraphs(consumerCtx)
					})
				}
				// SP1-D：readiness 后全量构建知识链接内存图（派生索引，
				// 失败仅降级——反链查询 DB 兜底，不阻塞启动）。
				if knowledgeSvc != nil {
					safego.Go(consumerCtx, "startup.knowledge_link_index", func() {
						_ = knowledgeSvc.LoadKnowledgeLinkIndex(consumerCtx)
					})
				}
				startReadinessDependentServices(consumerCtx, guard, orchCache, sideConsumers, sessions, eventInfra, pipeline, loggingSinks, spiritUC, vaultSyncSup, graphBuildDeps, lg)
				emitStartupFlows(consumerCtx, eventInfra, lg, startupBegin)
			}
			if d != nil {
				if gate := d.Readiness(); gate != nil {
					safego.Go(ctx, "startup.post_readiness", func() {
						if err := gate.Wait(ctx); err != nil {
							lg.Warn("post-readiness: data readiness wait failed", loggateway.StepID("startup.gate"), loggateway.Err(err))
							emitStartupMigrationFailure(consumerCtx, eventInfra, lg, gate.FailedReason())
							return
						}
						lg.Info("post-readiness: data ready, starting dependent services", loggateway.StepID("startup.gate"))
						runPostReadiness()
					})
				} else {
					// No readiness gate (unlikely), start immediately.
					runPostReadiness()
				}
			} else {
				// No data layer (unlikely), start immediately.
				runPostReadiness()
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
			shutdownFlow := newSystemFlowEmitter(ctx, eventInfra, lg)
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
			// P1-3：停止全部 vault 同步循环。
			if vaultSyncSup != nil {
				vaultSyncSup.Stop()
			}
			// Close process-level resources (build cache, etc.) in LIFO order
			// after the chat service has stopped accepting requests (A3).
			if lifecycleMgr != nil {
				lifecycleMgr.Close()
			}
			// Emit before consumerCancel/pipeline.Close so the event still reaches
			// the flow log persist consumer and the log pipeline.
			if shutdownFlow != nil {
				shutdownFlow.LogDone("system.startup.shutdown", "服务关闭完成")
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
	vaultSyncSup *knowledge.VaultSyncSupervisor,
	graphBuildDeps *chatagent.TRPCBuilderDeps,
	lg loggateway.Logger,
) {
	// P1-3：DB ready 后拉起全部存量 vault 的同步循环（root_path 非空）。
	if vaultSyncSup != nil {
		vaultSyncSup.StartAll(ctx)
	}
	// MCP 连接预热：后台建立全部启用服务器的池化连接（stdio 子进程/TCP
	// 会话），首个用户请求不再承担冷连接成本。单服务器失败仅告警不阻塞。
	if graphBuildDeps != nil {
		safego.Go(ctx, "startup.mcp_prewarm", func() {
			chatagent.PrewarmMCPToolSets(ctx, *graphBuildDeps, lg)
		})
	}
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

// newSystemFlowEmitter builds a system-domain flow emitter over the shared
// monitor bus. Returns nil when the bus is unavailable (tests).
func newSystemFlowEmitter(ctx context.Context, infra *event.Infra, lg loggateway.Logger) *event.TraceEmitter {
	if infra == nil || infra.MonitorEventBus == nil {
		return nil
	}
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     lg,
		Infra:  infra,
	})
}

// emitStartupFlows emits the one-shot startup flow logs (telemetry, migration,
// seed, ready, auth bypass) after readiness-dependent services — including the
// flow log persist consumer — have started, so the events are persisted.
// Migration/seed run inside data.NewData (pre-bus), so they are emitted here
// retroactively with a coarse duration measured from newApp entry.
func emitStartupFlows(ctx context.Context, infra *event.Infra, lg loggateway.Logger, startupBegin time.Time) {
	em := newSystemFlowEmitter(ctx, infra, lg)
	if em == nil {
		return
	}
	// Telemetry init ran pre-bus (main.go); emit its recorded outcome now.
	switch res := telemetry.LastInitResult(); res.Status {
	case telemetry.InitStatusOK:
		em.LogDone("system.telemetry.init", "遥测初始化完成",
			event.P("endpoint", res.Endpoint), event.P("protocol", res.Protocol))
	case telemetry.InitStatusNoop:
		em.LogSkip("system.telemetry.noop", "遥测未配置，使用 noop 提供者")
	case telemetry.InitStatusError:
		em.LogError("system.telemetry.error", "遥测初始化失败",
			event.P("endpoint", res.Endpoint), event.P("protocol", res.Protocol),
			event.P("error", res.ErrMessage))
	}
	// Gate 已开放 ⇒ P1（L1/L2/L3 迁移 + 种子）全部完成。
	em.LogDone("system.startup.migration", "数据库迁移完成",
		event.P("stages", "L1,L2,L3"),
		event.P("duration_ms", time.Since(startupBegin).Milliseconds()))
	em.LogDone("system.startup.seed", "基础数据种子完成")
	em.LogDone("system.startup.ready", "服务就绪")
	// 认证绕过状态（WarnIfBypassEnabled 在 main.go 早期运行，此处延迟发射）。
	if auth.HTTPAuthBypassEnabled() {
		em.LogWarn("system.auth.bypass_warn", "", "认证绕过告警：KRATOS_HTTP_AUTH_DISABLED 已启用，请勿在生产环境使用")
		em.LogWarn("system.auth.bypass_active", "", "认证绕过已启用：所有请求以 UserID=1 (admin) 身份执行")
	} else if auth.BypassRequested() {
		em.LogWarn("system.auth.bypass_refused", "", "认证绕过被拒绝：KRATOS_HTTP_AUTH_DISABLED 已设置但 DEPLOY_ENV 非 dev/test")
	}
}

// emitStartupMigrationFailure emits the migration failure flow log when the
// readiness gate reports failed (P1 步骤失败，服务不就绪).
func emitStartupMigrationFailure(ctx context.Context, infra *event.Infra, lg loggateway.Logger, reason string) {
	em := newSystemFlowEmitter(ctx, infra, lg)
	if em == nil {
		return
	}
	em.LogError("system.startup.migration", "数据库迁移失败",
		event.P("error", reason))
}
