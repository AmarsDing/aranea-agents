package configgraph

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
)

// channelExtractor: channel 节点 + routes_to（config_json.routing：
// default_agent_id/default_team_id/rules[].agent_id/rules[].team_id，agent
// 值 uuid/agent_key 双解）。
type channelExtractor struct{}

func (channelExtractor) NodeType() string { return NodeTypeChannel }

func (channelExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeChannel, r.ID, r.ChannelKey, r.Name, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt), nil))
	}
	return nodes, nil
}

func (channelExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	var edges []Edge
	for _, r := range rows {
		if r.ID == "" || statusFromDeletedAt(r.DeletedAt) == NodeStatusDeleted {
			continue
		}
		raw := strings.TrimSpace(r.ConfigJSON)
		if raw == "" {
			continue
		}
		routing, err := biz.ParseChannelRouting(raw)
		if err != nil {
			edges = append(edges, extractErrorEdge(NodeTypeChannel, r.ID, EdgeTypeRoutesTo, r.WorkspaceID,
				"channel", "config_json", "config_json.routing", err))
			continue
		}
		emit := func(dstType, value, path, peerPattern string) {
			value = strings.TrimSpace(value)
			if value == "" {
				return
			}
			edges = append(edges, Edge{
				SrcType: NodeTypeChannel, SrcRef: r.ID,
				DstType: dstType, DstRef: value, DstKey: value,
				Type: EdgeTypeRoutesTo,
				Evidence: withExtra(evidence("channel", "config_json", path),
					"peer_pattern", peerPattern),
				WorkspaceID: r.WorkspaceID,
			})
		}
		emit(NodeTypeAgent, routing.DefaultAgentID, "config_json.routing.default_agent_id", "")
		emit(NodeTypeTeam, routing.DefaultTeamID, "config_json.routing.default_team_id", "")
		for _, rule := range routing.Rules {
			emit(NodeTypeAgent, rule.AgentID, "config_json.routing.rules[].agent_id", rule.PeerPattern)
			emit(NodeTypeTeam, rule.TeamID, "config_json.routing.rules[].team_id", rule.PeerPattern)
		}
	}
	return edges, nil
}
