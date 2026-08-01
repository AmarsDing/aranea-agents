package biz

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// TeamRunObservatory is the REST bootstrap view for a team run observatory.
type TeamRunObservatory struct {
	RunID                  string
	TeamID                 string
	SessionID              string
	Status                 string
	Mode                   string
	GraphExecutionID       string
	TraceID                string
	DefinitionSnapshotJSON string
	CompiledTopology       TeamRunCompiledTopology
	Nodes                  []AgentNodeState
}

// TeamRunCompiledTopology is backend-compiled graph topology for Observatory canvas.
type TeamRunCompiledTopology struct {
	TemplateID       string
	Mode             string
	EntryPoint       string
	FinishPoint      string
	GraphJSON        string
	Valid            bool
	CompileError     string
	Nodes            []NodeDef
	Edges            []EdgeDef
	ConditionalEdges []ConditionalEdgeDef
}

// BuildOrchestrationRegistryFromDefinition builds node registry from team definition JSON and optional steps.
func BuildOrchestrationRegistryFromDefinition(definitionJSON string, steps []TeamRunStep) OrchestrationRegistry {
	members, _ := parseOrchestrationMembers(definitionJSON)
	stepByAgent := make(map[string]TeamRunStep, len(steps))
	for _, s := range steps {
		if id := strings.TrimSpace(s.AgentID); id != "" {
			stepByAgent[id] = s
		}
	}
	entries := make([]OrchestrationNodeRegistryEntry, 0, len(members))
	for i, m := range members {
		agentID := strings.TrimSpace(m.AgentID)
		if agentID == "" {
			continue
		}
		sortOrder := m.SortOrder
		if sortOrder <= 0 {
			sortOrder = i + 1
		}
		entry := OrchestrationNodeRegistryEntry{
			NodeID:  fmt.Sprintf("member-%d", sortOrder),
			AgentID: agentID,
			Role:    strings.TrimSpace(m.Role),
		}
		if step, ok := stepByAgent[agentID]; ok {
			entry.AgentKey = strings.TrimSpace(step.AgentKey)
			entry.AgentName = strings.TrimSpace(step.AgentName)
		}
		if entry.AgentName == "" {
			entry.AgentName = strings.TrimSpace(m.Name)
		}
		entries = append(entries, entry)
	}
	return NewOrchestrationRegistry(entries)
}

// BuildTeamRunObservatory assembles node states from run metadata and persisted steps.
func BuildTeamRunObservatory(run TeamRunRecord, steps []TeamRunStep, definitionJSON string) TeamRunObservatory {
	reg := BuildOrchestrationRegistryFromDefinition(definitionJSON, steps)
	store := NewOrchestrationStatusStore(reg)

	stepNode := make(map[string]TeamRunStep, len(steps))
	for _, s := range steps {
		nodeID := resolveStepNodeID(s, reg)
		if nodeID == "" {
			continue
		}
		stepNode[nodeID] = s
		st := store.Nodes[nodeID]
		if st == nil {
			continue
		}
		st.InputPreview = strings.TrimSpace(s.InputPreview)
		st.OutputPreview = strings.TrimSpace(s.OutputPreview)
		st.ErrorMessage = strings.TrimSpace(s.ErrorMessage)
		st.AgentKey = firstOrchestrationNonEmpty(st.AgentKey, s.AgentKey)
		st.AgentName = firstOrchestrationNonEmpty(st.AgentName, s.AgentName)
		status := AgentNodeStatusSuccess
		if isStepFailed(s.Status) {
			status = AgentNodeStatusFailed
		}
		st.Status = status
		st.DisplayStatus = AggregateDisplayStatus(status)
		st.Phase = WorkPhaseDelivered
		if status == AgentNodeStatusFailed {
			st.Phase = WorkPhaseDoing
		}
	}

	if run.Status == "running" || run.Status == "pending" {
		for nodeID, st := range store.Nodes {
			if _, done := stepNode[nodeID]; done {
				continue
			}
			if st.Status == AgentNodeStatusIdle {
				st.Status = AgentNodeStatusQueued
				st.DisplayStatus = DisplayStatusWaiting
				st.Phase = WorkPhaseReceived
			}
		}
	}

	nodes := make([]AgentNodeState, 0, len(store.Nodes))
	for _, st := range store.Nodes {
		if st == nil {
			continue
		}
		cp := *st
		nodes = append(nodes, cp)
	}

	return TeamRunObservatory{
		RunID:                  run.ID,
		TeamID:                 run.TeamID,
		SessionID:              run.SessionID,
		Status:                 run.Status,
		Mode:                   run.Mode,
		GraphExecutionID:       strings.TrimSpace(run.GraphExecutionID),
		TraceID:                strings.TrimSpace(run.TraceID),
		DefinitionSnapshotJSON: strings.TrimSpace(definitionJSON),
		Nodes:                  nodes,
	}
}

