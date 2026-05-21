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

// BuildDeps supplies runtime dependencies for typed graph nodes (LLM / Tool / Agent).
type BuildDeps struct {
	Models ModelResolver
	Tools  ToolResolver
	Agents AgentResolver
}
