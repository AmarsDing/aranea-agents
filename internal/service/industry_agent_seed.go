package service

import (
	"context"
	"fmt"
	"log"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/scenario/loader"
)

func SeedBuiltinIndustryAgents(
	ctx context.Context,
	agentUC *biz.AgentUsecase,
	teamUC *biz.TeamUsecase,
	taxonomyUC *biz.TaxonomyUsecase,
	scenarioDir string,
	seedRepo biz.SeedVersionRepo,
) {
	if seedRepo != nil {
		applied, err := seedRepo.IsApplied(ctx, biz.SeedVersionIndustryAgentsV1)
		if err != nil {
			event.CtxFlowLogError(ctx, "seed.industry_agents", "版本门控查询失败",
				event.P("error", err.Error()))
			log.Printf("[SEED] version check error: %v", err)
			return
		}
		if applied {
			log.Printf("[SEED] version %d already applied, skipping", biz.SeedVersionIndustryAgentsV1)
			return
		}
		log.Printf("[SEED] version %d not applied, running seed", biz.SeedVersionIndustryAgentsV1)
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
			event.CtxFlowLogError(ctx, "seed.industry_agents", fmt.Sprintf("种子 %s 失败", ind),
				event.P("industry", ind), event.P("error", err.Error()))
			log.Printf("[SEED] industry %s failed: agents=%d teams=%d error=%v", ind, ac, tc, err)
			hasError = true
			continue
		}
		log.Printf("[SEED] industry %s ok: agents=%d teams=%d", ind, ac, tc)
		totalAgents += ac
		totalTeams += tc
	}

	if totalAgents > 0 || totalTeams > 0 {
		event.CtxFlowLogDone(ctx, "seed.industry_agents", "行业模板种子完成",
			event.P("agents", fmt.Sprintf("%d", totalAgents)),
			event.P("teams", fmt.Sprintf("%d", totalTeams)))
	}

	if !hasError && seedRepo != nil {
		if err := seedRepo.MarkApplied(ctx, biz.SeedVersionIndustryAgentsV1, "industry_agents_v1"); err != nil {
			event.CtxFlowLogError(ctx, "seed.industry_agents", "版本标记失败",
				event.P("error", err.Error()))
		}
	}
}
