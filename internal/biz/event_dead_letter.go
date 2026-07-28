package biz

import (
	"context"
	"time"
)

// Event dead-letter states (P1-R2b). Mirrors the memory_job_deadletter
// lifecycle: pending → replayed | abandoned.
const (
	EventDeadLetterStatePending   = "pending"
	EventDeadLetterStateReplayed  = "replayed"
	EventDeadLetterStateAbandoned = "abandoned"
)

// EventDeadLetter is a durable record of a v2 sequencer event whose entity
// persist failed permanently (retry exhaustion or persist-queue overflow).
// The in-memory dead-letter ring survives only the process lifetime; this
// record makes the loss recoverable across restarts via startup+periodic
// replay.
//
// The payload is the marshaled ENTITY (Task/Turn/Step/...), not the event
// envelope: replay routes by EntityKind+EntityOp, so new event kinds over the
// same entity require no replay-mapping change.
type EventDeadLetter struct {
	ID          int64
	EventKind   string // original v2 event kind (diagnostics only)
	EntityKind  string // task|turn|step|team_stage|team_run|member_session|plan_board|plan_step|graph_stage|graph_node
	EntityOp    string // upsert|complete_task_terminal
	EntityID    string // dedup key (matches the in-memory ring semantics)
	SessionID   string
	PayloadJSON string
	Attempts    int
	LastError   string
	State       string
	FailedAt    time.Time
}

// EventDeadLetterRepo is the durable store port for v2 event dead-letters.
// Implementations must be goroutine-safe; SaveEventDeadLetter must be
// idempotent per (EntityKind, EntityID) among pending rows.
//
// Stability:evolving
type EventDeadLetterRepo interface {
	SaveEventDeadLetter(ctx context.Context, rec EventDeadLetter) error
	ListPendingEventDeadLetters(ctx context.Context, limit int) ([]EventDeadLetter, error)
	MarkEventDeadLetterReplayed(ctx context.Context, id int64) error
	MarkEventDeadLetterAbandoned(ctx context.Context, id int64, reason string) error
	IncrementEventDeadLetterAttempt(ctx context.Context, id int64, lastErr string) error
}
