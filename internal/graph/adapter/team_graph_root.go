package adapter

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// TeamGraphRootBuilder builds a GraphAgent root for team graph runtime.
type TeamGraphRootBuilder interface {
	BuildTeamGraphRoot(ctx context.Context, cfg biz.GraphBuildConfig) (trpcagent.Agent, error)
}

var _ TeamGraphRootBuilder = (*trpcGraphBuilderFactory)(nil)

// BuildTeamGraphRoot compiles a team GraphBuildConfig into a runnable GraphAgent.
func (f *trpcGraphBuilderFactory) BuildTeamGraphRoot(ctx context.Context, cfg biz.GraphBuildConfig) (trpcagent.Agent, error) {
	if f == nil {
		return nil, kerrors.InternalServer("GRAPH", "graph builder factory is nil")
	}
	g, subAgents, cbState, err := graphtrpc.BuildStateGraphWithRegistryAndLogger(ctx, cfg, f.registry, f.resolvers.ToBuildDepsPtr(), f.lg)
	if err != nil {
		return nil, err
	}
	name := cfg.EntryPoint
	if name == "" {
		name = "team-graph"
	}
	return f.createAgent(name, g, cfg.EnableCheckpoint, cfg.ExecutionEngine, cbState, subAgents)
}
