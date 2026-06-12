package biz

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ChannelTurnJobStatusAccepted    = "accepted"
	ChannelTurnJobStatusRunning     = "running"
	ChannelTurnJobStatusQueued      = "queued"
	ChannelTurnJobStatusCompleted   = "completed"
	ChannelTurnJobStatusFailed      = "failed"
	ChannelTurnJobStatusTimeout     = "timeout"
	ChannelTurnJobStatusCancelled   = "cancelled"
	ChannelTurnJobStatusAsyncQueued = "async_queued"
	// ChannelAsyncJobWatchMax is the in-process async completion watch ceiling (CC-F-01 interim; durable worker pending).
	ChannelAsyncJobWatchMax = 24 * time.Hour
)

// ChannelTurnJob transition events.
const (
	JobEventStart       = "start"       // accepted -> running
	JobEventQueue       = "queue"       // accepted -> queued
	JobEventDequeue     = "dequeue"     // queued -> running
	JobEventComplete    = "complete"    // running -> completed
	JobEventFail        = "fail"        // running -> failed
	JobEventTimeout     = "timeout"     // running -> timeout
	JobEventCancel      = "cancel"      // accepted/running/queued -> cancelled
	JobEventAsyncQueue  = "async_queue" // accepted/running -> async_queued
	JobEventAsyncStart  = "async_start" // async_queued -> running
	JobEventAsyncFail   = "async_fail"  // async_queued -> failed
	JobEventAsyncCancel = "async_cancel" // async_queued -> cancelled
)

// ChannelTurnJob tracks a single Channel inbound turn lifecycle (LT-07).
type ChannelTurnJob struct {
	ID               string
	ChannelID        string
	SessionID        string
	PeerID           string
	PeerKey          string
	IdempotencyKey   string
	Status           string
	PreviewMessageID string
	ContentPreview   string
	AsyncTargetType  string
	AsyncTargetID    string
	ErrorMessage     string
	StartedAt        string
	FinishedAt       string
	CreatedAt        string
	UpdatedAt        string
	// AgentID / GraphID are populated by ListFiltered joins only (not persisted on the row).
	AgentID string
	GraphID string
}

// ChannelTurnJobReader provides read-only access to channel turn jobs.
// Stability:evolving
type ChannelTurnJobReader interface {
	GetByID(ctx context.Context, id string) (ChannelTurnJob, error)
	GetByIdempotency(ctx context.Context, channelID, idempotencyKey string) (ChannelTurnJob, error)
	ListByChannel(ctx context.Context, channelID string, limit int) ([]ChannelTurnJob, error)
	ListFiltered(ctx context.Context, q ChannelTurnJobListQuery) ([]ChannelTurnJob, error)
	ListActiveBySession(ctx context.Context, channelID, sessionID string) ([]ChannelTurnJob, error)
}

// ChannelTurnJobRepo persists channel turn jobs.
// Embeds ChannelTurnJobReader for convenience.
// Stability:evolving
type ChannelTurnJobRepo interface {
	ChannelTurnJobReader
	Create(ctx context.Context, job ChannelTurnJob) (string, error)
	UpdateStatus(ctx context.Context, id, status, errMsg, previewMsgID, contentPreview string) error
	UpdateAsyncTarget(ctx context.Context, id, targetType, targetID string) error
}

// ChannelTurnJobListQuery filters jobs for chat background panel (M55 CC-D-01).
type ChannelTurnJobListQuery struct {
	SessionID string
	AgentID   string
	Status    string
	Limit     int
}

func NewChannelTurnJobID() string {
	return uuid.NewString()
}

func ChannelTurnJobNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func NormalizeChannelTurnJobListLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > MaxChannelTurnJobListLimit {
		return MaxChannelTurnJobListLimit
	}
	return limit
}

func NormalizeChannelTurnJobStatus(status string) string {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case ChannelTurnJobStatusAccepted, ChannelTurnJobStatusRunning, ChannelTurnJobStatusQueued, ChannelTurnJobStatusCompleted,
		ChannelTurnJobStatusFailed, ChannelTurnJobStatusTimeout, ChannelTurnJobStatusCancelled,
		ChannelTurnJobStatusAsyncQueued:
		return strings.TrimSpace(strings.ToLower(status))
	default:
		return ChannelTurnJobStatusAccepted
	}
}

func IsChannelTurnJobTerminalStatus(status string) bool {
	switch NormalizeChannelTurnJobStatus(status) {
	case ChannelTurnJobStatusCompleted, ChannelTurnJobStatusFailed,
		ChannelTurnJobStatusTimeout, ChannelTurnJobStatusCancelled:
		return true
	default:
		return false
	}
}

// IsChannelTurnJobIdempotentLockedStatus reports statuses that must not be overwritten by Create upsert.
func IsChannelTurnJobIdempotentLockedStatus(status string) bool {
	switch NormalizeChannelTurnJobStatus(status) {
	case ChannelTurnJobStatusCompleted, ChannelTurnJobStatusFailed,
		ChannelTurnJobStatusTimeout, ChannelTurnJobStatusCancelled, ChannelTurnJobStatusQueued,
		ChannelTurnJobStatusAsyncQueued:
		return true
	default:
		return false
	}
}

// ChannelTurnJobIdempotentLockedStatuses returns the list of statuses that must not be
// overwritten by the Create upsert. Exported so the data layer can build SQL IN clauses
// without hardcoding the status list, keeping it in sync with the biz layer.
var ChannelTurnJobIdempotentLockedStatuses = []string{
	ChannelTurnJobStatusCompleted,
	ChannelTurnJobStatusFailed,
	ChannelTurnJobStatusTimeout,
	ChannelTurnJobStatusCancelled,
	ChannelTurnJobStatusQueued,
	ChannelTurnJobStatusAsyncQueued,
}

// ChannelTurnJobTerminalStatuses returns the list of terminal statuses.
// Exported for data layer SQL queries that need to filter out completed jobs.
var ChannelTurnJobTerminalStatuses = []string{
	ChannelTurnJobStatusCompleted,
	ChannelTurnJobStatusFailed,
	ChannelTurnJobStatusTimeout,
	ChannelTurnJobStatusCancelled,
}

// ChannelTurnJobStartStatuses are statuses that trigger started_at timestamp.
// Exported for data layer SQL CASE expressions.
var ChannelTurnJobStartStatuses = []string{
	ChannelTurnJobStatusRunning,
	ChannelTurnJobStatusAsyncQueued,
}

// ChannelTurnJobFinishStatuses are statuses that trigger finished_at timestamp.
// Includes 'queued' because a cancelled-while-queued job also finishes.
// Exported for data layer SQL CASE expressions.
var ChannelTurnJobFinishStatuses = []string{
	ChannelTurnJobStatusCompleted,
	ChannelTurnJobStatusFailed,
	ChannelTurnJobStatusTimeout,
	ChannelTurnJobStatusCancelled,
	ChannelTurnJobStatusQueued,
}
