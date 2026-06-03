package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/scenario/loader"
	"aranea-agents/pkg/loggateway"
)

func SeedBuiltinIndustryAgents(
	ctx context.Context,
	agentUC *biz.AgentUsecase,
	teamUC *biz.TeamUsecase,
	taxonomyUC *biz.TaxonomyUsecase,
	scenarioDir string,
	seedRepo biz.SeedVersionRepo,
	lg loggateway.Logger,
) {
	if seedRepo != nil {
		applied, err := seedRepo.IsApplied(ctx, biz.SeedVersionIndustryAgentsV1)
		if err != nil {
			lg.Error("版本门控查询失败", loggateway.StepID("seed.industry_agents"), loggateway.Err(err))
			return
		}
		if applied {
			return
		}
	}

	industries := []string{"softwaredev", "selfmedia", "finance"}
	deps := loader.Deps{
		AgentUC:     agentUC,
		TeamUC:      teamUC,
		Taxonomy:    taxonomyUC,
		ScenarioDir: scenarioDir,
	}

	totalAgents, totalTeams := 0, 0
	hasError := false
	for _, ind := range industries {
		ac, tc, err := loader.SeedFromYAML(ctx, deps, ind, false)
		if err != nil {
			lg.Error(fmt.Sprintf("种子 %s 失败", ind), loggateway.StepID("seed.industry_agents"), loggateway.Str("industry", ind), loggateway.Err(err))
			hasError = true
			continue
		}
		totalAgents += ac
		totalTeams += tc
	}

	if totalAgents > 0 || totalTeams > 0 {
		lg.Info("行业模板种子完成", loggateway.StepID("seed.industry_agents"), loggateway.Str("flow_status", "done"),
			loggateway.Str("agents", fmt.Sprintf("%d", totalAgents)),
			loggateway.Str("teams", fmt.Sprintf("%d", totalTeams)))
	}

	if !hasError && seedRepo != nil {
		if err := seedRepo.MarkApplied(ctx, biz.SeedVersionIndustryAgentsV1, "industry_agents_v1"); err != nil {
			lg.Error("版本标记失败", loggateway.StepID("seed.industry_agents"), loggateway.Err(err))
		}
	}
}
