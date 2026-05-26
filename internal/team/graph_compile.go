package team

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
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
		return biz.GraphBuildConfig{}, fmt.Errorf("team: compile graph: no enabled members")
	}

	mode := normalizeCompileMode(def.Mode)
	nodes := make([]biz.NodeDef, 0, len(members))
	memberIDs := make([]string, 0, len(members))
	for i, m := range members {
		id := memberNodeID(m, i)
		memberIDs = append(memberIDs, id)
		key := resolveCompileAgentKey(m, agentKey)
		nodes = append(nodes, biz.NodeDef{
			ID:           id,
			Type:         "agent",
			Description:  memberDescription(m),
			Instruction:  strings.TrimSpace(m.TaskPrompt),
			AgentName:    key,
			RequiredRole: strings.TrimSpace(m.Role),
		})
	}

	edges, condEdges := compileEdges(mode, def, memberIDs)
	entry, finish := compileEntryFinish(mode, def, memberIDs)

	cfg := biz.GraphBuildConfig{
		Nodes:            nodes,
		Edges:            edges,
		ConditionalEdges: condEdges,
		EntryPoint:       entry,
		FinishPoint:      finish,
		ExecutionEngine:  biz.EngineBSP,
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

func compileEntryFinish(mode string, def Definition, ids []string) (entry, finish string) {
	if len(ids) == 0 {
		return "", ""
	}
	entry = ids[0]
	finish = ids[len(ids)-1]
	if mode == "parallel" {
		if synth := synthesizerNodeID(def, ids); synth != "" {
			finish = synth
		}
	}
	return entry, finish
}

func synthesizerNodeID(def Definition, ids []string) string {
	synth := strings.TrimSpace(SynthesizerAgentID(def))
	if synth == "" {
		return ""
	}
	for i, m := range EnabledMembers(def) {
		if strings.TrimSpace(m.AgentID) == synth {
			return memberNodeID(m, i)
		}
	}
	return ""
}

func compileEdges(mode string, def Definition, ids []string) ([]biz.EdgeDef, []biz.ConditionalEdgeDef) {
	if len(ids) == 0 {
		return nil, nil
	}
	switch mode {
	case "parallel":
		return compileParallelEdges(def, ids), nil
	case "critic_loop":
		return compileCriticLoopEdges(ids), compileCriticLoopConditional(ids)
	case "adaptive":
		return compileAdaptiveEdges(ids), nil
	case "coordinator":
		return compileCoordinatorEdges(ids), nil
	default:
		return compileSequentialEdges(ids), nil
	}
}

func compileSequentialEdges(ids []string) []biz.EdgeDef {
	out := make([]biz.EdgeDef, 0, len(ids)-1)
	for i := 0; i < len(ids)-1; i++ {
		out = append(out, biz.EdgeDef{From: ids[i], To: ids[i+1], Kind: "flow"})
	}
	return out
}

func compileParallelEdges(def Definition, ids []string) []biz.EdgeDef {
	workers := parallelWorkerNodeIDs(def, ids)
	if len(workers) == 0 {
		return compileSequentialEdges(ids)
	}
	finish := ids[len(ids)-1]
	if synth := synthesizerNodeID(def, ids); synth != "" {
		finish = synth
	}
	entry := workers[0]
	if entry == finish {
		return compileSequentialEdges(ids)
	}
	out := make([]biz.EdgeDef, 0, len(workers)*2)
	for _, w := range workers {
		if w == entry {
			continue
		}
		out = append(out, biz.EdgeDef{From: entry, To: w, Kind: "flow"})
	}
	for _, w := range workers {
		if w == finish {
			continue
		}
		out = append(out, biz.EdgeDef{From: w, To: finish, Kind: "flow"})
	}
	return out
}

func parallelWorkerNodeIDs(def Definition, ids []string) []string {
	synth := synthesizerNodeID(def, ids)
	workers := make([]string, 0, len(ids))
	for _, id := range ids {
		if synth != "" && id == synth {
			continue
		}
		workers = append(workers, id)
	}
	if len(workers) == 0 {
		return ids
	}
	return workers
}

func compileCoordinatorEdges(ids []string) []biz.EdgeDef {
	if len(ids) < 2 {
		return nil
	}
	hub := ids[0]
	finish := ids[len(ids)-1]
	out := make([]biz.EdgeDef, 0, len(ids)*2)
	for _, id := range ids[1:] {
		out = append(out, biz.EdgeDef{From: hub, To: id, Kind: "dispatch"})
		if id != finish {
			out = append(out, biz.EdgeDef{From: id, To: finish, Kind: "flow"})
		}
	}
	if hub != finish {
		out = append(out, biz.EdgeDef{From: hub, To: finish, Kind: "flow"})
	}
	return out
}

func compileCriticLoopEdges(ids []string) []biz.EdgeDef {
	out := make([]biz.EdgeDef, 0, len(ids))
	for i := 0; i < len(ids)-1; i++ {
		out = append(out, biz.EdgeDef{From: ids[i], To: ids[i+1], Kind: "flow"})
	}
	return out
}

func compileCriticLoopConditional(ids []string) []biz.ConditionalEdgeDef {
	if len(ids) < 2 {
		return nil
	}
	last := ids[len(ids)-1]
	first := ids[0]
	return []biz.ConditionalEdgeDef{{
		From: last,
		PathMap: map[string]string{
			"approved": last,
			"retry":    first,
		},
	}}
}

const adaptiveMaxTransferEdges = 30

func compileAdaptiveEdges(ids []string) []biz.EdgeDef {
	if len(ids) == 0 {
		return nil
	}
	out := compileSequentialEdges(ids)
	transferCount := 0
	for i := 0; i < len(ids) && transferCount < adaptiveMaxTransferEdges; i++ {
		for j := 0; j < len(ids) && transferCount < adaptiveMaxTransferEdges; j++ {
			if i == j || j == i+1 {
				continue
			}
			out = append(out, biz.EdgeDef{From: ids[i], To: ids[j], Kind: "transfer"})
			transferCount++
		}
	}
	return out
}

// CompileTemplateID returns the orchestration template id for a team mode.
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
