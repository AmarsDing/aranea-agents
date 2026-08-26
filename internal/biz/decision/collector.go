package decision

import (
	"context"
	"time"
)

// Collector is the single intake for all decision sources. Emit must never
// block the caller's turn chain (NFR-80-01): it validates, normalizes, and
// hands the record to the async outbox; on overflow it drops and warns.
//
// Stability: Evolving
type Collector interface {
	Emit(ctx context.Context, rec Record)
}

// Lifecycle is the production collector with its background-worker controls.
// newApp mounts it so the outbox worker runs for the process lifetime
// (Start after server boot, Stop during shutdown).
type Lifecycle interface {
	Collector
	Start(ctx context.Context)
	Stop()
}

// noopCollector is the zero-value-safe implementation for tests, CLI mode,
// and deployments where decision recording is disabled.
type noopCollector struct{}

// NewNoopCollector returns a Collector that silently discards everything.
func NewNoopCollector() Collector { return noopCollector{} }

func (noopCollector) Emit(context.Context, Record) {}

// OutboxRow is one persisted retry-queue entry (decision_record_outbox).
// Rows appear only when the inline flush path failed repeatedly (or the
// process is recovering after a crash); the steady-state table is near-empty.
type OutboxRow struct {
	ID          int64
	DecisionKey string
	Payload     []byte // marshaled Record JSON
	Status      string
	Attempts    int
	LastError   string
	CreatedAt   time.Time
	PublishedAt *time.Time
}

// Outbox status values.
const (
	OutboxStatusPending   = "pending"
	OutboxStatusPublished = "published"
)

// Repo is the persistence contract the outbox worker needs (implemented by
// internal/data). Insert methods are idempotent on decision_key so worker
// retries and replay never duplicate rows.
type Repo interface {
	// InsertRecords batch-inserts into decision_records, ignoring rows whose
	// decision_key already exists.
	InsertRecords(ctx context.Context, recs []Record) error
	// EnqueueOutbox persists records into the retry queue (decision_record_outbox)
	// after inline flush failures; idempotent on decision_key.
	EnqueueOutbox(ctx context.Context, recs []Record) error
	// ListPendingOutbox returns up to limit pending rows oldest-first.
	ListPendingOutbox(ctx context.Context, limit int) ([]OutboxRow, error)
	// MarkOutboxPublished marks rows published after successful replay.
	MarkOutboxPublished(ctx context.Context, ids []int64, publishedAt time.Time) error
	// MarkOutboxAttempt records a failed replay attempt for observability.
	MarkOutboxAttempt(ctx context.Context, id int64, lastError string) error
}
