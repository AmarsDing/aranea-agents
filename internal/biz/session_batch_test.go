package biz

import (
	"testing"
	"time"
)

func Test_resolveBatchTargets_retentionAndRunning(t *testing.T) {
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	old := cutoff.Add(-24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

	sessions := []Session{
		{ID: "old", LastMessageAt: old, Status: "completed"},
		{ID: "recent", LastMessageAt: recent, Status: "completed"},
		{ID: "run", LastMessageAt: old, Status: "running"},
		{ID: "arch", LastMessageAt: old, Status: "archived"},
		{ID: "del", LastMessageAt: old, Status: "completed", DeletedAt: "2026-01-01T00:00:00Z"},
	}

	matched, skipped := resolveBatchTargets(sessions, "delete", 30, false)
	if skipped != 1 {
		t.Fatalf("skipped running: got %d want 1", skipped)
	}
	if len(matched) != 1 || matched[0] != "old" {
		t.Fatalf("matched: %v want [old]", matched)
	}

	archMatched, _ := resolveBatchTargets(sessions, "archive", 30, false)
	if len(archMatched) != 1 || archMatched[0] != "old" {
		t.Fatalf("archive matched: %v want [old]", archMatched)
	}

	withArch, _ := resolveBatchTargets(sessions, "delete", 30, true)
	if len(withArch) != 2 {
		t.Fatalf("delete with archived: got %d want 2", len(withArch))
	}
}

func Test_effectiveActivityAt_fallback(t *testing.T) {
	created := "2026-01-01T00:00:00Z"
	at := effectiveActivityAt(Session{CreatedAt: created})
	if at.Format(time.RFC3339) != created {
		t.Fatalf("got %v", at)
	}
}

func Test_validateBatchParams(t *testing.T) {
	if err := validateBatchParams(0, 0); err == nil {
		t.Fatal("expected error for empty ids and zero days")
	}
	if err := validateBatchParams(0, 1); err != nil {
		t.Fatalf("retention mode should pass: %v", err)
	}
	if err := validateBatchParams(2, 0); err != nil {
		t.Fatalf("ids-only mode should pass: %v", err)
	}
	if err := validateBatchParams(0, -1); err == nil {
		t.Fatal("expected error for negative days")
	}
}
