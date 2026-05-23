package biz

import (
	"strings"

	"aranea-agents/internal/event"
)

// AgentNodeStatus is the fine-grained lifecycle status of an agent node in an orchestration run.
type AgentNodeStatus string

const (
	AgentNodeStatusIdle          AgentNodeStatus = "idle"
	AgentNodeStatusQueued        AgentNodeStatus = "queued"
	AgentNodeStatusScheduled     AgentNodeStatus = "scheduled"
	AgentNodeStatusRunning       AgentNodeStatus = "running"
	AgentNodeStatusThinking      AgentNodeStatus = "thinking"
	AgentNodeStatusToolRunning   AgentNodeStatus = "tool_running"
	AgentNodeStatusTransferring  AgentNodeStatus = "transferring"
	AgentNodeStatusRetrying      AgentNodeStatus = "retrying"
	AgentNodeStatusWaitingInput  AgentNodeStatus = "waiting_input"
	AgentNodeStatusWaitingReview AgentNodeStatus = "waiting_review"
	AgentNodeStatusWaitingAssign AgentNodeStatus = "waiting_assign"
	AgentNodeStatusBlocked       AgentNodeStatus = "blocked"
	AgentNodeStatusSuccess       AgentNodeStatus = "success"
	AgentNodeStatusFailed        AgentNodeStatus = "failed"
	AgentNodeStatusSkipped       AgentNodeStatus = "skipped"
	AgentNodeStatusCancelled     AgentNodeStatus = "cancelled"
	AgentNodeStatusTimedOut      AgentNodeStatus = "timed_out"
)

// DisplayStatus aggregates fine statuses for Graph/Kanban chrome.
type DisplayStatus string

const (
	DisplayStatusWaiting   DisplayStatus = "waiting"
	DisplayStatusActive    DisplayStatus = "active"
	DisplayStatusSuspended DisplayStatus = "suspended"
	DisplayStatusSuccess   DisplayStatus = "success"
	DisplayStatusFailed    DisplayStatus = "failed"
	DisplayStatusSkipped   DisplayStatus = "skipped"
	DisplayStatusCancelled DisplayStatus = "cancelled"
)

// WorkPhase is the Kanban column axis (orthogonal to AgentNodeStatus).
type WorkPhase string

const (
	WorkPhaseReceived  WorkPhase = "received"
	WorkPhaseDoing     WorkPhase = "doing"
	WorkPhaseDelivered WorkPhase = "delivered"
)

