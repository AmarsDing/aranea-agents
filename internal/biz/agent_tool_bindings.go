package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// ToolAgentBinding is one agent's effective state for a specific tool,
// computed server-side in bulk. It replaces the N+1 pattern where the
// frontend calls GetEffectiveTools once per agent.
type ToolAgentBinding struct {
	AgentID      string
	AgentKey     string
	AgentName    string
	AgentStatus  string
	ToolsEnabled bool
	Profile      string
	State        string // allowed | denied
	Reason       string
	OverrideMode string // allow | deny | inherit when an override row exists, "" otherwise
}

// listAllAgentsForBindings pages through the agent catalog (data layer caps a
// single page at 500) so large deployments are not silently truncated.
func (u *AgentUsecase) listAllAgentsForBindings(ctx context.Context, callerWS string) ([]Agent, error) {
	const pageSize = 500
	out := make([]Agent, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		page, err := u.reader.SearchAgents(ctx, AgentListQuery{WorkspaceID: callerWS, Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if len(page.Items) < pageSize || len(out) >= page.Total {
			return out, nil
		}
	}
}

// GetToolAgentBindings computes the effective state of one tool across every
// visible agent in a constant number of queries (tool + agents + settings +
// overrides + catalog), then resolves states purely in memory.
func (u *AgentUsecase) GetToolAgentBindings(ctx context.Context, toolID, callerWS string) ([]ToolAgentBinding, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		return nil, apierror.BadRequest("TOOL", "tool id is required")
	}
	tool, err := u.tools.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	snaps, _, err := u.policySnapshots(ctx, callerWS)
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return []ToolAgentBinding{}, nil
	}
	overrides, err := u.tools.ListToolAgentOverrides(ctx, tool.Key)
	if err != nil {
		return nil, err
	}
	overrideByAgent := make(map[string]ToolAgentOverride, len(overrides))
	for _, o := range overrides {
		overrideByAgent[strings.TrimSpace(o.AgentID)] = o
	}
	platform := loadWebResearchPlatformFromSys(ctx, u.sys, u.lg)

	out := make([]ToolAgentBinding, 0, len(snaps))
	for _, s := range snaps {
		var ov *ToolAgentOverride
		if o, ok := overrideByAgent[s.agent.ID]; ok {
			ov = &o
		}
		item, overrideMode := u.toolStateFor(s, tool, ov, platform)
		out = append(out, ToolAgentBinding{
			AgentID:      s.agent.ID,
			AgentKey:     s.agent.AgentKey,
			AgentName:    s.agent.DisplayName,
			AgentStatus:  s.agent.Status,
			ToolsEnabled: s.settings.ToolsEnabled,
			Profile:      CanonicalToolProfile(s.settings.ToolsProfile),
			State:        item.EffectiveState,
			Reason:       item.Reason,
			OverrideMode: overrideMode,
		})
	}
	return out, nil
}
