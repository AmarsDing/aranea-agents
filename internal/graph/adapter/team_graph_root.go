package adapter

import (
	"context"
	"fmt"

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
		return nil, fmt.Errorf("graph builder factory is nil")
	}
	trpcCfg := bizCfgToTrpc(cfg)
	g, subAgents, err := graphtrpc.BuildStateGraphWithRegistry(ctx, trpcCfg, f.registry, f.resolvers.ToBuildDepsPtr())
	if err != nil {
		return nil, err
	}
	name := cfg.EntryPoint
	if name == "" {
		name = "team-graph"
	}
	return f.createAgent(name, g, cfg.EnableCheckpoint, graphtrpc.ExecutionEngineType(cfg.ExecutionEngine), subAgents)
}
