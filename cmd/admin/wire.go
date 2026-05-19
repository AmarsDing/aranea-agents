//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	artifacttrpc "aranea-agents/internal/artifact/trpc"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/sessionmemory"
	aramemory "aranea-agents/internal/memory"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/internal/event"
	graphadapter "aranea-agents/internal/graph/adapter"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/mcp/health"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

func provideCronRunnerDeps(
	cron biz.CronRepo,
	session *biz.SessionUsecase,
	teams biz.TeamRepository,
	agents biz.AgentRepository,
	eventBus event.Bus,
	chat *service.ChatService,
) cronrunner.Deps {
	return cronrunner.Deps{
		Cron:     cron,
		Session:  session,
		Teams:    teams,
		Agents:   agents,
		EventBus: eventBus,
		Chat:     chat,
	}
}

func provideCronRunner(deps cronrunner.Deps) *cronrunner.Runner {
	if strings.TrimSpace(os.Getenv("CRON_RUNNER_DISABLED")) == "1" {
		return nil
	}
	return cronrunner.NewRunner(deps)
}

func provideSkillWatchRunner(skillUC *biz.SkillUsecase, sys biz.SystemSettingRepo, logger log.Logger) *watch.Runner {
	if strings.TrimSpace(os.Getenv("SKILL_WATCH_DISABLED")) == "1" {
		return nil
	}
	return watch.NewRunner(skillUC, sys, logger)
}

func provideSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, _ rt.PersistenceSet) biz.SessionTitleGenerator {
	if catalog == nil {
		return biz.NewNoopSessionTitleGenerator()
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	return service.NewLLMSessionTitleGenerator(catalog, &provider.RoundTrip{HTTP: httpClient})
}

func provideRunRegistry() *rt.RunRegistry {
	return rt.NewRunRegistry()
}

func provideChatServiceDeps(
	runs *rt.RunRegistry,
	teams biz.TeamRepository,
	teamsNative *team.Runner,
	usage *biz.UsageUsecase,
	sessions *biz.SessionUsecase,
	agents biz.AgentRepository,
	agentsUC *biz.AgentUsecase,
	toolsCatalog biz.ToolRepo,
	toolUC *biz.ToolUsecase,
	llmCatalog *biz.LlmProviderModelUsecase,
	skillUC *biz.SkillUsecase,
	sys biz.SystemSettingRepo,
	persist rt.PersistenceSet,
	compress biz.NativeTurnCompressor,
	eventBus event.Bus,
	pluginRT *plugintrpc.Runtime,
	pluginMgr *plugintrpc.Manager,
	skillDBRepo trpcskill.Repository,
	a2aUC *biz.A2AUsecase,
) service.ChatServiceDeps {
	return service.ChatServiceDeps{
		Runs:         runs,
		Teams:        teams,
		TeamsNative:  teamsNative,
		Usage:        usage,
		Sessions:     sessions,
		Agents:       agents,
		AgentsUC:     agentsUC,
		ToolsCatalog: toolsCatalog,
		ToolUC:       toolUC,
		LLMCatalog:   llmCatalog,
		SkillUC:      skillUC,
		Sys:          sys,
		Persist:      persist,
		Compress:     compress,
		EventBus:     eventBus,
		PluginRT:      pluginRT,
		PluginManager: pluginMgr,
		SkillDBRepo:   skillDBRepo,
		A2AUC:         a2aUC,
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
}

func provideMemoryService(persist rt.PersistenceSet) *service.MemoryService {
	return service.NewMemoryService(persist.Memory.Admin)
}

func provideTRPCSessionService(d *data.Data) trpcsession.Service {
	if d == nil {
		return rt.NewTRPCSessionService(nil)
	}
	return rt.NewTRPCSessionService(d.RawDB())
}

func provideGraphCheckpointSaver(d *data.Data) (*graphtrpc.SQLiteCheckpointSaver, error) {
	if d == nil {
		return nil, fmt.Errorf("data is nil")
	}
	return rt.NewGraphCheckpointSaver(d.RawDB())
}

func provideArtifactRuntimeService(uc *biz.ArtifactUsecase) trpcartifact.Service {
	if uc == nil {
		return nil
	}
	return artifacttrpc.NewServiceAdapter(uc)
}

// provideAutoMemoryWorker wires the cron auto-memory extraction worker.
// EP-RT-03: injects SessionUsecase + SQLite memory service so extraction writes to session_memory.
func provideAutoMemoryWorker(sessions *biz.SessionUsecase, agents *biz.AgentUsecase, memStore *sessionmemory.Store) *jobs.AutoMemoryWorker {
	var mem trpcmemory.Service
	if memStore != nil {
		mem = memtrpc.NewSQLiteMemoryService(memStore)
	}
	return jobs.NewAutoMemoryWorker(0, sessions, agents, mem, aramemory.NewL4GraphWriter(memStore))
}

func provideMCPHealthRunnerDeps(mcpRepo biz.MCPServerRepo, mcpUC *biz.MCPServerUsecase) health.Deps {
	return health.Deps{
		MCP: mcpRepo,
		UC:  mcpUC,
	}
}

func provideMCPHealthRunner(deps health.Deps) *health.Runner {
	if strings.TrimSpace(os.Getenv("MCP_HEALTH_DISABLED")) == "1" {
		return nil
	}
	return health.NewRunner(deps)
}

func providePluginStatsRecorder(repo biz.PluginRepo) plugintrpc.StatsRecorder {
	return plugintrpc.NewRepoStatsRecorder(repo)
}

// wireOut is non-cleanup inject outputs (cleanup must be a top-level injector return for Wire).
type wireOut struct {
	App            *kratos.App
	CronRunner     *cronrunner.Runner
	SkillWatch     *watch.Runner
	AutoMemory     *jobs.AutoMemoryWorker
	MCPHealthProbe *health.Runner
}

func provideWireOut(app *kratos.App, runner *cronrunner.Runner, skillWatch *watch.Runner, autoMem *jobs.AutoMemoryWorker, mcpHealth *health.Runner) wireOut {
	return wireOut{App: app, CronRunner: runner, SkillWatch: skillWatch, AutoMemory: autoMem, MCPHealthProbe: mcpHealth}
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, log.Logger) (wireOut, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		event.ProviderSet,
		service.ProviderSet,
		provideCronRunnerDeps,
		provideCronRunner,
		provideSkillWatchRunner,
		provideSessionTitleGenerator,
		provideRunRegistry,
		provideChatServiceDeps,
		provideRunCanceller,
		provideChatSender,
		provideArtifactRuntimeService,
		provideMemoryService,
		provideTRPCSessionService,
		provideGraphCheckpointSaver,
		wire.Bind(new(trpcgraph.CheckpointSaver), new(*graphtrpc.SQLiteCheckpointSaver)),
		rt.NewPersistenceSet,
		providePluginStatsRecorder,
		plugintrpc.NewRuntime,
		plugintrpc.NewManager,
		graphtrpc.NewRegistry,
		graphadapter.NewGraphBuilderFactory,
		provideAutoMemoryWorker,
		provideMCPHealthRunnerDeps,
		provideMCPHealthRunner,
		newApp,
		provideWireOut,
	))
}
