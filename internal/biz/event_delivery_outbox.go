package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrEventOutboxSeqConflict is returned by EventDeliveryOutboxRepo.Insert when
// the ON CONFLICT DO NOTHING insert matched zero rows under
// UNIQUE(session_id, seq): another writer already owns that delivery seq.
// Callers holding the shared SeqAssigner should re-derive the seq and retry
// (FR-9); a same-event_id pre-existing row is idempotent success instead.
var ErrEventOutboxSeqConflict = errors.New("event outbox seq conflict")

// EventDeliveryOutboxRow is one durable critical-delivery record (B-06).
// Payload is the marshaled WS v2_event envelope bytes (ready to push on replay).
type EventDeliveryOutboxRow struct {
	ID          string
	SessionID   string
	Seq         int64
	EventID     string
	Kind        string
	EntityID    string
	Payload     []byte
	PublishedAt *time.Time
	CreatedAt   time.Time
}

// EventDeliveryOutboxRepo persists critical v2 events for reconnect replay.
// Stability: Evolving
type EventDeliveryOutboxRepo interface {
	Insert(ctx context.Context, row EventDeliveryOutboxRow) error
	MarkPublished(ctx context.Context, id string, publishedAt time.Time) error
	// MarkPublishedBatch marks the given rows published in a single UPDATE
	// (WHERE id IN (...)). It is the WS compensation-sweeper path (P1-3):
	// one RTT per swept batch instead of N serial UPDATEs sharing one query
	// budget with the broadcast loop, so marking never starves under backlog.
	// Empty/whitespace-only id lists are a no-op. The delivery contract stays
	// at-least-once: on batch failure every listed row stays pending and is
	// re-broadcast next tick (clients dedupe by event_id / delivery seq).
	MarkPublishedBatch(ctx context.Context, ids []string, publishedAt time.Time) error
	// ListAfter returns rows for sessionID with seq strictly greater than the
	// cursor. When afterEventID is non-empty, the cursor seq is resolved from
	// that event_id (unknown id → empty result). When afterEventID is empty and
	// afterSeq > 0, afterSeq is used directly. limit <= 0 defaults to 100.
	ListAfter(ctx context.Context, sessionID, afterEventID string, afterSeq int64, limit int) ([]EventDeliveryOutboxRow, error)
	// ListPending returns up to limit rows whose live publish path never
	// completed (published_at IS NULL) created strictly before cutoff, oldest
	// first. Used by the WS outbox compensation sweeper (P1-3) to re-broadcast
	// frames whose fan-out dropped or whose MarkPublished failed. limit <= 0
	// defaults to 200; zero cutoff means "no age filter".
	ListPending(ctx context.Context, cutoff time.Time, limit int) ([]EventDeliveryOutboxRow, error)
}

// EventOutboxSeqAllocatingInserter allocates the delivery seq and inserts the
// outbox row in ONE database transaction (D7-1, P0-6 阶段 2,
// ARANEA_SEQ_ALLOCATOR=pg). The allocation runs as a session_seq_counters row
// UPSERT ... RETURNING inside the same tx as the outbox INSERT: concurrent
// replicas serialize on the counter row lock, so allocation order == commit
// order == visibility order, and a last_event_id strict-cursor replay can
// never observe seq N+1 without seq N (the dual-replica window where two
// process-local counters inverted that order).
//
// The row is produced by the build callback INSIDE the tx because the payload
// embeds event_id which embeds the freshly allocated seq — the caller cannot
// marshal before knowing it. A build error rolls the tx back (no counter
// increment consumed, no row).
//
// Kept separate from EventDeliveryOutboxRepo so streaming-side fakes needn't
// grow (same precedent as EventOutboxWatermarkReader): the v2 Sequencer
// resolves it via type assertion from the repo value, gated by the flag —
// off mode never touches this port (byte-level legacy behaviour).
// Stability: Internal
type EventOutboxSeqAllocatingInserter interface {
	// InsertAllocatingSeq allocates the next per-session delivery seq,
	// builds the row via build(seq), and inserts it — all in one tx.
	// sessionID empty or build nil = no-op nil. On INSERT (session_id, seq)
	// conflict (mixed rolling-version window with a legacy Go-counter
	// replica) the implementation repair-bumps the counter past the durable
	// watermark and retries a bounded number of times; a same-event_id
	// conflict is idempotent success (mirrors Insert semantics).
	InsertAllocatingSeq(ctx context.Context, sessionID string, build func(seq int64) (EventDeliveryOutboxRow, error)) error
}

