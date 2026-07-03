package biz

import (
	"testing"
	"time"
)

func TestRunStatusEvent(t *testing.T) {
	ev := NewRunStatusEvent("sess-1", "run-1", "running", map[string]any{"progress": 0.5})
	if ev.EventKind() != EventKindSystemRunStatus {
		t.Fatalf("kind: %s", ev.EventKind())
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Fatalf("session: %s", ev.SpiritSessionID())
	}
	if ev.TaskID() != "" {
		t.Fatalf("task id: %s", ev.TaskID())
	}
	if ev.EntityID() != "run-1" {
		t.Fatalf("entity id: %s", ev.EntityID())
	}
	if ev.RunID != "run-1" {
		t.Fatalf("run id: %s", ev.RunID)
	}
	if ev.Status != "running" {
		t.Fatalf("status: %s", ev.Status)
	}
	if ev.OccurredAt().IsZero() {
		t.Fatal("occurredAt is zero")
	}
}

func TestHeartbeatEvent(t *testing.T) {
	ts := time.Now()
	ev := NewHeartbeatEvent("sess-1", "still working", ts)
	if ev.EventKind() != EventKindSystemHeartbeat {
		t.Fatalf("kind: %s", ev.EventKind())
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Fatalf("session: %s", ev.SpiritSessionID())
	}
	if ev.TaskID() != "" {
		t.Fatalf("task id: %s", ev.TaskID())
	}
	if ev.EntityID() != "sess-1" {
		t.Fatalf("entity id: %s", ev.EntityID())
	}
	if ev.Message != "still working" {
		t.Fatalf("message: %s", ev.Message)
	}
	if ev.Meta != nil {
		t.Fatalf("meta should be nil for base constructor: %v", ev.Meta)
	}
	if !ev.OccurredAt().Equal(ts) {
		t.Fatalf("occurredAt: %v", ev.OccurredAt())
	}
}

func TestHeartbeatEventWithMeta(t *testing.T) {
	ts := time.Now()
	meta := map[string]any{"run_id": "run-1", "progress_percent": 0.5}
	ev := NewHeartbeatEventWithMeta("sess-1", "working", ts, meta)
	if ev.EventKind() != EventKindSystemHeartbeat {
		t.Fatalf("kind: %s", ev.EventKind())
	}
	if ev.Meta["run_id"] != "run-1" {
		t.Fatalf("meta run_id: %v", ev.Meta["run_id"])
	}
	if ev.Meta["progress_percent"] != 0.5 {
		t.Fatalf("meta progress_percent: %v", ev.Meta["progress_percent"])
	}
}

func TestSystemNoticeEvent(t *testing.T) {
	ev := NewSystemNoticeEvent("sess-1", "knowledge_indexed", "doc ready", map[string]any{"kb": "kb-1"})
	if ev.EventKind() != EventKindSystemNotice {
		t.Fatalf("kind: %s", ev.EventKind())
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Fatalf("session: %s", ev.SpiritSessionID())
	}
	if ev.TaskID() != "" {
		t.Fatalf("task id: %s", ev.TaskID())
	}
	if ev.EntityID() != "sess-1" {
		t.Fatalf("entity id: %s", ev.EntityID())
	}
	if ev.NoticeType != "knowledge_indexed" {
		t.Fatalf("notice type: %s", ev.NoticeType)
	}
	if ev.Message != "doc ready" {
		t.Fatalf("message: %s", ev.Message)
	}
}

// Compile-time checks that each event implements the Event interface.
func TestSystemEventsImplementInterface(t *testing.T) {
	var _ Event = (*RunStatusEvent)(nil)
	var _ Event = (*HeartbeatEvent)(nil)
	var _ Event = (*SystemNoticeEvent)(nil)
}
