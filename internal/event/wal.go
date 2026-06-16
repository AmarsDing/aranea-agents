package event

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	frameworkwal "trpc.group/trpc-go/trpc-agent-go/event/wal"
)

// EventWAL provides Write-Before-Publish-Fanout (WBPF) for Critical events.
// Delegates to the framework wal.WAL[Envelope] implementation.
//
// Critical events are persisted to SQLite BEFORE being published to the Bus,
// ensuring no data loss on process crash. On restart, unpublished entries
// are recovered and republished.
//
// Stability:evolving
type EventWAL struct {
	inner   *frameworkwal.WAL[Envelope]
	storage *sqliteWALStorage // kept for direct DB access in tests
	db      *sql.DB           // kept for ProvideEventWAL nil check
	lg      loggateway.Logger
}

// walLogger adapts loggateway.Logger to the framework wal.Logger interface.
type walLogger struct {
	lg loggateway.Logger
}

func (l *walLogger) Warn(msg string, kv ...any) {
	if l.lg != nil {
		l.lg.Warn(msg, toLoggatewayFields(kv)...)
	}
}

func (l *walLogger) Info(msg string, kv ...any) {
	if l.lg != nil {
		l.lg.Info(msg, toLoggatewayFields(kv)...)
	}
}

// toLoggatewayFields converts alternating key-value pairs from framework
// wal.Logger (any...any) to loggateway field functions.
// Framework passes pairs like ("id", "abc", "error", err).
// We convert known keys to typed loggateway fields, unknown to Str.
func toLoggatewayFields(kv []any) []loggateway.Field {
	fields := make([]loggateway.Field, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		val := kv[i+1]
		switch key {
		case "error":
			if err, ok := val.(error); ok {
				fields = append(fields, loggateway.Err(err))
			} else {
				fields = append(fields, loggateway.Str(key, fmt.Sprintf("%v", val)))
			}
		case "id":
			fields = append(fields, loggateway.Str("event_id", fmt.Sprintf("%v", val)))
		case "count":
			if n, ok := val.(int); ok {
				fields = append(fields, loggateway.Int(key, n))
			} else {
				fields = append(fields, loggateway.Str(key, fmt.Sprintf("%v", val)))
			}
		default:
			fields = append(fields, loggateway.Str(key, fmt.Sprintf("%v", val)))
		}
	}
	return fields
}

// NewEventWAL creates a new WAL backed by the given SQLite database.
// The database must be opened with WAL journal mode for best crash safety.
func NewEventWAL(db *sql.DB, lg loggateway.Logger) (*EventWAL, error) {
	storage, err := newSQLiteWALStorage(context.Background(), db, lg)
	if err != nil {
		return nil, err
	}

	isCritical := func(env Envelope) bool {
		return contract.IsCriticalWBPFType(env.Type)
	}

	fwLogger := &walLogger{lg: lg}
	inner, err := frameworkwal.New[Envelope](storage, isCritical, fwLogger)
	if err != nil {
		return nil, fmt.Errorf("event_wal: create framework WAL: %w", err)
	}

	return &EventWAL{
		inner:   inner,
		storage: storage,
		db:      db,
		lg:      lg,
	}, nil
}

// WriteBeforePublish persists a Critical event to WAL before publishing.
// For non-Critical events, it calls publish() directly (no WAL overhead).
// The publish callback is only called after successful WAL write.
func (w *EventWAL) WriteBeforePublish(ctx context.Context, env contract.Envelope, publish func()) error {
	return w.inner.WriteBeforePublish(ctx, env, env.ID, publish)
}

// Recover replays unpublished WAL entries after process restart.
// Must be called AFTER Bus and all subscribers are ready.
// Uses idempotent publish — if the event was already processed by a subscriber,
// the subscriber must handle duplicates gracefully.
func (w *EventWAL) Recover(ctx context.Context, bus contract.Bus, store EventStoreExistChecker) {
	if w == nil || w.inner == nil {
		return
	}

	// Adapt the bus.Publish to the framework's publish function signature
	publishFunc := func(env Envelope) {
		bus.Publish(ctx, env)
	}

	// Adapt the ExistChecker to the framework's interface
	var checker frameworkwal.ExistChecker
	if store != nil {
		checker = &existCheckerAdapter{inner: store}
	}

	w.inner.Recover(ctx, publishFunc, checker)
}

// PurgePublished removes published WAL entries older than the given TTL.
func (w *EventWAL) PurgePublished(ctx context.Context, ttl time.Duration) (int64, error) {
	if w == nil || w.inner == nil {
		return 0, nil
	}
	return w.inner.PurgePublished(ctx, ttl)
}

// EventStoreExistChecker checks if an event already exists in the EventStore.
// Used for idempotent recovery — if the event was already persisted by the
// async persistHandler before crash, we skip re-publishing.
// Stability:evolving
type EventStoreExistChecker interface {
	Exists(ctx context.Context, eventID string) bool
}

// existCheckerAdapter adapts project EventStoreExistChecker to framework wal.ExistChecker.
type existCheckerAdapter struct {
	inner EventStoreExistChecker
}

func (a *existCheckerAdapter) Exists(ctx context.Context, eventID string) bool {
	return a.inner.Exists(ctx, eventID)
}
