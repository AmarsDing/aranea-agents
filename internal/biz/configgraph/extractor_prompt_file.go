package configgraph

import (
	"context"
	"strings"
)

// promptFileExtractor: agent_prompt_files 节点（node_key=`{agent_key}/{file_name}`，
// attrs={body_hash, sort_order}）+ has_prompt_file（agent_id 直读列，方向
// agent→prompt_file）。prompt_files 随 agent 级联硬删，无软删标记。
type promptFileExtractor struct{}

func (promptFileExtractor) NodeType() string { return NodeTypePromptFile }

// promptFileKey derives the human-readable node key; agent_key may be empty
// when the owning agent row is gone (key degrades to the bare file name).
func promptFileKey(agentKey, fileName string) string {
	if k := strings.TrimSpace(agentKey); k != "" {
		return k + "/" + strings.TrimSpace(fileName)
	}
	return strings.TrimSpace(fileName)
}

func (promptFileExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListPromptFiles(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypePromptFile, r.ID,
			promptFileKey(r.AgentKey, r.FileName), r.FileName, "",
			NodeStatusActive,
			map[string]any{
				"body_hash":  bodyHash(r.Body),
				"sort_order": r.SortOrder,
			}))
	}
	return nodes, nil
}

func (promptFileExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListPromptFiles(ctx)
	if err != nil {
		return nil, err
	}
	var edges []Edge
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		if owner := strings.TrimSpace(r.AgentID); owner != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeAgent, SrcRef: owner,
				DstType: NodeTypePromptFile, DstRef: r.ID,
				DstKey:   promptFileKey(r.AgentKey, r.FileName),
				Type:     EdgeTypeHasPromptFile,
				Evidence: evidence("agent_prompt_files", "agent_id", "agent_id"),
			})
		}
	}
	return edges, nil
}
