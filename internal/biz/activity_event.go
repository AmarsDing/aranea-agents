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
type ActivityEvent struct {
	Event    ActivityEventType `json:"event"`
	Activity Activity           `json:"activity"`

	// DeltaField identifies the append field for streaming events
	// (content/reasoning/tool_arguments). Empty for non-streaming events.
	DeltaField string `json:"delta_field,omitempty"`

	// DeltaChunk carries the incremental text for streaming events.
	// Empty for non-streaming events.
	DeltaChunk string `json:"delta_chunk,omitempty"`
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
