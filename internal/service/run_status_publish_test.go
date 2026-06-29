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

// TestPublishMetricsUpdated_UsesNoticeKind verifies that system metrics events
// are rendered as notice Activities instead of session Activities.
func TestPublishMetricsUpdated_UsesNoticeKind(t *testing.T) {
	bus := &gateCaptureBus{}
	PublishMetricsUpdated(bus, "sess-1")

	events := bus.events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Activity.Kind != biz.ActivityKindNotice {
		t.Errorf("kind = %s, want %s", events[0].Activity.Kind, biz.ActivityKindNotice)
	}
	if events[0].Activity.AgentKey != "session-metrics" {
		t.Errorf("agent_key = %s, want session-metrics", events[0].Activity.AgentKey)
	}
}
