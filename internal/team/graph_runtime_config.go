package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// CompileToGraphRuntimeConfig builds a graph config for GraphAgent team runtime execution.
// When linked_graph_id is set, loads the persisted graph asset before mode/embedded compile.
func CompileToGraphRuntimeConfig(def Definition, agentKey CompileAgentKey, lg loggateway.Logger) (biz.GraphBuildConfig, error) {
	ct, err := CompileToCompiledTeam(context.Background(), def, "", agentKey, nil, lg)
	if err != nil {
		return biz.GraphBuildConfig{}, err
	}
	return ct.GraphBuildConfig, nil
}

func CompileToGraphRuntimeConfigFromJSON(
	ctx context.Context,
	def Definition,
	rawDefinitionJSON string,
	agentKey CompileAgentKey,
	linked GraphBuildConfigLoader,
	lg loggateway.Logger,
) (*biz.CompiledTeam, error) {
	return CompileToCompiledTeam(ctx, def, rawDefinitionJSON, agentKey, linked, lg)
}

// applyAdaptiveAgentDestinations moves transfer overlay edges into node Destinations
// for graph runtime dynamic routing; transfer edges are stripped later by FilterVisualizationEdges.
func applyAdaptiveAgentDestinations(cfg biz.GraphBuildConfig) biz.GraphBuildConfig {
	destByNode := make(map[string][]string)
	for _, e := range cfg.Edges {
		from := strings.TrimSpace(e.From)
		to := strings.TrimSpace(e.To)
		if from == "" || to == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(e.Kind))
		if kind != "transfer" && kind != "flow" && kind != "" {
			continue
		}
		destByNode[from] = appendAdaptiveDest(destByNode[from], to)
	}
	if len(destByNode) == 0 {
		return cfg
	}
	for i := range cfg.Nodes {
		id := cfg.Nodes[i].ID
		if extra, ok := destByNode[id]; ok {
			cfg.Nodes[i].Destinations = appendAdaptiveDests(cfg.Nodes[i].Destinations, extra...)
		}
	}
	return cfg
}

func appendAdaptiveDest(existing []string, dest string) []string {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return existing
	}
	for _, d := range existing {
		if d == dest {
			return existing
		}
	}
	return append(existing, dest)
}

func appendAdaptiveDests(existing []string, extra ...string) []string {
	for _, d := range extra {
		existing = appendAdaptiveDest(existing, d)
	}
	return existing
}

func finalizeRuntimeGraphConfig(cfg biz.GraphBuildConfig, def Definition, rawDefinitionJSON string, policy *biz.TeamFailurePolicy, parallelBranchIDs []string) biz.GraphBuildConfig {
	cfg = biz.FilterVisualizationEdges(cfg)
	cfg = biz.ApplyFailurePolicy(cfg, policy)
	cfg = biz.FinalizeGraphFailurePolicy(cfg, policy, parallelBranchIDs)
	cfg = applyTeamRuntimeExecutionOptions(cfg, def, rawDefinitionJSON)
	return cfg
}
