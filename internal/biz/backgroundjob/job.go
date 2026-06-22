// Package backgroundjob defines the domain types and port interfaces for the
// Unified BackgroundJob subsystem (M56 BLO-5).
//
// A BackgroundJob is any async unit of work whose lifecycle (queued → running →
// succeeded/failed/cancelled) must be persisted and observable across restarts.
// Concrete job kinds (session_run_durable, channel_async, …) register their
// Runners via BackgroundJobRegistry.
package backgroundjob

import "time"

// Status represents the lifecycle state of a BackgroundJob.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusClaimed   Status = "claimed" // claimed by a worker, running
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// OwnerType categorises the entity that owns the job.
type OwnerType string

const (
	OwnerTypeSession OwnerType = "session"
	OwnerTypeChannel OwnerType = "channel"
	OwnerTypeSystem  OwnerType = "system"
)

// Priority bands. Lower value = higher urgency (real-time pool: < 50, background pool: >= 50).
const (
	PriorityRealtime   = 10
	PriorityNormal     = 50
	PriorityBackground = 90
)

// Job is the persisted record of one unit of async work.
type Job struct {
	ID          string
	Kind        string // registered runner kind, e.g. "session_run_durable"
	OwnerType   OwnerType
	OwnerID     string
	ParentJobID string // empty for root jobs; set for child jobs in a DAG
	Priority    int    // lower = higher urgency
	Status      Status
	Payload     []byte // opaque JSON understood by the registered Runner
	WorkerID    string // worker that claimed this job
	Attempts    int
	MaxAttempts int
	LastError   string
	ScheduledAt time.Time // earliest time the job may be claimed
	ClaimedAt   time.Time
	FinishedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsDone returns true when the job is in a terminal state.
func (j *Job) IsDone() bool {
	return j.Status == StatusSucceeded || j.Status == StatusFailed || j.Status == StatusCancelled
}

// CreateRequest contains the fields needed to enqueue a new job.
type CreateRequest struct {
	Kind        string
	OwnerType   OwnerType
	OwnerID     string
	ParentJobID string
	Priority    int    // defaults to PriorityNormal when zero
	MaxAttempts int    // defaults to 3 when zero
	Payload     []byte // JSON payload consumed by the registered Runner
	ScheduledAt time.Time
}

// ListFilter constrains a job listing query.
type ListFilter struct {
	OwnerType OwnerType
	OwnerID   string
	Kind      string
	Status    Status
	Limit     int
	Offset    int
}
