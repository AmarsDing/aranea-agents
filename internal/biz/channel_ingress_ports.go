package biz

import "context"

// IngressDeduplicator suppresses duplicate inbound messages within a TTL window.
// Implementations are typically in-process memory stores; the persistent layer
// is handled by ChannelInboundReceiptRepo.
type IngressDeduplicator interface {
	// ClaimMessage returns false when the message was already seen within TTL.
	ClaimMessage(channelID, messageID string) bool
	// TryAcquireInflight returns false when the dedup key is already being processed.
	TryAcquireInflight(dedupKey string) bool
	// ReleaseInflight marks a dedup key as no longer in-flight.
	ReleaseInflight(dedupKey string)
}

// PeerDebouncer merges rapid sequential messages from the same peer
// into a single batch before execution.
type PeerDebouncer interface {
	// Submit enqueues the event for debounced execution.
	Submit(ctx context.Context, channelID string, peerID string, peerKey string, text string, idempotencyKey string, run InboundProcessFunc)
}

// InboundProcessFunc is the callback invoked when a debounced batch is flushed.
type InboundProcessFunc func(ctx context.Context) error

// ConcurrencyGate limits the number of concurrent inbound turns per channel+peer.
type ConcurrencyGate interface {
	// TryAcquire attempts to acquire a concurrency slot.
	// Returns a release function on success, nil on failure.
	TryAcquire(channelID, peerID string, isGroup bool, limit int) (release func(), ok bool)
	// Close stops background cleanup goroutines.
	Close()
}

// TurnPreviewManager tracks active preview coordinators per session,
// ensuring only one preview is active at a time and previous ones are cancelled.
type TurnPreviewManager interface {
	// Register stores a preview cancel function for the session, cancelling any previous one.
	Register(sessionID string, cancel context.CancelFunc) context.CancelFunc
	// Unregister removes the preview coordinator for the session.
	Unregister(sessionID string)
	// SetRunID associates a run ID with the session's active preview.
	SetRunID(sessionID, runID string)
	// ActiveRunID returns the run ID for the session's active preview.
	ActiveRunID(sessionID string) string
}
