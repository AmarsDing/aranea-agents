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
//
// PERF: this runs on the BeforeTool hot path for every tool call (wired via
// plugin/trpc Runtime.catalogConfirm). It deliberately uses the lightweight
// catalog lookup (ToolCatalogEntry already carries RequiresConfirmation)
// instead of GetTool, which would cost a full invocation-aggregation query
// plus a platform-settings read per call (3+ DB roundtrips total).
func (u *ToolUsecase) RequiresConfirmationForAgent(ctx context.Context, agentID, toolKey string) bool {
	if u == nil {
		return false
	}
	agentID = strings.TrimSpace(agentID)
	toolKey = strings.TrimSpace(toolKey)
	if agentID == "" || toolKey == "" {
		return false
	}
	entries, err := u.repo.ListToolCatalogEntries(ctx, []string{toolKey})
	catalogRequiresConfirm := false
	if err == nil && len(entries) > 0 {
		catalogRequiresConfirm = entries[0].RequiresConfirmation
	}
	overrides, err := u.repo.ListToolAgentOverridesByAgent(ctx, agentID)
	if err != nil {
		overrides = nil
	}
	for _, o := range overrides {
		if strings.TrimSpace(o.ToolKey) == toolKey {
			if o.RequiresConfirmation {
				return true
			}
			return catalogRequiresConfirm
		}
	}
	return catalogRequiresConfirm
}
