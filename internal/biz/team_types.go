package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// Team status constants — lifecycle states for a Team entity.
// These replace the old raw strings ("active", "waiting_deps", "assembled").
const (
	TeamStatusPending     = "pending"     // Created, waiting to execute
	TeamStatusRunning     = "running"     // Actively executing
	TeamStatusCompleted   = "completed"   // Finished successfully
	TeamStatusFailed      = "failed"      // Execution failed
	TeamStatusCancelled   = "cancelled"   // Was cancelled
	TeamStatusInterrupted = "interrupted" // Abnormally interrupted, recoverable
	TeamStatusArchived    = "archived"    // Auto-archived after completion
	TeamStatusDeleted     = "deleted"     // Soft-deleted (only in data layer, not a valid state machine state)

	// TeamStatusBlocked is a virtual status used only in cascade blocked results
	// to indicate a team was blocked by a failed dependency. It is never persisted.
	TeamStatusBlocked = "blocked"
)

// IsTeamStatusActive returns true if the team status means the team is
// considered "active" (i.e. not terminal and not deleted).
func IsTeamStatusActive(status string) bool {
	return status == TeamStatusPending || status == TeamStatusRunning || status == TeamStatusInterrupted
}

// TeamRun status constants and terminal-status helpers are defined in
// team_run_state_machine.go alongside the transition table (AS-FSM-01).
const OrchestrationControlToolName = "orchestration_control"

// CriticLoopCondFuncRef is kept as an alias for backward compatibility.
const CriticLoopCondFuncRef = CriticLoopDecisionFunc

// CriticLoopCondFuncRefForThreshold returns a CondFuncRef that embeds the score
// threshold so graph builds with different critic_loop.score_threshold do not
// overwrite each other on the shared registry.
func CriticLoopCondFuncRefForThreshold(threshold float64) string {
	if threshold <= 0 {
		return CriticLoopCondFuncRef
	}
	return CriticLoopCondFuncRef + "@" + formatCriticThreshold(threshold)
}

