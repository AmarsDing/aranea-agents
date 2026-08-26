package configgraph

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
)

// teamExtractor: teams 节点 + definition_json 派生边（has_member /
// synthesizer / intent_anchor / fallback_agent / graph_template /
// scoped_knowledge）+ 直读列边（dept_lead / cross_dept_member / belongs_to /
// linked_graph）。definition_json 解析失败不中断：记 broken 标记边后继续
// 处理直读列。
type teamExtractor struct{}

func (teamExtractor) NodeType() string { return NodeTypeTeam }

func (teamExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeTeam, r.ID, r.TeamKey, r.DisplayName, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt),
			map[string]any{
				"is_default": r.IsDefault,
				"kind":       r.Kind,
				"topology":   r.Topology,
			}))
	}
	return nodes, nil
}

func (teamExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	rows, err := src.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	var edges []Edge
	for _, r := range rows {
		if r.ID == "" || statusFromDeletedAt(r.DeletedAt) == NodeStatusDeleted {
			continue
		}
		emit := func(dstType, dstRef, dstKey, edgeType, field, path string, kv ...string) {
			ev := evidence("teams", field, path)
			if len(kv) > 0 {
				ev = withExtra(ev, kv...)
			}
			edges = append(edges, Edge{
				SrcType: NodeTypeTeam, SrcRef: r.ID,
				DstType: dstType, DstRef: dstRef, DstKey: dstKey,
				Type:        edgeType,
				Evidence:    ev,
				WorkspaceID: r.WorkspaceID,
			})
		}

		// 直读列边（不依赖 definition_json 解析）。
		if lead := strings.TrimSpace(r.DeptLeadAgentID); lead != "" {
			emit(NodeTypeAgent, lead, "", EdgeTypeDeptLead, "dept_lead_agent_id", "dept_lead_agent_id")
		}
		if dept := strings.TrimSpace(r.DepartmentID); dept != "" {
			emit(NodeTypeOrganization, dept, "", EdgeTypeBelongsTo, "department_id", "department_id")
		}
		if raw := strings.TrimSpace(r.CrossDeptMemberIDs); raw != "" {
			var ids []string
			if err := json.Unmarshal([]byte(raw), &ids); err != nil {
				edges = append(edges, extractErrorEdge(NodeTypeTeam, r.ID, EdgeTypeCrossDeptMember, r.WorkspaceID,
					"teams", "cross_dept_member_ids", "cross_dept_member_ids", err))
			}
			for _, id := range ids {
				if id = strings.TrimSpace(id); id != "" {
					emit(NodeTypeAgent, id, "", EdgeTypeCrossDeptMember, "cross_dept_member_ids", "cross_dept_member_ids")
				}
			}
		}
		if gid := team.ResolveLinkedGraphID(r.LinkedGraphID, r.DefinitionJSON); gid != "" {
			emit(NodeTypeGraph, gid, "", EdgeTypeLinkedGraph, "linked_graph_id", "linked_graph_id")
		}

		// definition_json 派生边。
		def, err := team.ParseDefinition(r.DefinitionJSON)
		if err != nil {
			edges = append(edges, extractErrorEdge(NodeTypeTeam, r.ID, EdgeTypeHasMember, r.WorkspaceID,
				"teams", "definition_json", "definition_json", err))
			continue
		}
		for _, m := range def.Members {
			if id := strings.TrimSpace(m.AgentID); id != "" {
				emit(NodeTypeAgent, id, "", EdgeTypeHasMember, "definition_json", "definition_json.members[].agent_id",
					"role", m.Role)
			}
		}
		if id := strings.TrimSpace(team.SynthesizerAgentID(def)); id != "" {
			emit(NodeTypeAgent, id, "", EdgeTypeSynthesizer, "definition_json", "definition_json.synthesizer_agent_id")
		}
		if id := strings.TrimSpace(def.IntentAnchorAgentID); id != "" {
			emit(NodeTypeAgent, id, "", EdgeTypeIntentAnchor, "definition_json", "definition_json.intent_anchor_agent_id")
		}
		if def.FailurePolicy != nil {
			for _, nodeKey := range sortedKeys(def.FailurePolicy.NodeOverrides) {
				if id := strings.TrimSpace(def.FailurePolicy.NodeOverrides[nodeKey].FallbackAgent); id != "" {
					emit(NodeTypeAgent, id, "", EdgeTypeFallbackAgent, "definition_json",
						"definition_json.failure_policy.node_overrides[].fallback_agent", "node_key", nodeKey)
				}
			}
		}
		if gid := team.GraphTemplateIDFromDefinition(r.DefinitionJSON); gid != "" && !biz.IsBuiltinGraphTemplateID(gid) {
			// 内置模板（pipeline 等）非 graph_definitions 资产，不成边（避免
			// 每个用内置模板的 team 都制造一条伪 broken 边）。
			emit(NodeTypeGraph, gid, "", EdgeTypeGraphTemplate, "definition_json", "definition_json.graph_template_id")
		}
		for _, cid := range team.CollectionIDsFromDefinition(r.DefinitionJSON) {
			if cid = strings.TrimSpace(cid); cid != "" {
				emit(NodeTypeKnowledgeCollection, cid, "", EdgeTypeScopedKnowledge, "definition_json", "definition_json.collection_ids")
			}
		}
	}
	return edges, nil
}
