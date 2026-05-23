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
	// MaxChannelTurnJobListLimit caps admin ListChannelTurnJobs page size.
	MaxChannelTurnJobListLimit = 200
	// ChannelAsyncJobWatchMax is the in-process async completion watch ceiling (CC-F-01 interim; durable worker pending).
	ChannelAsyncJobWatchMax = 24 * time.Hour
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

// ChannelTurnJobRepo persists channel turn jobs.
type ChannelTurnJobRepo interface {
	Create(ctx context.Context, job ChannelTurnJob) (string, error)
	UpdateStatus(ctx context.Context, id, status, errMsg, previewMsgID, contentPreview string) error
	UpdateAsyncTarget(ctx context.Context, id, targetType, targetID string) error
	GetByIdempotency(ctx context.Context, channelID, idempotencyKey string) (ChannelTurnJob, error)
  ListByChannel(ctx context.Context, channelID string, limit int) ([]ChannelTurnJob, error)
	ListFiltered(ctx context.Context, q ChannelTurnJobListQuery) ([]ChannelTurnJob, error)
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
	case ChannelTurnJobStatusRunning, ChannelTurnJobStatusQueued, ChannelTurnJobStatusCompleted,
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
		ChannelTurnJobStatusTimeout, ChannelTurnJobStatusCancelled, ChannelTurnJobStatusQueued:
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
