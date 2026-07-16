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
	ct, err := CompileToCompiledTeam(context.Background(), def, "", agentKey, nil, lg, nil)
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
	return CompileToCompiledTeam(ctx, def, rawDefinitionJSON, agentKey, linked, lg, nil)
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

// ensureDeliverableStateField injects the deliverable StateField when EnableStateDeliverable=true.
// Idempotent: skips injection if a field with the same name already exists.
func ensureDeliverableStateField(cfg biz.GraphBuildConfig) biz.GraphBuildConfig {
	for _, sf := range cfg.StateFields {
		if sf.Name == biz.DeliverableStateKey {
			return cfg
		}
	}
	cfg.StateFields = append(cfg.StateFields, biz.StateFieldDef{
		Name:         biz.DeliverableStateKey,
		Type:         "map[string]any",
		Reducer:      biz.ReducerCover,
		DefaultValue: map[string]any{},
	})
	return cfg
}

func finalizeRuntimeGraphConfig(cfg biz.GraphBuildConfig, def Definition, rawDefinitionJSON string, policy *biz.TeamFailurePolicy, parallelBranchIDs []string) biz.GraphBuildConfig {
	cfg = biz.FilterVisualizationEdges(cfg)
	cfg = biz.ApplyFailurePolicy(cfg, policy)
	cfg = biz.FinalizeGraphFailurePolicy(cfg, policy, parallelBranchIDs)
	cfg = applyTeamRuntimeExecutionOptions(cfg, def, rawDefinitionJSON)
	cfg = applySwarmGraphConfig(cfg, def)
	if def.EnableStateDeliverable {
		cfg = ensureDeliverableStateField(cfg)
	}
	return cfg
}

// applySwarmGraphConfig maps SwarmConfigDef onto GraphBuildConfig equivalents
// (graph path cannot use native team.Option from safety_adapter.go).
func applySwarmGraphConfig(cfg biz.GraphBuildConfig, def Definition) biz.GraphBuildConfig {
	if def.Swarm == nil {
		return cfg
	}
	swarm := def.Swarm
	cfg.SwarmSafety = &biz.SwarmSafetySpec{
		MaxHandoffs:                swarm.MaxHandoffs,
		RepetitiveHandoffWindow:    swarm.RepetitiveHandoffWindow,
		RepetitiveHandoffMinUnique: swarm.RepetitiveHandoffMinUnique,
		CrossRequestTransfer:       swarm.CrossRequestTransfer,
	}
	cfg = ensureSwarmSafetyStateFields(cfg)

	timeout := swarm.NodeTimeoutSeconds
	for i := range cfg.Nodes {
		nt := strings.ToLower(strings.TrimSpace(cfg.Nodes[i].Type))
		if nt != biz.NodeTypeAgent && nt != biz.NodeTypeLLM && nt != biz.NodeTypeTool {
			continue
		}
		if timeout > 0 && cfg.Nodes[i].TimeoutSeconds <= 0 {
			cfg.Nodes[i].TimeoutSeconds = timeout
		}
		// Session isolation equivalent: isolate member agent message history.
		if nt == biz.NodeTypeAgent {
			cfg.Nodes[i].IsolatedMessages = true
		}
	}
	return cfg
}

func ensureSwarmSafetyStateFields(cfg biz.GraphBuildConfig) biz.GraphBuildConfig {
	cfg = ensureStateField(cfg, biz.StateFieldDef{
		Name:         biz.SwarmHandoffCountStateKey,
		Type:         "int",
		Reducer:      biz.ReducerCover,
		DefaultValue: 0,
	})
	cfg = ensureStateField(cfg, biz.StateFieldDef{
		Name:         biz.SwarmRecentTargetsStateKey,
		Type:         "[]string",
		Reducer:      biz.ReducerCover,
		DefaultValue: []string{},
	})
	return cfg
}

func ensureStateField(cfg biz.GraphBuildConfig, field biz.StateFieldDef) biz.GraphBuildConfig {
	for _, sf := range cfg.StateFields {
		if sf.Name == field.Name {
			return cfg
		}
	}
	cfg.StateFields = append(cfg.StateFields, field)
	return cfg
}
