package biz

import (
	"encoding/json"
	"strings"
)

const (
	TeamRunStatusPending      = "pending"
	TeamRunStatusRunning      = "running"
	TeamRunStatusSuccess      = "success"
	TeamRunStatusFailed       = "failed"
	TeamRunStatusCancelled    = "cancelled"
	TeamRunStatusWaitingHuman = "waiting_human"
)

var teamRunTerminalStatuses = map[string]bool{
	TeamRunStatusSuccess:   true,
	TeamRunStatusFailed:    true,
	TeamRunStatusCancelled: true,
}

func IsTeamRunTerminalStatus(status string) bool {
	return teamRunTerminalStatuses[status]
}

var teamRunValidTransitions = map[string][]string{
	TeamRunStatusPending:      {TeamRunStatusRunning, TeamRunStatusCancelled},
	TeamRunStatusRunning:      {TeamRunStatusWaitingHuman, TeamRunStatusSuccess, TeamRunStatusFailed, TeamRunStatusCancelled},
	TeamRunStatusWaitingHuman: {TeamRunStatusRunning, TeamRunStatusSuccess, TeamRunStatusFailed, TeamRunStatusCancelled},
}

func ValidateTeamRunTransition(from, to string) bool {
	if from == to {
		return true
	}
	allowed, ok := teamRunValidTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

const OrchestrationControlToolName = "orchestration_control"

const CriticLoopCondFuncRef = "critic_loop_decision"

type OrchestrationDecision struct {
	Action string  `json:"action"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

func ParseOrchestrationDecision(args []byte) (OrchestrationDecision, error) {
	var d OrchestrationDecision
	if err := json.Unmarshal(args, &d); err != nil {
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

type Team struct {
	ID             string
	TeamKey        string
	DisplayName    string
	Status         string
	IsDefault      bool
	DefinitionJSON string
	ADKAppName     string
	CreatedAt      string
	UpdatedAt      string
	DeletedAt      string
}

type TeamRun struct {
	ID            string `json:"id"`
	TeamID        string `json:"team_id"`
	SessionID     string `json:"session_id"`
	MessageID     string `json:"message_id"`
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	InputPreview  string `json:"input_preview"`
	OutputPreview string `json:"output_preview"`
	TokenIn       int    `json:"token_in"`
	TokenOut      int    `json:"token_out"`
	CostMicroUSD  int64  `json:"cost_micro_usd"`
	DurationMS    int    `json:"duration_ms"`
	ErrorMessage  string `json:"error_message"`
	TopologyJSON  string `json:"topology_json"`
	GraphExecutionID         string `json:"graph_execution_id,omitempty"`
	DefinitionSnapshotJSON   string `json:"definition_snapshot_json,omitempty"`
	TraceID                  string `json:"trace_id,omitempty"`
	StartedAt                string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
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
