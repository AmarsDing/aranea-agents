// Package classify detects MCP tool invocations for session counters and metrics.
package classify

import (
	"strings"
)

// Broker tool names from trpc-agent-go tool/mcpbroker.
var brokerToolNames = map[string]struct{}{
	"mcp_call":          {},
	"mcp_list_servers":  {},
	"mcp_list_tools":    {},
	"mcp_inspect_tools": {},
}

// metaGetter matches MCP tool results that carry CallToolResult metadata.
// In trpc-agent-go, only *mcp.Tool.mcpToolResult implements this interface,
// making it a reliable type marker for MCP tool calls regardless of whether
// the server populates Meta (e.g. Playwright MCP returns no Meta).
type metaGetter interface {
	GetMeta() map[string]any
}

// IsMCPToolInvocation reports whether a completed tool call should count as MCP.
// toolName is the runtime Declaration().Name (not necessarily a catalog tool_key).
func IsMCPToolInvocation(toolName string, result any) bool {
	key := strings.ToLower(strings.TrimSpace(toolName))
	if _, ok := brokerToolNames[key]; ok {
		return true
	}
	if strings.HasPrefix(key, "mcp_") && strings.Contains(key, "__") {
		// Validate: after "mcp_", there must be a non-empty server key before "__".
		// This prevents false positives like "mcp___" or "mcp__something".
		rest := key[len("mcp_"):]
		sepIdx := strings.Index(rest, "__")
		if sepIdx > 0 {
			return true
		}
	}
	if result != nil {
		// MCP tool results implement GetMeta() — this is a reliable type marker
		// even when the MCP server returns no Meta (e.g. Playwright MCP tools
		// like `playwright_browser_navigate`). Only *mcpToolResult implements
		// this interface, so the assertion is safe without a nil-Meta check.
		if _, ok := result.(metaGetter); ok {
			return true
		}
	}
	return false
}
