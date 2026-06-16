// Package wal provides Write-Before-Publish-Fanout (WBPF) for critical events.
//
// Critical events are persisted to a WAL (Write-Ahead Log) BEFORE being
// published to the Bus, ensuring no data loss on process crash. On restart,
// unpublished entries are recovered and republished.
//
// The WAL uses a storage abstraction (Storage interface) so it can work with
// any backend (SQLite, PostgreSQL, in-memory for testing, etc.).
package wal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Storage is the persistence backend for the WAL.
// Implementations must be safe for concurrent use.
type Storage interface {
	// Insert stores a new WAL entry. Must be idempotent (INSERT OR IGNORE semantics).
	Insert(ctx context.Context, entry Entry) error

	// MarkPublished marks a WAL entry as published.
	MarkPublished(ctx context.Context, id string, publishedAt time.Time) error

	// ListUnpublished returns all unpublished entries in creation order.
	ListUnpublished(ctx context.Context) ([]Entry, error)

	// PurgePublished removes published entries older than the given cutoff.
	PurgePublished(ctx context.Context, cutoff time.Time) (int64, error)

	// Close releases any resources held by the storage.
	Close() error
}

// Entry represents a single WAL record.
type Entry struct {
	ID          string    `json:"id"`
	EventJSON   string    `json:"event_json"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	Published   bool      `json:"published"`
}

// ExistChecker checks if an event already exists in the downstream store.
// Used for idempotent recovery — if the event was already persisted by the
// async consumer before crash, we skip re-publishing.
type ExistChecker interface {
	Exists(ctx context.Context, eventID string) bool
}

// IsCriticalFunc determines if an event type requires WBPF.
// This is a function type so the WAL can work with any event type system.
type IsCriticalFunc[T any] func(event T) bool

// SerializeFunc serializes an event to JSON bytes.
type SerializeFunc[T any] func(event T) ([]byte, error)

// DeserializeFunc deserializes JSON bytes to an event.
type DeserializeFunc[T any] func(data []byte) (T, error)

// Logger is a minimal logging interface for the WAL.
type Logger interface {
	Warn(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
}

// WAL provides Write-Before-Publish-Fanout (WBPF) for critical events.
type WAL[T any] struct {
	storage     Storage
	isCritical  IsCriticalFunc[T]
	serialize   SerializeFunc[T]
	deserialize DeserializeFunc[T]
	lg          Logger
}

// New creates a new WAL instance.
// The isCritical function determines which events require WBPF protection.
// If isCritical is nil, ALL events will be treated as critical (conservative default).
// serialize and deserialize can be overridden via options; defaults to JSON.
func New[T any](
	storage Storage,
	isCritical IsCriticalFunc[T],
	lg Logger,
	opts ...WALOption[T],
) (*WAL[T], error) {
	if storage == nil {
		return nil, fmt.Errorf("wal: storage is required")
	}
	w := &WAL[T]{
		storage:    storage,
		isCritical: isCritical,
		lg:         lg,
		serialize: func(event T) ([]byte, error) {
			return json.Marshal(event)
		},
		deserialize: func(data []byte) (T, error) {
			var event T
			if err := json.Unmarshal(data, &event); err != nil {
				return event, err
			}
			return event, nil
		},
	}
	for _, opt := range opts {
		opt(w)
	}
	return w, nil
}

// WALOption configures a WAL instance.
type WALOption[T any] func(*WAL[T])

// WithSerializer sets custom serialization functions.
// Must be called before any WriteBeforePublish call.
func WithSerializer[T any](serialize SerializeFunc[T], deserialize DeserializeFunc[T]) WALOption[T] {
	return func(w *WAL[T]) {
		w.serialize = serialize
		w.deserialize = deserialize
	}
}

// WriteBeforePublish persists a critical event to WAL before publishing.
// For non-critical events, it calls publish() directly (no WAL overhead).
// The publish callback is only called after successful WAL write.
func (w *WAL[T]) WriteBeforePublish(ctx context.Context, event T, eventID string, publish func()) error {
	if w.isCritical != nil && !w.isCritical(event) {
		publish()
		return nil
	}

	// 1. Serialize event
	raw, err := w.serialize(event)
	if err != nil {
		return fmt.Errorf("wal: marshal event: %w", err)
	}

	// 2. Write to WAL (idempotent INSERT)
	entry := Entry{
		ID:        eventID,
		EventJSON: string(raw),
		CreatedAt: time.Now().UTC(),
	}
	if err := w.storage.Insert(ctx, entry); err != nil {
		return fmt.Errorf("wal: insert: %w", err)
	}

	// 3. Publish to Bus
	publish()

	// 4. Mark as published (synchronous — avoids crash-time inconsistency)
	// If marking fails, the event was still published, but on restart it may
	// be republished. Return a non-fatal error so the caller can log it.
	if markErr := w.markPublished(ctx, eventID); markErr != nil {
		return fmt.Errorf("wal: event published but mark failed (may republish on restart): %w", markErr)
	}
	return nil
}

// markPublished marks a WAL entry as published.
// Returns the error from the storage layer so callers can decide how to handle it.
func (w *WAL[T]) markPublished(ctx context.Context, id string) error {
	if err := w.storage.MarkPublished(ctx, id, time.Now().UTC()); err != nil {
		if w.lg != nil {
			w.lg.Warn("wal: mark published failed", "id", id, "error", err)
		}
		return err
	}
	return nil
}

// Recover replays unpublished WAL entries after process restart.
// Must be called AFTER Bus and all subscribers are ready.
// Uses ExistChecker for idempotent recovery — if the event was already
// processed by a subscriber, the subscriber must handle duplicates gracefully.
func (w *WAL[T]) Recover(ctx context.Context, publishFunc func(event T), checker ExistChecker) {
	entries, err := w.storage.ListUnpublished(ctx)
	if err != nil {
		if w.lg != nil {
			w.lg.Warn("wal: recover query failed", "error", err)
		}
		return
	}

	recovered := 0
	for _, e := range entries {
		// Idempotency check: if downstream store already has this event, skip
		if checker != nil && checker.Exists(ctx, e.ID) {
			w.markPublished(ctx, e.ID)
			continue
		}

		event, err := w.deserialize([]byte(e.EventJSON))
		if err != nil {
			if w.lg != nil {
				w.lg.Warn("wal: recover unmarshal failed", "id", e.ID, "error", err)
			}
			continue
		}

		publishFunc(event)
		w.markPublished(ctx, e.ID)
		recovered++
	}

	if recovered > 0 && w.lg != nil {
		w.lg.Info("wal: recovered events", "count", recovered)
	}
}

// PurgePublished removes published WAL entries older than the given TTL.
func (w *WAL[T]) PurgePublished(ctx context.Context, ttl time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-ttl)
	return w.storage.PurgePublished(ctx, cutoff)
}

// Close releases the storage backend.
func (w *WAL[T]) Close() error {
	if w.storage != nil {
		return w.storage.Close()
	}
	return nil
}
