package biz

import (
	"testing"
)

func TestMemoryCanaryStatus_RecordOKAndFail(t *testing.T) {
	s := NewMemoryCanaryStatus()
	if s == nil {
		t.Fatal("NewMemoryCanaryStatus() = nil")
	}
	snap := s.Snapshot()
	if snap.RunsTotal != 0 || snap.FailedTotal != 0 || snap.ConsecutiveFailures != 0 {
		t.Fatalf("initial snapshot = %+v, want zero counters", snap)
	}

	s.RecordOK()
	s.RecordOK()
	snap = s.Snapshot()
	if snap.RunsTotal != 2 || snap.FailedTotal != 0 || snap.ConsecutiveFailures != 0 {
		t.Fatalf("after 2 OK: snapshot = %+v", snap)
	}
	if snap.LastOKUnix == 0 || snap.LastRunUnix == 0 {
		t.Fatalf("timestamps not set: %+v", snap)
	}

	s.RecordFail("recall", "fact not found")
	snap = s.Snapshot()
	if snap.RunsTotal != 3 || snap.FailedTotal != 1 || snap.ConsecutiveFailures != 1 {
		t.Fatalf("after 1 fail: snapshot = %+v", snap)
	}
	if snap.LastFailStage != "recall" || snap.LastFailReason == "" {
		t.Fatalf("fail detail missing: %+v", snap)
	}

	s.RecordFail("write", "upsert boom")
	if got := s.ConsecutiveFailures(); got != 2 {
		t.Fatalf("ConsecutiveFailures() = %d, want 2", got)
	}

	// OK resets consecutive failures but keeps totals (2 OK + 2 fail + 1 OK).
	s.RecordOK()
	snap = s.Snapshot()
	if snap.ConsecutiveFailures != 0 || snap.FailedTotal != 2 || snap.RunsTotal != 5 {
		t.Fatalf("after recovery OK: snapshot = %+v", snap)
	}
	// Last failure detail is preserved for diagnosis even after recovery.
	if snap.LastFailStage != "write" {
		t.Fatalf("LastFailStage = %q, want preserved %q", snap.LastFailStage, "write")
	}
}

func TestMemoryCanaryStatus_NilSafe(t *testing.T) {
	var s *MemoryCanaryStatus
	s.RecordOK()
	s.RecordFail("write", "x")
	if got := s.ConsecutiveFailures(); got != 0 {
		t.Fatalf("nil ConsecutiveFailures() = %d, want 0", got)
	}
	if snap := s.Snapshot(); snap.RunsTotal != 0 {
		t.Fatalf("nil Snapshot() = %+v, want zero", snap)
	}
}
