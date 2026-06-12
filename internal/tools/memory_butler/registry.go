package memory_butler

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Deps holds the external dependencies for all memory butler tools.
type Deps struct {
	Analytics     *biz.ExperienceAnalyticsUsecase
	MemoryAdmin   *biz.MemoryAdminUsecase
	Embedder      skill.SkillEmbedder
	// EventBus is reserved for future event-driven memory operations.
	EventBus      contract.Bus
	Agents        biz.AgentRuntimeSettingsRepo
	LG            loggateway.Logger
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
