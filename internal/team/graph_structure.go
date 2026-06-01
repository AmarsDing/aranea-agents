package team

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ExportStructureSnapshot builds API structure export from compiled graph topology.
func ExportStructureSnapshot(teamKey, displayName string, def Definition, agentKey CompileAgentKey, lg loggateway.Logger) (*biz.TeamStructureSnapshot, error) {
	cfg, err := CompileToGraphBuildConfig(def, agentKey, lg)
	if err != nil {
		return nil, err
	}
	entry := strings.TrimSpace(teamKey)
	if entry == "" {
		entry = "team"
	}
	snapshot := &biz.TeamStructureSnapshot{
		EntryNodeID: "team-" + entry,
		Nodes: []biz.StructureNode{
			{NodeID: "team-" + entry, Kind: "team", Name: displayName},
		},
	}
	nodeNames := make(map[string]string, len(def.Members))
	for i, m := range EnabledMembers(def) {
		id := memberNodeID(m, i)
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = id
		}
		nodeNames[id] = name
	}
	for _, n := range cfg.Nodes {
		name := nodeNames[n.ID]
		if name == "" {
			name = strings.TrimSpace(n.AgentName)
		}
		if name == "" {
			name = n.ID
		}
		snapshot.Nodes = append(snapshot.Nodes, biz.StructureNode{
			NodeID: n.ID,
			Kind:   "agent",
			Name:   name,
		})
	}
	if entry := strings.TrimSpace(cfg.EntryPoint); entry != "" {
		snapshot.Edges = append(snapshot.Edges, biz.StructureEdge{
			FromNodeID: snapshot.EntryNodeID,
			ToNodeID:   entry,
		})
	}
	for _, e := range cfg.Edges {
		snapshot.Edges = append(snapshot.Edges, biz.StructureEdge{FromNodeID: e.From, ToNodeID: e.To})
	}
	for _, ce := range cfg.ConditionalEdges {
		for _, to := range ce.PathMap {
			snapshot.Edges = append(snapshot.Edges, biz.StructureEdge{FromNodeID: ce.From, ToNodeID: to})
		}
	}
	return snapshot, nil
}
