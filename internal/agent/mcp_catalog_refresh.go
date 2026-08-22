package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// mcpCatalogRefreshAfterHookPriority runs after the tool result is known
// so a failed/unknown-tool or catalog discovery can expire the MCP
// tools/list cache before the next model call's FilterTools.
const mcpCatalogRefreshAfterHookPriority = 3

func newMCPCatalogRefreshAfterHook(invs []tools.MCPCacheInvalidator, lg loggateway.Logger) callbacks.Callback {
	if len(invs) == 0 {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return callbacks.NewAfterToolHook(mcpCatalogRefreshAfterHookPriority, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil || !shouldRefreshMCPCatalog(args) {
			return &trpctool.AfterToolResult{}, nil
		}
		for _, inv := range invs {
			if inv != nil {
				inv.InvalidateToolsCache()
			}
		}
		lg.Info("mcp catalog cache invalidated for next model call",
			loggateway.StepID("agent.mcp.catalog_refresh"),
			loggateway.Str("tool", strings.TrimSpace(args.ToolName)))
		return &trpctool.AfterToolResult{}, nil
	})
}

func shouldRefreshMCPCatalog(args *trpctool.AfterToolArgs) bool {
	name := strings.ToLower(strings.TrimSpace(args.ToolName))
	if isMCPCatalogToolName(name) {
		return true
	}
	if args.Error == nil {
		return false
	}
	msg := strings.ToLower(args.Error.Error())
	return strings.Contains(msg, "unknown tool") ||
		strings.Contains(msg, "tool not found") ||
		strings.Contains(msg, "method not found")
}

func isMCPCatalogToolName(name string) bool {
	for _, suffix := range []string{
		"mcp_list_tools",
		"mcp_inspect_tools",
		"mcp_list_servers",
		"mcp_list_resources",
	} {
		if name == suffix || strings.HasSuffix(name, "_"+suffix) {
			return true
		}
	}
	return false
}
