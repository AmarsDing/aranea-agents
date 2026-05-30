package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/event"
	"aranea-agents/internal/scenario/loader"
)

func SeedBuiltinIndustryAgents(
	ctx context.Context,
	agentUC *biz.AgentUsecase,
	teamUC *biz.TeamUsecase,
	positionUC *biz.PositionUsecase,
	scenarioDir string,
	d *data.Data,
) {
	if d != nil && d.Ent() != nil {
		applied, err := data.IsSeedApplied(ctx, d.Ent(), data.SeedIndustryAgentsV1)
		if err != nil {
			event.CtxFlowLogError(ctx, "seed.industry_agents", "版本门控查询失败",
				event.P("error", err.Error()))
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
		PositionUC:  positionUC,
		ScenarioDir: scenarioDir,
	}

	totalAgents, totalTeams := 0, 0
	for _, ind := range industries {
		ac, tc, err := loader.SeedFromYAML(ctx, deps, ind, false)
		if err != nil {
			event.CtxFlowLogError(ctx, "seed.industry_agents", fmt.Sprintf("种子 %s 失败", ind),
				event.P("industry", ind), event.P("error", err.Error()))
			continue
		}
		totalAgents += ac
		totalTeams += tc
	}

	if totalAgents > 0 || totalTeams > 0 {
		event.CtxFlowLogDone(ctx, "seed.industry_agents", "行业模板种子完成",
			event.P("agents", fmt.Sprintf("%d", totalAgents)),
			event.P("teams", fmt.Sprintf("%d", totalTeams)))
	}

	if d != nil && d.Ent() != nil {
		if err := data.MarkSeedApplied(ctx, d.Ent(), data.SeedIndustryAgentsV1, "industry_agents_v1"); err != nil {
			event.CtxFlowLogError(ctx, "seed.industry_agents", "版本标记失败",
				event.P("error", err.Error()))
		}
	}
}
