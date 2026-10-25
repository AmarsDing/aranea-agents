package event

import (
	"os"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestCriticalJournal_AppendAndReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	j := NewCriticalJournal(dir)

	completed := biz.NewTaskCompletedEvent(biz.Task{
		ID: "t1", SessionID: "sess-1", Status: biz.TaskStatusCompleted,
	})
	created := biz.NewTaskCreatedEvent(biz.Task{
		ID: "t2", SessionID: "sess-1", Status: biz.TaskStatusPending,
	})
	if err := j.Append(completed); err != nil {
		t.Fatalf("Append completed: %v", err)
	}
	if err := j.Append(created); err != nil {
		t.Fatalf("Append created: %v", err)
	}

	entries, err := j.ReplayCritical("sess-1", time.Time{})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 critical entry (created skipped), got %d", len(entries))
	}
	if entries[0].Kind != string(biz.EventKindTaskCompleted) {
		t.Fatalf("kind=%q", entries[0].Kind)
	}
	if entries[0].SessionID != "sess-1" {
		t.Fatalf("session=%q", entries[0].SessionID)
	}
	if _, err := os.Stat(j.sessionPath("sess-1")); err != nil {
		t.Fatalf("expected journal file: %v", err)
	}
}

// TestCriticalJournal_TeamRunCancelledKind pins 87-D4-3: the dedicated
// team_run.cancelled kind is recorded verbatim (NoticeType empty) and is
// replay-distinguishable from team_run.failed — the pre-split failure mode
// where cancel and failure shared one kind.
func TestCriticalJournal_TeamRunCancelledKind(t *testing.T) {
	t.Parallel()
	j := NewCriticalJournal(t.TempDir())

	cancelled := biz.NewTeamRunCancelledEvent(biz.TeamRun{
		ID: "tr-1", TaskID: "task-1", SessionID: "sess-team-1", SpiritSessionID: "sess-1",
		Status: biz.TeamRunV2StatusCancelled,
	})
	failed := biz.NewTeamRunFailedEvent(biz.TeamRun{
		ID: "tr-2", TaskID: "task-1", SessionID: "sess-team-1", SpiritSessionID: "sess-1",
		Status: biz.TeamRunV2StatusFailed,
	})
	if err := j.Append(cancelled); err != nil {
		t.Fatalf("Append cancelled: %v", err)
	}
	if err := j.Append(failed); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	entries, err := j.ReplayCritical("sess-1", time.Time{})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 critical entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Kind != string(biz.EventKindTeamRunCancelled) {
		t.Fatalf("entries[0].Kind = %q, want %q", entries[0].Kind, biz.EventKindTeamRunCancelled)
	}
	if entries[0].NoticeType != "" {
		t.Fatalf("entries[0].NoticeType = %q, want empty", entries[0].NoticeType)
	}
	if entries[0].EntityID != "tr-1" {
		t.Fatalf("entries[0].EntityID = %q, want tr-1", entries[0].EntityID)
	}
	if entries[1].Kind != string(biz.EventKindTeamRunFailed) {
		t.Fatalf("entries[1].Kind = %q, want %q (cancelled/failed must stay distinguishable)", entries[1].Kind, biz.EventKindTeamRunFailed)
	}
}

func TestCriticalJournal_ReplayAfterTime(t *testing.T) {
	t.Parallel()
	j := NewCriticalJournal(t.TempDir())
	early := biz.NewTaskCompletedEvent(biz.Task{
		ID: "t1", SessionID: "s", Status: biz.TaskStatusCompleted,
	})
	early.SetOccurredAt(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	late := biz.NewTaskCompletedEvent(biz.Task{
		ID: "t2", SessionID: "s", Status: biz.TaskStatusCompleted,
	})
	late.SetOccurredAt(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	_ = j.Append(early)
	_ = j.Append(late)

	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	entries, err := j.ReplayCritical("s", cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry after cutoff, got %+v", entries)
	}
	if entries[0].TaskID != "t2" && entries[0].EntityID != "t2" {
		t.Fatalf("expected late task t2, got %+v", entries[0])
	}
}

func TestCriticalJournal_NilNoop(t *testing.T) {
	t.Parallel()
	var j *CriticalJournal
	if err := j.Append(biz.NewTaskCompletedEvent(biz.Task{ID: "t", SessionID: "s"})); err != nil {
		t.Fatal(err)
	}
	out, err := j.ReplayCritical("s", time.Time{})
	if err != nil || out != nil {
		t.Fatalf("nil journal should no-op, got %v %v", out, err)
	}
}

func TestCriticalJournal_EmptyDirNoop(t *testing.T) {
	t.Parallel()
	j := NewCriticalJournal("")
	if err := j.Append(biz.NewTaskCompletedEvent(biz.Task{ID: "t", SessionID: "s"})); err != nil {
		t.Fatal(err)
	}
}
