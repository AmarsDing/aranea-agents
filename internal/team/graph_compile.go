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
func CompileToGraphBuildConfig(def Definition, agentKey CompileAgentKey, lg loggateway.Logger) (biz.GraphBuildConfig, map[string]biz.NodeTaskMeta, error) {
	return compileToGraphBuildConfig(def, "", agentKey, lg)
}

// CompileToGraphBuildConfigFromJSON is like CompileToGraphBuildConfig but reads embedded graph from raw JSON.
func CompileToGraphBuildConfigFromJSON(def Definition, rawDefinitionJSON string, agentKey CompileAgentKey, lg loggateway.Logger) (biz.GraphBuildConfig, map[string]biz.NodeTaskMeta, error) {
	return compileToGraphBuildConfig(def, rawDefinitionJSON, agentKey, lg)
}

func compileToGraphBuildConfig(def Definition, rawDefinitionJSON string, agentKey CompileAgentKey, lg loggateway.Logger) (biz.GraphBuildConfig, map[string]biz.NodeTaskMeta, error) {
	cfg, taskMeta, _, err := compileToGraphBuildConfigWithLoader(context.Background(), def, rawDefinitionJSON, agentKey, nil, lg)
	return cfg, taskMeta, err
}

func compileToGraphBuildConfigWithLoader(ctx context.Context, def Definition, rawDefinitionJSON string, agentKey CompileAgentKey, loader GraphBuildConfigLoader, lg loggateway.Logger) (biz.GraphBuildConfig, map[string]biz.NodeTaskMeta, []string, error) {
	if spec, ok := parseEmbeddedGraph(rawDefinitionJSON); ok {
		cfg, taskMeta, branchIDs, err := compileFromEmbeddedGraph(ctx, def, spec, agentKey, loader)
		if err != nil {
			return biz.GraphBuildConfig{}, nil, nil, err
		}
		return cfg, taskMeta, branchIDs, nil
	}

	members := EnabledMembers(def)
	if len(members) == 0 {
		return biz.GraphBuildConfig{}, nil, nil, kerrors.BadRequest("TEAM", "compile graph: no enabled members")
	}

	mode := normalizeCompileMode(def.Mode)
	spec := generateGraphSpecFromMode(ctx, def, mode, lg)
	cfg, taskMeta, branchIDs, err := compileFromEmbeddedGraph(ctx, def, spec, agentKey, loader)
	if err != nil {
		return biz.GraphBuildConfig{}, nil, nil, err
	}
	return cfg, taskMeta, branchIDs, nil
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

func generateGraphSpecFromMode(ctx context.Context, def Definition, mode string, lg loggateway.Logger) *embeddedGraphSpec {
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

	edges := generateModeEdges(ctx, mode, def, nodes, lg)

	return &embeddedGraphSpec{
		Version: 1,
		Layout:  mode,
		Nodes:   nodes,
		Edges:   edges,
	}
}

func generateModeEdges(ctx context.Context, mode string, def Definition, nodes []embeddedGraphNode, lg loggateway.Logger) []embeddedGraphEdge {
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
		lg.Warn("transfer edges trimmed due to member count exceeding limit",
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

func CompileToCompiledTeam(
	ctx context.Context,
	def Definition,
	rawDefinitionJSON string,
	agentKey CompileAgentKey,
	linked GraphBuildConfigLoader,
	lg loggateway.Logger,
	functionResolver biz.FunctionResolver,
) (*biz.CompiledTeam, error) {
	raw := strings.TrimSpace(rawDefinitionJSON)
	if raw != "" && linked != nil {
		if linkedID := LinkedGraphIDFromDefinition(raw); linkedID != "" {
			if cfg, err := linked.LoadGraphBuildConfig(ctx, linkedID); err == nil {
				mode := normalizeCompileMode(def.Mode)
				if mode == "adaptive" {
					cfg = applyAdaptiveAgentDestinations(cfg)
				}
				cfg = finalizeRuntimeGraphConfig(cfg, def, raw, failurePolicyToBiz(def.FailurePolicy), nil)
				validateFunctionNodes(ctx, cfg, functionResolver, lg)
				return biz.NewCompiledTeam(cfg, nil, buildRoleManifest(cfg), failurePolicyToBiz(def.FailurePolicy)), nil
			}
		}
	}
	cfg, taskMeta, branchIDs, err := compileToGraphBuildConfigWithLoader(ctx, def, raw, agentKey, linked, lg)
	if err != nil {
		return nil, err
	}
	mode := normalizeCompileMode(def.Mode)
	if mode == "adaptive" {
		cfg = applyAdaptiveAgentDestinations(cfg)
	}
	cfg = finalizeRuntimeGraphConfig(cfg, def, raw, failurePolicyToBiz(def.FailurePolicy), branchIDs)
	validateFunctionNodes(ctx, cfg, functionResolver, lg)
	return biz.NewCompiledTeam(cfg, taskMeta, buildRoleManifest(cfg), failurePolicyToBiz(def.FailurePolicy)), nil
}

// validateFunctionNodes validates function-type node references via FunctionResolver.
// When FunctionResolver is nil or returns an error, a warning is logged and compilation
// continues (degradation). The actual function resolution happens at runtime in
// internal/graph/trpc wireNode.
func validateFunctionNodes(ctx context.Context, cfg biz.GraphBuildConfig, resolver biz.FunctionResolver, lg loggateway.Logger) {
	if resolver == nil {
		return
	}
	for _, node := range cfg.Nodes {
		if strings.ToLower(strings.TrimSpace(node.Type)) != biz.NodeTypeFunction {
			continue
		}
		funcRef := strings.TrimSpace(node.FuncRef)
		if funcRef == "" || funcRef == biz.SkipNodeFuncRef {
			continue
		}
		if err := resolver.Resolve(ctx, funcRef); err != nil {
			lg.Warn("FunctionResolver 降级：编译期函数引用校验失败，运行时将重新解析",
				loggateway.StepID("team.compile.function_resolver_degraded"),
				loggateway.Str("node_id", node.ID),
				loggateway.Str("func_ref", funcRef),
				loggateway.Err(err))
		}
	}
}

func buildRoleManifest(cfg biz.GraphBuildConfig) map[string]biz.RoleInfo {
	roleManifest := make(map[string]biz.RoleInfo)
	for _, node := range cfg.Nodes {
		agentName := strings.TrimSpace(node.AgentName)
		if agentName == "" {
			continue
		}
		roleManifest[node.ID] = biz.RoleInfo{
			AgentID:      agentName,
			AgentKey:     agentName,
			DisplayName:  agentName,
			Role:         node.Type,
			Capabilities: []string{},
		}
	}
	return roleManifest
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