func resolveStepNodeID(step TeamRunStep, reg OrchestrationRegistry) string {
	if key := strings.TrimSpace(step.AgentKey); key != "" {
		if e, ok := reg.ByAgentKey[key]; ok {
			return e.NodeID
		}
	}
	if id := strings.TrimSpace(step.AgentID); id != "" {
		if e, ok := reg.ByAgentID[id]; ok {
			return e.NodeID
		}
	}
	if step.SortOrder > 0 {
		return fmt.Sprintf("member-%d", step.SortOrder)
	}
	return ""
}

func isStepFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "fail", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func firstOrchestrationNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

type orchestrationMember struct {
	AgentID   string `json:"agent_id"`
	Role      string `json:"role"`
	SortOrder int    `json:"sort_order"`
	Name      string `json:"name"`
	Enabled   *bool  `json:"enabled"`
}

func parseOrchestrationMembers(definitionJSON string) ([]orchestrationMember, error) {
	raw := strings.TrimSpace(definitionJSON)
	if raw == "" {
		return nil, nil
	}
	var body struct {
		Members []orchestrationMember `json:"members"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, err
	}
	out := make([]orchestrationMember, 0, len(body.Members))
	for _, m := range body.Members {
		if m.Enabled != nil && !*m.Enabled {
			continue
		}
		if strings.TrimSpace(m.AgentID) == "" {
			continue
		}
		out = append(out, m)
	}
	normalizeOrchestrationSortOrders(out)
	return out, nil
}

// normalizeOrchestrationSortOrders mirrors team.normalizeMemberSortOrders
// （biz 不能反向依赖 team，两条规则必须保持同步）：sort_order 含 ≤0/重复值
// 时按 (sort_order, 声明序) 稳定序重编为稠密 1 基；全正互异（含稀疏）保持
// 不变。修复 0 基 sort_order 导致的 member-N 节点 ID 碰撞（observatory 只
// 显示一个节点）。
func normalizeOrchestrationSortOrders(members []orchestrationMember) {
	needsFix := false
	seen := make(map[int]struct{}, len(members))
	for _, m := range members {
		if m.SortOrder <= 0 {
			needsFix = true
			break
		}
		if _, dup := seen[m.SortOrder]; dup {
			needsFix = true
			break
		}
		seen[m.SortOrder] = struct{}{}
	}
	if !needsFix {
		return
	}
	order := make([]int, len(members))
	for i := range members {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return members[order[a]].SortOrder < members[order[b]].SortOrder
	})
	for pos, idx := range order {
		members[idx].SortOrder = pos + 1
	}
}

// HasActiveTeamRun reports whether the team has a running or pending run.
func HasActiveTeamRun(runs []TeamRunRecord) bool {
	for _, r := range runs {
		switch strings.ToLower(strings.TrimSpace(r.Status)) {
		case "running", "pending":
			return true
		}
	}
	return false
}

// BuildOrchestrationRegistryFromGraph maps agent-type graph nodes to orchestration registry entries.
func BuildOrchestrationRegistryFromGraph(def *GraphDefinition) OrchestrationRegistry {
	if def == nil {
		return NewOrchestrationRegistry(nil)
	}
	entries := make([]OrchestrationNodeRegistryEntry, 0, len(def.Nodes))
	for _, n := range def.Nodes {
		if strings.ToLower(strings.TrimSpace(n.Type)) != "agent" {
			continue
		}
		nodeID := strings.TrimSpace(n.ID)
		if nodeID == "" {
			continue
		}
		agentName := strings.TrimSpace(n.AgentName)
		if agentName == "" {
			agentName = nodeID
		}
		entries = append(entries, OrchestrationNodeRegistryEntry{
			NodeID:    nodeID,
			AgentKey:  agentName,
			AgentName: agentName,
		})
	}
	return NewOrchestrationRegistry(entries)
}
