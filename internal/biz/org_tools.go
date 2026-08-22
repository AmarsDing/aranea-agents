package biz

import (
	"encoding/json"
	"strings"
)

// SpiritReservedToolKeys are orchestration tools that belong to Spirit only.
// Specialists and governance leads must not inherit this set (R17).
func SpiritReservedToolKeys() []string {
	return []string{
		"plan_and_execute",
		"cancel_orchestration",
		"synthesize_results",
		"build_orchestration_graph",
	}
}

// ClampSpecialistToolFace stops employees from inheriting Spirit's toolbox.
// Governance leads stay on read_only; a worker with profile "spirit" is
// downgraded to coding. Spirit itself is unchanged.
func ClampSpecialistToolFace(s *AgentRuntimeSettings, a Agent) {
	if s == nil {
		return
	}
	if strings.TrimSpace(a.AgentKey) == SpiritAgentKey {
		return
	}
	if IsOrgGovernanceAgent(a) {
		s.ToolsProfile = "read_only"
		return
	}
	if canonicalToolProfile(s.ToolsProfile) == "spirit" {
		s.ToolsProfile = "coding"
	}
}

// SpecialistToolFaces is the Assemble-time deny overlay: every non-Spirit
// member is denied Spirit reserved tools. MCP stays on the agent's own
// ToolsAllowJSON / profile (coding includes mcp_broker; mcp: prefixes
// remain the per-server allow list).
func SpecialistToolFaces(agentKeys []string) map[string][]string {
	deny := SpiritReservedToolKeys()
	out := make(map[string][]string)
	for _, k := range agentKeys {
		k = strings.TrimSpace(k)
		if k == "" || k == SpiritAgentKey {
			continue
		}
		out[k] = append([]string(nil), deny...)
	}
	return out
}

// ApplyAssembleOrgFaces stamps graph_template_id, collection_ids, and
// specialist tool faces onto team definition JSON. It does not write
// linked_graph_id / source=linked_external — that would fight the Team
// save hook. Compile loads a persisted template id from graph_template_id
// when it is not a builtin. Playbook collection_ids win over user
// KnowledgeBases at team run.
func ApplyAssembleOrgFaces(defJSON string, agentKeys []string, graphTemplateID string, collectionIDs []string) string {
	raw := strings.TrimSpace(defJSON)
	if raw == "" {
		raw = "{}"
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return defJSON
	}
	if id := strings.TrimSpace(graphTemplateID); id != "" {
		body["graph_template_id"] = id
	}
	if ids := NormalizeCollectionIDs(collectionIDs); len(ids) > 0 {
		body["collection_ids"] = ids
	}
	if faces := SpecialistToolFaces(agentKeys); len(faces) > 0 {
		body["specialist_tool_faces"] = faces
	}
	b, err := json.Marshal(body)
	if err != nil {
		return defJSON
	}
	return string(b)
}

// NormalizeCollectionIDs drops blanks and trims ids.
func NormalizeCollectionIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// HighRiskVerificationGate reports whether a gate is one of the R18
// high-risk confirmation tiers (already HITL via existing approval flow).
func HighRiskVerificationGate(g VerificationGate) bool {
	switch g.GateType {
	case GateTypeDeptLeadApproval, GateTypeCrossDeptDelivery, GateTypeBorrowApproval:
		return true
	default:
		return false
	}
}
