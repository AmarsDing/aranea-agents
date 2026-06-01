package team

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// CompileAgentKey resolves catalog agent_key from agent_id; may return "" to fall back to agent_id.
type CompileAgentKey func(agentID string) string

// CompileToGraphBuildConfig maps a team definition to a graph build config for observability
// and future unified runtime. Topology mirrors web buildGraphFromDefinition() unless
// definition.graph embeds agent nodes (OrchestrationSpec custom/preset edits).
func CompileToGraphBuildConfig(def Definition, agentKey CompileAgentKey) (biz.GraphBuildConfig, error) {
	return compileToGraphBuildConfig(def, "", agentKey)
}

// CompileToGraphBuildConfigFromJSON is like CompileToGraphBuildConfig but reads embedded graph from raw JSON.
func CompileToGraphBuildConfigFromJSON(def Definition, rawDefinitionJSON string, agentKey CompileAgentKey) (biz.GraphBuildConfig, error) {
	return compileToGraphBuildConfig(def, rawDefinitionJSON, agentKey)
}

func compileToGraphBuildConfig(def Definition, rawDefinitionJSON string, agentKey CompileAgentKey) (biz.GraphBuildConfig, error) {
	return compileToGraphBuildConfigWithLoader(context.Background(), def, rawDefinitionJSON, agentKey, nil)
}

func compileToGraphBuildConfigWithLoader(ctx context.Context, def Definition, rawDefinitionJSON string, agentKey CompileAgentKey, loader GraphBuildConfigLoader) (biz.GraphBuildConfig, error) {
	if spec, ok := parseEmbeddedGraph(rawDefinitionJSON); ok {
		cfg, err := compileFromEmbeddedGraph(ctx, def, spec, agentKey, loader)
		if err != nil {
			return biz.GraphBuildConfig{}, err
		}
		return biz.ApplyFailurePolicy(cfg, def.FailurePolicy), nil
	}

	members := EnabledMembers(def)
	if len(members) == 0 {
		return biz.GraphBuildConfig{}, kerrors.BadRequest("TEAM", "compile graph: no enabled members")
	}

	mode := normalizeCompileMode(def.Mode)
	spec := generateGraphSpecFromMode(ctx, def, mode)
	cfg, err := compileFromEmbeddedGraph(ctx, def, spec, agentKey, loader)
	if err != nil {
		return biz.GraphBuildConfig{}, err
	}
	return biz.ApplyFailurePolicy(cfg, def.FailurePolicy), nil
}

func normalizeCompileMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "swarm":
		return "adaptive"
	default:
		if m == "" {
			return "sequential"
		}
		return m
	}
}

func generateGraphSpecFromMode(ctx context.Context, def Definition, mode string) *embeddedGraphSpec {
	members := EnabledMembers(def)
	if len(members) == 0 {
		return nil
	}

	nodes := make([]embeddedGraphNode, 0, len(members)+2)
	nodes = append(nodes, embeddedGraphNode{ID: "start", Type: "start", Label: "Start"})
	for i, m := range members {
		nodes = append(nodes, embeddedGraphNode{
			ID:      memberNodeID(m, i),
			Type:    "agent",
			Label:   memberDescription(m),
			AgentID: strings.TrimSpace(m.AgentID),
			Role:    strings.TrimSpace(m.Role),
		})
	}
	nodes = append(nodes, embeddedGraphNode{ID: "end", Type: "end", Label: "End"})

	edges := generateModeEdges(ctx, mode, def, nodes)

	return &embeddedGraphSpec{
		Version: 1,
		Layout:  mode,
		Nodes:   nodes,
		Edges:   edges,
	}
}

func generateModeEdges(ctx context.Context, mode string, def Definition, nodes []embeddedGraphNode) []embeddedGraphEdge {
	if len(nodes) == 0 {
		return nil
	}
	agentIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if isEmbeddedExecutableNode(n.Type) {
			agentIDs = append(agentIDs, n.ID)
		}
	}
	if len(agentIDs) == 0 {
		return nil
	}

	out := make([]embeddedGraphEdge, 0, len(agentIDs)+2)
	out = append(out, embeddedGraphEdge{Source: "start", Target: agentIDs[0]})

	t := LookupTemplate(mode)
	var modeEdges []embeddedGraphEdge
	if t != nil {
		modeEdges = t.BuildEdges(def, agentIDs)
	} else {
		modeEdges = pipelineTemplate{}.BuildEdges(def, agentIDs)
	}
	trimmed := countTransferEdges(modeEdges) > maxAdaptiveTransferEdges
	if trimmed {
		loggateway.Global().Warn("transfer edges trimmed due to member count exceeding limit",
			loggateway.StepID("team.compile.adaptive_trimmed"),
			loggateway.Int("member_count", len(agentIDs)),
			loggateway.Int("max_transfer_edges", maxAdaptiveTransferEdges),
			loggateway.Str("mode", mode))
	}
	out = append(out, modeEdges...)

	out = append(out, embeddedGraphEdge{Source: agentIDs[len(agentIDs)-1], Target: "end"})
	return out
}

func countTransferEdges(edges []embeddedGraphEdge) int {
	n := 0
	for _, e := range edges {
		if e.Label == "transfer" {
			n++
		}
	}
	return n
}

func memberNodeID(m MemberDef, index int) string {
	sortOrder := m.SortOrder
	if sortOrder <= 0 {
		sortOrder = index + 1
	}
	return fmt.Sprintf("member-%d", sortOrder)
}

func memberDescription(m MemberDef) string {
	if name := strings.TrimSpace(m.Name); name != "" {
		return name
	}
	if role := strings.TrimSpace(m.Role); role != "" {
		return role
	}
	return strings.TrimSpace(m.AgentID)
}

// MemberByCompileNodeID maps compiled member-* node ids to member definitions.
func MemberByCompileNodeID(def Definition) map[string]MemberDef {
	members := EnabledMembers(def)
	out := make(map[string]MemberDef, len(members))
	for i, m := range members {
		out[memberNodeID(m, i)] = m
	}
	return out
}

func resolveCompileAgentKey(m MemberDef, agentKey CompileAgentKey) string {
	id := strings.TrimSpace(m.AgentID)
	if agentKey != nil {
		if key := strings.TrimSpace(agentKey(id)); key != "" {
			return key
		}
	}
	if name := strings.TrimSpace(m.Name); name != "" {
		return name
	}
	return id
}

func CompileTemplateID(mode string) string {
	switch normalizeCompileMode(mode) {
	case "parallel":
		return "parallel_review"
	case "coordinator", "adaptive":
		return "dispatch"
	case "critic_loop":
		return "review_loop"
	default:
		return "pipeline"
	}
}
