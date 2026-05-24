package biz

import "context"

// ChannelJobGateway is the narrow interface Channel ingress uses for async job
// lifecycle management. It unifies turn job tracking and graph execution under
// a single port so that Channel adapters never reach into GraphExecutor or
// ChannelTurnJobUsecase directly.
//
// Implementations live in internal/service. Wire binding happens there.
type ChannelJobGateway interface {
	// StartJob creates a tracked async job and begins execution.
	// Returns the job ID.
	StartJob(ctx context.Context, input ChannelJobInput) (jobID string, err error)

	// GetJob returns the current state of a tracked job.
	GetJob(ctx context.Context, jobID string) (ChannelJobState, error)

	// CancelJob cancels a running job.
	CancelJob(ctx context.Context, jobID string) error

	// NotifyJobDone is called by async completion watchers when a job finishes.
	NotifyJobDone(ctx context.Context, jobID string, result ChannelJobResult) error
}

// ChannelJobInput is the transport-neutral input for starting a channel async job.
type ChannelJobInput struct {
	ChannelID      string
	SessionID      string
	PeerID         string
	PeerKey        string
	IdempotencyKey string
	Content        string
	// AsyncTargetType selects the execution backend: "graph" or "turn".
	AsyncTargetType string
	// AsyncTargetID is the graph ID (when AsyncTargetType="graph") or agent key.
	AsyncTargetID string
}

// ChannelJobState is the read model for a channel job's current status.
type ChannelJobState struct {
	JobID           string
	Status          string
	PreviewMessageID string
	ContentPreview  string
	ErrorMessage    string
}

// ChannelJobResult carries the outcome of an async job completion.
type ChannelJobResult struct {
	Status         string
	PreviewMsgID   string
	ContentPreview string
	ErrorMessage   string
}
