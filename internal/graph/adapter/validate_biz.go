package adapter

import (
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
)

// ValidateBizGraphBuildConfig validates a biz-level graph build config.
func ValidateBizGraphBuildConfig(cfg biz.GraphBuildConfig, agentChecker graphtrpc.AgentExistenceChecker) *graphtrpc.ValidationResult {
	trpcCfg := bizCfgToTrpc(cfg)
	return graphtrpc.ValidateGraph(&trpcCfg, agentChecker, nil)
}
