package adapter

import (
	"context"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
)

func ValidateBizGraphBuildConfig(ctx context.Context, cfg biz.GraphBuildConfig, agentChecker graphtrpc.AgentExistenceChecker) *graphtrpc.ValidationResult {
	return graphtrpc.ValidateGraph(ctx, &cfg, agentChecker, nil)
}
