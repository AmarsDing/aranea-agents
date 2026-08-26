package configgraph

import (
	"context"
	"strings"
)

// skillExtractor: skill 表节点 + skill_parent（skill.parent_id）+
// owns_skill（skill.agent_id，边方向 agent→skill，从 skill 行读出）。
type skillExtractor struct{}

func (skillExtractor) NodeType() string { return NodeTypeSkill }

func (skillExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListSkills(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeSkill, r.ID, r.SkillKey, r.Name, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt), nil))
	}
	return nodes, nil
}

func (skillExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListSkills(ctx)
	if err != nil {
		return nil, err
	}
	var edges []Edge
	for _, r := range rows {
		if r.ID == "" || statusFromDeletedAt(r.DeletedAt) == NodeStatusDeleted {
			continue
		}
		if parent := strings.TrimSpace(r.ParentID); parent != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeSkill, SrcRef: r.ID,
				DstType: NodeTypeSkill, DstRef: parent,
				Type:        EdgeTypeSkillParent,
				Evidence:    evidence("skill", "parent_id", "parent_id"),
				WorkspaceID: r.WorkspaceID,
			})
		}
		if owner := strings.TrimSpace(r.AgentID); owner != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeAgent, SrcRef: owner,
				DstType: NodeTypeSkill, DstRef: r.ID, DstKey: r.SkillKey,
				Type:        EdgeTypeOwnsSkill,
				Evidence:    evidence("skill", "agent_id", "agent_id"),
				WorkspaceID: r.WorkspaceID,
			})
		}
	}
	return edges, nil
}
