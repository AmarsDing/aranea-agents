package adapter

import (
	"context"

	"aranea-agents/pkg/apierror"

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
		return nil, apierror.Internal(apierror.DomainGraph, "graph builder factory is nil")
	}
	g, subAgents, err := graphtrpc.BuildStateGraphWithRegistryAndLogger(ctx, cfg, f.registry, &f.resolvers, f.lg)
	if err != nil {
		return nil, err
	}
	name := cfg.EntryPoint
	if name == "" {
		name = "team-graph"
	}
	return f.createAgent(name, g, cfg.EnableCheckpoint, cfg.ExecutionEngine, subAgents)
}
