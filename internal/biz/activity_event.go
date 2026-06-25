package biz

import "context"

// ActivityEventType labels the business-semantic event for an Activity lifecycle.
//
// These 7 types replace the technical "delta" term with business-oriented
// "streaming", and consolidate all lifecycle events into a single enum that
// combines with ActivityKind to express any business semantic.
type ActivityEventType string

const (
	// ActivityEventCreated indicates a new Activity was created.
	// Business meaning: a new thinking/tool call/reply/team stage etc. has started.
	// Frontend behavior: add a new Block component.
	ActivityEventCreated ActivityEventType = "created"

	// ActivityEventStreaming indicates streaming append (replaces technical "delta").
	// Business meaning: streaming text for thinking/reply, streaming tool arguments.
	// Frontend behavior: append text to the existing Block, cursor blinking.
	// meta.delta_field identifies the append field: content/reasoning/tool_arguments.
	ActivityEventStreaming ActivityEventType = "streaming"

	// ActivityEventUpdated indicates a non-streaming state change.
	// Business meaning: team stage change (assembled → executing), graph node
	// status change, progress update.
	// Frontend behavior: update Block status/stage/progress, no text append.
	// meta.changed_fields identifies the changed fields.
	ActivityEventUpdated ActivityEventType = "updated"

	// ActivityEventCompleted indicates normal completion.
	// Business meaning: thinking done, tool execution done, reply done, team stage done.
	// Frontend behavior: stop cursor, mark completed, expandable for details.
	ActivityEventCompleted ActivityEventType = "completed"

	// ActivityEventFailed indicates failure (independent event, not completed+status=failed).
	// Business meaning: tool execution failed, team execution failed, graph node error.
	// Frontend behavior: highlight error, show error details, retryable.
	// meta.error_code + meta.error_message identify the error.
	ActivityEventFailed ActivityEventType = "failed"

	// ActivityEventCancelled indicates cancellation (user-initiated stop).
	// Business meaning: user clicked stop button, team execution interrupted.
	// Frontend behavior: mark as cancelled, show cancel reason.
	// meta.cancel_reason identifies the cancel reason.
	ActivityEventCancelled ActivityEventType = "cancelled"

	// ActivityEventChildCreated indicates a child Activity was created.
	// Business meaning: tool call produced a sub-task, team stage produced a member task.
	// Frontend behavior: add a child Block under the parent Block (collapsed).
	// parent_activity_id identifies the parent Activity.
	ActivityEventChildCreated ActivityEventType = "child_created"
)

// ActivityDomain classifies the top-level domain an ActivityEvent belongs to.
//
// It distinguishes chat-related agent work (persisted to DB and pushed to
// chat subscribers) from transient system/domain events (WS-only, not
// persisted). This allows the unified ActivityEvent bus to carry both
// kinds of events with different reliability and persistence semantics.
type ActivityDomain string

const (
	// ActivityDomainChat marks events that belong to agent work on a chat
	// session (thinking/action/reply/team_stage/graph_stage etc.).
	// These events are persisted to the activities table and pushed to
	// chat subscribers via ActivityEventBus.
	ActivityDomainChat ActivityDomain = "chat"

	// ActivityDomainSystem marks transient system/domain events
	// (organization/borrow/skill/knowledge lifecycle, monitor alerts, etc.).
	// These events are NOT persisted to the activities table; they are only
	// pushed to WS subscribers via ActivityEventBus for live UI updates.
	ActivityDomainSystem ActivityDomain = "system"
)

// ActivityEvent is the unified transport format for Activity lifecycle events.
//
// It carries the event type (what happened) and the full Activity snapshot
// (the current state after the event). This replaces the legacy Envelope for
// Activity-related events, providing a simpler, business-semantic contract.
//
// Reliability levels (AS-EVT-01):
//   - Important: created/completed/failed/cancelled/child_created
//     → async persist with retry, sync publish
//   - Informational: streaming/updated
//     → async persist with drop-on-failure, sync publish (streaming may batch)
//
// The Domain field controls persistence:
//   - ActivityDomainChat   → persist to activities table (subject to event-level rules)
//   - ActivityDomainSystem → never persist (WS-only broadcast)
type ActivityEvent struct {
	Event    ActivityEventType `json:"event"`
	Activity Activity           `json:"activity"`

	// DeltaField identifies the append field for streaming events
	// (content/reasoning/tool_arguments). Empty for non-streaming events.
	DeltaField string `json:"delta_field,omitempty"`

	// DeltaChunk carries the incremental text for streaming events.
	// Empty for non-streaming events.
	DeltaChunk string `json:"delta_chunk,omitempty"`

	// Domain classifies the event as chat (persisted) or system (transient).
	// Defaults to ActivityDomainChat when empty (zero value of ActivityDomain).
	// System-domain events skip the activities persistence layer entirely
	// and are only delivered to live WS subscribers.
	Domain ActivityDomain `json:"domain,omitempty"`
}

// ActivityEventBus is the in-process event fanout hub for Activity lifecycle
// events. It replaces the legacy Envelope bus for chat-related events.
//
// Implementations must be safe for concurrent use.
// Stability:evolving
type ActivityEventBus interface {
	// Publish broadcasts an ActivityEvent to all matching subscribers.
	Publish(ctx context.Context, event ActivityEvent)

	// Subscribe registers a subscriber that receives ActivityEvents matching
	// the given options. Returns a channel of events and an unsubscribe function.
	// When globalMode is false, only events for the given sessionID are delivered.
	Subscribe(opts ActivityEventSubscribeOptions) (<-chan ActivityEvent, func())

	// DropCount returns the total number of dropped events due to full buffers.
	DropCount() uint64
}

// ActivityEventSubscribeOptions configures an ActivityEventBus subscription.
type ActivityEventSubscribeOptions struct {
	SessionID  string // empty = all sessions (global mode)
	BufferSize int    // subscriber channel buffer size
	GlobalMode bool   // true = receive events for all sessions
}
