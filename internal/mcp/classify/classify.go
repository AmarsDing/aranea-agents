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
		return true
	}
	if result != nil {
		if mg, ok := result.(metaGetter); ok && mg.GetMeta() != nil {
			return true
		}
	}
	return false
}