// ActivitySnapshot is a single in-flight or completed activity on a node.
type ActivitySnapshot struct {
	Kind          string `json:"kind,omitempty"`
	DisplayLabel  string `json:"display_label,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	Status        string `json:"status,omitempty"`
	Summary       string `json:"summary,omitempty"`
	ArgumentsJSON string `json:"arguments_json,omitempty"`
	ResultJSON    string `json:"result_json,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
	DurationMS    int64  `json:"duration_ms,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
}

// OrchestrationNodeRegistryEntry maps runtime identity to graph node metadata.
type OrchestrationNodeRegistryEntry struct {
	NodeID    string
	AgentID   string
	AgentKey  string
	AgentName string
	Role      string
}

// OrchestrationRegistry indexes nodes by agent_key and agent_id.
type OrchestrationRegistry struct {
	ByAgentKey map[string]OrchestrationNodeRegistryEntry
	ByAgentID  map[string]OrchestrationNodeRegistryEntry
	ByNodeID   map[string]OrchestrationNodeRegistryEntry
}

func NewOrchestrationRegistry(entries []OrchestrationNodeRegistryEntry) OrchestrationRegistry {
	reg := OrchestrationRegistry{
		ByAgentKey: make(map[string]OrchestrationNodeRegistryEntry, len(entries)),
		ByAgentID:  make(map[string]OrchestrationNodeRegistryEntry, len(entries)),
		ByNodeID:   make(map[string]OrchestrationNodeRegistryEntry, len(entries)),
	}
	for _, e := range entries {
		if id := strings.TrimSpace(e.NodeID); id != "" {
			reg.ByNodeID[id] = e
		}
		if key := strings.TrimSpace(e.AgentKey); key != "" {
			reg.ByAgentKey[key] = e
		}
		if id := strings.TrimSpace(e.AgentID); id != "" {
			reg.ByAgentID[id] = e
		}
	}
	return reg
}

// AgentNodeState is the projected observability record for one agent node in a run.
type AgentNodeState struct {
	RunID           string
	NodeID          string
	AgentID         string
	AgentKey        string
	AgentName       string
	Role            string
	Status          AgentNodeStatus
	DisplayStatus   DisplayStatus
	Phase           WorkPhase
	RetryCount      int
	InputPreview    string
	OutputPreview   string
	ErrorMessage    string
	CurrentActivity *ActivitySnapshot
	ActivityHistory []ActivitySnapshot
}

// OrchestrationStatusStore holds per-run node states keyed by node_id.
type OrchestrationStatusStore struct {
	Nodes map[string]*AgentNodeState
}

func NewOrchestrationStatusStore(reg OrchestrationRegistry) *OrchestrationStatusStore {
	nodes := make(map[string]*AgentNodeState, len(reg.ByNodeID))
	for nodeID, entry := range reg.ByNodeID {
		nodes[nodeID] = &AgentNodeState{
			NodeID:        nodeID,
			AgentID:       entry.AgentID,
			AgentKey:      entry.AgentKey,
			AgentName:     entry.AgentName,
			Role:          entry.Role,
			Status:        AgentNodeStatusIdle,
			DisplayStatus: DisplayStatusWaiting,
			Phase:         WorkPhaseReceived,
		}
	}
	return &OrchestrationStatusStore{Nodes: nodes}
}

func (s *OrchestrationStatusStore) ApplyEnvelope(env event.Envelope, reg OrchestrationRegistry) []*AgentNodeState {
	if s == nil {
		return nil
	}
	var changed []*AgentNodeState
	switch env.Type {
	case event.EnvelopeTypeTeamStepStarted:
		if st := s.applyToResolved(env, reg, AgentNodeStatusRunning, WorkPhaseDoing); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeMemberMessageStart:
		if st := s.applyToResolved(env, reg, AgentNodeStatusThinking, WorkPhaseDoing); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeToolCall:
		if st := s.applyToolCall(env, reg); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeToolResult:
		if st := s.applyToolResult(env, reg); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeMemberMessageDone:
		if st := s.applyMemberDone(env, reg); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeTeamStepFinished:
		if st := s.applyStepFinished(env, reg); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeGraphNodeStart:
		if st := s.applyGraphNode(env, reg, AgentNodeStatusRunning, WorkPhaseDoing); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeGraphNodeEnd:
		skipped := metaBool(env.Metadata, "skipped")
		status := AgentNodeStatusSuccess
		if skipped {
			status = AgentNodeStatusSkipped
		}
		if st := s.applyGraphNode(env, reg, status, WorkPhaseDelivered); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeGraphNodeError:
		if st := s.applyGraphNodeError(env, reg); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeTransfer:
		changed = append(changed, s.applyTransfer(env, reg)...)
	case event.EnvelopeTypeCheckpoint:
		if st := s.applyToResolved(env, reg, AgentNodeStatusWaitingInput, WorkPhaseDoing); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeGraphTaskStatus:
		if st := s.applyGraphTaskStatus(env, reg); st != nil {
			changed = append(changed, st)
		}
	case event.EnvelopeTypeRunStatus:
		if isRunCancelled(env) {
			for _, st := range s.Nodes {
				if isTerminalStatus(st.Status) {
					continue
				}
				if s.setStatus(st, AgentNodeStatusCancelled) {
					changed = append(changed, cloneNodeState(st))
				}
			}
		}
	}
	return changed
}

func (s *OrchestrationStatusStore) applyToResolved(
	env event.Envelope,
	reg OrchestrationRegistry,
	status AgentNodeStatus,
	phase WorkPhase,
) *AgentNodeState {
	st := s.resolveState(env, reg)
	if st == nil {
		return nil
	}
	if !s.setStatus(st, status) {
		return nil
	}
	st.Phase = phase
	return cloneNodeState(st)
}

func (s *OrchestrationStatusStore) applyToolCall(env event.Envelope, reg OrchestrationRegistry) *AgentNodeState {
	st := s.resolveState(env, reg)
	if st == nil || env.ToolCall == nil {
		return nil
	}
	tc := env.ToolCall
	if !s.setStatus(st, AgentNodeStatusToolRunning) {
		return nil
	}
	st.Phase = WorkPhaseDoing
	st.CurrentActivity = &ActivitySnapshot{
		Kind:          strings.TrimSpace(tc.ActivityKind),
		DisplayLabel:  strings.TrimSpace(tc.DisplayLabel),
		ToolName:      strings.TrimSpace(tc.Name),
		Status:        "running",
		Summary:       strings.TrimSpace(tc.Summary),
		ArgumentsJSON: strings.TrimSpace(tc.ArgumentsJSON),
		StartedAt:     strings.TrimSpace(tc.StartedAt),
	}
	if st.CurrentActivity.Kind == "" {
		st.CurrentActivity.Kind = "tool"
	}
	if st.CurrentActivity.DisplayLabel == "" {
		st.CurrentActivity.DisplayLabel = st.CurrentActivity.ToolName
	}
	appendActivityHistory(st, *st.CurrentActivity)
	return cloneNodeState(st)
}

func (s *OrchestrationStatusStore) applyToolResult(env event.Envelope, reg OrchestrationRegistry) *AgentNodeState {
	st := s.resolveState(env, reg)
	if st == nil || env.ToolCall == nil {
		return nil
	}
	tc := env.ToolCall
	if st.CurrentActivity != nil {
		st.CurrentActivity.Status = strings.TrimSpace(tc.Status)
		if st.CurrentActivity.Status == "" {
			st.CurrentActivity.Status = "success"
		}
		st.CurrentActivity.ResultJSON = strings.TrimSpace(tc.ResultJSON)
		st.CurrentActivity.FinishedAt = strings.TrimSpace(tc.FinishedAt)
		st.CurrentActivity.DurationMS = tc.DurationMS
		st.CurrentActivity.ErrorCode = strings.TrimSpace(tc.ErrorCode)
		appendActivityHistory(st, *st.CurrentActivity)
	}
	if st.Status == AgentNodeStatusToolRunning {
		s.setStatus(st, AgentNodeStatusThinking)
	}
	return cloneNodeState(st)
}

func (s *OrchestrationStatusStore) applyMemberDone(env event.Envelope, reg OrchestrationRegistry) *AgentNodeState {
	st := s.resolveState(env, reg)
	if st == nil {
		return nil
	}
	if env.Content != nil {
		st.OutputPreview = strings.TrimSpace(env.Content.Text)
	}
	if !s.setStatus(st, AgentNodeStatusSuccess) {
		return nil
	}
	st.Phase = WorkPhaseDelivered
	st.CurrentActivity = nil
	return cloneNodeState(st)
}

func (s *OrchestrationStatusStore) applyStepFinished(env event.Envelope, reg OrchestrationRegistry) *AgentNodeState {
	st := s.resolveState(env, reg)
	if st == nil {
		return nil
	}
	status := AgentNodeStatusSuccess
	if meta := env.Metadata; meta != nil {
		if step, ok := meta["step"].(map[string]any); ok {
			if preview, ok := step["output_preview"].(string); ok && strings.TrimSpace(preview) != "" {
				st.OutputPreview = strings.TrimSpace(preview)
			}
			if preview, ok := step["input_preview"].(string); ok && strings.TrimSpace(preview) != "" {
				st.InputPreview = strings.TrimSpace(preview)
			}
			if stepStatus, ok := step["status"].(string); ok {
				switch strings.ToLower(strings.TrimSpace(stepStatus)) {
				case "error", "failed", "fail":
					status = AgentNodeStatusFailed
				}
			}
			if errMsg, ok := step["error_message"].(string); ok {
				st.ErrorMessage = strings.TrimSpace(errMsg)
			}
		}
	}
	if !s.setStatus(st, status) {
		return nil
	}
	if status == AgentNodeStatusSuccess {
		st.Phase = WorkPhaseDelivered
	}
	return cloneNodeState(st)
}

func (s *OrchestrationStatusStore) applyGraphNode(
	env event.Envelope,
	reg OrchestrationRegistry,
	status AgentNodeStatus,
	phase WorkPhase,
) *AgentNodeState {
	nodeID := metaString(env.Metadata, "node_id")
	if nodeID == "" {
		return s.applyToResolved(env, reg, status, phase)
	}
	st := s.nodeByID(nodeID)
	if st == nil {
		entry, ok := reg.ByNodeID[nodeID]
		if !ok {
			return nil
		}
		st = &AgentNodeState{
			NodeID:    nodeID,
			AgentID:   entry.AgentID,
			AgentKey:  entry.AgentKey,
			AgentName: entry.AgentName,
			Role:      entry.Role,
		}
		s.Nodes[nodeID] = st
	}
	if !s.setStatus(st, status) {
		return nil
	}
	st.Phase = phase
	return cloneNodeState(st)
}

func (s *OrchestrationStatusStore) applyGraphNodeError(env event.Envelope, reg OrchestrationRegistry) *AgentNodeState {
	status := AgentNodeStatusFailed
	if metaBool(env.Metadata, "retrying") {
		status = AgentNodeStatusRetrying
	}
	st := s.applyGraphNode(env, reg, status, WorkPhaseDoing)
	if st == nil {
		return nil
	}
	stored := s.Nodes[st.NodeID]
	if stored == nil {
		return st
	}
	if env.Error != nil {
		stored.ErrorMessage = strings.TrimSpace(env.Error.Message)
	}
	return cloneNodeState(stored)
}

func (s *OrchestrationStatusStore) applyGraphTaskStatus(env event.Envelope, reg OrchestrationRegistry) *AgentNodeState {
	taskStatus := strings.ToLower(strings.TrimSpace(metaString(env.Metadata, "task_status")))
	nodeID := metaString(env.Metadata, "node_id")
	if nodeID == "" {
		return nil
	}
	var status AgentNodeStatus
	switch taskStatus {
	case "review_required":
		status = AgentNodeStatusWaitingReview
	case "blocked":
		status = AgentNodeStatusBlocked
	case "pending_assignment":
		status = AgentNodeStatusWaitingAssign
	case "pending":
		status = AgentNodeStatusQueued
	case "claimed":
		status = AgentNodeStatusRunning
	case "complete":
		status = AgentNodeStatusSuccess
	case "failed", "crashed":
		status = AgentNodeStatusFailed
	case "timed_out":
		status = AgentNodeStatusTimedOut
	case "cancelled":
		status = AgentNodeStatusCancelled
	default:
		return nil
	}
	phase := WorkPhaseDoing
	switch status {
	case AgentNodeStatusSuccess:
		phase = WorkPhaseDelivered
	case AgentNodeStatusQueued, AgentNodeStatusWaitingAssign, AgentNodeStatusWaitingReview, AgentNodeStatusBlocked:
		phase = WorkPhaseReceived
	case AgentNodeStatusFailed, AgentNodeStatusTimedOut, AgentNodeStatusCancelled:
		phase = WorkPhaseDelivered
	}
	st := s.applyGraphNode(env, reg, status, phase)
	if st == nil {
		return nil
	}
	stored := s.Nodes[nodeID]
	if stored == nil {
		return st
	}
	if metaBool(env.Metadata, "review_rejected") {
		stored.Phase = WorkPhaseDoing
		if comment := metaString(env.Metadata, "review_comment"); comment != "" {
			stored.ErrorMessage = comment
		}
	}
	if summary := metaString(env.Metadata, "summary"); summary != "" {
		stored.OutputPreview = summary
	}
	return cloneNodeState(stored)
}

func (s *OrchestrationStatusStore) applyTransfer(env event.Envelope, reg OrchestrationRegistry) []*AgentNodeState {
	if env.Transfer == nil {
		return nil
	}
	var changed []*AgentNodeState
	if from := s.resolveByAgentKey(reg, env.Transfer.FromAgent); from != nil {
		if s.setStatus(from, AgentNodeStatusIdle) {
			from.Phase = WorkPhaseDelivered
			changed = append(changed, cloneNodeState(from))
		}
	}
	if to := s.resolveByAgentKey(reg, env.Transfer.ToAgent); to != nil {
		to.Phase = WorkPhaseDoing
		if s.setStatus(to, AgentNodeStatusRunning) {
			changed = append(changed, cloneNodeState(to))
		}
	}
	return changed
}

func (s *OrchestrationStatusStore) resolveState(env event.Envelope, reg OrchestrationRegistry) *AgentNodeState {
	if nodeID := metaString(env.Metadata, "node_id"); nodeID != "" {
		return s.nodeByID(nodeID)
	}
	if env.ToolCall != nil {
		if st := s.resolveByAgentKey(reg, env.ToolCall.AgentKey); st != nil {
			return st
		}
		if st := s.resolveByAgentID(reg, env.ToolCall.AgentID); st != nil {
			return st
		}
	}
	if st := s.resolveByAgentKey(reg, env.Author); st != nil {
		return st
	}
	if agentKey := metaString(env.Metadata, "agent_key"); agentKey != "" {
		if st := s.resolveByAgentKey(reg, agentKey); st != nil {
			return st
		}
	}
	if agentID := metaString(env.Metadata, "agent_id"); agentID != "" {
		return s.resolveByAgentID(reg, agentID)
	}
	return nil
}

func (s *OrchestrationStatusStore) resolveByAgentKey(reg OrchestrationRegistry, agentKey string) *AgentNodeState {
	key := strings.TrimSpace(agentKey)
	if key == "" {
		return nil
	}
	entry, ok := reg.ByAgentKey[key]
	if !ok {
		return nil
	}
	return s.nodeByID(entry.NodeID)
}

func (s *OrchestrationStatusStore) resolveByAgentID(reg OrchestrationRegistry, agentID string) *AgentNodeState {
	id := strings.TrimSpace(agentID)
	if id == "" {
		return nil
	}
	entry, ok := reg.ByAgentID[id]
	if !ok {
		return nil
	}
	return s.nodeByID(entry.NodeID)
}

func (s *OrchestrationStatusStore) nodeByID(nodeID string) *AgentNodeState {
	if s == nil || nodeID == "" {
		return nil
	}
	return s.Nodes[nodeID]
}

func (s *OrchestrationStatusStore) setStatus(st *AgentNodeState, next AgentNodeStatus) bool {
	if st == nil {
		return false
	}
	if isTerminalStatus(st.Status) && !canOverrideTerminal(st.Status, next) {
		return false
	}
	if st.Status == next {
		st.DisplayStatus = AggregateDisplayStatus(next)
		return false
	}
	if statusPriority(next) < statusPriority(st.Status) && !isTerminalStatus(next) {
		return false
	}
	st.Status = next
	st.DisplayStatus = AggregateDisplayStatus(next)
	return true
}

func canOverrideTerminal(current, next AgentNodeStatus) bool {
	if !isTerminalStatus(current) {
		return false
	}
	switch next {
	case AgentNodeStatusRetrying, AgentNodeStatusRunning:
		return true
	case AgentNodeStatusSkipped:
		return current == AgentNodeStatusFailed
	default:
		return false
	}
}

func isTerminalStatus(status AgentNodeStatus) bool {
	switch status {
	case AgentNodeStatusSuccess, AgentNodeStatusFailed, AgentNodeStatusSkipped,
		AgentNodeStatusCancelled, AgentNodeStatusTimedOut:
		return true
	default:
		return false
	}
}

func statusPriority(status AgentNodeStatus) int {
	switch status {
	case AgentNodeStatusBlocked, AgentNodeStatusWaitingReview, AgentNodeStatusWaitingAssign, AgentNodeStatusWaitingInput:
		return 100
	case AgentNodeStatusRetrying:
		return 90
	case AgentNodeStatusToolRunning:
		return 80
	case AgentNodeStatusThinking:
		return 70
	case AgentNodeStatusTransferring:
		return 65
	case AgentNodeStatusRunning:
		return 60
	case AgentNodeStatusScheduled:
		return 40
	case AgentNodeStatusQueued:
		return 30
	case AgentNodeStatusIdle:
		return 10
	default:
		if isTerminalStatus(status) {
			return 200
		}
		return 0
	}
}

// AggregateDisplayStatus maps a fine status to UI aggregate bucket.
func AggregateDisplayStatus(status AgentNodeStatus) DisplayStatus {
	switch status {
	case AgentNodeStatusIdle, AgentNodeStatusQueued, AgentNodeStatusScheduled:
		return DisplayStatusWaiting
	case AgentNodeStatusRunning, AgentNodeStatusThinking, AgentNodeStatusToolRunning,
		AgentNodeStatusTransferring, AgentNodeStatusRetrying:
		return DisplayStatusActive
	case AgentNodeStatusWaitingInput, AgentNodeStatusWaitingReview,
		AgentNodeStatusWaitingAssign, AgentNodeStatusBlocked:
		return DisplayStatusSuspended
	case AgentNodeStatusSuccess:
		return DisplayStatusSuccess
	case AgentNodeStatusFailed, AgentNodeStatusTimedOut:
		return DisplayStatusFailed
	case AgentNodeStatusSkipped:
		return DisplayStatusSkipped
	case AgentNodeStatusCancelled:
		return DisplayStatusCancelled
	default:
		return DisplayStatusWaiting
	}
}

func isRunCancelled(env event.Envelope) bool {
	if env.Metadata == nil {
		return false
	}
	status := strings.ToLower(metaString(env.Metadata, "status"))
	return status == "cancelled" || status == "canceled"
}

func cloneNodeState(st *AgentNodeState) *AgentNodeState {
	if st == nil {
		return nil
	}
	out := *st
	if st.CurrentActivity != nil {
		act := *st.CurrentActivity
		out.CurrentActivity = &act
	}
	if len(st.ActivityHistory) > 0 {
		out.ActivityHistory = append([]ActivitySnapshot(nil), st.ActivityHistory...)
	}
	return &out
}

const maxActivityHistory = 20

func appendActivityHistory(st *AgentNodeState, snap ActivitySnapshot) {
	if st == nil {
		return
	}
	st.ActivityHistory = append(st.ActivityHistory, snap)
	if len(st.ActivityHistory) > maxActivityHistory {
		st.ActivityHistory = st.ActivityHistory[len(st.ActivityHistory)-maxActivityHistory:]
	}
}
