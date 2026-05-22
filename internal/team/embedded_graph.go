package team

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

type embeddedGraphSpec struct {
	Version int                 `json:"version"`
	Layout  string              `json:"layout"`
	Nodes   []embeddedGraphNode `json:"nodes"`
	Edges   []embeddedGraphEdge `json:"edges"`
}

type embeddedGraphNode struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Label   string `json:"label"`
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
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
		if isEmbeddedAgentNode(n.Type) {
			agentCount++
		}
	}
	if agentCount == 0 {
		return nil, false
	}
	return body.Graph, true
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

func compileFromEmbeddedGraph(def Definition, spec *embeddedGraphSpec, agentKey CompileAgentKey) (biz.GraphBuildConfig, error) {
	if spec == nil {
		return biz.GraphBuildConfig{}, fmt.Errorf("team: embedded graph is nil")
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
	agentNodes := make([]biz.NodeDef, 0, len(spec.Nodes))
	for _, n := range spec.Nodes {
		id := strings.TrimSpace(n.ID)
		if id == "" || !isEmbeddedAgentNode(n.Type) {
			continue
		}
		nodeTypeByID[id] = n.Type
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
		agentNodes = append(agentNodes, biz.NodeDef{
			ID:           id,
			Type:         "agent",
			Description:  desc,
			AgentName:    key,
			RequiredRole: role,
		})
	}
	if len(agentNodes) == 0 {
		return biz.GraphBuildConfig{}, fmt.Errorf("team: embedded graph has no agent nodes")
	}

	edges, entry, finish := compileEmbeddedEdges(def, spec, nodeTypeByID)
	if entry == "" {
		entry = agentNodes[0].ID
	}
	if finish == "" {
		finish = agentNodes[len(agentNodes)-1].ID
	}

	cfg := biz.GraphBuildConfig{
		Nodes:            agentNodes,
		Edges:            edges,
		EntryPoint:       entry,
		FinishPoint:      finish,
		ExecutionEngine:  biz.EngineBSP,
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

func compileEmbeddedEdges(def Definition, spec *embeddedGraphSpec, nodeTypeByID map[string]string) ([]biz.EdgeDef, string, string) {
	mode := normalizeCompileMode(def.Mode)
	agentIDs := make(map[string]struct{}, len(nodeTypeByID))
	for id := range nodeTypeByID {
		agentIDs[id] = struct{}{}
	}

	var entry, finish string
	out := make([]biz.EdgeDef, 0, len(spec.Edges))
	for _, e := range spec.Edges {
		from := strings.TrimSpace(e.Source)
		to := strings.TrimSpace(e.Target)
		if from == "" || to == "" {
			continue
		}
		_, fromAgent := agentIDs[from]
		_, toAgent := agentIDs[to]
		fromType := nodeTypeByID[from]
		toType := nodeTypeByID[to]
		if (isEmbeddedDecorID(from) || isEmbeddedDecorNode(fromType)) && toAgent {
			if strings.EqualFold(from, "start") && entry == "" {
				entry = to
			}
		}
		if fromAgent && (isEmbeddedDecorID(to) || isEmbeddedDecorNode(toType)) {
			if strings.EqualFold(to, "end") && finish == "" {
				finish = from
			}
		}
		if fromAgent && toAgent {
			kind := embeddedEdgeKind(mode, e)
			out = append(out, biz.EdgeDef{From: from, To: to, Kind: kind})
		}
	}
	return out, entry, finish
}

func embeddedEdgeKind(mode string, e embeddedGraphEdge) string {
	label := strings.ToLower(strings.TrimSpace(e.Label))
	cond := strings.ToLower(strings.TrimSpace(e.Condition))
	if mode == "adaptive" && (strings.Contains(label, "transfer") || strings.Contains(cond, "transfer")) {
		return "transfer"
	}
	if mode == "coordinator" && strings.Contains(label, "dispatch") {
		return "dispatch"
	}
	return "flow"
}
