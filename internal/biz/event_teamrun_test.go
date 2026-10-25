package biz

import (
	"testing"
	"time"
)

// TestNewTeamRunCancelledEvent_Contract pins the 87-D4-1 contract:
// team_run.cancelled 是 TeamRun 取消终态的专用 kind（不再与 failed 共用
// team_run.failed），事件实体平面的 TeamRun.Status 恒为 cancelled。
func TestNewTeamRunCancelledEvent_Contract(t *testing.T) {
	if EventKindTeamRunCancelled != "team_run.cancelled" {
		t.Fatalf("EventKindTeamRunCancelled = %q, want team_run.cancelled", EventKindTeamRunCancelled)
	}
	completedAt := time.Now().UTC()
	tr := TeamRun{
		ID:              "tr-1",
		TeamStageID:     "ts-1",
		TaskID:          "task-1",
		SessionID:       "sess-team-1",
		SpiritSessionID: "sess-1",
		Status:          TeamRunV2StatusCancelled,
		StartedAt:       completedAt.Add(-time.Hour),
		CompletedAt:     &completedAt,
		Version:         3,
	}
	ev := NewTeamRunCancelledEvent(tr)
	if ev == nil {
		t.Fatal("NewTeamRunCancelledEvent returned nil")
	}
	if ev.EventKind() != EventKindTeamRunCancelled {
		t.Fatalf("EventKind = %q, want %q", ev.EventKind(), EventKindTeamRunCancelled)
	}
	if ev.EventKind() == EventKindTeamRunFailed {
		t.Fatal("cancelled kind must NOT reuse team_run.failed (critical_journal 回放不可区分取消/失败)")
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Fatalf("SpiritSessionID = %q, want sess-1", ev.SpiritSessionID())
	}
	if ev.TaskID() != "task-1" {
		t.Fatalf("TaskID = %q, want task-1", ev.TaskID())
	}
	if ev.EntityID() != "tr-1" {
		t.Fatalf("EntityID = %q, want tr-1", ev.EntityID())
	}
	if ev.OccurredAt().IsZero() {
		t.Fatal("OccurredAt must be non-zero (factory defaults to now)")
	}
	if ev.TeamRun.Status != TeamRunV2StatusCancelled {
		t.Fatalf("embedded TeamRun.Status = %q, want cancelled", ev.TeamRun.Status)
	}
}

// TestIsTerminalEventKind_TeamRunCancelled pins that the dedicated cancelled
// kind classifies as terminal via the .cancelled suffix rule.
func TestIsTerminalEventKind_TeamRunCancelled(t *testing.T) {
	if !IsTerminalEventKind(EventKindTeamRunCancelled) {
		t.Fatal("IsTerminalEventKind(team_run.cancelled) = false, want true")
	}
}

// TestIsCriticalDeliveryEvent_TeamRunCancelled pins that cancelled rides the
// Critical delivery lane (WBPF + outbox + CriticalJournal) without any
// per-kind code change — the suffix rule covers it.
func TestIsCriticalDeliveryEvent_TeamRunCancelled(t *testing.T) {
	ev := NewTeamRunCancelledEvent(TeamRun{
		ID: "tr-1", TaskID: "task-1", SpiritSessionID: "sess-1",
		Status: TeamRunV2StatusCancelled,
	})
	if !IsCriticalDeliveryEvent(ev) {
		t.Fatal("IsCriticalDeliveryEvent(team_run.cancelled) = false, want true")
	}
}

// TestTeamRunCancelledEvent_DeliverySeqCarrier pins the deliverySeqCarrier
// embed: without it insertCriticalOutbox warns "critical event lacks
// delivery-seq carrier, outbox skipped" and the durable outbox row is lost.
func TestTeamRunCancelledEvent_DeliverySeqCarrier(t *testing.T) {
	ev := NewTeamRunCancelledEvent(TeamRun{
		ID: "tr-1", TaskID: "task-1", SpiritSessionID: "sess-1",
		Status: TeamRunV2StatusCancelled,
	})
	de, ok := interface{}(ev).(DeliverySeqEvent)
	if !ok {
		t.Fatal("TeamRunCancelledEvent must implement DeliverySeqEvent (deliverySeqCarrier embed)")
	}
	if got := de.DeliverySeqValue(); got != 0 {
		t.Fatalf("initial delivery seq = %d, want 0", got)
	}
	de.SetDeliverySeqValue(42)
	if got := de.DeliverySeqValue(); got != 42 {
		t.Fatalf("after SetDeliverySeqValue = %d, want 42", got)
	}
	// Domain entity seq must stay untouched by delivery allocation.
	if ev.TeamRun.Seq != 0 {
		t.Fatalf("domain TeamRun.Seq mutated to %d, want 0", ev.TeamRun.Seq)
	}
}
