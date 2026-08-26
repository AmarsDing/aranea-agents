package configgraph

import (
	"context"
	"strings"
)

// knowledgeCollectionExtractor: knowledge_collections 表节点（无出边）。
// 该表无 deleted_at 列；status == "deleted" 才视为删除，其余一律 active。
type knowledgeCollectionExtractor struct{}

func (knowledgeCollectionExtractor) NodeType() string { return NodeTypeKnowledgeCollection }

func (knowledgeCollectionExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListKnowledgeCollections(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		status := NodeStatusActive
		if strings.EqualFold(strings.TrimSpace(r.Status), "deleted") {
			status = NodeStatusDeleted
		}
		nodes = append(nodes, newNode(NodeTypeKnowledgeCollection, r.ID, r.Name, r.Name, r.Workspace,
			status, nil))
	}
	return nodes, nil
}

func (knowledgeCollectionExtractor) ExtractEdges(context.Context, SourceRepo) ([]Edge, error) {
	return nil, nil
}
