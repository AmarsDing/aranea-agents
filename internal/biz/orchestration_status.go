package biz

import (
	"strings"
	"sync"

	"aranea-agents/internal/event/contract"
)

// activityJSONPreviewLimit is the maximum byte length of ArgumentsJSON / ResultJSON
// surfaced through Observatory WS/RPC. Full content could expose PII from tool calls
// (e.g., file contents, credentials) to observers who lack data-plane access (SEC-04).
const activityJSONPreviewLimit = 512

// redactActivityJSON truncates tool call arguments/results to a safe preview size.
// If the value exceeds the limit it is truncated and appended with a redaction marker.
func redactActivityJSON(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= activityJSONPreviewLimit {
		return s
	}
	return s[:activityJSONPreviewLimit] + "…[truncated]"
}

// RedactActivityJSON is the exported version of redactActivityJSON for use by
// packages outside biz (e.g., agent.ActivityProjector) that need to apply the
// same SEC-04 redaction when building WS envelopes.
func RedactActivityJSON(s string) string {
	return redactActivityJSON(s)
}

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
	mu    sync.RWMutex
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

func (s *OrchestrationStatusStore) ApplyEnvelope(env contract.Envelope, reg OrchestrationRegistry) []*AgentNodeState {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var changed []*AgentNodeState
	switch env.Type {
	case contract.EnvelopeTypeTeamStepStarted:
		if st := s.applyToResolved(env, reg, AgentNodeStatusRunning, WorkPhaseDoing); st != nil {
			changed = append(changed, st)
		}
	case contract.EnvelopeTypeTeamStepFinished:
		if st := s.applyStepFinished(env, reg); st != nil {
			changed = append(changed, st)
		}
	case contract.EnvelopeTypeGraphNodeStart:
		if st := s.applyGraphNode(env, reg, AgentNodeStatusRunning, WorkPhaseDoing); st != nil {
			changed = append(changed, st)
		}
	case contract.EnvelopeTypeGraphNodeEnd:
		skipped := metaBool(env.Metadata, "skipped")
		status := AgentNodeStatusSuccess
		if skipped {
			status = AgentNodeStatusSkipped
		}
		if st := s.applyGraphNode(env, reg, status, WorkPhaseDelivered); st != nil {
			changed = append(changed, st)
		}
	case contract.EnvelopeTypeGraphNodeError:
		if st := s.applyGraphNodeError(env, reg); st != nil {
			changed = append(changed, st)
		}
	case contract.EnvelopeTypeCheckpoint:
		if st := s.applyToResolved(env, reg, AgentNodeStatusWaitingInput, WorkPhaseDoing); st != nil {
			changed = append(changed, st)
		}
	case contract.EnvelopeTypeGraphTaskStatus:
		if st := s.applyGraphTaskStatus(env, reg); st != nil {
			changed = append(changed, st)
		}
	case contract.EnvelopeTypeRunStatus:
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

// GetNode returns a snapshot of the node state for read-only external access (e.g., WebSocket handlers).
func (s *OrchestrationStatusStore) GetNode(nodeID string) (*AgentNodeState, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.Nodes[nodeID]
	if !ok {
		return nil, false
	}
	return cloneNodeState(st), true
}

func (s *OrchestrationStatusStore) applyToResolved(
	env contract.Envelope,
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

func (s *OrchestrationStatusStore) applyStepFinished(env contract.Envelope, reg OrchestrationRegistry) *AgentNodeState {
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
	env contract.Envelope,
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
			NodeID:        nodeID,
			AgentID:       entry.AgentID,
			AgentKey:      entry.AgentKey,
			AgentName:     entry.AgentName,
			Role:          entry.Role,
			Status:        AgentNodeStatusIdle,
			DisplayStatus: DisplayStatusWaiting,
			Phase:         WorkPhaseReceived,
		}
		s.Nodes[nodeID] = st
	}
	if !s.setStatus(st, status) {
		return nil
	}
	st.Phase = phase
	return cloneNodeState(st)
}

func (s *OrchestrationStatusStore) applyGraphNodeError(env contract.Envelope, reg OrchestrationRegistry) *AgentNodeState {
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

func (s *OrchestrationStatusStore) applyGraphTaskStatus(env contract.Envelope, reg OrchestrationRegistry) *AgentNodeState {
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

func (s *OrchestrationStatusStore) resolveState(env contract.Envelope, reg OrchestrationRegistry) *AgentNodeState {
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

// setStatus applies a status transition validated against the AgentNodeStatus
// state machine (AS-FSM-01). Returns true when the status actually changed.
func (s *OrchestrationStatusStore) setStatus(st *AgentNodeState, next AgentNodeStatus) bool {
	if st == nil {
		return false
	}
	current := st.Status
	if current == "" {
		current = AgentNodeStatusIdle
	}
	if current == next {
		st.DisplayStatus = AggregateDisplayStatus(next)
		return false
	}
	if !CanTransitionAgentNodeStatus(current, next) {
		return false
	}
	st.Status = next
	st.DisplayStatus = AggregateDisplayStatus(next)
	return true
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

// IsTerminalAgentNodeStatus reports whether orchestration node status is terminal.
func IsTerminalAgentNodeStatus(status AgentNodeStatus) bool {
	return isTerminalStatus(status)
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

func isRunCancelled(env contract.Envelope) bool {
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
