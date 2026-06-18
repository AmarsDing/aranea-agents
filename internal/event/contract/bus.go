package contract

import (
	"context"
	"time"
)

// DropPolicy controls what happens when a subscriber's buffer is full.
type DropPolicy int

const (
	// DropOldest evicts the oldest event in the buffer to make room for the new one.
	DropOldest DropPolicy = iota
	// DropNewest silently discards the new event when the buffer is full.
	DropNewest
	// BlockUpTo blocks for a configurable duration before falling back to DropOldest.
	BlockUpTo DropPolicy = 2
)

// ChannelPriority controls subscription priority.
type ChannelPriority int

const (
	ChannelPriorityCritical ChannelPriority = iota
	ChannelPriorityNormal
)

// SubscribeOptions configures a single Bus subscription.
type SubscribeOptions struct {
	SessionID   string
	TeamID      string
	Channel     string
	FilterKey   string
	EventTypes  []EnvelopeType
	LevelFilter string

	Priority ChannelPriority

	BufferSize int
	Reliable   bool
	DropPolicy DropPolicy
	BlockFor   time.Duration
	Selector   func(EnvelopeType) bool
}

// Bus is the in-process event fanout hub interface.
// Implementations must be safe for concurrent use.
type Bus interface {
	Publish(ctx context.Context, envelope Envelope)
	Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
	DropCount() uint64
}

// CrossProcessStore is an optional event persistence target for cross-process
// replay (WS reconnect across server instances). When configured (e.g.
// PostgresEventStore), events are dual-written here in addition to the
// in-process EventStore, and WS replay falls back to this store when the
// in-memory buffer has no events for the session.
//
// Stability:evolving
type CrossProcessStore interface {
	Save(ctx context.Context, env *Envelope) error
	Replay(ctx context.Context, sessionID string, afterEventID string, limit int) ([]*Envelope, error)
}
