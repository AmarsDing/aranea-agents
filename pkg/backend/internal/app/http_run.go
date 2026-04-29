package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	toolstorage "arenea/backend/internal/capability/storage"
	adkr "arenea/backend/internal/conversation/adapters/adkruntime"
	"arenea/backend/internal/repository"
	"arenea/backend/internal/service"
	"arenea/backend/internal/telemetry"
	"arenea/backend/internal/transport"
	"arenea/backend/internal/util"
)

// ServerOptions 控制 [Run] 一次调用。零值会回退到与旧版 main() 相同的环境变量
// （DB_PATH、HTTP_ADDR），以便现有部署行为不变。
type ServerOptions struct {
	// DBPath 为要打开的 SQLite 数据库文件。为空时依次使用 DB_PATH，最终默认 data/arenea.db。
	DBPath string
	// Addr 为监听地址（host:port）。为空则 HTTP_ADDR，再默认 :8080。
	Addr string
	// TelemetryService 为遥测上报的服务名。为空则 "arenea-backend"。
	TelemetryService string
	// SkipTelemetry 跳过全局遥测初始化。适用于继承父 CLI 遥测的进程内 Web SubLauncher。
	SkipTelemetry bool
	// Ready 在 HTTP 服务已绑定并开始接受连接后关闭。调用方可用其打印横幅或打开浏览器标签，
	// 而无需与监听器竞态。
	Ready chan<- ListenInfo
	// Logger 覆盖默认的 *log.Logger。在 CLI 内嵌服务时可传入 discard 日志以静音。
	Logger *log.Logger
}

// ListenInfo 在监听器一旦绑定后通过 ServerOptions.Ready 发布，暴露服务实际监听的地址
// （在调用方使用 ":0" 随机端口时尤其有用）。
type ListenInfo struct {
	Addr string
}

// Run 启动后端并阻塞，直到 ctx 取消或 HTTP 监听失败。它是服务装配的唯一事实来源，
// 独立服务器与内嵌 Web 启动器共用。
func Run(ctx context.Context, opts ServerOptions) error {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	dbPath := firstNonEmptyString(opts.DBPath, os.Getenv("DB_PATH"), "data/arenea.db")
	addr := firstNonEmptyString(opts.Addr, os.Getenv("HTTP_ADDR"), ":8080")
	if !opts.SkipTelemetry {
		telemetry.Setup(firstNonEmptyString(opts.TelemetryService, "arenea-backend"))
	}

	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		return fmt.Errorf("init repository: %w", err)
	}
	defer repo.Close()
	if err = repo.Migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	toolStore := toolstorage.NewSQLiteStore(repo.DB())
	runtimeAdapter := adkr.NewADKRuntimeAdapter()
	runtimeAdapter.SetToolCatalogSource(toolStore)
	agentSvc := service.NewAgentService(repo)
	teamSvc := service.NewTeamService(repo)
	sessionSvc := service.NewSessionService(repo)
	chatSvc := service.NewChatService(repo, runtimeAdapter)
	auditSvc := service.NewAuditService(repo)
	platformSvc := service.NewPlatformService(repo)
	usageSvc := service.NewUsageService(repo)
	pluginSvc := service.NewPluginService(repo)
	if err = pluginSvc.SyncBuiltins(); err != nil {
		return fmt.Errorf("sync builtin plugins: %w", err)
	}
	runtimeAdapter.SetPluginSource(pluginSvc)
	channelSvc := service.NewChannelService(repo)
	channelSvc.SetRuntimeReloader(runtimeAdapter)
	runtimeAdapter.SetChannelSource(channelSvc)
	if err = runtimeAdapter.ReloadChannels(ctx); err != nil {
		return fmt.Errorf("load channel runtime configs: %w", err)
	}
	skillStorageRoot := util.ResolveSkillStorageRoot()
	logger.Printf("skill storage root: %s", skillStorageRoot)
	skillSvc := service.NewSkillService(repo, runtimeAdapter, skillStorageRoot)
	toolSvc := service.NewToolService(toolStore)
	toolSvc.SetRuntimeSettingsStore(repo)
	if evo := chatSvc.AgentEvolution(); evo != nil {
		toolSvc.SetEvolutionPolicySource(evo)
	}
	cronRunner := service.NewCronRunner(repo, chatSvc)

	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		skillSvc.StartDirectorySync(ctx, 1)
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		cronRunner.Start(ctx, time.Minute)
	}()
	if l3Svc := chatSvc.MemoryL3(); l3Svc != nil {
		background.Add(1)
		go func() {
			defer background.Done()
			runMemoryL3DecayLoop(ctx, l3Svc, logger)
		}()
	}
	if evoSvc := chatSvc.AgentEvolution(); evoSvc != nil {
		background.Add(1)
		go func() {
			defer background.Done()
			runEvolutionScannerLoop(ctx, repo, evoSvc, logger)
		}()
	}

	handler := transport.NewHTTPHandler(transport.Services{
		Agent:    agentSvc,
		Team:     teamSvc,
		Session:  sessionSvc,
		Chat:     chatSvc,
		Audit:    auditSvc,
		Platform: platformSvc,
		Usage:    usageSvc,
		Skill:    skillSvc,
		Tool:     toolSvc,
		Plugin:   pluginSvc,
		Channel:  channelSvc,
	})
	wrapped := StackTransportMiddleware(handler)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	if opts.Ready != nil {
		select {
		case opts.Ready <- ListenInfo{Addr: listener.Addr().String()}:
		default:
		}
	}

	srv := &http.Server{Handler: wrapped, ReadHeaderTimeout: 30 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		logger.Printf("server listening on %s", listener.Addr().String())
		serveErr <- srv.Serve(listener)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Printf("shutdown signal received")
	case e := <-serveErr:
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			runErr = e
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil && runErr == nil {
		runErr = fmt.Errorf("shutdown: %w", shutdownErr)
	}
	background.Wait()
	return runErr
}

