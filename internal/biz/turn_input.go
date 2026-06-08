package biz

import (
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

// ChannelTurnGateway is the complete Chat surface that Channel ingress needs.
// Channel only depends on this interface instead of the full ChatService,
// ensuring it never imports proto types or reaches Chat internals.
type ChannelTurnGateway interface {
	TurnGateway
	TurnControlGateway
	PendingQueueGateway
}

// NativeTurnGateway is an alias for ChannelTurnGateway.
//
// Deprecated: use ChannelTurnGateway instead.
type NativeTurnGateway = ChannelTurnGateway

// PendingQueueGateway is the narrow interface for pending message queue operations.
type PendingQueueGateway interface {
	// TryEnqueueUserMessage enqueues a user message into the active turn's pending queue.
	TryEnqueueUserMessage(sessionID, content string) (bool, error)
	// SetSessionPendingMergeFollowup configures whether followup messages merge into the active turn.
	SetSessionPendingMergeFollowup(sessionID string, merge bool)
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

// AllowPendingQueue reports whether this turn may enqueue while another run is active.
// Legacy paths without EntryPoint default to true (Web/WS behavior).
func (t TurnInput) AllowPendingQueue() bool {
	if t.EntryConfig.EntryPoint == "" {
		return true
	}
	return t.EntryConfig.AllowQueue
}
