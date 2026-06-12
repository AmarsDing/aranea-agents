package biz

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// Compile-time assertion: EventStoreUsecase implements EventStoreExistChecker
// (defined in internal/event/wal.go) for WAL recovery idempotency.
var _ interface {
	Exists(ctx context.Context, eventID string) bool
} = (*EventStoreUsecase)(nil)

const defaultEventStoreTTLDays = 7

// EventStoreRecord is a persisted Envelope snapshot.
type EventStoreRecord struct {
	ID           string
	SessionID    string
	Type         string
	Author       string
	Channel      string
	EnvelopeJSON string
	CreatedAt    time.Time
}

// EventStoreQuery filters stored envelopes.
type EventStoreQuery struct {
	SessionID string
	Since     time.Time
	Until     time.Time
	Type      string
	Limit     int
	Offset    int
}

// EventStoreListResult is a paginated list of stored envelopes.
type EventStoreListResult struct {
	Items []EventStoreRecord
	Total int
}

// EventStoreRepo persists Envelope snapshots.
// Stability:evolving
type EventStoreRepo interface {
	Insert(ctx context.Context, rec EventStoreRecord) error
	List(ctx context.Context, q EventStoreQuery) (EventStoreListResult, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	ExistsByID(ctx context.Context, id string) bool
}

// EventStoreUsecase manages event persistence and replay queries.
type EventStoreUsecase struct {
	repo EventStoreRepo
}

func NewEventStoreUsecase(repo EventStoreRepo) *EventStoreUsecase {
	if repo == nil {
		return nil
	}
	return &EventStoreUsecase{repo: repo}
}

func (uc *EventStoreUsecase) SaveRecord(ctx context.Context, rec EventStoreRecord) error {
	if uc == nil || uc.repo == nil {
		return nil
	}
	if strings.TrimSpace(rec.ID) == "" {
		return apierror.BadRequest("EVENT_STORE", "id is required")
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	return uc.repo.Insert(ctx, rec)
}

func (uc *EventStoreUsecase) List(ctx context.Context, q EventStoreQuery) (EventStoreListResult, error) {
	if uc == nil || uc.repo == nil {
		return EventStoreListResult{}, apierror.Internal("EVENT_STORE", "event store not configured")
	}
	if strings.TrimSpace(q.SessionID) == "" {
		return EventStoreListResult{}, apierror.BadRequest("EVENT_STORE", "session_id is required")
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 500 {
		q.Limit = 500
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return uc.repo.List(ctx, q)
}

func (uc *EventStoreUsecase) PurgeExpired(ctx context.Context) (int64, error) {
	if uc == nil || uc.repo == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-eventStoreTTL())
	return uc.repo.DeleteOlderThan(ctx, cutoff)
}

// Exists checks if an event with the given ID already exists in the EventStore.
// This implements the EventStoreExistChecker interface for WAL recovery idempotency.
func (uc *EventStoreUsecase) Exists(ctx context.Context, eventID string) bool {
	if uc == nil || uc.repo == nil {
		return false
	}
	return uc.repo.ExistsByID(ctx, eventID)
}

func eventStoreTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("EVENT_STORE_TTL_DAYS"))
	if raw == "" {
		return time.Duration(defaultEventStoreTTLDays) * 24 * time.Hour
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		return time.Duration(defaultEventStoreTTLDays) * 24 * time.Hour
	}
	return time.Duration(days) * 24 * time.Hour
}
