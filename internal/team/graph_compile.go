package team

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
	spec, _ := resolveDefinitionGraphSpec(ctx, def, rawDefinitionJSON, lg)
	if spec == nil {
		return biz.GraphBuildConfig{}, nil, nil, apierror.BadRequest(apierror.DomainTeam, "compile graph: no enabled members")
	}
	cfg, taskMeta, branchIDs, err := compileFromEmbeddedGraph(ctx, def, spec, agentKey, loader)
	if err != nil {
		return biz.GraphBuildConfig{}, nil, nil, err
	}
	return cfg, taskMeta, branchIDs, nil
}

// resolveDefinitionGraphSpec returns the effective embedded graph spec with
// priority embedded graph (from raw JSON) > mode template generation.
// generated=true when the template path produced the spec (ADR-08 A2: frontend
// editors consume the backend-generated spec instead of local re-implementation).
func resolveDefinitionGraphSpec(ctx context.Context, def Definition, rawDefinitionJSON string, lg loggateway.Logger) (*embeddedGraphSpec, bool) {
	if spec, ok := parseEmbeddedGraph(rawDefinitionJSON); ok {
		return spec, false
	}
	if len(EnabledMembers(def)) == 0 {
		return nil, false
	}
	spec := generateGraphSpecFromMode(ctx, def, normalizeCompileMode(def.Mode), lg)
	if spec == nil {
		return nil, false
	}
	return spec, true
}

// DefinitionGraphSpecJSON marshals the template-generated canonical embedded
// graph spec (incl. start/end decor nodes). Returns "" when the definition
// carries its own embedded graph (custom path) or generation is impossible.
func DefinitionGraphSpecJSON(ctx context.Context, def Definition, rawDefinitionJSON string, lg loggateway.Logger) string {
	spec, generated := resolveDefinitionGraphSpec(ctx, def, rawDefinitionJSON, lg)
	if !generated || spec == nil {
		return ""
	}
	b, err := json.Marshal(spec)
	if err != nil {
		return ""
	}
	return string(b)
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
		// S9（K3 降级覆盖）：未知编排模式静默 fallback 为 pipeline，用户配置了
		// 无效模式（如拼写错误）却毫无感知。必须打 warn 进程日志 + 流程日志。
		lg.Warn("未知团队编排模式，降级为 pipeline 拓扑",
			loggateway.StepID("team.compile.unknown_mode_fallback"),
			loggateway.Str("mode", mode))
		if em := event.TraceEmitterFromContext(ctx); em != nil {
			em.LogWarn("team.compile.unknown_mode_fallback", "未知编排模式已降级",
				fmt.Sprintf("模式 %q 不支持，已按顺序执行（pipeline）编排", mode))
		}
		modeEdges = pipelineTemplate{}.BuildEdges(def, agentIDs)
	}
	trimmed := countTransferEdges(modeEdges) > adaptiveTransferEdgeLimit(def)
	if trimmed {
		lg.Warn("transfer edges trimmed due to member count exceeding limit",
			loggateway.StepID("team.compile.adaptive_trimmed"),
			loggateway.Int("member_count", len(agentIDs)),
			loggateway.Int("max_transfer_edges", adaptiveTransferEdgeLimit(def)),
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

func adaptiveTransferEdgeLimit(def Definition) int {
	if def.Swarm != nil && def.Swarm.MaxHandoffs > 0 {
		return def.Swarm.MaxHandoffs
	}
	return maxAdaptiveTransferEdges
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
	if msg := parallelDeliverableAdvisory(def); msg != "" && lg != nil {
		lg.Warn(msg, loggateway.Domain("team"), loggateway.Str("mode", def.Mode))
	}
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
