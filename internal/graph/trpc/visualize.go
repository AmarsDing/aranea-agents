package graph

import (
	"regexp"
	"strings"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

type VisualGraphNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Shape       string `json:"shape"`
	FillColor   string `json:"fill_color"`
	BorderColor string `json:"border_color"`
}

type VisualGraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

type VisualGraph struct {
	Nodes []VisualGraphNode `json:"nodes"`
	Edges []VisualGraphEdge `json:"edges"`
	DOT   string            `json:"dot"`
}

func NodeTypeStyle(nodeType string) (shape, fill, border string) {
	switch trpcgraph.NodeType(nodeType) {
	case trpcgraph.NodeTypeLLM:
		return "box", "#e3f2fd", "#2196f3"
	case trpcgraph.NodeTypeTool:
		return "box", "#fff3e0", "#ff9800"
	case trpcgraph.NodeTypeAgent:
		return "box", "#e8f5e9", "#4caf50"
	case trpcgraph.NodeTypeJoin:
		return "diamond", "#f3e5f5", "#9c27b0"
	case trpcgraph.NodeTypeRouter:
		return "diamond", "#eeeeee", "#757575"
	default:
		return "box", "#f3e5f5", "#9c27b0"
	}
}

func ParseDOTToVisualGraph(dot string, nodes []biz.NodeDef, condEdges []biz.ConditionalEdgeDef) *VisualGraph {
	vg := &VisualGraph{
		DOT: dot,
	}

	nodeTypeMap := make(map[string]string, len(nodes))
	nodeLabelMap := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeTypeMap[n.ID] = n.Type
		if n.ID != "" {
			nodeLabelMap[n.ID] = n.ID
		}
		if n.Instruction != "" && nodeLabelMap[n.ID] == n.ID {
			nodeLabelMap[n.ID] = n.Instruction
		}
	}

	nodeIDs := extractNodeIDsFromDOT(dot)
	for _, id := range nodeIDs {
		nt := nodeTypeMap[id]
		if nt == "" {
			nt = "function"
		}
		shape, fill, border := NodeTypeStyle(nt)
		label := nodeLabelMap[id]
		if label == "" {
			label = id
		}
		vg.Nodes = append(vg.Nodes, VisualGraphNode{
			ID:          id,
			Label:       label,
			Type:        nt,
			Shape:       shape,
			FillColor:   fill,
			BorderColor: border,
		})
	}

	edges := extractEdgesFromDOT(dot)
	vg.Edges = edges

	return vg
}

var (
	nodeDeclRe  = regexp.MustCompile(`"([^"]+)"\s*\[`)
	edgeRe      = regexp.MustCompile(`"([^"]+)"\s*->\s*"([^"]+)"`)
	edgeLabelRe = regexp.MustCompile(`label="([^"]*)"`)
	edgeStyleRe = regexp.MustCompile(`style=(\w+)`)
)

func extractNodeIDsFromDOT(dot string) []string {
	matches := nodeDeclRe.FindAllStringSubmatch(dot, -1)
	seen := make(map[string]bool, len(matches))
	var ids []string
	for _, m := range matches {
		id := m[1]
		if id == "__start__" || id == "__end__" {
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func extractEdgesFromDOT(dot string) []VisualGraphEdge {
	matches := edgeRe.FindAllStringSubmatch(dot, -1)
	var edges []VisualGraphEdge
	for _, m := range matches {
		from := m[1]
		to := m[2]
		edgeType := "runtime"
		label := ""

		line := findEdgeLine(dot, from, to)
		if line != "" {
			if strings.Contains(line, "style=dashed") {
				edgeType = "conditional"
			} else if strings.Contains(line, "style=dotted") {
				edgeType = "destination"
			}
			lblMatches := edgeLabelRe.FindStringSubmatch(line)
			if len(lblMatches) > 1 {
				label = lblMatches[1]
			}
		}

		edges = append(edges, VisualGraphEdge{
			From:  from,
			To:    to,
			Type:  edgeType,
			Label: label,
		})
	}
	return edges
}

func findEdgeLine(dot, from, to string) string {
	lines := strings.Split(dot, "\n")
	pattern := `"` + from + `" -> "` + to + `"`
	for _, line := range lines {
		if strings.Contains(line, pattern) {
			return line
		}
	}
	return ""
}

func BuildStartEndNodes() []VisualGraphNode {
	return []VisualGraphNode{
		{
			ID:          "__start__",
			Label:       "start",
			Type:        "start",
			Shape:       "oval",
			FillColor:   "#e1f5e1",
			BorderColor: "#4caf50",
		},
		{
			ID:          "__end__",
			Label:       "finish",
			Type:        "end",
			Shape:       "oval",
			FillColor:   "#ffe1e1",
			BorderColor: "#f44336",
		},
	}
}
