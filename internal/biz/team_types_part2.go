package biz

import (
	"context"
)

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
	ExecID    string `json:"exec_id"`
	TeamRunID string `json:"team_run_id"`
	TeamID    string `json:"team_id"`
	SessionID string `json:"session_id"`
	// SpiritSessionID is the watch subscription filter key
	// (EventSubscribeOptions.SpiritSessionID). Persisted (Y5) so
	// RecoverSessions can rebuild the step watch after a restart.
	SpiritSessionID string `json:"spirit_session_id"`
	InputPreview    string `json:"input_preview"`
	DefinitionJSON  string `json:"definition_json"`
	Status          string `json:"status"`
	RegisteredAt    string `json:"registered_at"`
	LastActivityAt  string `json:"last_activity_at"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
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
