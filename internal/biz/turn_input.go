package biz

import (
	"context"
	"time"
)

// TurnInput is the transport-neutral input for a single chat/team turn.
// It replaces direct proto (chatv1.SendChatMessageRequest) usage in internal
// turn paths so that internal/team, internal/agent, and internal/channel
// never import api/*/v1.
type TurnInput struct {
	SessionID   string
	Content     string
	AgentKey    string
	TeamID      string
	Options     TurnOptions
	Timeouts    TurnTimeouts
	EntryConfig TurnEntryPointConfig
}

// TurnOptions carries per-turn overrides (dialog mode, provider, model, attachments).
type TurnOptions struct {
	DialogMode     string
	Provider       string
	Model          string
	AttachmentIDs  []string
	KnowledgeBases []string
}

// TurnTimeouts centralises all timeout/deadline policies for a single turn.
// Previously these were scattered across channel_turn_context.go and
// chat_orchestrator_turn.go; this struct is the single source of truth.
type TurnTimeouts struct {
	// TurnTimeout is the maximum wall-clock duration for a single turn.
	// Zero means "use default" (5 min for chat, configurable for channel).
	TurnTimeout time.Duration
	// FirstByteTimeout is the maximum wait for the first model event.
	// Zero means "use default" (30s).
	FirstByteTimeout time.Duration
}

// Defaults — shared across all entry points.
const (
	DefaultTurnTimeout      = 5 * time.Minute
	DefaultFirstByteTimeout = 30 * time.Second
)

// Resolved applies defaults for zero-valued fields.
func (t TurnTimeouts) Resolved() TurnTimeouts {
	r := t
	if r.TurnTimeout <= 0 {
		r.TurnTimeout = DefaultTurnTimeout
	}
	if r.FirstByteTimeout <= 0 {
		r.FirstByteTimeout = DefaultFirstByteTimeout
	}
	return r
}

// NativeTurnGateway is the narrow Chat surface Channel ingress needs.
// Channel only depends on this interface instead of the full ChatService,
// ensuring it never imports proto types or reaches Chat internals.
type NativeTurnGateway interface {
	// RunNativeTurn executes a synchronous agent/team turn and returns user + assistant messages.
	RunNativeTurn(ctx context.Context, input TurnInput) (ChatMessage, ChatMessage, error)
	// RunNativeTurnWithOutcome returns an explicit outcome (completed / queued / failed) for Channel ingress.
	RunNativeTurnWithOutcome(ctx context.Context, input TurnInput) (NativeTurnResult, error)
	// HasActiveRun reports whether a session has an in-flight run.
	HasActiveRun(sessionID string) bool
	// LastPendingMessageID returns the most recently enqueued pending message id.
	LastPendingMessageID(sessionID string) string
	// CancelRun stops the active run for a session.
	CancelRun(ctx context.Context, sessionID string) bool
	// SetRunStatus atomically updates the run status and publishes a WS envelope.
	SetRunStatus(sessionID, runID, status, errMsg string)
	// CancelSessionRunForCard cancels a session run by ID for card action callbacks.
	CancelSessionRunForCard(ctx context.Context, sessionRunID, expectedSessionID string) (cancelled bool, reply string)
	// ActiveSessionRunPhase returns the phase of the active session run, if any.
	ActiveSessionRunPhase(ctx context.Context, sessionID string) string
	// EscalateActiveSessionRun escalates the active session run to background for a session.
	EscalateActiveSessionRun(ctx context.Context, sessionID string) (escalated bool, reply string, err error)
	// EscalateSessionRun escalates a specific session run to background.
	EscalateSessionRun(ctx context.Context, sessionRunID, expectedSessionID string) (reply string, err error)
}

// ---------------------------------------------------------------------------
// Entry Point Classification
// ---------------------------------------------------------------------------
// TurnEntryPoint identifies the origin of a turn request. This allows the
// TurnExecutor to apply entry-point-specific policies (e.g. Channel has
// different timeout defaults than Web) without the executor knowing the
// concrete transport details.

type TurnEntryPoint string

const (
	EntryPointWeb     TurnEntryPoint = "web"     // HTTP REST API
	EntryPointWS      TurnEntryPoint = "ws"      // WebSocket streaming
	EntryPointChannel TurnEntryPoint = "channel" // IM platform (Lark, Slack, etc.)
	EntryPointCron    TurnEntryPoint = "cron"    // Scheduled task trigger
	EntryPointA2A     TurnEntryPoint = "a2a"     // Agent-to-Agent protocol
	EntryPointDurable TurnEntryPoint = "durable" // Durable resume (background worker)
)

// TurnEntryPointConfig carries entry-point-specific configuration that
// influences turn execution behaviour (timeouts, admission policy, etc.).
type TurnEntryPointConfig struct {
	// EntryPoint identifies the origin of the turn.
	EntryPoint TurnEntryPoint
	// AllowQueue indicates whether this entry point supports pending queue
	// (Web/WS yes, Channel/Cron typically no — they use async jobs instead).
	AllowQueue bool
	// AllowStream indicates whether this entry point supports streaming responses.
	AllowStream bool
	// Platform identifies the channel platform (only relevant when EntryPoint == channel).
	Platform string
}

// WithEntryPoint returns a copy of TurnInput with the entry point config set.
func (t TurnInput) WithEntryPoint(cfg TurnEntryPointConfig) TurnInput {
	t.EntryConfig = cfg
	return t
}
