// Package activityevent provides the ActivityEventBus implementation
// that transports biz.ActivityEvent events. It bridges the generic
// framework bus to the biz.ActivityEventBus interface.
//
// This package exists separately from internal/event to avoid an import
// cycle (biz imports event, so event cannot import biz).
package activityevent

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// Bus implements biz.ActivityEventBus using a generic framework bus.
// It is safe for concurrent use.
//
// In addition to fanout, Publish normalizes direct-publish events
// (those that bypass the ActivityProjector) so they survive page refresh
// and reach session-scoped subscribers:
//
//  1. SessionID normalization: events with empty SessionID but non-empty
//     SpiritSessionID get SessionID = SpiritSessionID. This makes
//     session-scoped WS subscribers receive spirit-orchestration events
//     (team_stage/graph_stage/session) without relying on the global WS
//     hub fallback.
//
//  2. Persistence: chat-domain direct-publish events are persisted to
//     the activities table via async UpsertActivity. Projector events
//     (SequencerHandled=true) are already persisted by the projector's
//     sequencer, so the bus skips them to avoid double-write and to
//     avoid overwriting the original (non-redacted) data with the
//     redacted snapshot that the bus sees.
//
// Direct-publish detection uses the ActivityEvent.SequencerHandled flag:
//   - SequencerHandled=true  → emitted via ActivityProjector (already persisted)
//   - SequencerHandled=false → direct-publish from business orchestrators
//     (plan/graph_stage/team_stage/session), needs normalization + persistence
type Bus struct {
	inner *event.GenericBus[biz.ActivityEvent]
	repo  biz.ActivityWriter
	lg    loggateway.Logger
}

// New creates a new ActivityEventBus.
//
// repo is the ActivityWriter used to persist direct-publish events so
// they survive page refresh. May be nil — in that case direct-publish
// events are not persisted by the bus (callers that need persistence
// must persist themselves).
//
// lg is the logger. May be nil — a noop logger is used.
func New(repo biz.ActivityWriter, lg loggateway.Logger) *Bus {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Bus{
		inner: event.NewGenericBus[biz.ActivityEvent](lg),
		repo:  repo,
		lg:    lg,
	}
}

// Publish broadcasts an ActivityEvent to all matching subscribers.
//
// Before fanout, direct-publish events (SequencerHandled=false) are
// normalized and persisted to the activities table.
func (b *Bus) Publish(ctx context.Context, ev biz.ActivityEvent) {
	// Detect direct-publish events: those with SequencerHandled=false were
	// not emitted through the ActivityProjector (which sets the flag to
	// true in its sequencer's processTask before publishing). These events
	// need SessionID normalization and persistence to survive page refresh.
	isDirectPublish := !ev.SequencerHandled

	if isDirectPublish {
		// Normalize SessionID: when empty, fall back to SpiritSessionID.
		// This makes session-scoped WS subscribers receive spirit-orchestration
		// events (team_stage/graph_stage/session) that were previously only
		// delivered via the global WS hub fallback.
		if ev.Activity.SessionID == "" && ev.Activity.SpiritSessionID != "" {
			ev.Activity.SessionID = ev.Activity.SpiritSessionID
		}
		// Persist chat-domain direct-publish events so they survive page
		// refresh. System-domain events are transient (WS-only) and must
		// NOT be persisted.
		if b.repo != nil && ev.Domain == biz.ActivityDomainChat {
			activity := ev.Activity
			safego.Go(ctx, "activity_bus_persist", func() {
				persistCtx := context.Background()
				if _, err := b.repo.UpsertActivity(persistCtx, activity); err != nil {
					b.lg.Warn("activityevent.Bus: failed to persist direct-publish activity",
						loggateway.StepID("activityevent.bus.persist"),
						loggateway.Str("activity_id", activity.ID),
						loggateway.Str("kind", string(activity.Kind)),
						loggateway.Str("session_id", activity.SessionID),
						loggateway.Err(err),
					)
				}
			})
		}
	}

	b.inner.Publish(ctx, ev)
}

// Subscribe registers a subscriber that receives ActivityEvents matching
// the given options.
//
// When opts.Filter is set, it is applied directly. Otherwise, when GlobalMode
// is false and SessionID is set, a session-scoped filter is derived. In
// GlobalMode (or when SessionID is empty and no Filter is set), all events
// are delivered.
func (b *Bus) Subscribe(opts biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	genericOpts := event.GenericSubscribeOptions[biz.ActivityEvent]{
		BufferSize: opts.BufferSize,
	}
	if opts.Filter != nil {
		genericOpts.Filter = opts.Filter
	} else if !opts.GlobalMode && opts.SessionID != "" {
		sessionID := opts.SessionID
		genericOpts.Filter = func(ev biz.ActivityEvent) bool {
			return ev.Activity.SessionID == sessionID
		}
	}
	return b.inner.Subscribe(genericOpts)
}

// DropCount returns the total number of dropped events.
func (b *Bus) DropCount() uint64 {
	return b.inner.DropCount()
}

// Compile-time interface check.
var _ biz.ActivityEventBus = (*Bus)(nil)
