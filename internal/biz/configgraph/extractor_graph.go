package configgraph

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

// graphExtractor: graph_definitions 节点 + graph_agent（nodes[].agent_name /
// fallback_agent / reviewer_agent，值按 agent_key 双解）+ graph_tool
// （nodes[].tool_names，按 tool_key 双解）+ graph_owned_by（team_id 直读列）。
// nodes 列即 []biz.NodeDef 的 JSON（internal/data/graph.go SaveDefinition 同构）。
type graphExtractor struct{}

func (graphExtractor) NodeType() string { return NodeTypeGraph }

func (graphExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListGraphs(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeGraph, r.ID, r.Name, r.Name, r.WorkspaceID, NodeStatusActive, nil))
	}
	return nodes, nil
}

func (graphExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListGraphs(ctx)
	if err != nil {
		return nil, err
	}
	var edges []Edge
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		if owner := strings.TrimSpace(r.TeamID); owner != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeGraph, SrcRef: r.ID,
				DstType: NodeTypeTeam, DstRef: owner,
				Type:        EdgeTypeGraphOwnedBy,
				Evidence:    evidence("graph_definitions", "team_id", "team_id"),
				WorkspaceID: r.WorkspaceID,
			})
		}
		var defs []biz.NodeDef
		if raw := strings.TrimSpace(r.NodesJSON); raw != "" {
			if err := json.Unmarshal([]byte(raw), &defs); err != nil {
				edges = append(edges, extractErrorEdge(NodeTypeGraph, r.ID, EdgeTypeGraphAgent, r.WorkspaceID,
					"graph_definitions", "nodes", "nodes", err))
				continue
			}
		}
		for _, n := range defs {
			nodeID := strings.TrimSpace(n.ID)
			agentRefs := []struct {
				value string
				role  string
				path  string
			}{
				{n.AgentName, "agent", "agent_name"},
				{n.FallbackAgent, "fallback", "fallback_agent"},
				{n.ReviewerAgent, "reviewer", "reviewer_agent"},
			}
			for _, ref := range agentRefs {
				v := strings.TrimSpace(ref.value)
				if v == "" {
					continue
				}
				edges = append(edges, Edge{
					SrcType: NodeTypeGraph, SrcRef: r.ID,
					DstType: NodeTypeAgent, DstRef: v, DstKey: v,
					Type: EdgeTypeGraphAgent,
					Evidence: withExtra(evidence("graph_definitions", "nodes", "nodes[]."+ref.path),
						"node_id", nodeID, "role", ref.role),
					WorkspaceID: r.WorkspaceID,
				})
			}
			for _, tn := range n.ToolNames {
				tn = strings.TrimSpace(tn)
				if tn == "" {
					continue
				}
				edges = append(edges, Edge{
					SrcType: NodeTypeGraph, SrcRef: r.ID,
					DstType: NodeTypeTool, DstRef: tn, DstKey: tn,
					Type: EdgeTypeGraphTool,
					Evidence: withExtra(evidence("graph_definitions", "nodes", "nodes[].tool_names"),
						"node_id", nodeID),
					WorkspaceID: r.WorkspaceID,
				})
			}
		}
	}
	return edges, nil
}
