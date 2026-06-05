package graph

import (
	"context"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ModelResolver resolves an LLM model for graph LLM nodes.
type ModelResolver interface {
	ResolveModel(ctx context.Context, modelName string) (trpcmodel.Model, error)
}

// ToolResolver resolves named catalog tools for graph tool nodes.
type ToolResolver interface {
	ResolveTools(ctx context.Context, toolNames []string) (map[string]trpctool.Tool, error)
}

// AgentResolver resolves a catalog agent for graph agent nodes.
type AgentResolver interface {
	ResolveAgent(ctx context.Context, agentRef string) (trpcagent.Agent, error)
}

// FunctionResolver resolves a named function for graph function nodes.
// Function nodes are lightweight, stateless callables (HTTP, script, etc.)
// that don't require the full Agent or Tool lifecycle.
type FunctionResolver interface {
	ResolveFunction(ctx context.Context, funcRef string) (trpctool.CallableTool, error)
}

// GraphNodeResolverSet groups all resolver interfaces for graph node types.
// Wire assembles this set in internal/service; graph/trpc consumes it.
type GraphNodeResolverSet struct {
	Models    ModelResolver
	Tools     ToolResolver
	Agents    AgentResolver
	Functions FunctionResolver
}
