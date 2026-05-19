package graph

import (
	"context"

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

// BuildDeps supplies runtime dependencies for typed graph nodes (LLM / Tool).
type BuildDeps struct {
	Models ModelResolver
	Tools  ToolResolver
}
