package configgraph

import "context"

// toolExtractor: tools 表节点。工具是纯被引用方，无出边。
type toolExtractor struct{}

func (toolExtractor) NodeType() string { return NodeTypeTool }

func (toolExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeTool, r.ID, r.ToolKey, r.DisplayName, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt),
			map[string]any{
				"risk_level":            r.RiskLevel,
				"category":              r.Category,
				"requires_confirmation": r.RequiresConfirmation,
			}))
	}
	return nodes, nil
}

func (toolExtractor) ExtractEdges(context.Context, SourceRepo) ([]Edge, error) {
	return nil, nil
}
