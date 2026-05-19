package agent

import (
	"context"
	"strings"
)

// classifyToolInvocation returns whether a tool call should bump MCP / skill session counters.
func classifyToolInvocation(ctx context.Context, toolKey string, deps TRPCBuilderDeps) (mcp, skill bool) {
	key := strings.ToLower(strings.TrimSpace(toolKey))
	if key == "mcp_call" {
		return true, false
	}
	switch key {
	case "use_skill", "skill_search":
		return false, true
	}
	if deps.ToolUC != nil {
		if tool, err := deps.ToolUC.GetTool(ctx, toolKey); err == nil {
			src := strings.ToLower(strings.TrimSpace(tool.Source))
			cat := strings.ToLower(strings.TrimSpace(tool.Category))
			rk := strings.ToLower(strings.TrimSpace(tool.RuntimeKind))
			if src == "mcp" || cat == "mcp" || rk == "mcp" || strings.HasPrefix(src, "mcp/") {
				return true, false
			}
			if src == "skill" || cat == "skill" {
				return false, true
			}
		}
	}
	return false, false
}