// EventOutboxClaimer atomically claims pending outbox rows for one sweeper
// worker (D7-2, ARANEA_OUTBOX_SWEEP_CLAIM=on): SELECT ... FOR UPDATE SKIP
// LOCKED inside a CTE, then stamps claimed_by/claimed_at in the same
// statement. Multi-replica sweepers no longer double-broadcast the same
// backlog rows; lease-expired claims (crashed worker) are reclaimable.
//
// Kept separate from EventDeliveryOutboxRepo so streaming-side fakes needn't
// grow (EventOutboxWatermarkReader precedent); the WS sweeper resolves it via
// type assertion from the repo value, gated by the flag — off mode keeps the
// legacy ListPending scan byte-identical.
// Stability: Internal
type EventOutboxClaimer interface {
	// ClaimPendingOutboxDeliveries claims up to limit unpublished rows
	// created strictly before cutoff whose claim is free (claimed_by='') or
	// lease-expired (claimed_at < claimLeaseDeadline), oldest first, and
	// returns them with this worker's claim stamped. Rows claimed by another
	// live worker are skipped (SKIP LOCKED), never waited on. Empty
	// workerID = misconfiguration, returns nil (fail-closed no claim).
	ClaimPendingOutboxDeliveries(ctx context.Context, workerID string, cutoff, claimLeaseDeadline time.Time, limit int) ([]EventDeliveryOutboxRow, error)
}

// EventOutboxWatermarkReader exposes the per-session persisted delivery-seq
// high-watermark (FR-9). It is kept separate from EventDeliveryOutboxRepo so
// streaming-side fakes needn't grow: the seq-restore fold in the chat
// orchestrator resolves it via type assertion from the repo value.
// Stability: Internal
type EventOutboxWatermarkReader interface {
	// MaxSeqBySession returns COALESCE(MAX(seq), 0) for the session — the
	// durable lower bound the shared SeqAssigner must stay ahead of after
	// restart (0 = no rows, fresh counter).
	MaxSeqBySession(ctx context.Context, sessionID string) (int64, error)
}

// deliverySeqCarrier carries the Sequencer-assigned outbox delivery sequence
// (B-06). It is delivery metadata only: entity Seq fields (Task.Seq, Turn.Seq,
// Step.Seq, ...) stay domain data in their own counter spaces and are never
// reused as delivery cursors. Delivery seqs are allocated from the shared
// per-session counter (SeqAssigner.NextSeq) by the v2 Sequencer's single
// publish worker, so outbox rows are strictly monotonic in insertion order per
// session and ListAfter(seq > cursor) replay can neither skip nor duplicate
// terminal frames when a session mixes turn-counter and shared-counter entity
// seqs (pre-2026-10-18 this mixed the two spaces and violated
// UNIQUE(session_id, seq) on event_delivery_outbox).
type deliverySeqCarrier struct {
	deliverySeq int64
}

// DeliverySeqValue returns the assigned outbox delivery sequence (0 = not yet
// assigned).
func (c *deliverySeqCarrier) DeliverySeqValue() int64 { return c.deliverySeq }

// SetDeliverySeqValue records the outbox delivery sequence assigned by the
// Sequencer so live WS frames carry the same event_id as the durable outbox row.
func (c *deliverySeqCarrier) SetDeliverySeqValue(seq int64) { c.deliverySeq = seq }

// DeliverySeqEvent is implemented by critical-delivery events (B-06) that carry
// a Sequencer-assigned outbox delivery sequence. The v2 Sequencer assigns it in
// insertCriticalOutbox; the WS v2 subscriber reads it to attach the outbox
// cursor event_id to live frames, keeping live and replayed frames identical.
type DeliverySeqEvent interface {
	DeliverySeqValue() int64
	SetDeliverySeqValue(seq int64)
}

// DeliveryEventID builds a stable cursor id for outbox + WS last_event_id.
// Format: v2:{session}:{seq}:{kind}:{entity}
func DeliveryEventID(e Event, seq int64) string {
	if e == nil {
		return ""
	}
	sessionID := strings.TrimSpace(e.SpiritSessionID())
	kind := string(e.EventKind())
	entityID := strings.TrimSpace(e.EntityID())
	return fmt.Sprintf("v2:%s:%d:%s:%s", sessionID, seq, kind, entityID)
}
