package memory_butler

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
	"aranea-agents/internal/event/contract"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Deps holds the external dependencies for all memory butler tools.
type Deps struct {
	Analytics     *biz.ExperienceAnalyticsUsecase
	MemoryAdmin   *biz.MemoryAdminUsecase
	Embedder      skill.SkillEmbedder
	EventBus      contract.Bus
	Agents        biz.AgentRuntimeSettingsRepo
}

// RegisterAll creates and returns all memory butler tools.
func RegisterAll(deps Deps) []trpctool.Tool {
	return []trpctool.Tool{
		newAnalyzeMemoryQualityTool(deps),
		newSelectiveRememberTool(deps),
		newForgetLowQualityTool(deps),
		newForgetInactiveTool(deps),
		newDeduplicateMemoriesTool(deps),
		newConsolidateEpisodesTool(deps),
		newDreamCycleTool(deps),
	}
}