func runMemoryL3DecayLoop(ctx context.Context, svc *service.MemoryL3Service, logger *log.Logger) {
	const interval = time.Hour
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report, err := svc.RunDecayBatch(ctx)
			if err != nil {
				logger.Printf("memory l3 decay: %v", err)
				continue
			}
			if report.Processed > 0 {
				logger.Printf("memory l3 decay: processed=%d archived=%d drop=%.3f", report.Processed, report.Archived, report.ConfidenceDrop)
			}
		}
	}
}

func runEvolutionScannerLoop(ctx context.Context, repo repository.Store, svc *service.AgentEvolutionService, logger *log.Logger) {
	const interval = 30 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	runEvolutionScannerOnce(ctx, repo, svc, logger)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runEvolutionScannerOnce(ctx, repo, svc, logger)
		}
	}
}

func runEvolutionScannerOnce(ctx context.Context, repo repository.Store, svc *service.AgentEvolutionService, logger *log.Logger) {
	agents, err := repo.ListAgents()
	if err != nil {
		logger.Printf("evolution scanner: list agents: %v", err)
		return
	}
	for _, agent := range agents {
		if ctx.Err() != nil {
			return
		}
		settings, _ := repo.GetAgentRuntimeSettings(agent.ID)
		if !settings.EvoEnabled {
			continue
		}
		report, err := svc.RunEvolutionScan(ctx, agent.ID)
		if err != nil {
			logger.Printf("evolution scanner: agent=%s err=%v", agent.ID, err)
			continue
		}
		if report.NewProposals > 0 || report.AutoApplied > 0 || report.Errors > 0 {
			logger.Printf("evolution scanner: agent=%s episodes=%d new=%d auto_applied=%d throttled=%d errors=%d note=%q",
				agent.ID, report.EpisodesScanned, report.NewProposals, report.AutoApplied, report.ThrottledProposals, report.Errors, report.Note)
		}
	}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
