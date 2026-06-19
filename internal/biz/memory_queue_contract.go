package biz

import (
	"context"
	"time"
)

// memory_queue_contract.go — H-01: shared memory queue contracts.
//
// MemoryDeadLetterSink and related types are domain contracts. They belong in biz
// so that both the memory adapter (internal/memory/trpc) and the persistence layer
// (internal/data) can import them without creating a data→memory/trpc dependency.

// MemoryJobPriority classifies the urgency of an AutoMemory enqueue request.
type MemoryJobPriority int

const (
	MemoryJobPriorityHigh   MemoryJobPriority = 0 // feedback-triggered, preference extraction
	MemoryJobPriorityNormal MemoryJobPriority = 1 // runner turn completion (default)
	MemoryJobPriorityLow    MemoryJobPriority = 2 // episode backfill, migration reconcile
)

// MemoryDeadLetterReason categorises why a job entered the dead-letter store.
type MemoryDeadLetterReason string

const (
	MemoryDeadLetterReasonQueueFull           MemoryDeadLetterReason = "queue_full"
	MemoryDeadLetterReasonQuotaExceeded       MemoryDeadLetterReason = "quota_exceeded"
	MemoryDeadLetterReasonRetryExhausted      MemoryDeadLetterReason = "retry_exhausted"
	MemoryDeadLetterReasonPendingQueueFailure MemoryDeadLetterReason = "pending_queue_failure"
)

// MemoryDeadLetterRequest carries the minimal fields needed to persist a dead-letter entry.
type MemoryDeadLetterRequest struct {
	SessionID         string
	AppName           string
	UserID            string
	FeedbackMessageID string
	FeedbackRating    string
	FeedbackComment   string
	Priority          MemoryJobPriority
	TenantID          string
}

// MemoryDeadLetterSink receives jobs that cannot be processed (MEM-OPT-03).
// Implementations must be goroutine-safe.
type MemoryDeadLetterSink interface {
	WriteMemoryDeadLetter(r MemoryDeadLetterRequest, reason MemoryDeadLetterReason, lastErr string)
}

// MemoryDeadLetterEntry holds the fields returned by dead-letter admin queries.
type MemoryDeadLetterEntry struct {
	ID         int64
	EnqueuedAt time.Time
	FailedAt   time.Time
	SessionID  string
	AppName    string
	DropReason string
	Priority   int
	Attempts   int
	State      string
	LastError  string
}

// MemoryDeadLetterAdminRepo is the biz-level port for dead-letter admin operations.
// Service layer depends on this interface instead of the concrete data repo,
// keeping the dependency direction: service → biz (not service → data).
type MemoryDeadLetterAdminRepo interface {
	ListDeadLetters(ctx context.Context, state string, limit int) ([]MemoryDeadLetterEntry, error)
	GetDeadLetter(ctx context.Context, id int64) (MemoryDeadLetterEntry, error)
	MarkDeadLetterReplayed(ctx context.Context, id int64) error
	MarkDeadLetterAbandoned(ctx context.Context, id int64, reason string) error
	CountDeadLettersByState(ctx context.Context) (pending, replayed, abandoned int64, err error)
	ReplayDeadLetterIntoQueue(ctx context.Context, id int64, enqueue func(sessionID, appName, userID, feedbackMsgID string, priority MemoryJobPriority)) error
}
