package tools

import trpctool "trpc.group/trpc-go/trpc-agent-go/tool"

// MCPCacheInvalidator expires a cached MCP tools/list. Implemented by
// trpc-agent-go mcp.ToolSet and forwarded by mcpSchemaGovernedToolSet.
type MCPCacheInvalidator interface {
	InvalidateToolsCache()
}

// CollectMCPCacheInvalidators walks assembled toolsets (including
// governance wrappers) and returns those that can expire an MCP catalog
// cache. Used to refresh direct mcp_tool_set mounts mid-turn without
// enabling a network tools/list on every model call.
func CollectMCPCacheInvalidators(sets []trpctool.ToolSet) []MCPCacheInvalidator {
	var out []MCPCacheInvalidator
	for _, s := range sets {
		if inv := asMCPCacheInvalidator(s); inv != nil {
			out = append(out, inv)
		}
	}
	return out
}

func asMCPCacheInvalidator(s trpctool.ToolSet) MCPCacheInvalidator {
	if s == nil {
		return nil
	}
	if v, ok := s.(MCPCacheInvalidator); ok {
		return v
	}
	return nil
}
