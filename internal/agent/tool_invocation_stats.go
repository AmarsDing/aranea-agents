package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/mcp/classify"
)

// classifyToolInvocation returns whether a tool call should bump MCP / skill session counters.
func classifyToolInvocation(ctx context.Context, toolKey string, result any, deps TRPCBuilderDeps) (mcp, skill bool) {
	_ = ctx
	if classify.IsMCPToolInvocation(toolKey, result) {
		return true, false
	}
	key := strings.ToLower(strings.TrimSpace(toolKey))
	switch key {
	case "use_skill", "skill_search", "skill_load", "skill_run", "skill_list_docs":
		// skill_load/skill_run/skill_list_docs 是 trpc-agent-go skill 包的内建工具
		// （不在 tools 表注册），skill 运行时调用的真实入口。
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
