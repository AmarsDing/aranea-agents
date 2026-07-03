package service

import (
	"testing"

	"aranea-agents/internal/biz"
)

// TestPublishRunStatus_UsesRunStatusEvent verifies that run-service status
// events are published as v2 RunStatusEvent (replacing v1 notice Activity).
func TestPublishRunStatus_UsesRunStatusEvent(t *testing.T) {
	bus := &captureEventBus{}
	PublishRunStatus(bus, "sess-1", "run-1", "running", "")

	events := bus.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev, ok := events[0].(*biz.RunStatusEvent)
	if !ok {
		t.Fatalf("expected *RunStatusEvent, got %T", events[0])
	}
	if ev.EventKind() != biz.EventKindSystemRunStatus {
		t.Errorf("kind = %s, want %s", ev.EventKind(), biz.EventKindSystemRunStatus)
	}
	if ev.RunID != "run-1" {
		t.Errorf("run_id = %s, want run-1", ev.RunID)
	}
	if ev.Status != "running" {
		t.Errorf("status = %s, want running", ev.Status)
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Errorf("session_id = %s, want sess-1", ev.SpiritSessionID())
	}
	if ev.Meta["run_id"] != "run-1" {
		t.Errorf("meta.run_id = %v, want run-1", ev.Meta["run_id"])
	}
	if ev.Meta["status"] != "running" {
		t.Errorf("meta.status = %v, want running", ev.Meta["status"])
	}
}

// TestPublishSessionStatusChanged_UsesSystemNotice verifies that
// session-service status events are published as v2 SystemNoticeEvent.
func TestPublishSessionStatusChanged_UsesSystemNotice(t *testing.T) {
	bus := &captureEventBus{}
	PublishSessionStatusChanged(bus, "sess-1", "running", "", "")

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
	if ev.NoticeType != "session_status_changed" {
		t.Errorf("notice_type = %s, want session_status_changed", ev.NoticeType)
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Errorf("session_id = %s, want sess-1", ev.SpiritSessionID())
	}
	if ev.Meta["status"] != "running" {
		t.Errorf("meta.status = %v, want running", ev.Meta["status"])
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
