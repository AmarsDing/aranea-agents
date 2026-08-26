package configgraph

import "context"

// mcpServerExtractor: mcp_server 表节点（纯被引用方，无出边）。
type mcpServerExtractor struct{}

func (mcpServerExtractor) NodeType() string { return NodeTypeMCPServer }

func (mcpServerExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListMCPServers(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeMCPServer, r.ID, r.ServerKey, r.Name, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt), nil))
	}
	return nodes, nil
}

func (mcpServerExtractor) ExtractEdges(context.Context, SourceRepo) ([]Edge, error) {
	return nil, nil
}
