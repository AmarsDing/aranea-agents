// Package adkdeps holds lightweight runtime wiring structs shared by service and team (no transport imports).
package adkdeps

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// Runtime groups optional SQLite session memory and MCP effective resolution for ADK runner construction.
type Runtime struct {
	SessionMemory *sessionmemory.Store
	AgentMCP      *biz.AgentMCPTooling
}

// NewRuntime wires optional dependencies; all fields may be nil.
func NewRuntime(store *sessionmemory.Store, mcp *biz.AgentMCPTooling) *Runtime {
	return &Runtime{SessionMemory: store, AgentMCP: mcp}
}
