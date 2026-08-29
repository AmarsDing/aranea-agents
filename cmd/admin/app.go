package main

import (
	"context"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/data"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/internal/sandbox"
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
	embedder *knowledge.MultiProviderEmbedder,
	gateCards *service.ChannelGateCards,
	agentBridgeSvc *service.AgentBridgeService,
	sandboxMgr *sandbox.Manager,
	decisionLC decision.Lifecycle,
	decisions decision.Collector,
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
			if chatSvc != nil && agentBridgeSvc != nil {
				chatSvc.BindAgentBridge(agentBridgeSvc)
			}
			// M80 1.5: AgentBridge 审批链双写 decision_records（hitl_approval）。
		if agentBridgeSvc != nil && decisions != nil {
			agentBridgeSvc.SetDecisionCollector(decisions)
		}
		// P1-④（2026-08-30）：team run 恢复（hitl_approval 留痕）与
		// output_policy 输出拦截（system_guard 留痕）的证据通道接线。
		if teamSvc != nil {
			var monBus contract.MonitorBus
			if eventInfra != nil {
				monBus = eventInfra.MonitorEventBus
			}
			teamSvc.SetDecisionEvidence(decisions, monBus)
		}
		if pluginRT != nil && decisions != nil {
			pluginRT.SetDecisionCollector(decisions)
		}

		if knowledgeSvc != nil && vaultSyncSup != nil {
			knowledgeSvc.SetVaultSyncController(vaultSyncSup)
		}
			// Wave 2：写回重放双点接线 + 跨进程重建/重嵌入租约。
			if knowledgeSvc != nil {
				knowledgeSvc.BindDerivedIndexHooks()
				if d != nil {
					knowledgeSvc.SetJobLocker(service.NewPGKnowledgeJobLocker(d.Postgres(), lg))
				}
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
				// 76：编程桥接活跃任务恢复——服务重启后残留的 dispatched/running
				// 任务全部标记 failed（reason=service_restart），幂等。
				if agentBridgeSvc != nil {
					safego.Go(consumerCtx, "startup.agentbridge_recover", func() {
						if err := agentBridgeSvc.RecoverActiveTasks(consumerCtx); err != nil {
							lg.Warn("agentbridge active-task recovery failed", loggateway.StepID("startup.agentbridge_recover"), loggateway.Err(err))
						}
					})
				}
				startReadinessDependentServices(consumerCtx, guard, orchCache, sideConsumers, sessions, eventInfra, pipeline, loggingSinks, spiritUC, vaultSyncSup, graphBuildDeps, chatSvc, embedder, gateCards, lg)
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
			// M82: start the sandbox pool replenisher + GC + startup reconcile.
			// consumerCtx is long-lived (cancelled in AfterStop) and the manager
			// has no DB dependency, so it starts right after server boot.
			// No-op (with a warn log) when disabled or the engine is degraded.
			if sandboxMgr != nil {
				sandboxMgr.Start(consumerCtx)
			}
			// M80: start the decision outbox worker (flush + retry-queue replay).
			// Emits only happen on user turns (readiness-gated), so starting here
			// races nothing; the startup replay self-heals on the 30s ticker when
			// the DB is not ready yet. Logs "decision collector started".
			if decisionLC != nil {
				decisionLC.Start(consumerCtx)
			}

			return nil
		}),
		kratos.AfterStop(func(ctx context.Context) error {
			shutdownFlow := newSystemFlowEmitter(ctx, eventInfra, lg)
			// M82: stop pool/gc loops. Live sandboxes are intentionally NOT
			// destroyed here — the next boot's reconcile pass reaps them.
			if sandboxMgr != nil {
				sandboxMgr.Close()
			}
			// M80: stop the decision worker — Stop drains the buffered batch with
			// a final flush before the data layer closes.
			if decisionLC != nil {
				decisionLC.Stop()
			}
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
	chatSvc *service.ChatService,
	embedder *knowledge.MultiProviderEmbedder,
	gateCards *service.ChannelGateCards,
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
	// __spirit__ agent 启动预构建：消除进程首个语音/聊天 Turn 的冷构建
	// （实测 2.6-8s）。后台执行，失败仅 Warn 不阻塞启动（Always-Ready）。
	if chatSvc != nil {
		safego.Go(ctx, "startup.spirit_agent_prewarm", func() {
			chatSvc.PrewarmSpiritAgent(ctx)
		})
	}
	// C3：embedding 冷启动预热——最小 ping 请求把 TCP/TLS 握手 + 远端模型
	// 冷启动（实测 ~2.4s）移出首个记忆召回 Turn。与 spirit 预构建并列；
	// 内部 60s 成功去重，失败仅 Warn（K3），未配置静默跳过。
	if embedder != nil {
		safego.Go(ctx, "startup.embedding_prewarm", func() {
			_ = embedder.Prewarm(ctx)
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
	// 渠道交互门卡片（确认/澄清）：订阅 step 事件发卡/PATCH。
	if gateCards != nil {
		gateCards.Start(ctx)
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
