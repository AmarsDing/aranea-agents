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
	EnsureCriticLoopCondFuncs(f.registry, cfg, f.lg)
	// Use the NodeAgents variant so team-graph agent nodes can be resolved
	// by node ID via FindSubAgent (see runtime_adapter.go buildRuntime for
	// the detailed rationale). Without this, agent node execution fails
	// with "parent agent not found in state for agent node X".
	g, subAgents, nodeAgents, err := graphtrpc.BuildStateGraphWithRegistryAndNodeAgents(ctx, cfg, f.registry, &f.resolvers, f.lg)
	if err != nil {
		return nil, err
	}
	name := cfg.EntryPoint
	if name == "" {
		name = "team-graph"
	}
	return f.createAgent(name, g, cfg, cfg.EnableCheckpoint, cfg.ExecutionEngine, subAgents, nodeAgents)
}
