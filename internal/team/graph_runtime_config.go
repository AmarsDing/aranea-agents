package team

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// CompileToGraphRuntimeConfig builds a graph config for GraphAgent team runtime execution.
// When linked_graph_id is set, loads the persisted graph asset before mode/embedded compile.
func CompileToGraphRuntimeConfig(def Definition, agentKey CompileAgentKey) (biz.GraphBuildConfig, error) {
	return CompileToGraphRuntimeConfigFromJSON(context.Background(), def, "", agentKey, nil)
}

// CompileToGraphRuntimeConfigFromJSON applies linked graph, embedded graph, and failure policy.
func CompileToGraphRuntimeConfigFromJSON(
	ctx context.Context,
	def Definition,
	rawDefinitionJSON string,
	agentKey CompileAgentKey,
	linked GraphBuildConfigLoader,
) (biz.GraphBuildConfig, error) {
	raw := strings.TrimSpace(rawDefinitionJSON)
	if raw != "" && linked != nil {
		if linkedID := LinkedGraphIDFromDefinition(raw); linkedID != "" {
			if cfg, err := linked.LoadGraphBuildConfig(ctx, linkedID); err == nil {
				return finalizeRuntimeGraphConfig(cfg, def, raw), nil
			}
		}
	}
	cfg, err := compileToGraphBuildConfigWithLoader(ctx, def, raw, agentKey, linked)
	if err != nil {
		return cfg, err
	}
	mode := normalizeCompileMode(def.Mode)
	if mode == "adaptive" {
		cfg = applyAdaptiveAgentDestinations(cfg)
	}
	return finalizeRuntimeGraphConfig(cfg, def, raw), nil
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

func finalizeRuntimeGraphConfig(cfg biz.GraphBuildConfig, def Definition, rawDefinitionJSON string) biz.GraphBuildConfig {
	cfg = biz.FilterVisualizationEdges(cfg)
	cfg = biz.ApplyFailurePolicy(cfg, def.FailurePolicy)
	cfg = biz.FinalizeGraphFailurePolicy(cfg)
	cfg = applyTeamRuntimeExecutionOptions(cfg, def, rawDefinitionJSON)
	return cfg
}
