package biz

import (
	"context"
	"time"

	artifactbiz "aranea-agents/internal/biz/artifact"
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
	// ParentTaskID is set by the system-push pattern (e.g. synthesis trigger
	// after all teams complete) to attach the new Turn to an existing Task
	// instead of creating a new one. Empty for normal user-input turns.
	// Design: docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md
	// §3.2.1 — a Task corresponds to one user input; system-push Turns are
	// continuation Turns on the same Task, not new Tasks.
	ParentTaskID string
	// Synthesis 标记本 turn 为精灵总结 turn（所有团队完成后由 TeamStarter
	// 触发）。沿 TurnIntent→执行链路→v2 ProjectMeta 透传，reply step 的
	// AuthorAgentKey 覆盖为 SynthesisAuthorAgentKey，供前端总结徽章高亮。
	Synthesis bool
	// Voice 携带语音输入溯源元数据（M74 V2-T6）：随用户消息 options_json
	// 持久化（input_modality / asr_provider / asr_duration_ms）并附展示态
	// 留档音频引用。nil = 非语音输入。
	Voice *VoiceTurnMeta
}

// VoiceTurnMeta 是语音输入的溯源元数据（M74 V2-T6）。
// Archive 为展示态附件引用（合并进 options_json.attachments 供 UI 回放）——
// 刻意不经 Options.AttachmentIDs：避免 LLM 附件能力校验拒绝（audio/* 视为
// file 附件）及 WAV 字节注入 LLM 上下文。
type VoiceTurnMeta struct {
	ASRProvider string
	DurationMs  int
	Archive     *artifactbiz.Ref // nil = 未留档（开关关闭或降级）
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
// Stability:evolving
type ChannelTurnGateway interface {
	TurnGateway
	TurnControlGateway
	PendingQueueGateway
}

// PendingQueueGateway is the narrow interface for pending message queue operations.
// Stability:evolving
type PendingQueueGateway interface {
	// TryEnqueueUserMessage enqueues a user message into the active turn's pending queue.
	TryEnqueueUserMessage(sessionID, content string) (bool, error)
	// SetSessionPendingMergeFollowup configures whether followup messages merge into the active turn.
	SetSessionPendingMergeFollowup(sessionID string, merge bool)
	// InterruptAndSend promotes a pending message to the front, marks it high priority,
	// and cancels the current turn so the pending queue processor picks it up next.
	InterruptAndSend(ctx context.Context, sessionID, pendingEntryID string) error
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
	// EntryPointEvaluation marks synthetic turns executed by the evaluation
	// runner (one per dataset case). They must not retrigger post-turn side
	// effects (e.g. after_turn auto-eval) — otherwise each case turn would
	// spawn a new eval run and cascade recursively.
	EntryPointEvaluation TurnEntryPoint = "evaluation"
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
