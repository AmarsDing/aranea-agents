package tools

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/registry"
	"aranea-agents/internal/tools/transfer_spawn"

	"google.golang.org/adk/tool"
)

// ToolsFromAgentEffective maps the biz effective-tools matrix to ADK tools for llmagent.
func ToolsFromAgentEffective(eff biz.AgentEffectiveTools) ([]tool.Tool, error) {
	if !eff.ToolsEnabled {
		return nil, nil
	}
	enabled := map[string]bool{}
	for _, it := range eff.Items {
		if it.Enabled && strings.TrimSpace(it.ToolKey) != "" {
			enabled[it.ToolKey] = true
		}
	}
	if len(enabled) == 0 {
		return nil, nil
	}
	registry.ApplyEffectiveAliases(enabled)
	return registry.ADKToolsFromEnabled(enabled)
}

// ToolsForAgent loads effective tools for agentID and returns ADK tool bindings.
func ToolsForAgent(ctx context.Context, uc *biz.AgentUsecase, agentID string) []tool.Tool {
	return ToolsForAgentWithDeps(ctx, uc, nil, nil, agentID)
}

// ToolsForAgentWithDeps loads tools like ToolsForAgent when uc is non-nil; if uc is nil but
// agents and catalog are non-nil, builds a short-lived AgentUsecase so chat still mounts tools
// when the wire graph omits *AgentUsecase for ChatService.
func ToolsForAgentWithDeps(ctx context.Context, uc *biz.AgentUsecase, agents biz.AgentRepository, catalog biz.ToolRepo, agentID string) []tool.Tool {
	if strings.TrimSpace(agentID) == "" {
		return nil
	}
	if uc == nil {
		if agents == nil || catalog == nil {
			return nil
		}
		uc = biz.NewAgentUsecase(agents, catalog)
	}
	eff, err := uc.GetEffectiveTools(ctx, strings.TrimSpace(agentID))
	if err != nil {
		return nil
	}
	tools, err := ToolsFromAgentEffective(eff)
	if err != nil {
		return nil
	}
	return tools
}

// ADKToolsForAgentPolicy loads platform tools from effective policy and optionally appends spawn_subagent.
func ADKToolsForAgentPolicy(ctx context.Context, uc *biz.AgentUsecase, agents biz.AgentRepository, catalog biz.ToolRepo, agentID string, subagentsEnabled bool) []tool.Tool {
	out := ToolsForAgentWithDeps(ctx, uc, agents, catalog, agentID)
	if !subagentsEnabled {
		return out
	}
	st, err := transfer_spawn.New()
	if err != nil {
		return out
	}
	return append(out, st)
}
