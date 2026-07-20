package biz

import (
	"time"
)

// System-domain EventKind values (not tied to a specific entity).
const (
	EventKindSystemRunStatus EventKind = "system.run_status"
	EventKindSystemHeartbeat EventKind = "system.heartbeat"
	EventKindSystemNotice    EventKind = "system.notice"
)

// RunStatusEvent signals a run status change (replaces v1 system-domain run_status ActivityEvent).
type RunStatusEvent struct {
	sessionID  string
	RunID      string
	Status     string
	Meta       map[string]any
	occurredAt time.Time
}

// NewRunStatusEvent constructs a RunStatusEvent.
func NewRunStatusEvent(sessionID, runID, status string, meta map[string]any) *RunStatusEvent {
	return &RunStatusEvent{
		sessionID:  sessionID,
		RunID:      runID,
		Status:     status,
		Meta:       meta,
		occurredAt: time.Now(),
	}
}

func (e *RunStatusEvent) EventKind() EventKind      { return EventKindSystemRunStatus }
func (e *RunStatusEvent) SpiritSessionID() string   { return e.sessionID }
func (e *RunStatusEvent) TaskID() string            { return "" }
func (e *RunStatusEvent) EntityID() string          { return e.RunID }
func (e *RunStatusEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *RunStatusEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// HeartbeatEvent signals a run heartbeat (replaces v1 system-domain heartbeat ActivityEvent).
type HeartbeatEvent struct {
	sessionID  string
	Message    string
	Meta       map[string]any
	occurredAt time.Time
}

// NewHeartbeatEvent constructs a HeartbeatEvent.
func NewHeartbeatEvent(sessionID, message string, ts time.Time) *HeartbeatEvent {
	return &HeartbeatEvent{sessionID: sessionID, Message: message, occurredAt: ts}
}

// NewHeartbeatEventWithMeta constructs a HeartbeatEvent with progress metadata.
func NewHeartbeatEventWithMeta(sessionID, message string, ts time.Time, meta map[string]any) *HeartbeatEvent {
	return &HeartbeatEvent{sessionID: sessionID, Message: message, Meta: meta, occurredAt: ts}
}

func (e *HeartbeatEvent) EventKind() EventKind      { return EventKindSystemHeartbeat }
func (e *HeartbeatEvent) SpiritSessionID() string   { return e.sessionID }
func (e *HeartbeatEvent) TaskID() string            { return "" }
func (e *HeartbeatEvent) EntityID() string          { return e.sessionID }
func (e *HeartbeatEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *HeartbeatEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// SystemNoticeEvent is a generic system notice (replaces v1 system-domain notice ActivityEvent).
type SystemNoticeEvent struct {
	sessionID  string
	NoticeType string
	Message    string
	Meta       map[string]any
	occurredAt time.Time
}

// NewSystemNoticeEvent constructs a SystemNoticeEvent.
func NewSystemNoticeEvent(sessionID, noticeType, message string, meta map[string]any) *SystemNoticeEvent {
	return &SystemNoticeEvent{
		sessionID:  sessionID,
		NoticeType: noticeType,
		Message:    message,
		Meta:       meta,
		occurredAt: time.Now(),
	}
}

func (e *SystemNoticeEvent) EventKind() EventKind      { return EventKindSystemNotice }
func (e *SystemNoticeEvent) SpiritSessionID() string   { return e.sessionID }
func (e *SystemNoticeEvent) TaskID() string            { return "" }
func (e *SystemNoticeEvent) EntityID() string          { return e.sessionID }
func (e *SystemNoticeEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *SystemNoticeEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// EventKindActivityBridge is the bridge event kind for legacy v1 ActivityEvent
// payloads that have not yet been (or cannot be) mapped to a typed v2 entity
// event. The bridge preserves the full v1 ActivityEvent shape (Meta/Content/
// ParentActivityID/etc.) so consumers (OrchestrationStatusStore, frontend v1
// inbound handler) can reuse existing field-aware logic without data loss.
//
// Phase 3b-D: introduced to eliminate the remaining v1 ActivityEventBus.Publish
// callers (graph_stage / team_stage / session / plan / task_failed sites) in a
// single stroke. The v1 ActivityEventBus itself remains for the WS broadcast
// pump until Tier 4 deletion; only the publish callers are migrated.
const EventKindActivityBridge EventKind = "activity.bridge"

// ActivityBridgeEvent wraps a v1 ActivityEvent as a v2 Event so it can be
// published on the v2 EventBus. The embedded ActivityEvent retains all v1
// fields (Meta, Content, ParentActivityID, AgentKey, Stage, etc.) and serializes
// to JSON using the existing snake_case tags on biz.Activity / biz.ActivityEvent.
//
// Consumers extract the embedded event via the ActivityEvent() accessor and
// reuse existing v1 field-aware logic. The bridge does NOT alter the payload.
type ActivityBridgeEvent struct {
	sessionID string
	// Event is the wrapped v1 ActivityEvent (the source of truth for routing,
	// persistence, and frontend rendering). Carries Activity.Kind/Stage/Meta.
	Event      ActivityEvent
	occurredAt time.Time
}

// NewActivityBridgeEvent constructs an ActivityBridgeEvent from a v1 ActivityEvent.
// spiritSessionID is derived from Event.Activity.SpiritSessionID (falls back to
// SessionID) so the v2 EventBus can route the event to the correct WS subscribers.
func NewActivityBridgeEvent(ev ActivityEvent) *ActivityBridgeEvent {
	spiritSessionID := ev.Activity.SpiritSessionID
	if spiritSessionID == "" {
		spiritSessionID = ev.Activity.SessionID
	}
	return &ActivityBridgeEvent{
		sessionID:  spiritSessionID,
		Event:      ev,
		occurredAt: time.Now(),
	}
}

func (e *ActivityBridgeEvent) EventKind() EventKind      { return EventKindActivityBridge }
func (e *ActivityBridgeEvent) SpiritSessionID() string   { return e.sessionID }
func (e *ActivityBridgeEvent) TaskID() string            { return "" }
func (e *ActivityBridgeEvent) EntityID() string          { return e.Event.Activity.ID }
func (e *ActivityBridgeEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *ActivityBridgeEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// Ensure interface compliance.
var (
	_ Event = (*RunStatusEvent)(nil)
	_ Event = (*HeartbeatEvent)(nil)
	_ Event = (*SystemNoticeEvent)(nil)
	_ Event = (*ActivityBridgeEvent)(nil)
)
