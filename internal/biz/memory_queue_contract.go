package biz

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
	MemoryDeadLetterReasonQueueFull     MemoryDeadLetterReason = "queue_full"
	MemoryDeadLetterReasonQuotaExceeded MemoryDeadLetterReason = "quota_exceeded"
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
