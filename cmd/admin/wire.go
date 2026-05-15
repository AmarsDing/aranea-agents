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
	"aranea-agents/internal/data"
	"aranea-agents/internal/provider"
	"aranea-agents/internal/runtimedeps"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/internal/skill/watch"
	"aranea-agents/internal/team"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func provideCronRunnerDeps(
	cron biz.CronRepo,
	session *biz.SessionUsecase,
	teams biz.TeamRepository,
	agents biz.AgentRepository,
	teamSSE *biz.TeamRunEventBroker,
) cronrunner.Deps {
	return cronrunner.Deps{
		Cron:    cron,
		Session: session,
		Teams:   teams,
		Agents:  agents,
		TeamSSE: teamSSE,
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

func provideSessionTitleGenerator(catalog *biz.LlmProviderModelUsecase, rt *runtimedeps.Runtime) biz.SessionTitleGenerator {
	if catalog == nil {
		return biz.NewNoopSessionTitleGenerator()
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	return service.NewLLMSessionTitleGenerator(catalog, &provider.RoundTrip{HTTP: httpClient})
}

func provideChatServiceDeps(
	broker *biz.TeamRunEventBroker,
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
	rt *runtimedeps.Runtime,
	compress biz.NativeTurnCompressor,
	monitorLogs *biz.MonitorLogBroker,
) service.ChatServiceDeps {
	return service.ChatServiceDeps{
		Broker:       broker,
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
		RT:           rt,
		Compress:     compress,
		MonitorLogs:  monitorLogs,
	}
}

// wireOut is non-cleanup inject outputs (cleanup must be a top-level injector return for Wire).
type wireOut struct {
	App        *kratos.App
	CronRunner *cronrunner.Runner
	SkillWatch *watch.Runner
}

func provideWireOut(app *kratos.App, runner *cronrunner.Runner, skillWatch *watch.Runner) wireOut {
	return wireOut{App: app, CronRunner: runner, SkillWatch: skillWatch}
}

// wireApp init kratos application.
func wireApp(*conf.Server, *conf.Data, log.Logger) (wireOut, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		provideCronRunnerDeps,
		provideCronRunner,
		provideSkillWatchRunner,
		provideSessionTitleGenerator,
		provideChatServiceDeps,
		runtimedeps.NewRuntime,
		newApp,
		provideWireOut,
	))
}
