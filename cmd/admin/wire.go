//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"net/http"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/internal/cronrunner"
	"aranea-agents/internal/cronrunner/jobs"
	"aranea-agents/internal/data"
	"aranea-agents/internal/event"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/mcp/health"
	memtrpc "aranea-agents/internal/memory/trpc"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"

	"aranea-agents/internal/data/sessionmemory"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
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

func provideChatServiceDeps(
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
	skillDBRepo trpcskill.Repository,
) service.ChatServiceDeps {
	return service.ChatServiceDeps{
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
		PluginRT:     pluginRT,
		SkillDBRepo:  skillDBRepo,
	}
}

func provideRunCanceller(svc *service.ChatService) server.RunCanceller {
	return svc
}

func provideChatSender(svc *service.ChatService) server.ChatSender {
	return svc
}

// provideAutoMemoryWorker wires the cron auto-memory extraction worker.
// EP-RT-03: injects SessionUsecase + SQLite memory service so extraction writes to session_memory.
func provideAutoMemoryWorker(sessions *biz.SessionUsecase, store *sessionmemory.Store) *jobs.AutoMemoryWorker {
	mem := memtrpc.NewSQLiteMemoryService(store)
	return jobs.NewAutoMemoryWorker(0, sessions, mem)
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
		provideChatServiceDeps,
		provideRunCanceller,
		provideChatSender,
		rt.NewPersistenceSet,
		plugintrpc.NewRuntime,
		graphtrpc.NewRegistry,
		graphadapter.NewGraphBuilderFactory,
		provideAutoMemoryWorker,
		provideMCPHealthRunnerDeps,
		provideMCPHealthRunner,
		newApp,
		provideWireOut,
	))
}
