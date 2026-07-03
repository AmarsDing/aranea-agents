package service

import (
	"testing"

	"aranea-agents/internal/biz"
)

// TestPublishRunStatus_UsesNoticeKind verifies that run-service status events are
// rendered as notice Activities, not session Activities.
func TestPublishRunStatus_UsesNoticeKind(t *testing.T) {
	bus := &gateCaptureBus{}
	PublishRunStatus(bus, "sess-1", "run-1", "running", "")

	events := bus.events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Activity.Kind != biz.ActivityKindNotice {
		t.Errorf("kind = %s, want %s", events[0].Activity.Kind, biz.ActivityKindNotice)
	}
	if events[0].Activity.AgentKey != "run-service" {
		t.Errorf("agent_key = %s, want run-service", events[0].Activity.AgentKey)
	}
}

// TestPublishSessionStatusChanged_UsesNoticeKind verifies that session-service
// status events are rendered as notice Activities.
func TestPublishSessionStatusChanged_UsesNoticeKind(t *testing.T) {
	bus := &gateCaptureBus{}
	PublishSessionStatusChanged(bus, "sess-1", "running", "", "")

	events := bus.events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Activity.Kind != biz.ActivityKindNotice {
		t.Errorf("kind = %s, want %s", events[0].Activity.Kind, biz.ActivityKindNotice)
	}
	if events[0].Activity.AgentKey != "session-service" {
		t.Errorf("agent_key = %s, want session-service", events[0].Activity.AgentKey)
	}
}

// TestPublishMetricsUpdated_UsesSystemNotice verifies that system metrics events
// are published as v2 SystemNoticeEvent with the metrics_updated notice type.
func TestPublishMetricsUpdated_UsesSystemNotice(t *testing.T) {
	bus := &captureEventBus{}
	PublishMetricsUpdated(bus, "sess-1")

	events := bus.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev, ok := events[0].(*biz.SystemNoticeEvent)
	if !ok {
		t.Fatalf("expected *SystemNoticeEvent, got %T", events[0])
	}
	if ev.EventKind() != biz.EventKindSystemNotice {
		t.Errorf("kind = %s, want %s", ev.EventKind(), biz.EventKindSystemNotice)
	}
	if ev.NoticeType != "metrics_updated" {
		t.Errorf("notice_type = %s, want metrics_updated", ev.NoticeType)
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Errorf("session_id = %s, want sess-1", ev.SpiritSessionID())
	}
}
