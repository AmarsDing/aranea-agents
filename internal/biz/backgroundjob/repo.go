package backgroundjob

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ErrNotFound is returned by Repo.Get when the job does not exist.
var ErrNotFound = kerrors.NotFound("BACKGROUND_JOB", "background job not found")

// Repo is the port interface for persisting and querying BackgroundJobs.
// Implementations live in internal/data/.
//
// All write methods must be safe for concurrent use and transactionally atomic.
type Repo interface {
	// Create enqueues a new job. Returns the persisted Job with generated ID.
	Create(ctx context.Context, req CreateRequest) (*Job, error)

	// Get retrieves a job by ID. Returns ErrNotFound when the job does not exist.
	Get(ctx context.Context, id string) (*Job, error)

	// List returns jobs matching the filter, ordered by priority ASC, created_at ASC.
	List(ctx context.Context, f ListFilter) ([]*Job, error)

	// TryClaim atomically claims the next claimable job of the given kinds for workerID.
	// Claimable means: status='queued', scheduled_at <= now, attempts < max_attempts,
	// and (parent_job_id is empty OR parent job has succeeded).
	// Returns nil, nil when no job is available.
	TryClaim(ctx context.Context, workerID string, kinds []string) (*Job, error)

	// MarkRunning transitions the job to status='claimed'. No-op if already in a
	// terminal state. Sets claimed_at and increments attempts.
	MarkRunning(ctx context.Context, id, workerID string) error

	// MarkSucceeded transitions the job to status='succeeded' and sets finished_at.
	MarkSucceeded(ctx context.Context, id string) error

	// MarkFailed transitions the job to status='failed', sets last_error, and
	// sets finished_at. If attempts < max_attempts the caller should re-enqueue.
	MarkFailed(ctx context.Context, id, errMsg string) error

	// Cancel transitions the job to status='cancelled'. No-op for terminal jobs.
	Cancel(ctx context.Context, id string) error

	// CancelByOwner cancels all non-terminal jobs for the given owner.
	CancelByOwner(ctx context.Context, ownerType OwnerType, ownerID string) (int, error)

	// DeleteTerminated removes finished/cancelled/failed jobs older than the given age.
	// Used for periodic cleanup to bound table growth.
	DeleteTerminated(ctx context.Context, f ListFilter) (int, error)
}
