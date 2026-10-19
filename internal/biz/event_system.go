package biz

import (
	"time"
)

// System-domain EventKind values (not tied to a specific entity).
const (
	EventKindSystemRunStatus EventKind = "system.run_status"
	EventKindSystemHeartbeat EventKind = "system.heartbeat"
	EventKindSystemNotice    EventKind = "system.notice"
	EventKindSkillCatalog    EventKind = "skill.catalog"
)

// RunStatusEvent signals a run status change (replaces v1 system-domain run_status ActivityEvent).
type RunStatusEvent struct {
	sessionID string
	RunID     string
	Status    string
	// Meta carries run_id/status/error_message and optional await metadata.
	Meta map[string]any
	// Seq is the Sequencer-assigned outbox delivery sequence. Only terminal /
	// reset-required statuses (see IsResetRequiredRunStatus) take the Critical
	// lane and receive a Seq (+ outbox row + WS event_id); non-terminal
	// progress frames stay Informational and keep Seq=0 — no outbox replay.
	Seq        int64
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

// DeliverySeqValue implements DeliverySeqEvent: RunStatusEvent.Seq is the
// Sequencer-assigned outbox delivery sequence, not an entity domain seq.
func (e *RunStatusEvent) DeliverySeqValue() int64 { return e.Seq }

// SetDeliverySeqValue implements DeliverySeqEvent.
func (e *RunStatusEvent) SetDeliverySeqValue(seq int64) { e.Seq = seq }

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
	// Seq is assigned by the v2 Sequencer for critical delivery outbox ordering (B-06).
	Seq        int64
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

// DeliverySeqValue implements DeliverySeqEvent: SystemNoticeEvent.Seq is the
// Sequencer-assigned outbox delivery sequence (B-06), not an entity domain seq.
func (e *SystemNoticeEvent) DeliverySeqValue() int64 { return e.Seq }

// SetDeliverySeqValue implements DeliverySeqEvent.
func (e *SystemNoticeEvent) SetDeliverySeqValue(seq int64) { e.Seq = seq }

// SkillCatalogEntry is one entry of the skill catalog pushed to the chat UI
// (design 69 Phase 3). Tags carry plain tag names (dimension prefix stripped).
type SkillCatalogEntry struct {
	Slug        string
	Name        string
	Description string
	Tags        []string
}

// SkillCatalogEvent carries the visible skill catalog for a session's agent
// (design 69 Phase 3). Published once per chat WS connection setup so the
// frontend can render the skill entry strip above the composer.
//
// Delivery is best-effort (Informational, AS-EVT-01): the event is not
// persisted and not replayed; a reconnect re-pushes it via the WS init path.
type SkillCatalogEvent struct {
	sessionID  string
	Skills     []SkillCatalogEntry
	occurredAt time.Time
}

// NewSkillCatalogEvent constructs a SkillCatalogEvent.
func NewSkillCatalogEvent(sessionID string, skills []SkillCatalogEntry) *SkillCatalogEvent {
	return &SkillCatalogEvent{
		sessionID:  sessionID,
		Skills:     skills,
		occurredAt: time.Now(),
	}
}

func (e *SkillCatalogEvent) EventKind() EventKind      { return EventKindSkillCatalog }
func (e *SkillCatalogEvent) SpiritSessionID() string   { return e.sessionID }
func (e *SkillCatalogEvent) TaskID() string            { return "" }
func (e *SkillCatalogEvent) EntityID() string          { return e.sessionID }
func (e *SkillCatalogEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *SkillCatalogEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// Ensure interface compliance.
var (
	_ Event = (*RunStatusEvent)(nil)
	_ Event = (*HeartbeatEvent)(nil)
	_ Event = (*SystemNoticeEvent)(nil)
	_ Event = (*SkillCatalogEvent)(nil)
)
