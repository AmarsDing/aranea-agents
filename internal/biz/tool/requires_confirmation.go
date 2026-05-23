package tool

import (
	"context"
	"strings"
)

// ToolRequiresConfirmation returns whether a tool needs user approval before execution.
// Catalog default applies; an agent override with requires_confirmation=true also forces it.
func ToolRequiresConfirmation(tool Tool, ov ToolAgentOverride, hasOverride bool) bool {
	if tool.RequiresConfirmation {
		return true
	}
	if hasOverride && ov.RequiresConfirmation {
		return true
	}
	return false
}

// RequiresConfirmationForAgent reports whether toolKey requires user confirmation for agentID.
func (u *ToolUsecase) RequiresConfirmationForAgent(ctx context.Context, agentID, toolKey string) bool {
	if u == nil {
		return false
	}
	agentID = strings.TrimSpace(agentID)
	toolKey = strings.TrimSpace(toolKey)
	if agentID == "" || toolKey == "" {
		return false
	}
	t, err := u.GetTool(ctx, toolKey)
	if err != nil {
		return false
	}
	overrides, err := u.ListToolAgentOverridesByAgent(ctx, agentID)
	if err != nil {
		overrides = nil
	}
	for _, o := range overrides {
		if strings.TrimSpace(o.ToolKey) == toolKey {
			return ToolRequiresConfirmation(t, o, true)
		}
	}
	return ToolRequiresConfirmation(t, ToolAgentOverride{}, false)
}
