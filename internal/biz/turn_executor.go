package biz

import "context"

// TurnOutcome classifies the result of a turn execution for any entry point
// (Web, WS, Channel, Cron, A2A). It unifies ChannelTurnOutcome into a single
// canonical type.
type TurnOutcome string

const (
	TurnOutcomeCompleted TurnOutcome = "completed"
	TurnOutcomeQueued    TurnOutcome = "queued"
	TurnOutcomeRejected  TurnOutcome = "rejected"
	TurnOutcomeFailed    TurnOutcome = "failed"
)

// TurnResult is the unified return type for TurnExecutor.Execute.
// It carries the outcome classification plus the relevant messages.
type TurnResult struct {
	Outcome      TurnOutcome
	UserMsg      ChatMessage
	AssistantMsg ChatMessage
	PendingID    string
	Reply        string // plain-text reply for non-chat callers (Channel, Cron)
}

// ---------------------------------------------------------------------------
// Turn lifecycle hooks — each hook isolates one cross-cutting concern so that
// Agent turns and Team turns share the same admission / locking / persistence
// / observability infrastructure without duplicating code.
// ---------------------------------------------------------------------------

// TurnAdmissionDecider determines whether a new turn should be admitted,
// enqueued, or rejected when a session already has an active run.
// Stability:evolving
type TurnAdmissionDecider interface {
	Decide(hasActiveRun bool, hasRunner bool) TurnAdmissionDecision
}

// TurnAdmissionDecision represents the outcome of an admission check.
type TurnAdmissionDecision int

const (
	AdmitRun     TurnAdmissionDecision = iota // proceed with turn execution
	AdmitEnqueue                              // enqueue as pending message
	AdmitReject                               // reject the turn
)

// SessionLocker provides session-level mutual exclusion for turn execution.
// Implementations must be reentrant-safe for the same session.
// Stability:evolving
type SessionLocker interface {
	// LockSession acquires a lock for the given session. Returns an unlock function.
	LockSession(sessionID string) (unlock func())
}

// PendingQueueManager manages the pending message queue for a session.
// When a turn is admitted while another is running, the message is enqueued
// and processed after the active run completes.
// Stability:evolving
type PendingQueueManager interface {
	// Enqueue adds a user message to the pending queue.
	Enqueue(ctx context.Context, sessionID, content string) (accepted bool, pendingID string, rejectReason string, err error)
	// Dequeue processes pending messages after a turn completes.
	Dequeue(sessionID string, sess Session, agent Agent, provider, model, dialogMode string)
	// DropPending removes a pending message after it has been injected during a run.
	DropPending(sessionID, pendingUserID string)
	// LastPendingID returns the most recently enqueued pending message ID.
	LastPendingID(sessionID string) string
}

// RunRegistry tracks active runs and provides cancellation.
// Stability:evolving
type RunRegistry interface {
	// HasActive reports whether a session has an in-flight run.
	HasActive(sessionID string) bool
	// StoreCancelable registers a cancel function for a session run.
	StoreCancelable(sessionID, runID string, cancel context.CancelFunc)
	// ActiveRunner returns the runner and cancel for an active run.
	ActiveRunner(sessionID string) (runID string, cancel context.CancelFunc, ok bool)
	// Finish removes the active run entry for a session when runID matches
	// (or runID is empty for backward-compat delete-any).
	Finish(sessionID, runID string)
}

// TurnTracer provides structured tracing for turn lifecycle events.
// Stability:evolving
type TurnTracer interface {
	// StartTrace begins a trace for a turn.
	StartTrace(ctx context.Context, sessionID, flowName, description string, params ...TraceParam) TurnTraceSpan
}

// TraceParam is a key-value pair for trace annotations.
type TraceParam struct {
	Key   string
	Value any
}

// TurnTraceSpan represents an in-progress trace span for a turn.
// Stability:evolving
type TurnTraceSpan interface {
	Log(phase, description string, params ...TraceParam)
	LogDone(phase, description string, params ...TraceParam)
	LogError(phase, description string, params ...TraceParam)
	Finish()
}

// TurnUsageRecorder records token usage and quota checks for a turn.
// Stability:evolving
type TurnUsageRecorder interface {
	// CheckQuotas verifies that the user/session has sufficient quota before a turn.
	CheckQuotas(ctx context.Context, agentID, userID string) error
	// RecordUsage persists token usage after a turn completes.
	RecordUsage(ctx context.Context, sessionID string, tokenIn, tokenOut int)
}

// TurnPersistenceHook persists turn results (messages, metadata) after execution.
// Agent and Team runtimes implement this to handle their own persistence format.
// Stability:evolving
type TurnPersistenceHook interface {
	// PersistTurnRecord saves the turn result to the data store.
	PersistTurnRecord(ctx context.Context, sessionID string, result TurnResult) error
}

// TurnEventProjector projects runtime events into UI-consumable envelopes.
// Both Agent and Team runtimes implement this to emit domain-specific events.
// Stability:evolving
type TurnEventProjector interface {
	// ProjectEvents emits envelopes for the turn lifecycle (start, progress, completion, error).
	ProjectEvents(ctx context.Context, sessionID, runID string, result TurnResult) error
}

// ---------------------------------------------------------------------------
// TurnExecutor — the shared entry point
// ---------------------------------------------------------------------------

// TurnExecutor is the shared entry point for all turn execution paths.
// Web, WS, Channel, Cron, and A2A should call Execute instead of reaching
// into ChatService or ChatOrchestrator internals.
//
// Implementations live in internal/service (ChatOrchestrator implements this).
// Wire binding happens in internal/service.
// Stability:evolving
type TurnExecutor interface {
	// Execute runs a single turn (agent or team) and returns a classified result.
	Execute(ctx context.Context, input TurnInput) (TurnResult, error)

	// HasActiveRun reports whether a session has an in-flight run.
	HasActiveRun(sessionID string) bool

	// CancelRun stops the active run for a session.
	CancelRun(ctx context.Context, sessionID string) bool
}
