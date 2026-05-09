package tools

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/mcpmount"
	"aranea-agents/internal/tools/skillruntime"

	"google.golang.org/adk/tool"
)

// TurnMount wires effective tools + skill toolsets + MCP toolsets for one llmagent turn.
type TurnMount struct {
	AgentsUC     *biz.AgentUsecase
	Agents       biz.AgentRepository
	ToolsCatalog biz.ToolRepo
	SkillUC      *biz.SkillUsecase
	Sys          biz.SystemSettingRepo
	MCP          *biz.AgentMCPTooling
}

// Attach fills tools and toolsets into pre-allocated slices using agent policy and optional user query for skill routing.
func (m TurnMount) Attach(ctx context.Context, ag biz.Agent, userQuery string, tools *[]tool.Tool, toolsets *[]tool.Toolset) error {
	if tools == nil || toolsets == nil {
		return nil
	}
	sub := false
	if ag.Settings != nil {
		sub = ag.Settings.SubagentsEnabled
	}
	adkTools := ADKToolsForAgentPolicy(ctx, m.AgentsUC, m.Agents, m.ToolsCatalog, ag.ID, sub)
	*tools = append([]tool.Tool(nil), adkTools...)

	opts := &skillruntime.SkillToolsetOptions{
		Runtime:   ag.Settings,
		UserQuery: userQuery,
	}
	ts := *toolsets
	ts = ts[:0]
	if err := skillruntime.AppendEnabledPublishedSkillToolsets(ctx, &ts, m.SkillUC, m.Sys, opts); err != nil {
		return err
	}
	if m.MCP != nil {
		servers, err := m.MCP.EffectiveServersForAgent(ctx, ag.ID)
		if err != nil {
			return err
		}
		if err := mcpmount.AppendEffectiveMCPServerToolsets(ctx, &ts, servers); err != nil {
			return err
		}
	}
	*toolsets = ts
	return nil
}
