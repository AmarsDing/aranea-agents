package memory_butler

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Deps holds the external dependencies for all memory butler tools.
type Deps struct {
	Analytics   *biz.ExperienceAnalyticsUsecase
	MemoryAdmin *biz.MemoryAdminUsecase
	Embedder    skill.SkillEmbedder
	Agents      biz.AgentRuntimeSettingsRepo
	LG          loggateway.Logger
}

// RegisterAll creates and returns all memory butler tools. It validates that
// required dependencies are present and ensures LG is non-nil (defaulting to
// a Noop logger) so that downstream tools can call deps.LG.Warn without nil
// checks.
func RegisterAll(deps Deps) []trpctool.Tool {
	if deps.Analytics == nil || deps.MemoryAdmin == nil || deps.Agents == nil {
		return nil
	}
	if deps.LG == nil {
		deps.LG = loggateway.NewNoop()
	}
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
