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
	occurredAt time.Time
}

// NewHeartbeatEvent constructs a HeartbeatEvent.
func NewHeartbeatEvent(sessionID, message string, ts time.Time) *HeartbeatEvent {
	return &HeartbeatEvent{sessionID: sessionID, Message: message, occurredAt: ts}
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

// Ensure interface compliance.
var (
	_ Event = (*RunStatusEvent)(nil)
	_ Event = (*HeartbeatEvent)(nil)
	_ Event = (*SystemNoticeEvent)(nil)
)
