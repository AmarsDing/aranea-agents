package biz

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
