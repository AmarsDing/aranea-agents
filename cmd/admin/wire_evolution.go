package main

import (
	"net/http"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	skillevolution "aranea-agents/internal/skill/evolution"
	"aranea-agents/pkg/loggateway"

	trpcevolution "trpc.group/trpc-go/trpc-agent-go/evolution"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// provideEvolutionService 装配框架 v1.11 技能演化 service（hold-all 模式，
// 详见 internal/skill/evolution 包注释）。模型目录/技能 repo 缺失时返回 nil，
// runner 侧自动跳过演化学习。
func provideEvolutionService(catalog *biz.LlmProviderModelUsecase, repo trpcskill.Repository, usageRef *biz.UsageUsecaseRef, lg loggateway.Logger) trpcevolution.Service {
	svc := skillevolution.NewService(skillevolution.Config{
		Catalog:  catalog,
		RT:       &provider.RoundTrip{HTTP: &http.Client{Timeout: 90 * time.Second}},
		Repo:     repo,
		UsageRef: usageRef,
		Lg:       lg,
	})
	if svc == nil {
		return nil
	}
	// SKL-02: hot-add / catalog change re-resolves the review model without restart.
	catalog.OnCatalogChange(func() {
		if err := svc.ResetBuild(); err != nil && lg != nil {
			lg.Warn("evolution: ResetBuild after catalog change failed",
				loggateway.StepID("skill.evolution_reset_build"),
				loggateway.Err(err))
		}
	})
	return svc
}

