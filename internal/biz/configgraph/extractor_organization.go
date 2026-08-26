package configgraph

import (
	"context"
	"strings"
)

// organizationExtractor: organizations 表节点 + org_parent（parent_id）+
// org_dept_lead（dept_lead_agent_id，org→agent）。
type organizationExtractor struct{}

func (organizationExtractor) NodeType() string { return NodeTypeOrganization }

func (organizationExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeOrganization, r.ID, r.OrgKey, r.Name, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt),
			map[string]any{"org_type": r.Level}))
	}
	return nodes, nil
}

func (organizationExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListOrganizations(ctx)
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
				SrcType: NodeTypeOrganization, SrcRef: r.ID,
				DstType: NodeTypeOrganization, DstRef: parent,
				Type:        EdgeTypeOrgParent,
				Evidence:    evidence("organizations", "parent_id", "parent_id"),
				WorkspaceID: r.WorkspaceID,
			})
		}
		if lead := strings.TrimSpace(r.DeptLeadAgentID); lead != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeOrganization, SrcRef: r.ID,
				DstType: NodeTypeAgent, DstRef: lead,
				Type:        EdgeTypeOrgDeptLead,
				Evidence:    evidence("organizations", "dept_lead_agent_id", "dept_lead_agent_id"),
				WorkspaceID: r.WorkspaceID,
			})
		}
	}
	return edges, nil
}
