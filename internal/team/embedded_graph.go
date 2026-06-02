package team

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type embeddedGraphSpec struct {
	Version int                 `json:"version"`
	Layout  string              `json:"layout"`
	Nodes   []embeddedGraphNode `json:"nodes"`
	Edges   []embeddedGraphEdge `json:"edges"`
}

type embeddedGraphNode struct {
	ID                 string   `json:"id"`
	Type               string   `json:"type"`
	Label              string   `json:"label"`
	AgentID            string   `json:"agent_id"`
	Role               string   `json:"role"`
	SubgraphID         string   `json:"subgraph_id"`
	AssignmentMode     string   `json:"assignment_mode"`
	AssignmentStrategy string   `json:"assignment_strategy"`
	ReviewerAgent      string   `json:"reviewer_agent"`
	ReviewRules        string   `json:"review_rules"`
	InterruptBefore    bool     `json:"interrupt_before"`
	InterruptAfter     bool     `json:"interrupt_after"`
	Destinations       []string `json:"destinations"`
	RetryMaxAttempts   int      `json:"retry_max_attempts"`
	FallbackAgent      string   `json:"fallback_agent"`
}

type embeddedGraphEdge struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Label    string `json:"label"`
	Condition string `json:"condition"`
}

func parseEmbeddedGraph(rawDefinitionJSON string) (*embeddedGraphSpec, bool) {
	raw := strings.TrimSpace(rawDefinitionJSON)
	if raw == "" {
		return nil, false
	}
	var body struct {
		Graph *embeddedGraphSpec `json:"graph"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil || body.Graph == nil {
		return nil, false
	}
	agentCount := 0
	for _, n := range body.Graph.Nodes {
		if isEmbeddedExecutableNode(n.Type) {
			agentCount++
		}
	}
	if agentCount == 0 {
		return nil, false
	}
	return body.Graph, true
}

func isEmbeddedExecutableNode(nodeType string) bool {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "agent", "task", "review", "subgraph":
		return true
	default:
		return false
	}
}

func isEmbeddedAgentNode(nodeType string) bool {
	return strings.EqualFold(strings.TrimSpace(nodeType), "agent")
}

func isEmbeddedDecorNode(nodeType string) bool {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "start", "end", "join":
		return true
	default:
		return false
	}
}

func isEmbeddedDecorID(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "start", "end", "join":
		return true
	default:
		return false
	}
}

func compileFromEmbeddedGraph(ctx context.Context, def Definition, spec *embeddedGraphSpec, agentKey CompileAgentKey, loader GraphBuildConfigLoader) (biz.GraphBuildConfig, []string, error) {
	if spec == nil {
		return biz.GraphBuildConfig{}, nil, kerrors.BadRequest("TEAM", "embedded graph is nil")
	}
	memberByAgentID := map[string]MemberDef{}
	for i, m := range EnabledMembers(def) {
		id := strings.TrimSpace(m.AgentID)
		if id == "" {
			continue
		}
		memberByAgentID[id] = m
		if m.SortOrder <= 0 {
			copy := m
			copy.SortOrder = i + 1
			memberByAgentID[id] = copy
		}
	}

	nodeTypeByID := map[string]string{}
	executableIDs := make(map[string]struct{})
	nodes := make([]biz.NodeDef, 0, len(spec.Nodes))
	subgraphs := make([]biz.SubgraphDef, 0)
	taskMeta := make(map[string]biz.NodeTaskMeta)
	loading := map[string]struct{}{}

	for _, n := range spec.Nodes {
		id := strings.TrimSpace(n.ID)
		if id == "" {
			continue
		}
		nodeType := strings.ToLower(strings.TrimSpace(n.Type))
		nodeTypeByID[id] = n.Type
		switch nodeType {
		case "agent":
			if !isEmbeddedAgentNode(n.Type) {
				continue
			}
			agentID := strings.TrimSpace(n.AgentID)
			member, ok := memberByAgentID[agentID]
			key := resolveEmbeddedAgentKey(n, member, agentKey)
			role := strings.TrimSpace(n.Role)
			if role == "" && ok {
				role = strings.TrimSpace(member.Role)
			}
			desc := strings.TrimSpace(n.Label)
			if desc == "" && ok {
				desc = strings.TrimSpace(member.Name)
			}
			instruction := ""
			if ok {
				instruction = strings.TrimSpace(member.TaskPrompt)
			}
			executableIDs[id] = struct{}{}
			nd := compileEmbeddedBizNode(n, biz.NodeDef{
				ID: id, Type: "agent", Description: desc, Instruction: instruction,
				AgentName: key,
			})
			nodes = append(nodes, nd)
			if role != "" {
				taskMeta[id] = biz.NodeTaskMeta{RequiredRole: role}
			}
		case "task":
			executableIDs[id] = struct{}{}
			nd := compileEmbeddedBizNode(n, biz.NodeDef{
				ID: id, Type: "task", Description: strings.TrimSpace(n.Label),
				InterruptAfter: true,
			})
			nodes = append(nodes, nd)
			taskMeta[id] = biz.NodeTaskMeta{
				RequiredRole:       strings.TrimSpace(n.Role),
				AssignmentMode:     strings.TrimSpace(n.AssignmentMode),
				AssignmentStrategy: strings.TrimSpace(n.AssignmentStrategy),
			}
		case "review":
			executableIDs[id] = struct{}{}
			nd := compileEmbeddedBizNode(n, biz.NodeDef{
				ID: id, Type: "review", Description: strings.TrimSpace(n.Label),
				InterruptAfter: true,
			})
			nodes = append(nodes, nd)
			taskMeta[id] = biz.NodeTaskMeta{
				ReviewerAgent: strings.TrimSpace(n.ReviewerAgent),
				ReviewRules:   strings.TrimSpace(n.ReviewRules),
			}
		case "subgraph":
			ref := strings.TrimSpace(n.SubgraphID)
			if ref == "" {
				return biz.GraphBuildConfig{}, nil, kerrors.BadRequest("TEAM", fmt.Sprintf("subgraph node %q requires subgraph_id", id))
			}
			if loader == nil {
				return biz.GraphBuildConfig{}, nil, kerrors.BadRequest("TEAM", fmt.Sprintf("subgraph node %q requires graph loader", id))
			}
			subCfg, err := loadEmbeddedSubgraphConfig(ctx, loader, ref, loading)
			if err != nil {
				return biz.GraphBuildConfig{}, nil, kerrors.BadRequest("TEAM", fmt.Sprintf("subgraph node %q: %s", id, err.Error()))
			}
			executableIDs[id] = struct{}{}
			subgraphs = append(subgraphs, biz.SubgraphDef{ID: id, GraphID: ref, BuildConfig: subCfg})
		}
	}
	if len(executableIDs) == 0 {
		return biz.GraphBuildConfig{}, nil, kerrors.BadRequest("TEAM", "embedded graph has no executable nodes")
	}

	edges, condEdges, entry, finish, branchIDs := compileEmbeddedEdges(def, spec, nodeTypeByID, executableIDs)
	if entry == "" {
		ids := make([]string, 0, len(executableIDs))
		for id := range executableIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		entry = ids[0]
	}
	if finish == "" {
		finish = entry
	}

	cfg := biz.GraphBuildConfig{
		Nodes:            nodes,
		Subgraphs:        subgraphs,
		Edges:            edges,
		ConditionalEdges: condEdges,
		EntryPoint:       entry,
		FinishPoint:      finish,
		ExecutionEngine:  biz.EngineBSP,
		TaskMeta:         taskMeta,
	}
	return cfg, branchIDs, nil
}

func compileEmbeddedBizNode(n embeddedGraphNode, base biz.NodeDef) biz.NodeDef {
	if n.InterruptBefore {
		base.InterruptBefore = true
	}
	if n.InterruptAfter {
		base.InterruptAfter = true
	}
	if len(n.Destinations) > 0 {
		base.Destinations = append([]string(nil), n.Destinations...)
	}
	if n.RetryMaxAttempts > 0 {
		base.RetryMaxAttempts = n.RetryMaxAttempts
	}
	if fb := strings.TrimSpace(n.FallbackAgent); fb != "" {
		base.FallbackAgent = fb
	}
	return base
}

func loadEmbeddedSubgraphConfig(ctx context.Context, loader GraphBuildConfigLoader, graphID string, loading map[string]struct{}) (biz.GraphBuildConfig, error) {
	graphID = strings.TrimSpace(graphID)
	if graphID == "" {
		return biz.GraphBuildConfig{}, kerrors.BadRequest("TEAM", "subgraph_id is required")
	}
	if _, ok := loading[graphID]; ok {
		return biz.GraphBuildConfig{}, kerrors.BadRequest("TEAM", fmt.Sprintf("subgraph cycle detected at %q", graphID))
	}
	loading[graphID] = struct{}{}
	defer delete(loading, graphID)
	cfg, err := loader.LoadGraphBuildConfig(ctx, graphID)
	if err != nil {
		return biz.GraphBuildConfig{}, err
	}
	for _, sub := range cfg.Subgraphs {
		if sub.GraphID == "" {
			continue
		}
		if _, err := loadEmbeddedSubgraphConfig(ctx, loader, sub.GraphID, loading); err != nil {
			return biz.GraphBuildConfig{}, err
		}
	}
	return cfg, nil
}

func resolveEmbeddedAgentKey(n embeddedGraphNode, member MemberDef, agentKey CompileAgentKey) string {
	if agentKey != nil {
		if id := strings.TrimSpace(n.AgentID); id != "" {
			if key := strings.TrimSpace(agentKey(id)); key != "" {
				return key
			}
		}
		if id := strings.TrimSpace(member.AgentID); id != "" {
			if key := strings.TrimSpace(agentKey(id)); key != "" {
				return key
			}
		}
	}
	if label := strings.TrimSpace(n.Label); label != "" {
		return label
	}
	if name := strings.TrimSpace(member.Name); name != "" {
		return name
	}
	return strings.TrimSpace(n.AgentID)
}

func compileEmbeddedEdges(def Definition, spec *embeddedGraphSpec, nodeTypeByID map[string]string, executableIDs map[string]struct{}) ([]biz.EdgeDef, []biz.ConditionalEdgeDef, string, string, []string) {
	mode := normalizeCompileMode(def.Mode)
	var entry, finish string
	joinFeeders := make([]string, 0, 4)
	joinTarget := ""
	out := make([]biz.EdgeDef, 0, len(spec.Edges))
	var condEdges []biz.ConditionalEdgeDef
	for _, e := range spec.Edges {
		from := strings.TrimSpace(e.Source)
		to := strings.TrimSpace(e.Target)
		if from == "" || to == "" {
			continue
		}
		_, fromExec := executableIDs[from]
		_, toExec := executableIDs[to]
		fromType := nodeTypeByID[from]
		toType := nodeTypeByID[to]
		if (isEmbeddedDecorID(from) || isEmbeddedDecorNode(fromType)) && toExec {
			if strings.EqualFold(from, "start") && entry == "" {
				entry = to
			}
		}
		if fromExec && strings.EqualFold(to, "join") {
			joinFeeders = append(joinFeeders, from)
			continue
		}
		if strings.EqualFold(from, "join") && toExec {
			joinTarget = to
			continue
		}
		if fromExec && (isEmbeddedDecorID(to) || isEmbeddedDecorNode(toType)) {
			if strings.EqualFold(to, "end") && finish == "" {
				finish = from
			}
		}
		if fromExec && toExec {
			kind := embeddedEdgeKind(mode, e)
			out = append(out, biz.EdgeDef{From: from, To: to, Kind: kind})
		}
	}
	var branchIDs []string
	if len(joinFeeders) >= 2 && joinTarget != "" {
		seen := map[string]struct{}{}
		for _, feeder := range joinFeeders {
			if _, ok := seen[feeder]; ok {
				continue
			}
			seen[feeder] = struct{}{}
			out = append(out, biz.EdgeDef{From: feeder, To: joinTarget, Kind: "flow"})
			branchIDs = append(branchIDs, feeder)
		}
		finish = joinTarget
	}
	if mode == "critic_loop" && len(executableIDs) >= 2 {
		firstExec := entry
		lastExec := finish
		if firstExec == "" {
			for id := range executableIDs {
				if firstExec == "" || id < firstExec {
					firstExec = id
				}
			}
		}
		if lastExec == "" {
			for id := range executableIDs {
				if lastExec == "" || id > lastExec {
					lastExec = id
				}
			}
		}
		if firstExec != "" && lastExec != "" {
			condEdges = append(condEdges, biz.ConditionalEdgeDef{
				From:        lastExec,
				CondFuncRef: biz.CriticLoopCondFuncRef,
				PathMap: map[string]string{
					"approved": lastExec,
					"retry":    firstExec,
				},
			})
		}
	}
	return out, condEdges, entry, finish, branchIDs
}

func embeddedEdgeKind(mode string, e embeddedGraphEdge) string {
	label := strings.ToLower(strings.TrimSpace(e.Label))
	cond := strings.ToLower(strings.TrimSpace(e.Condition))
	// Condition takes precedence over label for edge kind determination.
	switch {
	case cond == "transfer" || cond == "handoff":
		return "transfer"
	case cond == "dispatch" || cond == "delegate":
		return "dispatch"
	case cond == "approve" || cond == "reject" || cond == "retry" || cond == "fallback":
		return cond
	case strings.Contains(cond, "transfer") || strings.Contains(cond, "handoff"):
		return "transfer"
	}
	if mode == "adaptive" && (strings.Contains(label, "transfer") || strings.Contains(cond, "transfer")) {
		return "transfer"
	}
	if mode == "coordinator" && strings.Contains(label, "dispatch") {
		return "dispatch"
	}
	return "flow"
}
