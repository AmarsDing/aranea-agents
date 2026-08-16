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
	// Knowledge 为 M4 知识库词条治理入口（可选；nil 时不挂载 knowledge_curate
	// 工具，dream_cycle 跳过 curate_knowledge 步骤）。
	Knowledge *biz.KnowledgeUsecase
	LG        loggateway.Logger
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
	tools := []trpctool.Tool{
		newAnalyzeMemoryQualityTool(deps),
		newSelectiveRememberTool(deps),
		newForgetLowQualityTool(deps),
		newForgetInactiveTool(deps),
		newDeduplicateMemoriesTool(deps),
		newConsolidateEpisodesTool(deps),
		newDreamCycleTool(deps),
	}
	// M4 知识库词条治理：Knowledge 未接线时不挂载（与既有工具解耦，降级安全）。
	// governance_proposals/governance_resolve 为提案人工二审出口（M4 补丁）。
	if deps.Knowledge != nil {
		tools = append(tools,
			newKnowledgeCurateTool(deps),
			newGovernanceProposalsTool(deps),
			newGovernanceResolveTool(deps),
		)
	}
	return tools
}