func formatCriticThreshold(threshold float64) string {
	// Trim trailing zeros for stable refs (0.8 not 0.800000).
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", threshold), "0"), ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

type OrchestrationDecision struct {
	Action string  `json:"action"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

func ParseOrchestrationDecision(args []byte, lg loggateway.Logger) (OrchestrationDecision, error) {
	var d OrchestrationDecision
	if err := json.Unmarshal(args, &d); err != nil {
		lg.Warn("解析 orchestration decision 失败", loggateway.StepID("team.orchestration_decision"), loggateway.Err(err))
		return d, err
	}
	return d, nil
}

func IsApprovedDecision(d OrchestrationDecision, threshold float64) bool {
	if d.Action == "approve" {
		return true
	}
	if d.Score > 0 && threshold > 0 && d.Score >= threshold {
		return true
	}
	return false
}

func ExtractScore(content string) float64 {
	type scorePayload struct {
		Score float64 `json:"score"`
	}
	var payloads []scorePayload
	if err := json.Unmarshal([]byte(content), &payloads); err == nil {
		for _, p := range payloads {
			if p.Score > 0 {
				return p.Score
			}
		}
	}
	var single scorePayload
	if err := json.Unmarshal([]byte(content), &single); err == nil && single.Score > 0 {
		return single.Score
	}
	return 0
}

func ExtractScoreFromLowerContent(content string) float64 {
	return ExtractScore(strings.ToLower(content))
}

// TeamMemberStepStatus constants — per-member step status used in
// ChatMessage.Status, TeamRunStep.Status and the in-memory `turnStatus` tracker
// inside team Runner. Keep distinct from TeamRunStatus* (run-level) values.
const (
	TeamMemberStepStatusOK      = "ok"
	TeamMemberStepStatusError   = "error"
	TeamMemberStepStatusSkipped = "skipped"
)

// TokenUsageStatus constants — values written to model_token_usage_events.status.
// Producers should map TeamMemberStepStatusOK to TokenUsageStatusSuccess before
// recording, while TeamMemberStepStatusError maps directly to TokenUsageStatusError.
const (
	TokenUsageStatusSuccess = "success"
	TokenUsageStatusError   = "error"
)

// NormalizeTokenUsageStatus maps an in-memory member step status string to the
// canonical token usage status value. Empty / "ok" become "success"; everything
// else is passed through unchanged so callers can pass through "error" / custom
// terminal states.
func NormalizeTokenUsageStatus(status string) string {
	switch status {
	case "", TeamMemberStepStatusOK:
		return TokenUsageStatusSuccess
	default:
		return status
	}
}

// TECH-DEBT(COG): struct字段=23, 上限=15 — 下一迭代拆分：
//   - TeamOrgMeta: DepartmentID, DeptLeadAgentID, Deliverables, InputContract, CrossDeptMemberIDs
//   - TeamOrchestrationMeta: LinkedGraphID, SpiritSessionID, TaskDescription, AutoCreated, DagNodeID, DependsOn, ParallelConfigJSON, Topology, Readonly, InterruptReason
type Team struct {
	ID                 string
	TeamKey            string
	DisplayName        string
	Status             string
	IsDefault          bool
	DefinitionJSON     string
	ADKAppName         string
	DepartmentID       string
	DeptLeadAgentID    string
	Deliverables       string // JSON array of DeliverableContract
	InputContract      string // JSON array of DeliverableContract (expected from upstream)
	DeliverablesOutput string // JSON object keyed by dag_node_id: DeliverableRef envelopes (P2) with legacy plain-string summaries tolerated on read (TECH-DEBT #B-03 fix)
	CrossDeptMemberIDs string // JSON array of cross-department member agent IDs
	LinkedGraphID      string // FK to graph_definitions; bidirectional reference with graph.team_id
	SpiritSessionID    string
	TaskDescription    string
	AutoCreated        bool
	DagNodeID          string
	DependsOn          []string
	ParallelConfigJSON string
	Topology           string
	Readonly           bool
	Kind               string // user | system_builtin | ecosystem_preset | marketplace | certified (maps from DB kind column)
	Source             string // user | system | imported
	InterruptReason    string // reason for team interruption (e.g. server restart, user cancel)
	CreatedAt          string
	UpdatedAt          string
	DeletedAt          string
	// WorkspaceID is the owning workspace ID for tenant isolation (P2-B).
	// empty = shared/legacy (visible to all workspaces); non-empty = tenant-private.
	WorkspaceID string
}

// ---------------------------------------------------------------------------
// P2 产物引用化：DeliverableRef 信封
// ---------------------------------------------------------------------------

// DeliverableRef is the P2 envelope stored per dag_node_id in
// Team.DeliverablesOutput. Instead of persisting the (already truncated)
// summary only, the envelope carries enough metadata for downstream teams to
// decide whether they need the full deliverable — retrievable on demand via
// the read_upstream_deliverable tool (full text lives in steps_v2, not here).
type DeliverableRef struct {
	Summary       string `json:"summary"`
	KeyFindings   string `json:"key_findings,omitempty"`
	TeamID        string `json:"team_id"`
	TeamSessionID string `json:"team_session_id"`
	SizeChars     int    `json:"size_chars"`
	Truncated     bool   `json:"truncated"`
}

// ParseDeliverableRefs parses Team.DeliverablesOutput into per-node refs.
// Dual-mode: values written before P2 are plain JSON strings (the summary);
// values written after P2 are DeliverableRef objects. Both forms coexist.
// Empty/corrupt input yields an empty map; individually invalid values are
// skipped (tolerant read, consistent with the pre-P2 behavior).
func ParseDeliverableRefs(deliverablesOutput string) map[string]DeliverableRef {
	out := make(map[string]DeliverableRef)
	if deliverablesOutput == "" || deliverablesOutput == "{}" {
		return out
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(deliverablesOutput), &raw); err != nil {
		return out
	}
	for key, val := range raw {
		// Legacy form: plain JSON string → summary-only ref.
		var legacy string
		if err := json.Unmarshal(val, &legacy); err == nil {
			out[key] = DeliverableRef{Summary: legacy}
			continue
		}
		var ref DeliverableRef
		if err := json.Unmarshal(val, &ref); err != nil {
			continue
		}
		out[key] = ref
	}
	return out
}

type TeamRunRecord struct {
	ID                     string `json:"id"`
	TeamID                 string `json:"team_id"`
	SessionID              string `json:"session_id"`
	MessageID              string `json:"message_id"`
	Mode                   string `json:"mode"`
	Status                 string `json:"status"`
	InputPreview           string `json:"input_preview"`
	OutputPreview          string `json:"output_preview"`
	TokenIn                int    `json:"token_in"`
	TokenOut               int    `json:"token_out"`
	CostMicroUSD           int64  `json:"cost_micro_usd"`
	DurationMS             int    `json:"duration_ms"`
	ErrorMessage           string `json:"error_message"`
	TopologyJSON           string `json:"topology_json"`
	GraphExecutionID       string `json:"graph_execution_id,omitempty"`
	DefinitionSnapshotJSON string `json:"definition_snapshot_json,omitempty"`
	TraceID                string `json:"trace_id,omitempty"`
	StartedAt              string `json:"started_at"`
	FinishedAt             string `json:"finished_at"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`

	// SpiritSessionID is the root spirit session ID for cross-session activity
	// aggregation (chat domain). Non-persistent runtime metadata — set by the
	// team runner when creating/loading a TeamRunRecord before publishing ActivityEvents.
	// The Bus layer normalizes empty SpiritSessionID by falling back to SessionID
	// (design doc B.6.2), so missing values degrade gracefully rather than fail.
	SpiritSessionID string `json:"-"`
}

type TeamRunStep struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	TeamID        string `json:"team_id"`
	AgentID       string `json:"agent_id"`
	AgentKey      string `json:"agent_key"`
	AgentName     string `json:"agent_name"`
	Role          string `json:"role"`
	SortOrder     int    `json:"sort_order"`
	Status        string `json:"status"`
	InputPreview  string `json:"input_preview"`
	OutputPreview string `json:"output_preview"`
	TokenIn       int    `json:"token_in"`
	TokenOut      int    `json:"token_out"`
	CostMicroUSD  int64  `json:"cost_micro_usd"`
	DurationMS    int    `json:"duration_ms"`
	ErrorMessage  string `json:"error_message"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	CreatedAt     string `json:"created_at"`
	ToolCallCount int    `json:"tool_call_count"`
}

type TeamStructureSnapshot struct {
	EntryNodeID string
	Nodes       []StructureNode
	Edges       []StructureEdge
	Surfaces    []StructureSurface
}

type StructureNode struct {
	NodeID string
	Kind   string
	Name   string
}

type StructureEdge struct {
	FromNodeID string
	ToNodeID   string
}

type StructureSurface struct {
	NodeID string
	Name   string
}

// TeamRunMemberSummaryData is per-member aggregate for a team run summary.
type TeamRunMemberSummaryData struct {
	AgentID       string
	AgentKey      string
	AgentName     string
	Role          string
	SortOrder     int
	Status        string
	TokenIn       int
	TokenOut      int
	DurationMS    int
	CostMicroUSD  int64
	ToolCallCount int
	OutputPreview string
	SessionID     string // 成员所在的子会话 ID，用于前端 lazy-load 成员执行过程
}

type TeamGraphSession struct {
	ExecID         string `json:"exec_id"`
	TeamRunID      string `json:"team_run_id"`
	TeamID         string `json:"team_id"`
	SessionID      string `json:"session_id"`
	InputPreview   string `json:"input_preview"`
	DefinitionJSON string `json:"definition_json"`
	Status         string `json:"status"`
	RegisteredAt   string `json:"registered_at"`
	LastActivityAt string `json:"last_activity_at"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// TeamGraphSessionReader provides read access to team graph sessions.
// Stability:stable
type TeamGraphSessionReader interface {
	GetSession(ctx context.Context, execID string) (TeamGraphSession, error)
	ListActiveSessions(ctx context.Context) ([]TeamGraphSession, error)
}

// TeamGraphSessionWriter provides write access to team graph sessions.
// Stability:stable
type TeamGraphSessionWriter interface {
	SaveSession(ctx context.Context, sess TeamGraphSession) error
	UpdateSessionStatus(ctx context.Context, execID, status string) error
	DeleteSession(ctx context.Context, execID string) error
	MarkOrphanedSessionsTerminal(ctx context.Context) (int, error)
}

// TeamGraphSessionRepo combines read and write access for backward compatibility.
// New code should depend on TeamGraphSessionReader or TeamGraphSessionWriter instead.
type TeamGraphSessionRepo interface {
	TeamGraphSessionReader
	TeamGraphSessionWriter
}

// TeamRunSummaryData aggregates run-level and per-member stats for RPC / Monitor.
type TeamRunSummaryData struct {
	RunID         string
	TeamID        string
	SessionID     string
	Mode          string
	Status        string
	DurationMS    int
	TokenIn       int
	TokenOut      int
	CostMicroUSD  int64
	MemberCount   int
	ToolCallCount int
	OutputPreview string
	ErrorMessage  string
	Members       []TeamRunMemberSummaryData
}
